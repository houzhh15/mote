package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"mote/internal/compaction"
	"mote/internal/hooks"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/provider"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/storage"
	"mote/internal/tools"
)

const (
	maxConsecutiveToolErrors = 3
	maxTransientRetries      = 2
	heartbeatInterval        = 15 * time.Second
)

// StandardOrchestrator 实现标准工具调用循环
type StandardOrchestrator struct {
	*BaseOrchestrator
	
	// Additional dependencies
	policyExecutor  policy.PolicyChecker
	approvalManager approval.ApprovalHandler
	compactionConfig *compaction.CompactionConfig
}

// NewStandardOrchestrator 创建标准协调器
func NewStandardOrchestrator(base *BaseOrchestrator) *StandardOrchestrator {
	return &StandardOrchestrator{
		BaseOrchestrator: base,
	}
}

// SetPolicyExecutor 设置策略执行器
func (o *StandardOrchestrator) SetPolicyExecutor(pe policy.PolicyChecker) {
	o.policyExecutor = pe
}

// SetApprovalManager 设置审批管理器
func (o *StandardOrchestrator) SetApprovalManager(am approval.ApprovalHandler) {
	o.approvalManager = am
}

// SetCompactionConfig 设置压缩配置
func (o *StandardOrchestrator) SetCompactionConfig(cc *compaction.CompactionConfig) {
	o.compactionConfig = cc
}

