package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	maxConsecutiveToolErrors = 5
	maxTransientRetries      = 2
	heartbeatInterval        = 15 * time.Second
)

// StandardOrchestrator 实现标准工具调用循环
type StandardOrchestrator struct {
	*BaseOrchestrator

	// Additional dependencies
	policyExecutor   policy.PolicyChecker
	approvalManager  approval.ApprovalHandler
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

		// Reset per-task compaction count so the first compaction in this
		// execution uses LLM summarization (not truncation-only).
		if o.compactor != nil {
			o.compactor.ResetCompactionCount(request.SessionID)
		}

		// InjectedMessages mode (PDA frame-local context): skip all session message
		// persistence — the PDA engine owns its own context and the factory layer
		// handles persisting the final summary. Writing intermediate messages
		// would pollute the main session's history and inflate future LLM context.
		isInjectedMode := request.InjectedMessages != nil

		// persistMsg is a helper that writes an assistant message to session storage,
		// but only when NOT in InjectedMessages mode (PDA sub-agent calls).
		persistMsg := func(role, content string, toolCalls []storage.ToolCall, toolCallID string) {
			if isInjectedMode {
				return
			}
			_, _ = o.sessions.AddMessage(request.SessionID, role, content, toolCalls, toolCallID)
		}

		// Add user message to session (skip for InjectedMessages mode — PDA engine owns context)
		if !isInjectedMode {
			_, err := o.sessions.AddMessage(request.SessionID, provider.RoleUser, request.UserInput, nil, "")
			if err != nil {
				events <- types.NewErrorEvent(err)
				return
			}
		}

		// Build messages - use direct method from runner for now
		// TODO: Integrate MessageBuilder component

		// Inject model's max output token limit into system prompt so the LLM
		// can self-regulate its output size and avoid truncation.
		if o.systemPrompt != nil {
			// Inject session-bound workspace directory into system prompt
			if o.workspaceResolver != nil && request.SessionID != "" {
				if wsPath := o.workspaceResolver(request.SessionID); wsPath != "" {
					o.systemPrompt.SetWorkspaceDir(wsPath)
				}
			}
			if mop, ok := request.Provider.(provider.MaxOutputProvider); ok {
				sessionModel := ""
				if request.CachedSession.Session != nil {
					sessionModel = request.CachedSession.Session.Model
				}
				if maxOut := mop.MaxOutput(sessionModel); maxOut > 0 {
					o.systemPrompt.SetMaxOutputTokens(maxOut)
				}
			}
		}

		// Build initial messages: use InjectedMessages if provided (PDA frame-local context),
		// otherwise load from session history (standard path).
		var messages []provider.Message
		if request.InjectedMessages != nil {
			// PDA mode: build system prompt dynamically then prepend to injected context
			var sysContent string
			if o.systemPrompt != nil {
				if c, err := o.systemPrompt.Build(ctx, request.UserInput); err == nil && c != "" {
					sysContent = c
				}
			} else if o.config.SystemPrompt != "" {
				sysContent = o.config.SystemPrompt
			}
			if sysContent != "" {
				messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: sysContent})
			}
			messages = append(messages, request.InjectedMessages...)
		} else {
			var err error
			messages, err = o.buildMessagesLegacy(ctx, request.CachedSession, request.UserInput)
			if err != nil {
				events <- types.NewErrorEvent(err)
				return
			}
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

		// Track whether we've already retried after an empty stop response
		emptyResponseRetried := false

		// Track whether we've already retried after all tool calls had invalid JSON args
		invalidArgsRetried := false

		// Track whether we've already attempted non-streaming fallback for truncated tool args
		nonStreamRetried := false

		// Counter for consecutive output-too-long truncations (Strategy 3).
		// After maxOutputTooLongRetries, hard stop to avoid infinite loop.
		outputTooLongRetries := 0
		const maxOutputTooLongRetries = 2

		// Track transient provider error retries
		transientRetries := 0

		// Number of messages the last tool-call round appended to messages.
		// Used to tell BudgetMessages which tail messages are the current
		// iteration's tool results and must NOT be truncated — the model
		// needs them in full to formulate its next response.
		lastRoundMsgCount := 0

		// Adapt compaction config to the model's actual context window.
		// This is a one-time operation per Run() call.
		// sessionCompactor is used for config-dependent operations (NeedsCompaction,
		// BudgetMessages, Compact, TruncateOnly).  Count management (Reset/Get/
		// IncrementCompactionCount) stays on o.compactor (shared).
		sessionCompactor := o.compactor
		if o.compactor != nil {
			if cwp, ok := request.Provider.(provider.ContextWindowProvider); ok {
				sessionModel := ""
				if request.CachedSession.Session != nil {
					sessionModel = request.CachedSession.Session.Model
				}
				if cw := cwp.ContextWindow(sessionModel); cw > 0 {
					sessionCompactor = o.compactor.WithContextWindow(cw)
				}
			}
		}

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
				persistMsg(provider.RoleAssistant, "[任务被取消或超时]", nil, "")
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
			if sessionCompactor != nil {
				if sessionCompactor.NeedsCompaction(messages) {
					priorCompactions := o.compactor.GetCompactionCount(request.SessionID)
					slog.Info("StandardOrchestrator: compacting messages",
						"sessionID", request.SessionID,
						"iteration", iteration,
						"messageCount", len(messages),
						"priorCompactions", priorCompactions)

					var compacted []provider.Message

					if priorCompactions == 0 {
						// First compaction: use LLM summarization for best context retention.
						events <- types.NewContentEvent("\n⏳ 正在压缩历史上下文，请稍候…\n\n")
						compacted = sessionCompactor.CompactWithFallback(ctx, messages, request.Provider)
						slog.Info("StandardOrchestrator: compaction done (LLM summary)",
							"sessionID", request.SessionID,
							"iteration", iteration,
							"newMessageCount", len(compacted))

						// Reset connections and cooldown ONLY after LLM-based compaction,
						// because the summarization prov.Chat() call is what poisons ALB.
						if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
							resettable.ResetConnections()
							slog.Info("StandardOrchestrator: reset provider connections after compaction",
								"sessionID", request.SessionID)
						}
						slog.Info("StandardOrchestrator: post-compaction cooldown",
							"sessionID", request.SessionID, "delay", "5s")
						select {
						case <-ctx.Done():
							events <- types.NewErrorEvent(ctx.Err())
							return
						case <-time.After(5 * time.Second):
						}
					} else {
						// Second+ compaction: truncation only, NO LLM call.
						// Calling prov.Chat() mid-task for summarization causes MiniMax
						// ALB session degradation. The first compaction's summary
						// provides enough context for continuity.
						events <- types.NewContentEvent("\n⏳ 正在截断历史上下文（无LLM调用）…\n\n")
						compacted = sessionCompactor.TruncateOnly(messages)
						slog.Info("StandardOrchestrator: compaction done (truncation only, no LLM call)",
							"sessionID", request.SessionID,
							"iteration", iteration,
							"newMessageCount", len(compacted),
							"priorCompactions", priorCompactions)
						// No ResetConnections or cooldown needed — no LLM call was made.
					}

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

			// Proactive budget enforcement: truncate old tool results to keep
			// request size within provider limits.  This is fast (no LLM calls)
			// and prevents the oversized-request → empty-body failure pattern.
			//
			// We compute the actual tools JSON size so that the estimate is
			// accurate.  Without this, the default 20 KB baseline can underestimate
			// by ~30 KB when the registry has 15-25 tools (~47 KB JSON), causing
			// oversized requests (119 KB) to slip through a 65 KB budget and
			// eventually poison the MiniMax ALB session.
			toolsOverhead := 0
			if sessionCompactor != nil {
				toolsOverhead = o.calculateToolsOverhead()
				// protectedTail = lastRoundMsgCount: the assistant message with
				// tool_calls + all tool result messages from the previous iteration.
				// These are the model's freshly requested data and must be preserved
				// in full (e.g. read_file results the model needs to analyze).
				//
				// CRITICAL: on the first iteration (lastRoundMsgCount=0), we must
				// still protect at least the user's latest message and the preceding
				// LLM message.  Without this floor, BudgetMessages Phase 2 can
				// truncate the LLM's question (which is often at the END of a long
				// message) to 512 bytes, and Phase 3 can drop messages entirely.
				// The result: the LLM loses the Q&A context and re-asks the same
				// question.
				protectedTail := lastRoundMsgCount
				if protectedTail < 2 {
					protectedTail = 2
				}
				budgeted := sessionCompactor.BudgetMessages(messages, toolsOverhead, protectedTail)
				if len(budgeted) > 0 {
					messages = budgeted
				}
			}

			sessionModel := ""
			if request.CachedSession.Session != nil {
				sessionModel = request.CachedSession.Session.Model
			}
			req := o.buildChatRequest(messages, sessionModel, request.SessionID, request.Attachments)

			// Call provider
			resp, err := o.callProvider(ctx, request.Provider, req, events)
			if err != nil {
				// Reactive retry: on context window overflow, truncate and retry once
				if provider.IsContextWindowExceeded(err) && !contextRetried && sessionCompactor != nil {
					contextRetried = true
					slog.Warn("StandardOrchestrator: context window exceeded, truncating and retrying",
						"sessionID", request.SessionID, "iteration", iteration)
					events <- types.NewContentEvent("\n\n⚠️ Context window exceeded — truncating history and retrying…\n\n")
					truncated := sessionCompactor.TruncateOnly(messages)
					if len(truncated) > 0 && len(truncated) < len(messages) {
						messages = truncated
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
					// Reset connections before retry to try a fresh ALB session
					if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
						resettable.ResetConnections()
						slog.Info("StandardOrchestrator: reset provider connections before retry",
							"sessionID", request.SessionID)
					}
					select {
					case <-ctx.Done():
						events <- types.NewErrorEvent(ctx.Err())
						return
					case <-time.After(backoff):
					}
					continue
				}

				// Last resort: if retries exhausted but we have a compactor, try
				// emergency truncation to significantly reduce context size.
				// Uses TruncateOnly (no LLM call) to avoid ALB poisoning.
				if provider.IsRetryable(err) && sessionCompactor != nil && !contextRetried {
					contextRetried = true
					slog.Warn("StandardOrchestrator: retries exhausted, attempting emergency truncation",
						"sessionID", request.SessionID, "iteration", iteration,
						"messageCount", len(messages))
					events <- types.NewContentEvent("\n\n⚠️ Provider errors persist — performing emergency context truncation…\n\n")
					truncated := sessionCompactor.TruncateOnly(messages)
					if len(truncated) > 0 && len(truncated) < len(messages) {
						messages = truncated
						transientRetries = 0
						if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
							resettable.ResetConnections()
						}
						continue
					}
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

			// Validate tool call arguments JSON BEFORE execution.
			// Models can produce truncated/invalid JSON arguments in several scenarios:
			//   1. FinishReason=length: output truncated by max_tokens
			//   2. FinishReason=tool_calls but arguments incomplete: observed with
			//      MiniMax-M2.5 on large contexts where the model emits only a few
			//      bytes of arguments (e.g., '{"arguments": ') before finishing.
			// We validate ALL tool calls regardless of finish reason and drop any
			// with unparseable JSON — BUT only when some calls are valid.
			// When ALL calls are invalid, we keep them so the tool executor can
			// return error messages (giving the model feedback) and the
			// consecutiveToolErrors counter can eventually terminate the loop.
			if len(resp.ToolCalls) > 0 {
				validCalls := make([]provider.ToolCall, 0, len(resp.ToolCalls))
				droppedCount := 0
				for _, tc := range resp.ToolCalls {
					rawArgs := tc.Arguments
					if tc.Function != nil {
						rawArgs = tc.Function.Arguments
					}
					if rawArgs == "" || json.Valid([]byte(rawArgs)) {
						validCalls = append(validCalls, tc)
					} else {
						droppedCount++
						// Log both head and tail of truncated args for diagnosis:
						// head shows the tool call start, tail shows where truncation occurred.
						argsTail := ""
						if len(rawArgs) > 200 {
							argsTail = rawArgs[len(rawArgs)-200:]
						}
						slog.Warn("StandardOrchestrator: tool call has invalid JSON arguments",
							"sessionID", request.SessionID,
							"toolName", tc.Name,
							"finishReason", resp.FinishReason,
							"argsLen", len(rawArgs),
							"contentLen", len(resp.Content),
							"rawArgsHead", truncateForLog(rawArgs, 200),
							"rawArgsTail", argsTail)
					}
				}

				if droppedCount > 0 {
					slog.Warn("StandardOrchestrator: tool call JSON validation",
						"sessionID", request.SessionID,
						"validToolCalls", len(validCalls),
						"droppedToolCalls", droppedCount,
						"finishReason", resp.FinishReason)
				}

				if len(validCalls) == 0 && droppedCount > 0 {
					// ALL tool calls have invalid JSON.
					// Strategy 1: try context reduction + retry
					if !invalidArgsRetried && sessionCompactor != nil {
						invalidArgsRetried = true
						currentBytes := sessionCompactor.EstimateRequestBytes(messages, toolsOverhead)
						slog.Warn("StandardOrchestrator: all tool calls have invalid JSON args, forcing context reduction and retrying",
							"sessionID", request.SessionID,
							"iteration", iteration,
							"messageCount", len(messages),
							"estimatedBytes", currentBytes,
							"finishReason", resp.FinishReason)
						events <- types.NewContentEvent("\n\n⚠️ 模型生成的工具调用参数不完整，正在压缩上下文后重试…\n\n")

						reducedBudget := currentBytes * 60 / 100
						if reducedBudget < 20000 {
							reducedBudget = 20000
						}
						reducedCompactor := sessionCompactor.WithMaxRequestBytes(reducedBudget)
						truncated := reducedCompactor.BudgetMessages(messages, toolsOverhead, lastRoundMsgCount)

						newBytes := sessionCompactor.EstimateRequestBytes(truncated, toolsOverhead)
						if newBytes < currentBytes {
							slog.Info("StandardOrchestrator: context reduction for invalid tool args succeeded",
								"sessionID", request.SessionID,
								"beforeBytes", currentBytes,
								"afterBytes", newBytes,
								"beforeMsgs", len(messages),
								"afterMsgs", len(truncated))
							messages = truncated
							o.compactor.IncrementCompactionCount(request.SessionID)
							if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
								resettable.ResetConnections()
							}
							continue
						}
						slog.Warn("StandardOrchestrator: context reduction did not help for invalid tool args",
							"sessionID", request.SessionID,
							"beforeBytes", currentBytes,
							"afterBytes", newBytes)
					}

					// Strategy 2: retry with non-streaming Chat() endpoint.
					// Some providers (MiniMax-M2.5) have SSE streaming bugs where
					// tool call argument chunks are incomplete.  The non-streaming
					// endpoint returns the complete response in one JSON payload.
					//
					// Additionally, when reason_split=true, reasoning tokens may
					// consume part of the max_tokens budget, leaving insufficient
					// room for tool call arguments.  We increase max_tokens for
					// the retry to give the model more output room.
					if !nonStreamRetried {
						nonStreamRetried = true
						slog.Info("StandardOrchestrator: retrying with non-streaming Chat() due to truncated tool args",
							"sessionID", request.SessionID,
							"iteration", iteration,
							"originalMaxTokens", req.MaxTokens)
						events <- types.NewContentEvent("\n\n⚠️ 流式响应工具参数不完整，正在使用非流式模式重试…\n\n")

						// Boost max_tokens for retry: reasoning tokens may have
						// depleted the output budget.  Use 2x or at least 16384.
						retryReq := req
						retryReq.Stream = false
						boostedMaxTokens := req.MaxTokens * 2
						if boostedMaxTokens < 16384 {
							boostedMaxTokens = 16384
						}
						retryReq.MaxTokens = boostedMaxTokens
						slog.Info("StandardOrchestrator: non-streaming retry with boosted max_tokens",
							"sessionID", request.SessionID,
							"originalMaxTokens", req.MaxTokens,
							"boostedMaxTokens", boostedMaxTokens)

						chatResp, chatErr := request.Provider.Chat(ctx, retryReq)
						if chatErr == nil && chatResp != nil && len(chatResp.ToolCalls) > 0 {
							// Validate the non-streaming tool calls too
							allValid := true
							for _, tc := range chatResp.ToolCalls {
								rawArgs := tc.Arguments
								if tc.Function != nil {
									rawArgs = tc.Function.Arguments
								}
								if rawArgs != "" && !json.Valid([]byte(rawArgs)) {
									allValid = false
									break
								}
							}
							if allValid {
								slog.Info("StandardOrchestrator: non-streaming retry produced valid tool calls",
									"sessionID", request.SessionID,
									"toolCallCount", len(chatResp.ToolCalls))
								// Replace the response with the non-streaming one
								resp = chatResp
								// Emit tool call events for frontend display
								for _, tc := range resp.ToolCalls {
									storageTc := providerToStorageToolCall(tc)
									select {
									case events <- types.NewToolCallEvent(storageTc):
									case <-ctx.Done():
										return
									}
								}
								// Reset droppedCount so we skip the error-feedback path
								droppedCount = 0
							} else {
								slog.Warn("StandardOrchestrator: non-streaming retry also produced invalid tool call args",
									"sessionID", request.SessionID)
							}
						} else if chatErr != nil {
							slog.Warn("StandardOrchestrator: non-streaming Chat() retry failed",
								"sessionID", request.SessionID,
								"error", chatErr)
						}
					}

					// Strategy 3: all retries exhausted, tool calls still invalid.
					// The real cause is max_tokens exhaustion: model output was
					// too long and got truncated, but the provider incorrectly
					// reported finishReason=tool_calls instead of length.
					//
					// Do NOT execute invalid tool calls.  Instead, override
					// finishReason to "length", clear tool calls, and feed
					// the LLM an explicit error message so it can retry with
					// a smaller output (e.g. split large files into chunks).
					if droppedCount > 0 {
						outputTooLongRetries++
						slog.Warn("StandardOrchestrator: all retries exhausted, treating truncated tool args as output-too-long",
							"sessionID", request.SessionID,
							"droppedCount", droppedCount,
							"originalFinishReason", resp.FinishReason,
							"outputTooLongRetries", outputTooLongRetries,
							"maxOutputTooLongRetries", maxOutputTooLongRetries,
							"hasCompactor", sessionCompactor != nil,
							"invalidArgsRetried", invalidArgsRetried,
							"nonStreamRetried", nonStreamRetried)

						// Collect truncated tool names before clearing
						truncatedToolNames := make([]string, 0, len(resp.ToolCalls))
						for _, tc := range resp.ToolCalls {
							truncatedToolNames = append(truncatedToolNames, tc.Name)
						}

						// Override: treat as length-limited stop, do NOT execute
						resp.FinishReason = provider.FinishReasonLength
						resp.ToolCalls = nil

						// Hard stop if the model keeps generating oversized output
						if outputTooLongRetries >= maxOutputTooLongRetries {
							slog.Error("StandardOrchestrator: output-too-long hard stop, model keeps generating oversized tool calls",
								"sessionID", request.SessionID,
								"outputTooLongRetries", outputTooLongRetries,
								"tools", strings.Join(truncatedToolNames, ", "))
							errMsg := fmt.Sprintf(
								"[模型连续 %d 次生成超长工具调用，输出 token 不足以容纳完整参数。"+
									"请手动将大文件拆分为多次小写入（每次 <8KB），或缩短输出内容后重试。]",
								outputTooLongRetries)
							persistMsg(provider.RoleAssistant, errMsg, nil, "")
							events <- types.NewContentEvent("\n\n❌ " + errMsg + "\n\n")
							events <- types.NewDoneEvent(&totalUsage)
							return
						}

						errMsg := fmt.Sprintf(
							"[System: Your tool call output was too long and got truncated "+
								"(exceeded max output tokens). The tool call arguments were "+
								"incomplete / invalid JSON and cannot be executed. "+
								"Total %d tool call(s) failed (tools: %s). "+
								"You MUST reduce the output size — for example, when writing "+
								"large files, split the content into multiple smaller write_file "+
								"calls (each under 8KB). Do NOT repeat the same large tool call. "+
								"This is attempt %d of %d before the task is terminated.]",
							droppedCount, strings.Join(truncatedToolNames, ", "),
							outputTooLongRetries, maxOutputTooLongRetries)

						events <- types.NewContentEvent("\n\n⚠️ 模型输出超长导致工具调用参数被截断（超出最大输出 token 限制），已中止执行。请缩小输出内容后重试。\n\n")

						// Append as assistant message so the LLM sees the feedback
						messages = append(messages, provider.Message{
							Role:    provider.RoleAssistant,
							Content: errMsg,
						})
						persistMsg(provider.RoleAssistant, errMsg, nil, "")
						continue
					}
				} else if droppedCount > 0 {
					// Some valid, some invalid — keep only valid ones
					resp.ToolCalls = validCalls
				}
			}

			shouldStop := resp.FinishReason == provider.FinishReasonStop ||
				(resp.FinishReason == provider.FinishReasonLength && len(resp.ToolCalls) == 0) ||
				(len(resp.ToolCalls) == 0 && resp.FinishReason != provider.FinishReasonToolCalls)

			if shouldStop {
				// Detect abnormal empty stop response: model returned stop with
				// no content AND no tool calls.  This typically happens when the
				// request payload is too large for the provider to process (even
				// if it didn't return an explicit error).  The pattern has been
				// observed with GLM-5 at ~167 KB request size.
				//
				// Strategy: use BudgetMessages with an artificially reduced byte
				// budget (60% of current estimated size) to force aggressive
				// truncation, then retry once.  TruncateOnly (token-based) is
				// insufficient because the problem is byte-size, not token count.
				if resp.Content == "" && len(resp.ToolCalls) == 0 && iteration > 0 {
					if !emptyResponseRetried && sessionCompactor != nil {
						emptyResponseRetried = true
						currentBytes := sessionCompactor.EstimateRequestBytes(messages, toolsOverhead)
						slog.Warn("StandardOrchestrator: empty stop response detected, forcing byte-budget truncation and retrying",
							"sessionID", request.SessionID,
							"iteration", iteration,
							"messageCount", len(messages),
							"estimatedBytes", currentBytes,
							"finishReason", resp.FinishReason)
						events <- types.NewContentEvent("\n\n⚠️ 模型返回空响应（可能上下文过长），正在压缩后重试…\n\n")

						// Force truncation: use 60% of current size as budget
						reducedBudget := currentBytes * 60 / 100
						if reducedBudget < 20000 {
							reducedBudget = 20000 // floor: 20 KB
						}
						reducedCompactor := sessionCompactor.WithMaxRequestBytes(reducedBudget)
						truncated := reducedCompactor.BudgetMessages(messages, toolsOverhead, lastRoundMsgCount)

						newBytes := sessionCompactor.EstimateRequestBytes(truncated, toolsOverhead)
						if newBytes < currentBytes {
							slog.Info("StandardOrchestrator: byte-budget truncation succeeded",
								"sessionID", request.SessionID,
								"beforeBytes", currentBytes,
								"afterBytes", newBytes,
								"beforeMsgs", len(messages),
								"afterMsgs", len(truncated))
							messages = truncated
							o.compactor.IncrementCompactionCount(request.SessionID)
							// Reset connections to get a fresh ALB session
							if resettable, ok := request.Provider.(provider.ConnectionResettable); ok {
								resettable.ResetConnections()
							}
							continue
						}
						slog.Warn("StandardOrchestrator: byte-budget truncation did not reduce size, cannot retry",
							"sessionID", request.SessionID,
							"beforeBytes", currentBytes,
							"afterBytes", newBytes)
					}

					// Already retried or truncation didn't help: report error
					slog.Error("StandardOrchestrator: empty stop response, giving up",
						"sessionID", request.SessionID,
						"iteration", iteration,
						"emptyResponseRetried", emptyResponseRetried)
					errMsg := "[模型返回了空响应，可能是上下文过长导致。请尝试新建会话或缩短输入内容。]"
					persistMsg(provider.RoleAssistant, errMsg, nil, "")
					events <- types.NewContentEvent(errMsg)
					events <- types.NewDoneEvent(&totalUsage)
					return
				}

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
					persistMsg(provider.RoleAssistant, respContent, nil, "")
				} else {
					persistMsg(provider.RoleAssistant, "[任务已完成，无文本响应]", nil, "")
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

			// Successful provider call — reset transient retry counter
			transientRetries = 0

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

			// Track how many messages this round added (1 assistant + N tool results)
			// so BudgetMessages on the next iteration knows what to protect.
			lastRoundMsgCount = 1 + len(toolResults)

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
					// First time hitting threshold: give the model one final chance.
					// Second time (counter exceeded threshold): hard terminate.
					if consecutiveToolErrors > maxConsecutiveToolErrors {
						slog.Error("StandardOrchestrator: hard stop after final chance exhausted",
							"sessionID", request.SessionID,
							"consecutiveErrors", consecutiveToolErrors)
						errMsg := fmt.Sprintf("[连续 %d 轮工具调用全部失败，任务已终止]", consecutiveToolErrors)
						events <- types.Event{
							Type:    types.EventTypeDone,
							Content: errMsg,
						}
						return
					}

					slog.Error("StandardOrchestrator: consecutive tool errors hit threshold, giving model one final chance",
						"sessionID", request.SessionID,
						"consecutiveErrors", consecutiveToolErrors)
					errMsg := fmt.Sprintf("[System: %d consecutive tool call rounds have ALL failed. "+
						"The tools or services you are calling may be unavailable. "+
						"Do NOT call the same failing tool again. "+
						"Either try a completely different approach, use a different tool, or explain to the user what went wrong and stop.]",
						consecutiveToolErrors)
					persistMsg(provider.RoleAssistant, errMsg, nil, "")
					messages = append(messages, provider.Message{
						Role:    provider.RoleAssistant,
						Content: errMsg,
					})
					events <- types.Event{
						Type:    types.EventTypeContent,
						Content: "\n\n⚠️ " + errMsg + "\n\n",
					}
					// Give the model one final chance: continue the loop so
					// the model sees the error message and can respond with
					// text (stop) or try a different tool.  If it fails again,
					// consecutiveToolErrors > maxConsecutiveToolErrors triggers hard stop above.
					continue
				}
			} else {
				// Reset error counter on successful tool execution
				consecutiveToolErrors = 0
			}

			// Save to session (skip in InjectedMessages mode — PDA engine owns context)
			storageTcs := convertToolCalls(resp.ToolCalls)
			persistMsg(provider.RoleAssistant, resp.Content, storageTcs, "")
			for _, result := range toolResults {
				persistMsg(result.Role, result.Content, nil, result.ToolCallID)
			}
			slog.Info("StandardOrchestrator: iteration saved, continuing to next",
				"sessionID", request.SessionID, "iteration", iteration)
		}

		// Max iterations reached
		slog.Warn("StandardOrchestrator: max iterations reached",
			"sessionID", request.SessionID,
			"maxIterations", o.config.MaxIterations)
		maxIterMsg := fmt.Sprintf("[已达到最大迭代次数 (%d)，任务执行被中止]", o.config.MaxIterations)
		persistMsg(provider.RoleAssistant, maxIterMsg, nil, "")
		events <- types.NewErrorEvent(ErrMaxIterations)
	}()

	return events, nil
}

