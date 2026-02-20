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
		
		// Trigger session_create hook for new sessions
		if len(request.CachedSession.Messages) == 0 {
			hookCtx := hooks.NewContext(hooks.HookSessionCreate)
			hookCtx.Session = &hooks.SessionContext{
				ID:        request.SessionID,
				CreatedAt: time.Now(),
			}
			_, _ = o.triggerHook(ctx, hookCtx)
		}
		
		// Trigger before_message hook
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		hookCtx.Message = &hooks.MessageContext{
			Content: request.UserInput,
			Role:    string(provider.RoleUser),
			From:    "user",
		}
		hookCtx.Session = &hooks.SessionContext{ID: request.SessionID}
		
		if result, _ := o.triggerHook(ctx, hookCtx); result != nil && !result.Continue {
			events <- types.NewErrorEvent(ErrHookInterrupted)
			return
		}
		
		// Apply modifications from hook
		userInput := request.UserInput
		if result, _ := o.triggerHook(ctx, hookCtx); result != nil && result.Modified {
			if modified, ok := result.Data["content"].(string); ok {
				userInput = modified
			}
		}
		
		// Add user message to session
		_, err := o.sessions.AddMessage(request.SessionID, provider.RoleUser, userInput, nil, "")
		if err != nil {
			events <- types.NewErrorEvent(err)
			return
		}
		
		// Build messages - use direct method from runner for now
		// TODO: Integrate MessageBuilder component
		messages, err := o.buildMessagesLegacy(ctx, request.CachedSession, userInput)
		if err != nil {
			events <- types.NewErrorEvent(err)
			return
		}
		
		// Track state
		state := &LoopState{
			Iteration:              0,
			ConsecutiveErrors:      0,
			TotalConsecutiveErrors: 0,
			ContextRetried:         false,
			TransientRetries:       0,
			UseChat:                false,
		}
		
		// Smart MCP injection: Full details on first iteration, summary on subsequent
		if o.systemPrompt != nil {
			o.systemPrompt.SetMCPInjectionMode(1) // Full mode
		}
		
		// Main iteration loop
		for state.Iteration < o.config.MaxIterations {
			slog.Debug("StandardOrchestrator: starting iteration",
				"sessionID", request.SessionID,
				"iteration", state.Iteration,
				"maxIterations", o.config.MaxIterations)
			
			// Check context cancellation
			select {
			case <-ctx.Done():
				slog.Warn("StandardOrchestrator: context cancelled", "sessionID", request.SessionID)
				_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, "[任务被取消或超时]", nil, "")
				events <- types.NewErrorEvent(ErrContextCanceled)
				return
			default:
			}
			
			// Switch to summary mode after first iteration
			if state.Iteration > 0 && o.systemPrompt != nil {
				o.systemPrompt.SetMCPInjectionMode(2) // Summary mode
			}
			
			// Compress history if needed
			if o.compactor != nil && o.compactor.NeedsCompaction(messages) {
				slog.Info("StandardOrchestrator: compacting messages",
					"sessionID", request.SessionID,
					"iteration", state.Iteration,
					"messageCount", len(messages))
				
				compacted := o.compactor.CompactWithFallback(ctx, messages, request.Provider)
				
				// Reset provider connections after compaction
				if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
					resettable.ResetConnections()
					slog.Info("StandardOrchestrator: reset provider connections after compaction",
						"sessionID", request.SessionID)
				}
				
				// Use Chat mode for next iteration
				state.UseChat = true
				
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
					"sessionID", request.SessionID)
				}
			}
			
			// Build chat request
			sessionModel := ""
			if request.CachedSession.Session != nil {
				sessionModel = request.CachedSession.Session.Model
			}
			req := o.buildChatRequest(messages, sessionModel, request.SessionID, request.Attachments)
			
			// Call provider with retry logic
			resp, err := o.callProviderWithRetry(ctx, request.Provider, req, events, state)
			if err != nil {
				events <- types.NewErrorEvent(err)
				return
			}
			
			// Check if LLM signaled stop
			if resp.FinishReason == "stop" || resp.FinishReason == "end_turn" || len(resp.ToolCalls) == 0 {
				slog.Info("StandardOrchestrator: LLM signaled stop",
					"sessionID", request.SessionID,
					"finishReason", resp.FinishReason)
				
				// Send final content
				if resp.Content != "" {
					events <- types.Event{Type: types.EventTypeContent, Content: resp.Content}
				}
				
				// Save assistant message
				_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, resp.Content, nil, "")
				
				// Send done event
				events <- types.Event{Type: types.EventTypeDone}
				return
			}
			
			// Execute tools
			toolResults, errorCount := o.executeToolsSimple(ctx, resp.ToolCalls, events, request.SessionID)
			
			// Update state
			state.Iteration++
			state.ConsecutiveErrors = errorCount
			if errorCount > 0 {
				state.TotalConsecutiveErrors += errorCount
			} else {
				state.TotalConsecutiveErrors = 0
			}
			
			// Check consecutive tool errors
			if state.TotalConsecutiveErrors >= maxConsecutiveToolErrors {
				slog.Warn("StandardOrchestrator: too many consecutive tool errors", "sessionID", request.SessionID)
				events <- types.NewErrorEvent(fmt.Errorf("too many consecutive tool errors"))
				return
			}
			
			// Append tool results to messages
			messages = append(messages, toolResults...)
			
			// Save assistant message with tool calls
			storageTcs := convertToolCalls(resp.ToolCalls)
			_, _ = o.sessions.AddMessage(request.SessionID, provider.RoleAssistant, resp.Content, storageTcs, "")
			
			// Save tool results
			for _, result := range toolResults {
				_, _ = o.sessions.AddMessage(request.SessionID, result.Role, result.Content, nil, result.ToolCallID)
			}
		}
		
		// Max iterations reached
		events <- types.NewErrorEvent(fmt.Errorf("max iterations (%d) reached", o.config.MaxIterations))
	}()
	
	return events, nil
}

