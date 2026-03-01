package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"mote/internal/provider"
	"mote/internal/runner/message"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/storage"
)

// ACPOrchestrator 处理 ACP 协议的 provider 执行
// ACP providers 在内部处理工具调用循环和上下文管理，因此我们只需：
// 1. 构建消息（包括历史）
// 2. 注入技能
// 3. 单次 Stream 调用
// 4. 转发所有事件
//
// 注意：ACP server 自行管理会话上下文，mote 侧不需要主动压缩。
// 正常情况下 ACP 只收到最新一条用户消息（session 已存在时）。
// 仅在上下文溢出时使用 TruncateOnly 截断历史（不调用 LLM）。
type ACPOrchestrator struct {
	*BaseOrchestrator
	messageBuilder message.Builder
}

// NewACPOrchestrator 创建 ACP 协调器
func NewACPOrchestrator(base *BaseOrchestrator) *ACPOrchestrator {
	return &ACPOrchestrator{
		BaseOrchestrator: base,
	}
}

// SetMessageBuilder 设置消息构建器
func (o *ACPOrchestrator) SetMessageBuilder(builder message.Builder) {
	o.messageBuilder = builder
}

// Run 执行 ACP 模式的运行循环
func (o *ACPOrchestrator) Run(ctx context.Context, request *RunRequest) (<-chan types.Event, error) {
	events := make(chan types.Event, 100)

	go func() {
		defer close(events)
		o.runACPLoop(ctx, request, events)
	}()

	return events, nil
}

// runACPLoop 执行 ACP 循环的核心逻辑
func (o *ACPOrchestrator) runACPLoop(ctx context.Context, request *RunRequest, events chan<- types.Event) {
	sessionID := request.SessionID
	userInput := request.UserInput
	prov := request.Provider
	cached := request.CachedSession

	// Inject session-bound workspace directory into system prompt
	if o.systemPrompt != nil && o.workspaceResolver != nil && sessionID != "" {
		if wsPath := o.workspaceResolver(sessionID); wsPath != "" {
			o.systemPrompt.SetWorkspaceDir(wsPath)
		}
	}

	// 1. 添加用户消息到会话
	_, err := o.sessions.AddMessage(sessionID, provider.RoleUser, userInput, nil, "")
	if err != nil {
		events <- types.NewErrorEvent(err)
		return
	}

	// 2. 构建消息（包括历史）
	var messages []provider.Message
	if o.messageBuilder != nil {
		// 使用新的 MessageBuilder
		messages, err = o.messageBuilder.BuildMessages(ctx, &message.BuildRequest{
			CachedSession: cached,
			UserInput:     userInput,
		})
	} else {
		// 临时回退：使用 legacy buildMessages
		messages, err = o.buildMessagesLegacy(ctx, cached, userInput)
	}

	if err != nil {
		events <- types.NewErrorEvent(fmt.Errorf("build messages: %w", err))
		return
	}

	slog.Info("ACPOrchestrator: built messages with history",
		"sessionID", sessionID,
		"messageCount", len(messages),
		"cachedMessageCount", len(cached.Messages))

	// 3. ACP server 自行管理会话上下文，不需要 mote 侧压缩。
	//    正常情况下 ACP 只收到最新一条用户消息（ensureSession 时 isNewSession=false），
	//    不存在上下文膨胀问题。唯一可能膨胀的场景是 isNewSession=true（mote 重启后）,
	//    此时 buildPromptWithAttachments 会注入完整历史。但即使如此，也应由上下文溢出
	//    重试路径处理（见下方 step 7），而非在此主动调用 LLM summarization，以避免：
	//    - 浪费一次 premium request 做无意义摘要
	//    - 向 ACP session 发送摘要请求导致 session 状态污染

	// 4. 注入技能到系统消息
	messages = o.injectSkills(sessionID, cached, messages)

	// 5. 清理消息
	messages = provider.SanitizeMessages(messages)

	// 6. 构建请求
	// ACP providers 在内部处理工具调用，因此不传递外部 Tools/Temperature/MaxTokens
	// 这与原始 runACPMode 行为一致
	req := o.buildACPChatRequest(messages, cached.Session.Model, sessionID, request.Attachments)

	slog.Info("ACPOrchestrator: starting ACP execution",
		"sessionID", sessionID,
		"model", req.Model,
		"finalMessageCount", len(messages))

	// 7. 调用 provider.Stream()
	provEvents, err := prov.Stream(ctx, req)
	if err != nil {
		// 重试逻辑：如果上下文窗口超限，截断后重试一次。
		// 使用 TruncateOnly（不调用 prov.Chat()）：
		//   - ACP 自行管理上下文，LLM 摘要是多余的
		//   - 避免浪费 premium request
		//   - 避免 MiniMax ALB session 退化（如果通过 ACP 使用 MiniMax 模型）
		if provider.IsContextWindowExceeded(err) && o.compactor != nil {
			slog.Warn("ACPOrchestrator: context window exceeded, truncating and retrying",
				"sessionID", sessionID)
			events <- types.NewContentEvent("\n\n⚠️ Context window exceeded — truncating history and retrying…\n\n")
			truncated := o.compactor.TruncateOnly(messages)
			if len(truncated) > 0 && len(truncated) < len(messages) {
				req.Messages = truncated
				provEvents, err = prov.Stream(ctx, req)
			}
		}
		if err != nil {
			events <- types.NewErrorEvent(err)
			return
		}
	}

	// 8. 转发事件并累积结果
	o.forwardAndSaveEvents(sessionID, provEvents, events)
}

