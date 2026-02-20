package orchestrator

import (
	"mote/internal/compaction"
	"mote/internal/hooks"
	"mote/internal/mcp/client"
	"mote/internal/prompt"
	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/tools"
)

// BuilderOptions 用于构建 Orchestrator 的选项
type BuilderOptions struct {
	Sessions     *scheduler.SessionManager
	Registry     *tools.Registry
	Config       Config
	Compactor    *compaction.Compactor
	SystemPrompt *prompt.SystemPromptBuilder
	SkillManager *skills.Manager
	HookManager  *hooks.Manager
	MCPManager   *client.Manager
	ToolExecutor ToolExecutorFunc
}

// OrchestratorBuilder 构建器用于创建 Orchestrator
type OrchestratorBuilder struct {
	opts BuilderOptions
}

// NewBuilder 创建新的构建器
func NewBuilder(opts BuilderOptions) *OrchestratorBuilder {
	return &OrchestratorBuilder{opts: opts}
}

// Build 根据 provider 类型构建合适的 orchestrator
func (b *OrchestratorBuilder) Build(prov provider.Provider) Orchestrator {
	// 创建基础 orchestrator
	base := NewBaseOrchestrator(b.opts.Sessions, b.opts.Registry, b.opts.Config)
	
	// 设置可选组件
	if b.opts.Compactor != nil {
		base.SetCompactor(b.opts.Compactor)
	}
	if b.opts.SystemPrompt != nil {
		base.SetSystemPrompt(b.opts.SystemPrompt)
	}
	if b.opts.SkillManager != nil {
		base.SetSkillManager(b.opts.SkillManager)
	}
	if b.opts.HookManager != nil {
		base.SetHookManager(b.opts.HookManager)
	}
	if b.opts.MCPManager != nil {
		base.SetMCPManager(b.opts.MCPManager)
	}
	if b.opts.ToolExecutor != nil {
		base.SetToolExecutor(b.opts.ToolExecutor)
	}

	// 根据 provider 类型选择合适的 orchestrator
	if acpProv, ok := prov.(provider.ACPCapable); ok && acpProv.IsACPProvider() {
		return NewACPOrchestrator(base)
	}

	// 默认使用 StandardOrchestrator
	return NewStandardOrchestrator(base)
}