// Run 执行标准循环
func (o *StandardOrchestrator) Run(ctx context.Context, request *RunRequest) (<-chan types.Event, error) {
	events := make(chan types.Event, 100)

	go func() {
		defer close(events)

		// NOTE: session_create / before_message / MCP preprocessing hooks
		// are handled by the caller (runLoopCoreWithOrchestrator).
		// This method focuses on the core iteration loop only.

		// Add user message to session
		_, err := o.sessions.AddMessage(request.SessionID, provider.RoleUser, request.UserInput, nil, "")
		if err != nil {
			events <- types.NewErrorEvent(err)
			return
		}

		// Build messages - use direct method from runner for now
		// TODO: Integrate MessageBuilder component
		messages, err := o.buildMessagesLegacy(ctx, request.CachedSession, request.UserInput)
		if err != nil {
			events <- types.NewErrorEvent(err)
			return
		}

		var totalUsage types.Usage

		// Smart MCP injection: Full details on first iteration, summary on subsequent
		if o.systemPrompt != nil {
			o.systemPrompt.SetMCPInjectionMode(1) // Full mode
		}

		// Track consecutive tool errors to prevent infinite loops
		// Only incremented when ALL tools in a batch fail (matching original behavior)
		consecutiveToolErrors := 0

		// Track whether we've already retried after a context window overflow
		contextRetried := false

		// Track transient provider error retries
		transientRetries := 0

		// After compaction, use Chat mode for next iteration
		useChat := false

		// Main iteration loop
		for iteration := 0; iteration < o.config.MaxIterations; iteration++ {
			slog.Debug("StandardOrchestrator: starting iteration",
				"sessionID", request.SessionID,
				"iteration", iteration,
				"maxIterations", o.config.MaxIterations)

			// Check context cancellation
			select {
			case <-ctx.Done():
				slog.Warn("StandardOrchestrator: context cancelled",
					"sessionID", request.SessionID,
					"iteration", iteration,
					"error", ctx.Err())
				_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, "[任务被取消或超时]", nil, "")
				events <- types.NewErrorEvent(ErrContextCanceled)
				return
			default:
			}

			// Switch to summary mode after first iteration
			if iteration > 0 && o.systemPrompt != nil {
				o.systemPrompt.SetMCPInjectionMode(2) // Summary mode
			}

			// Compress history if needed
			slog.Debug("StandardOrchestrator: checking compaction",
				"sessionID", request.SessionID,
				"iteration", iteration,
				"messageCount", len(messages))
			if o.compactor != nil {
				if o.compactor.NeedsCompaction(messages) {
					slog.Info("StandardOrchestrator: compacting messages",
						"sessionID", request.SessionID,
						"iteration", iteration,
						"messageCount", len(messages))
					compacted := o.compactor.CompactWithFallback(ctx, messages, request.Provider)
					slog.Info("StandardOrchestrator: compaction done",
						"sessionID", request.SessionID,
						"iteration", iteration,
						"newMessageCount", len(compacted))

					// Reset provider connections after compaction
					if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
						resettable.ResetConnections()
						slog.Info("StandardOrchestrator: reset provider connections after compaction",
							"sessionID", request.SessionID)
					}

					// Use Chat mode for next iteration
					useChat = true
					slog.Info("StandardOrchestrator: will use Chat mode for next iteration after compaction",
						"sessionID", request.SessionID)

					// Sanity check
					hasConv := false
					for _, m := range compacted {
						if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
							hasConv = true
							break
						}
					}
					if hasConv {
						messages = compacted
						o.compactor.IncrementCompactionCount(request.SessionID)
					} else {
						slog.Warn("StandardOrchestrator: compaction result has no conversation messages, skipping",
							"sessionID", request.SessionID, "iteration", iteration)
					}
				}
			} else {
				// Fallback: simple compression
				compressed := o.compressIfNeeded(ctx, messages, request.Provider)
				if len(compressed) < len(messages) {
					slog.Debug("StandardOrchestrator: history compressed (fallback)",
						"sessionID", request.SessionID,
						"iteration", iteration)
					messages = compressed
				}
			}

			// Build chat request
			slog.Debug("StandardOrchestrator: building chat request",
				"sessionID", request.SessionID,
				"iteration", iteration)
			sessionModel := ""
			if request.CachedSession.Session != nil {
				sessionModel = request.CachedSession.Session.Model
			}
			req := o.buildChatRequest(messages, sessionModel, request.SessionID, request.Attachments)

			// Call provider
			resp, err := o.callProvider(ctx, request.Provider, req, events, useChat)
			if useChat {
				useChat = false // Only one iteration in Chat mode
			}
			if err != nil {
				// Reactive retry: on context window overflow, compact and retry once
				if provider.IsContextWindowExceeded(err) && !contextRetried && o.compactor != nil {
					contextRetried = true
					slog.Warn("StandardOrchestrator: context window exceeded, compacting and retrying",
						"sessionID", request.SessionID, "iteration", iteration)
					events <- types.NewContentEvent("\n\n⚠️ Context window exceeded — compacting history and retrying…\n\n")
					compacted := o.compactor.CompactWithFallback(ctx, messages, request.Provider)
					if len(compacted) > 0 {
						messages = compacted
						continue
					}
				}
				// Reactive retry: on transient provider errors
				if provider.IsRetryable(err) && transientRetries < maxTransientRetries {
					transientRetries++
					backoff := time.Duration(transientRetries*10) * time.Second
					slog.Warn("StandardOrchestrator: transient provider error, retrying with backoff",
						"sessionID", request.SessionID, "iteration", iteration,
						"retry", transientRetries, "maxRetries", maxTransientRetries,
						"backoff", backoff, "error", err)
					events <- types.NewContentEvent(fmt.Sprintf("\n\n⚠️ Provider transient error — retrying in %s… (%d/%d)\n\n", backoff, transientRetries, maxTransientRetries))
					select {
					case <-ctx.Done():
						events <- types.NewErrorEvent(ctx.Err())
						return
					case <-time.After(backoff):
					}
					continue
				}
				events <- types.NewErrorEvent(err)
				return
			}

			// Update usage tracking
			if resp.Usage != nil {
				totalUsage.PromptTokens += resp.Usage.PromptTokens
				totalUsage.CompletionTokens += resp.Usage.CompletionTokens
				totalUsage.TotalTokens += resp.Usage.TotalTokens
			}

			// Handle response - use FinishReason as the authoritative signal
			slog.Info("StandardOrchestrator: iteration completed",
				"sessionID", request.SessionID,
				"iteration", iteration,
				"toolCallsCount", len(resp.ToolCalls),
				"contentLen", len(resp.Content),
				"finishReason", resp.FinishReason,
				"hasUsage", resp.Usage != nil)

			// Special handling for "length" with pending tool calls:
			// Don't stop! Continue executing tools and warn the user.
			if resp.FinishReason == provider.FinishReasonLength && len(resp.ToolCalls) > 0 {
				slog.Warn("StandardOrchestrator: response truncated due to max_tokens, continuing with pending tool calls",
					"sessionID", request.SessionID,
					"pendingToolCalls", len(resp.ToolCalls),
					"finishReason", resp.FinishReason)
				// Fall through to execute the tool calls
			}

			shouldStop := resp.FinishReason == provider.FinishReasonStop ||
				(resp.FinishReason == provider.FinishReasonLength && len(resp.ToolCalls) == 0) ||
				(len(resp.ToolCalls) == 0 && resp.FinishReason != provider.FinishReasonToolCalls)

			if shouldStop {
				// LLM signaled completion - we're done
				slog.Info("StandardOrchestrator: LLM signaled stop",
					"sessionID", request.SessionID,
					"finishReason", resp.FinishReason)

				// NOTE: Content was already streamed by callProvider (Stream mode).
				// For Chat mode, content hasn't been streamed yet.
				respContent := resp.Content

				// Trigger before_response hook
				if respContent != "" {
					hookCtx := hooks.NewContext(hooks.HookBeforeResponse)
					hookCtx.Response = &hooks.ResponseContext{
						Content:    respContent,
						TokensUsed: int(totalUsage.TotalTokens),
					}
					hookCtx.Session = &hooks.SessionContext{ID: request.SessionID}

					result, _ := o.triggerHook(ctx, hookCtx)
					if result != nil && result.Modified {
						if modified, ok := result.Data["content"].(string); ok {
							respContent = modified
						}
					}
				}

				if respContent != "" {
					_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, respContent, nil, "")
				} else {
					_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, "[任务已完成，无文本响应]", nil, "")
				}

				// Trigger after_response hook
				if respContent != "" {
					hookCtx := hooks.NewContext(hooks.HookAfterResponse)
					hookCtx.Response = &hooks.ResponseContext{
						Content:    respContent,
						TokensUsed: int(totalUsage.TotalTokens),
					}
					hookCtx.Session = &hooks.SessionContext{ID: request.SessionID}
					_, _ = o.triggerHook(ctx, hookCtx)
				}

				events <- types.NewDoneEvent(&totalUsage)
				return
			}

			// Process tool calls: append assistant message (with tool calls) FIRST
			assistantMsg := provider.Message{
				Role:      provider.RoleAssistant,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			}
			messages = append(messages, assistantMsg)

			// Execute tools
			slog.Info("StandardOrchestrator: executing tools",
				"sessionID", request.SessionID,
				"iteration", iteration,
				"toolCount", len(resp.ToolCalls))

			var toolResults []provider.Message
			var toolErrorCount int

			if o.toolExecutor != nil {
				// Use injected tool executor (full implementation from Runner)
				toolResults, toolErrorCount = o.toolExecutor(ctx, resp.ToolCalls, request.SessionID)
			} else {
				// Fallback to simple implementation (for tests)
				toolResults, toolErrorCount = o.executeToolsSimple(ctx, resp.ToolCalls, events, request.SessionID)
			}

			slog.Info("StandardOrchestrator: tools executed",
				"sessionID", request.SessionID,
				"iteration", iteration,
				"resultsCount", len(toolResults),
				"errorCount", toolErrorCount)
			messages = append(messages, toolResults...)

			// Check for consecutive tool errors to prevent infinite loops.
			// Only count as consecutive when ALL tools in the batch fail (matching original behavior).
			allErrors := len(toolResults) > 0 && toolErrorCount == len(toolResults)

			if allErrors {
				consecutiveToolErrors++
				slog.Warn("StandardOrchestrator: tool execution returned errors",
					"sessionID", request.SessionID,
					"iteration", iteration,
					"consecutiveErrors", consecutiveToolErrors,
					"maxConsecutiveErrors", maxConsecutiveToolErrors)

				if consecutiveToolErrors >= maxConsecutiveToolErrors {
					slog.Error("StandardOrchestrator: stopping due to consecutive tool errors",
						"sessionID", request.SessionID,
						"consecutiveErrors", consecutiveToolErrors)
					// Add a message to inform the LLM
					errMsg := "[System: Multiple consecutive tool call errors detected. Stopping to prevent infinite loop. Please review the error messages and try a different approach.]"
					_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, errMsg, nil, "")
					events <- types.Event{
						Type:    types.EventTypeContent,
						Content: "\n\n" + errMsg + "\n",
					}
					events <- types.NewDoneEvent(&totalUsage)
					return
				}
			} else {
				// Reset error counter on successful tool execution
				consecutiveToolErrors = 0
			}

			// Save to session
			storageTcs := convertToolCalls(resp.ToolCalls)
			_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, resp.Content, storageTcs, "")
			for _, result := range toolResults {
				_, _ = o.sessions.AddMessage(request.SessionID, result.Role, result.Content, nil, result.ToolCallID)
			}
			slog.Info("StandardOrchestrator: iteration saved, continuing to next",
				"sessionID", request.SessionID, "iteration", iteration)
		}

		// Max iterations reached
		slog.Warn("StandardOrchestrator: max iterations reached",
			"sessionID", request.SessionID,
			"maxIterations", o.config.MaxIterations)
		maxIterMsg := fmt.Sprintf("[已达到最大迭代次数 (%d)，任务执行被中止]", o.config.MaxIterations)
		_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, maxIterMsg, nil, "")
		events <- types.NewErrorEvent(ErrMaxIterations)
	}()

	return events, nil
}