// injectSkills 将技能注入到系统消息中
func (o *ACPOrchestrator) injectSkills(sessionID string, cached *scheduler.CachedSession, messages []provider.Message) []provider.Message {
	if o.skillManager == nil {
		return messages
	}

	// 查找系统消息索引
	systemMsgIdx := -1
	for i, msg := range messages {
		if msg.Role == provider.RoleSystem {
			systemMsgIdx = i
			break
		}
	}

	// 构建技能内容
	skillsSection := skills.NewPromptSection(o.skillManager)
	if cached.Session != nil && len(cached.Session.SelectedSkills) > 0 {
		skillsSection.WithSelectedSkills(cached.Session.SelectedSkills)
		slog.Info("ACPOrchestrator: applying selected skills filter",
			"sessionID", sessionID,
			"selectedSkills", cached.Session.SelectedSkills)
	}

	var skillsContent string
	if section := skillsSection.Build(); section != "" {
		skillsContent += section
	}
	if activePrompts := skillsSection.BuildActivePrompts(); activePrompts != "" {
		skillsContent += "\n" + activePrompts
	}

	if skillsContent == "" {
		return messages
	}

	// 注入技能
	if systemMsgIdx >= 0 {
		// 追加到现有系统消息
		messages[systemMsgIdx].Content += "\n\n" + skillsContent
		slog.Info("ACPOrchestrator: appended skills to existing system message",
			"sessionID", sessionID,
			"skillsLen", len(skillsContent))
	} else {
		// 前置新的系统消息
		messages = append([]provider.Message{{
			Role:    provider.RoleSystem,
			Content: skillsContent,
		}}, messages...)
		slog.Info("ACPOrchestrator: prepended new system message with skills",
			"sessionID", sessionID,
			"skillsLen", len(skillsContent))
	}

	return messages
}