// callProviderWithRetry 调用提供商并处理重试
func (o *StandardOrchestrator) callProviderWithRetry(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- types.Event, state *LoopState) (*provider.ChatResponse, error) {
	var resp *provider.ChatResponse
	var err error
	
	// Use Chat or Stream based on state
	if state.UseChat {
		slog.Info("StandardOrchestrator: using Chat mode (post-compaction)", "useChat", true)
		resp, err = prov.Chat(ctx, req)
		state.UseChat = false // Only one iteration in Chat mode
	} else {
		// For streaming, we need to collect the response
		eventChan, streamErr := prov.Stream(ctx, req)
		if streamErr != nil {
			return nil, streamErr
		}
		
		resp = &provider.ChatResponse{}
		for event := range eventChan {
			if event.Error != nil {
				err = event.Error
				break
			}
			if event.Delta != "" {
				resp.Content += event.Delta
				events <- types.Event{Type: types.EventTypeContent, Content: event.Delta}
			}
			if event.FinishReason != "" {
				resp.FinishReason = event.FinishReason
			}
			if event.ToolCall != nil {
				resp.ToolCalls = append(resp.ToolCalls, *event.ToolCall)
			}
			if event.Usage != nil {
				resp.Usage = event.Usage
			}
		}
	}
	
	if err != nil {
		// Handle context window overflow
		if provider.IsContextWindowExceeded(err) && !state.ContextRetried && o.compactor != nil {
			state.ContextRetried = true
			slog.Warn("StandardOrchestrator: context window exceeded, will retry after compression", "sessionID", req.ConversationID)
			events <- types.NewContentEvent("\n\n⚠️ Context window exceeded — compacting history and retrying…\n\n")
			return nil, err // Caller will handle retry
		}
		
		// Handle transient errors
		if provider.IsRetryable(err) && state.TransientRetries < maxTransientRetries {
			state.TransientRetries++
			backoff := time.Duration(state.TransientRetries*10) * time.Second
			slog.Warn("StandardOrchestrator: transient error, retrying",
				"retry", state.TransientRetries,
				"backoff", backoff,
				"error", err)
			events <- types.NewContentEvent(fmt.Sprintf("\n\n⚠️ Provider transient error — retrying in %s… (%d/%d)\n\n", backoff, state.TransientRetries, maxTransientRetries))
			time.Sleep(backoff)
			return nil, err // Caller will handle retry
		}
		
		return nil, err
	}
	
	return resp, nil
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
	// Simplified version - just load history
	var messages []provider.Message
	
	// Add system prompt if available
	if o.systemPrompt != nil {
		content, err := o.systemPrompt.Build(ctx, userInput)
		if err == nil {
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: content,
			})
		}
	}
	
	// Add history messages
	for _, msg := range cached.Messages {
		messages = append(messages, provider.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		})
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