// callProvider 调用提供商（Chat 或 Stream 模式）并收集响应
func (o *StandardOrchestrator) callProvider(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- types.Event, useChat bool) (*provider.ChatResponse, error) {
	if useChat {
		slog.Info("StandardOrchestrator: using Chat mode (post-compaction)", "useChat", true)
		return prov.Chat(ctx, req)
	}

	// Stream mode: collect response while forwarding events
	eventChan, streamErr := prov.Stream(ctx, req)
	if streamErr != nil {
		return nil, streamErr
	}

	resp := &provider.ChatResponse{
		FinishReason: provider.FinishReasonStop, // Default to stop
	}
	var thinkingBuilder string
	// pendingToolCalls accumulates incremental tool call chunks by index.
	// Streaming providers send tool call arguments in multiple events
	// with the same Index; we must concatenate them before use.
	pendingToolCalls := make(map[int]*provider.ToolCall)

	for event := range eventChan {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if event.Error != nil {
			return nil, event.Error
		}
		if event.Delta != "" {
			resp.Content += event.Delta
			events <- types.Event{Type: types.EventTypeContent, Content: event.Delta}
		}
		if event.Thinking != "" {
			thinkingBuilder += event.Thinking
			events <- types.Event{Type: types.EventTypeThinking, Thinking: event.Thinking}
		}
		if event.FinishReason != "" {
			resp.FinishReason = event.FinishReason
		}
		if event.ToolCall != nil {
			tc := event.ToolCall
			if existing, ok := pendingToolCalls[tc.Index]; ok {
				// Accumulate arguments for existing tool call
				existing.Arguments += tc.Arguments
				if tc.Function != nil {
					if existing.Function == nil {
						existing.Function = tc.Function
					} else {
						existing.Function.Arguments += tc.Function.Arguments
					}
				}
			} else {
				// New tool call — store a copy
				newTc := &provider.ToolCall{
					ID:        tc.ID,
					Index:     tc.Index,
					Type:      tc.Type,
					Name:      tc.Name,
					Arguments: tc.Arguments,
					Function:  tc.Function,
				}
				pendingToolCalls[tc.Index] = newTc
			}
		}
		if event.ToolCallUpdate != nil {
			events <- types.Event{
				Type: types.EventTypeToolCallUpdate,
				ToolCallUpdate: &types.ToolCallUpdateEvent{
					ToolCallID: event.ToolCallUpdate.ID,
					ToolName:   event.ToolCallUpdate.Name,
					Status:     event.ToolCallUpdate.Status,
					Arguments:  event.ToolCallUpdate.Arguments,
				},
			}
		}
		if event.Usage != nil {
			resp.Usage = event.Usage
		}
	}

	// Fallback: if content is empty but thinking was received and no tool calls,
	// use thinking as content (handles models that put output in reasoning_content only)
	if resp.Content == "" && thinkingBuilder != "" && len(pendingToolCalls) == 0 {
		slog.Warn("callProvider: content empty but thinking received, using thinking as fallback",
			"thinkingLen", len(thinkingBuilder))
		resp.Content = thinkingBuilder
		events <- types.Event{Type: types.EventTypeContent, Content: thinkingBuilder}
	}

	// Convert pending tool calls to final slice and emit tool call events
	for _, tc := range pendingToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, *tc)
		// Emit tool call event for frontend display
		storageTc := providerToStorageToolCall(*tc)
		events <- types.NewToolCallEvent(storageTc)
	}

	// Adjust FinishReason based on actual tool calls
	// (Some providers may not set FinishReason correctly in stream mode)
	if len(resp.ToolCalls) > 0 && resp.FinishReason == provider.FinishReasonStop {
		resp.FinishReason = provider.FinishReasonToolCalls
	}

	slog.Info("callProvider: stream completed",
		"contentLen", len(resp.Content),
		"thinkingLen", len(thinkingBuilder),
		"toolCallCount", len(resp.ToolCalls),
		"finishReason", resp.FinishReason)

	return resp, nil
}

