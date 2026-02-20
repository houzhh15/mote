package orchestrator

import (
	"context"

	"mote/internal/compaction"
	"mote/internal/hooks"
	"mote/internal/mcp/client"
	"mote/internal/prompt"
	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/tools"
)

// BaseOrchestrator 提供共享的基础功能
type BaseOrchestrator struct {
	// Core dependencies
	sessions     *scheduler.SessionManager
	registry     *tools.Registry
	compactor    *compaction.Compactor
	systemPrompt *prompt.SystemPromptBuilder

	// Optional components
	skillManager *skills.Manager
	hookManager  *hooks.Manager
	mcpManager   *client.Manager

	// Configuration
	config Config
}

// NewBaseOrchestrator 创建基础协调器
func NewBaseOrchestrator(
	sessions *scheduler.SessionManager,
	registry *tools.Registry,
	config Config,
) *BaseOrchestrator {
	return &BaseOrchestrator{
		sessions: sessions,
		registry: registry,
		config:   config,
	}
}

// SetCompactor 设置压缩器
func (b *BaseOrchestrator) SetCompactor(c *compaction.Compactor) {
	b.compactor = c
}

// SetSystemPrompt 设置系统提示词构建器
func (b *BaseOrchestrator) SetSystemPrompt(sp *prompt.SystemPromptBuilder) {
	b.systemPrompt = sp
}

// SetSkillManager 设置技能管理器
func (b *BaseOrchestrator) SetSkillManager(sm *skills.Manager) {
	b.skillManager = sm
}

// SetHookManager 设置钩子管理器
func (b *BaseOrchestrator) SetHookManager(hm *hooks.Manager) {
	b.hookManager = hm
}

// SetMCPManager 设置 MCP 管理器
func (b *BaseOrchestrator) SetMCPManager(m *client.Manager) {
	b.mcpManager = m
}

// triggerHook 触发钩子
func (b *BaseOrchestrator) triggerHook(ctx context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	if b.hookManager == nil {
		return nil, nil
	}
	return b.hookManager.Trigger(ctx, hookCtx)
}

// compressIfNeeded 如果需要则压缩历史
func (b *BaseOrchestrator) compressIfNeeded(ctx context.Context, messages []provider.Message, prov provider.Provider) []provider.Message {
	if b.compactor != nil && b.compactor.NeedsCompaction(messages) {
		compacted := b.compactor.CompactWithFallback(ctx, messages, prov)
		
		// Sanity check: compacted result must have at least a user or assistant message
		hasConv := false
		for _, m := range compacted {
			if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
				hasConv = true
				break
			}
		}
		if hasConv {
			b.compactor.IncrementCompactionCount(messages[0].Content) // Use first message as session indicator
			return compacted
		}
	}
	return messages
}

// buildChatRequest 构建聊天请求
func (b *BaseOrchestrator) buildChatRequest(messages []provider.Message, model string, sessionID string, attachments []provider.Attachment) provider.ChatRequest {
	tools, _ := b.registry.ToProviderTools()

	// Sanitize messages to remove corrupted tool call data
	messages = provider.SanitizeMessages(messages)

	req := provider.ChatRequest{
		Model:          model,
		Messages:       messages,
		Attachments:    attachments,
		Tools:          tools,
		Temperature:    b.config.Temperature,
		MaxTokens:      b.config.MaxTokens,
		Stream:         b.config.StreamOutput,
		ConversationID: sessionID,
	}
	return req
}
