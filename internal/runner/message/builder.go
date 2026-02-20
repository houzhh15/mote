package message

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	internalContext "mote/internal/context"
	"mote/internal/memory"
	"mote/internal/prompt"
	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/skills"
)

// Builder 负责构建 LLM 请求消息
type Builder interface {
	// BuildMessages 构建完整的消息列表
	BuildMessages(ctx context.Context, request *BuildRequest) ([]provider.Message, error)
}

// BuildRequest 封装构建请求
type BuildRequest struct {
	SessionID     string
	UserInput     string
	CachedSession *scheduler.CachedSession
	SystemPrompt  string // Optional static system prompt override
}

// StandardBuilder 实现标准消息构建逻辑
type StandardBuilder struct {
	systemPrompt   *prompt.SystemPromptBuilder
	skillManager   *skills.Manager
	contextManager *internalContext.Manager
	memoryIndex    *memory.MemoryIndex
	staticPrompt   string // Fallback static prompt
}

// NewStandardBuilder 创建标准消息构建器
func NewStandardBuilder() *StandardBuilder {
	return &StandardBuilder{}
}

// SetSystemPrompt 设置系统提示词构建器
func (b *StandardBuilder) SetSystemPrompt(sp *prompt.SystemPromptBuilder) {
	b.systemPrompt = sp
}

// SetSkillManager 设置技能管理器
func (b *StandardBuilder) SetSkillManager(sm *skills.Manager) {
	b.skillManager = sm
}

// SetContextManager 设置上下文管理器
func (b *StandardBuilder) SetContextManager(cm *internalContext.Manager) {
	b.contextManager = cm
}

// SetMemoryIndex 设置记忆索引
func (b *StandardBuilder) SetMemoryIndex(m *memory.MemoryIndex) {
	b.memoryIndex = m
}

// SetStaticPrompt 设置静态提示词
func (b *StandardBuilder) SetStaticPrompt(prompt string) {
	b.staticPrompt = prompt
}

// BuildMessages 构建消息列表
func (b *StandardBuilder) BuildMessages(ctx context.Context, request *BuildRequest) ([]provider.Message, error) {
	// Build system prompt using SystemPromptBuilder (primary) or static config (fallback)
	var sysPromptContent string
	var err error

	if b.systemPrompt != nil {
		// SystemPromptBuilder handles: memory search, MCP injection, tool listing, slots
		sysPromptContent, err = b.systemPrompt.Build(ctx, request.UserInput)
		if err != nil {
			return nil, fmt.Errorf("build system prompt: %w", err)
		}
	} else if request.SystemPrompt != "" {
		// Use request-level override
		sysPromptContent = request.SystemPrompt
		sysPromptContent = b.injectMemoryContext(ctx, sysPromptContent, request.UserInput)
	} else if b.staticPrompt != "" {
		// Static config fallback - manually inject memory if available
		sysPromptContent = b.staticPrompt
		sysPromptContent = b.injectMemoryContext(ctx, sysPromptContent, request.UserInput)
	} else {
		// No prompt configured - use minimal default
		sysPromptContent = "You are a helpful AI assistant."
		sysPromptContent = b.injectMemoryContext(ctx, sysPromptContent, request.UserInput)
	}

	// Inject skills section if skillManager is available
	if b.skillManager != nil {
		skillsSection := skills.NewPromptSection(b.skillManager)
		// Apply session-level skill selection filter
		if request.CachedSession.Session != nil && len(request.CachedSession.Session.SelectedSkills) > 0 {
			skillsSection.WithSelectedSkills(request.CachedSession.Session.SelectedSkills)
		}
		if section := skillsSection.Build(); section != "" {
			sysPromptContent += "\n\n" + section
		}
		if activePrompts := skillsSection.BuildActivePrompts(); activePrompts != "" {
			sysPromptContent += "\n" + activePrompts
		}
	}

	// Use Context Manager if available (preferred for advanced compression)
	if b.contextManager != nil {
		messages, err := b.contextManager.BuildContext(ctx, request.CachedSession.Session.ID, sysPromptContent, request.UserInput)
		if err != nil {
			slog.Warn("context manager failed, falling back to legacy method",
				"sessionID", request.CachedSession.Session.ID,
				"error", err)
			// Fall through to legacy method
		} else {
			return messages, nil
		}
	}

	// Legacy method: manual message loading
	var messages []provider.Message

	// Add system message
	if sysPromptContent != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: sysPromptContent,
		})
	}

	// Add history messages
	for _, msg := range request.CachedSession.Messages {
		provMsg := provider.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				provTc := provider.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
				}
				// Parse function from json.RawMessage
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
		Content: request.UserInput,
	})

	return messages, nil
}

// injectMemoryContext 注入记忆上下文（如果可用）
func (b *StandardBuilder) injectMemoryContext(ctx context.Context, prompt, userInput string) string {
	if b.memoryIndex == nil {
		return prompt
	}

	// Search for relevant memories
	memories, err := b.memoryIndex.Search(ctx, userInput, 5)
	if err != nil || len(memories) == 0 {
		return prompt
	}

	// Build memory context section
	var memoryContext string
	memoryContext += "\n\n## Relevant Memory Context\n\n"
	for i, mem := range memories {
		memoryContext += fmt.Sprintf("%d. %s\n", i+1, mem.Content)
	}

	return prompt + memoryContext
}