// providerToStorageToolCall converts a provider.ToolCall to a storage.ToolCall.
func providerToStorageToolCall(tc provider.ToolCall) *storage.ToolCall {
	name := tc.Name
	args := tc.Arguments
	if tc.Function != nil {
		if tc.Function.Name != "" {
			name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			args = tc.Function.Arguments
		}
	}

	fnData, _ := json.Marshal(map[string]string{
		"name":      name,
		"arguments": args,
	})

	return &storage.ToolCall{
		ID:       tc.ID,
		Type:     "function",
		Function: json.RawMessage(fnData),
	}
}

// executeToolsSimple 简化的工具执行（暂时直接调用 registry）
func (o *StandardOrchestrator) executeToolsSimple(ctx context.Context, toolCalls []provider.ToolCall, events chan<- types.Event, sessionID string) ([]provider.Message, int) {
	var results []provider.Message
	errorCount := 0
	
	// Inject session ID into context
	if sessionID != "" {
		ctx = tools.WithSessionID(ctx, sessionID)
	}
	
	for _, tc := range toolCalls {
		toolName := tc.Name
		if tc.Function != nil {
			toolName = tc.Function.Name
		}
		
		args := tc.Arguments
		if tc.Function != nil {
			args = tc.Function.Arguments
		}
		
		// Parse arguments
		var argsMap map[string]any
		if args != "" {
			if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
				errMsg := fmt.Sprintf("Error: Invalid tool arguments: %v", err)
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    errMsg,
					ToolCallID: tc.ID,
				})
				events <- types.NewToolResultEvent(tc.ID, toolName, errMsg, true, 0)
				errorCount++
				continue
			}
		}
		
		// Execute tool
		start := time.Now()
		result, err := o.registry.Execute(ctx, toolName, argsMap)
		duration := time.Since(start)
		
		var output string
		var isError bool
		if err != nil {
			output = err.Error()
			isError = true
			errorCount++
		} else {
			output = result.Content
			isError = result.IsError
			if isError {
				errorCount++
			}
		}
		
		// Emit tool result event
		events <- types.NewToolResultEvent(tc.ID, toolName, output, isError, duration.Milliseconds())
		
		// Add tool result message
		results = append(results, provider.Message{
			Role:       provider.RoleTool,
			Content:    output,
			ToolCallID: tc.ID,
		})
	}
	
	return results, errorCount
}