// callProvider calls the provider in Stream mode and collects the response.
func (o *StandardOrchestrator) callProvider(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- types.Event) (*provider.ChatResponse, error) {
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
			select {
			case events <- types.Event{Type: types.EventTypeContent, Content: event.Delta}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if event.Thinking != "" {
			thinkingBuilder += event.Thinking
			select {
			case events <- types.Event{Type: types.EventTypeThinking, Thinking: event.Thinking}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
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
			select {
			case events <- types.Event{
				Type: types.EventTypeToolCallUpdate,
				ToolCallUpdate: &types.ToolCallUpdateEvent{
					ToolCallID: event.ToolCallUpdate.ID,
					ToolName:   event.ToolCallUpdate.Name,
					Status:     event.ToolCallUpdate.Status,
					Arguments:  event.ToolCallUpdate.Arguments,
				},
			}:
			case <-ctx.Done():
				return nil, ctx.Err()
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
		select {
		case events <- types.Event{Type: types.EventTypeContent, Content: thinkingBuilder}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Convert pending tool calls to final slice and emit tool call events
	for _, tc := range pendingToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, *tc)
		// Emit tool call event for frontend display
		storageTc := providerToStorageToolCall(*tc)
		select {
		case events <- types.NewToolCallEvent(storageTc):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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

	// Build system prompt: SystemPromptBuilder (primary) or static config (fallback)
	var sysPromptContent string
	if o.systemPrompt != nil {
		// SystemPromptBuilder handles: memory search, MCP injection, tool listing, slots
		content, err := o.systemPrompt.Build(ctx, userInput)
		if err == nil && content != "" {
			sysPromptContent = content
		}
	} else if o.config.SystemPrompt != "" {
		// Static config fallback
		sysPromptContent = o.config.SystemPrompt
	} else {
		// No prompt configured - use minimal default
		sysPromptContent = "You are a helpful AI assistant."
	}

	slog.Info("standard orchestrator: system prompt built",
		"usedBuilder", o.systemPrompt != nil,
		"staticConfigPrompt", o.config.SystemPrompt,
		"finalPromptLen", len(sysPromptContent),
		"finalPromptPreview", truncateForLog(sysPromptContent, 500),
	)
	// Inject skills into system prompt before passing to ContextManager
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
			sysPromptContent += "\n\n" + skillsContent
		}
	}

	// Use Context Manager if available (preferred for advanced compression)
	if o.contextManager != nil && cached.Session != nil {
		ctxMessages, err := o.contextManager.BuildContext(ctx, cached.Session.ID, sysPromptContent, userInput)
		if err != nil {
			slog.Warn("context manager failed, falling back to legacy method",
				"sessionID", cached.Session.ID,
				"error", err)
			// Fall through to legacy method below
		} else {
			return ctxMessages, nil
		}
	}

	// Legacy method: manual message loading
	if sysPromptContent != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: sysPromptContent,
		})
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

// calculateToolsOverhead returns the JSON-serialized size of the tool
// definitions that will be included in every ChatRequest.  This allows
// BudgetMessages to use an accurate baseline instead of the default 20 KB
// estimate, which can be off by 2-3x for large registries.
func (o *StandardOrchestrator) calculateToolsOverhead() int {
	if o.registry == nil {
		return 0
	}
	tools, err := o.registry.ToProviderTools()
	if err != nil || len(tools) == 0 {
		return 0
	}
	data, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return len(data)
}