// forwardAndSaveEvents 转发 provider 事件并保存结果
func (o *ACPOrchestrator) forwardAndSaveEvents(sessionID string, provEvents <-chan provider.ChatEvent, events chan<- types.Event) {
	var assistantContent strings.Builder
	var totalUsage types.Usage
	var toolCallEvents []provider.ToolCall

	for event := range provEvents {
		switch event.Type {
		case provider.EventTypeContent:
			assistantContent.WriteString(event.Delta)
			events <- types.NewContentEvent(event.Delta)

		case provider.EventTypeThinking:
			if event.Thinking != "" {
				slog.Debug("ACPOrchestrator: forwarding thinking event",
					"sessionID", sessionID,
					"thinkingLen", len(event.Thinking))
				events <- types.Event{
					Type:     types.EventTypeThinking,
					Thinking: event.Thinking,
				}
			}

		case provider.EventTypeToolCall:
			if event.ToolCall != nil {
				toolCallEvents = append(toolCallEvents, *event.ToolCall)
				// 转换为 storage 格式并发出事件
				tc := &storage.ToolCall{
					ID:   event.ToolCall.ID,
					Type: "function",
				}
				if event.ToolCall.Name != "" {
					funcData, _ := json.Marshal(map[string]string{
						"name":      event.ToolCall.Name,
						"arguments": event.ToolCall.Arguments,
					})
					tc.Function = funcData
				}
				events <- types.NewToolCallEvent(tc)
			}

		case provider.EventTypeToolCallUpdate:
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

		case provider.EventTypeDone:
			// 更新使用量
			if event.Usage != nil {
				totalUsage.PromptTokens += event.Usage.PromptTokens
				totalUsage.CompletionTokens += event.Usage.CompletionTokens
				totalUsage.TotalTokens += event.Usage.TotalTokens
			}

			// 保存助手消息
			content := assistantContent.String()
			if content != "" {
				_, _ = o.sessions.AddMessage(sessionID, provider.RoleAssistant, content, nil, "")
			}

			// 记录完成信息
			contentPreview := content
			if len(contentPreview) > 500 {
				contentPreview = contentPreview[:500] + "...[truncated]"
			}
			slog.Info("ACPOrchestrator: completed",
				"sessionID", sessionID,
				"contentLen", len(content),
				"contentPreview", contentPreview,
				"toolCalls", len(toolCallEvents),
				"finishReason", event.FinishReason)

			events <- types.NewDoneEvent(&totalUsage)
			return

		case provider.EventTypeError:
			slog.Error("ACPOrchestrator: error from provider",
				"sessionID", sessionID,
				"error", event.Error)
			events <- types.NewErrorEvent(event.Error)
			return
		}
	}

	// 如果没有收到 done 事件就结束了，记录警告
	slog.Warn("ACPOrchestrator: provider events ended without done event",
		"sessionID", sessionID)
	events <- types.NewDoneEvent(&totalUsage)
}

// buildMessagesLegacy 是临时的回退实现，用于在 MessageBuilder 未设置时使用
// TODO: 在完全集成后移除
func (o *ACPOrchestrator) buildMessagesLegacy(ctx context.Context, cached *scheduler.CachedSession, userInput string) ([]provider.Message, error) {
	var sysPromptContent string
	var err error

	if o.systemPrompt != nil {
		// SystemPromptBuilder handles: memory search, MCP injection, tool listing, slots
		sysPromptContent, err = o.systemPrompt.Build(ctx, userInput)
		if err != nil {
			return nil, fmt.Errorf("build system prompt: %w", err)
		}
	} else if o.config.SystemPrompt != "" {
		// Static config fallback
		sysPromptContent = o.config.SystemPrompt
	} else {
		sysPromptContent = "You are a helpful AI assistant."
	}

	slog.Info("acp orchestrator: system prompt built",
		"usedBuilder", o.systemPrompt != nil,
		"staticConfigPrompt", o.config.SystemPrompt,
		"finalPromptLen", len(sysPromptContent),
		"finalPromptPreview", truncateForLog(sysPromptContent, 500),
	)

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: sysPromptContent},
	}

	// 添加历史消息
	for _, msg := range cached.Messages {
		messages = append(messages, provider.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 添加当前用户输入
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userInput,
	})

	return messages, nil
}

// buildACPChatRequest 构建 ACP 模式专用的聊天请求
// ACP providers 在内部处理工具调用循环，因此：
// - 不传递外部 Tools（工具由 ACP 协议内部管理）
// - 不传递 Temperature/MaxTokens（由 ACP provider 内部控制）
// 这与原始 runACPMode 行为一致
func (o *ACPOrchestrator) buildACPChatRequest(messages []provider.Message, model string, sessionID string, attachments []provider.Attachment) provider.ChatRequest {
	// Sanitize messages to remove corrupted tool call data
	messages = provider.SanitizeMessages(messages)

	return provider.ChatRequest{
		Model:          model,
		Messages:       messages,
		Attachments:    attachments,
		Stream:         true,
		ConversationID: sessionID,
	}
}