// buildMessagesLegacy - 临时方法，后续集成 MessageBuilder
func (o *StandardOrchestrator) buildMessagesLegacy(ctx context.Context, cached *scheduler.CachedSession, userInput string) ([]provider.Message, error) {
	var messages []provider.Message

	// Add system prompt if available
	if o.systemPrompt != nil {
		content, err := o.systemPrompt.Build(ctx, userInput)
		if err == nil && content != "" {
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: content,
			})
		}
	}

	// Add history messages (including tool calls)
	for _, msg := range cached.Messages {
		provMsg := provider.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		// Parse tool calls from stored JSON
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				provTc := provider.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
				}
				if len(tc.Function) > 0 {
					var fn struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}
					if err := json.Unmarshal(tc.Function, &fn); err == nil {
						provTc.Name = fn.Name
						provTc.Arguments = fn.Arguments
						provTc.Function = &struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      fn.Name,
							Arguments: fn.Arguments,
						}
					}
				}
				provMsg.ToolCalls = append(provMsg.ToolCalls, provTc)
			}
		}
		messages = append(messages, provMsg)
	}

	// Inject skills section if skillManager is available
	if o.skillManager != nil {
		skillsSection := skills.NewPromptSection(o.skillManager)
		if cached.Session != nil && len(cached.Session.SelectedSkills) > 0 {
			skillsSection.WithSelectedSkills(cached.Session.SelectedSkills)
		}
		var skillsContent string
		if section := skillsSection.Build(); section != "" {
			skillsContent += section
		}
		if activePrompts := skillsSection.BuildActivePrompts(); activePrompts != "" {
			skillsContent += "\n" + activePrompts
		}
		if skillsContent != "" {
			// Find system message and append, or prepend new one
			found := false
			for i, msg := range messages {
				if msg.Role == provider.RoleSystem {
					messages[i].Content += "\n\n" + skillsContent
					found = true
					break
				}
			}
			if !found {
				messages = append([]provider.Message{{
					Role:    provider.RoleSystem,
					Content: skillsContent,
				}}, messages...)
			}
		}
	}

	// Add current user input
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userInput,
	})

	return messages, nil
}

// convertToolCalls converts provider tool calls to storage tool calls
func convertToolCalls(tcs []provider.ToolCall) []storage.ToolCall {
	var result []storage.ToolCall
	for _, tc := range tcs {
		name := tc.Name
		args := tc.Arguments
		if tc.Function != nil {
			name = tc.Function.Name
			args = tc.Function.Arguments
		}

		fnData, _ := json.Marshal(map[string]string{
			"name":      name,
			"arguments": args,
		})

		result = append(result, storage.ToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: json.RawMessage(fnData),
		})
	}
	return result
}
