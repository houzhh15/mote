package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	internalChannel "mote/internal/channel"
	"mote/internal/channel/imessage"
	"mote/internal/channel/notes"
	"mote/internal/channel/reminders"
	"mote/internal/compaction"
	"mote/internal/config"
	internalContext "mote/internal/context"
	"mote/internal/hooks"
	"mote/internal/mcp/client"
	"mote/internal/memory"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/prompt"

	"mote/internal/provider"
	"mote/internal/runner/delegate"
	cfg "mote/internal/runner/delegate/cfg"
	"mote/internal/runner/orchestrator"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/tools"
	"mote/pkg/channel"

	"github.com/google/uuid"
)

// Runner executes agent runs with tool calling capabilities.
type Runner struct {
	provider     provider.Provider           // legacy single provider (deprecated, use providerPool)
	providerPool *provider.Pool              // dynamic provider pool for multi-model support
	multiPool    *provider.MultiProviderPool // multi-provider pool for Ollama + Copilot support
	defaultModel string                      // default model for this runner instance
	registry     *tools.Registry
	sessions     *scheduler.SessionManager
	config       Config
	systemPrompt *prompt.SystemPromptBuilder // Primary system prompt builder
	mu           sync.RWMutex

	// M04: Optional advanced components
	compactor      *compaction.Compactor
	contextManager *internalContext.Manager // M04-Enhanced: Context compression with persistence

	// M07: Skills and Hooks integration
	skillManager *skills.Manager
	hookManager  *hooks.Manager

	// M08: Policy and Approval integration
	policyExecutor  policy.PolicyChecker
	approvalManager approval.ApprovalHandler

	// M08B+: Compiled custom scrub rules (from policy config)
	compiledScrubRules []CompiledScrubRule

	// Multi-agent direct delegation
	delegateFactory *delegate.SubRunnerFactory

	// M08B+: Block message template and circuit breaker
	blockMessageTemplate    string
	circuitBreakerThreshold int
	blockCounts             map[string]map[string]int // sessionID → toolName → count
	blockCountsMu           sync.Mutex

	// M08B+: Workspace resolver for PathPrefix $WORKSPACE expansion
	workspaceResolver func(sessionID string) string

	// MCP integration
	mcpManager *client.Manager

	// Channel system integration
	channelRegistry *internalChannel.Registry

	// Pause control
	pauseController PauseController
	pauseMu         sync.RWMutex

	// Session-level execution queue: serializes runs per session to prevent
	// concurrent access when cron/API trigger overlapping requests.
	runQueue *scheduler.RunQueue
}

// NewRunner creates a new Runner with a single provider.
// Deprecated: Use NewRunnerWithPool for multi-model support.
func NewRunner(
	prov provider.Provider,
	registry *tools.Registry,
	sessions *scheduler.SessionManager,
	config Config,
) *Runner {
	return &Runner{
		provider: prov,
		registry: registry,
		sessions: sessions,
		config:   config,
		runQueue: scheduler.NewRunQueue(10, 5*time.Minute),
	}
}

// NewRunnerWithPool creates a new Runner with a provider pool for multi-model support.
// The defaultModel is used when a session doesn't specify a model.
func NewRunnerWithPool(
	pool *provider.Pool,
	defaultModel string,
	registry *tools.Registry,
	sessions *scheduler.SessionManager,
	config Config,
) *Runner {
	return &Runner{
		providerPool: pool,
		defaultModel: defaultModel,
		registry:     registry,
		sessions:     sessions,
		config:       config,
		runQueue:     scheduler.NewRunQueue(10, 5*time.Minute),
	}
}

// SetProviderPool sets the provider pool for dynamic model selection.
// This enables the runner to use different models based on session configuration.
func (r *Runner) SetProviderPool(pool *provider.Pool, defaultModel string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providerPool = pool
	r.defaultModel = defaultModel
}

// SetMultiProviderPool sets the multi-provider pool for Ollama + Copilot support.
// defaultModel is the model to use when a session doesn't specify one (e.g. "ollama:llama3.2").
func (r *Runner) SetMultiProviderPool(pool *provider.MultiProviderPool, defaultModel string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.multiPool = pool
	if defaultModel != "" {
		r.defaultModel = defaultModel
	}
	slog.Info("SetMultiProviderPool called", "pool_not_nil", pool != nil, "defaultModel", defaultModel, "providers", func() []string {
		if pool != nil {
			return pool.ListProviders()
		}
		return nil
	}())
}

// UpdateDefaultModel updates the fallback model used when a session or agent
// doesn't specify an explicit model. This propagates to the delegate factory
// so PDA sub-agents inherit the correct model at runtime.
func (r *Runner) UpdateDefaultModel(model string) {
	r.mu.Lock()
	r.defaultModel = model
	factory := r.delegateFactory
	r.mu.Unlock()

	if factory != nil {
		factory.SetDefaultModel(model)
	}
	slog.Info("Runner.UpdateDefaultModel", "model", model)
}

// ResetSession performs a full resource cleanup for a session.
// This is called when model/workspace changes require rebuilding all runtime state.
// It clears: 1) session cache, 2) provider session state (ACP sessions, CLI mappings).
// The next request will re-read from DB and create fresh provider sessions.
func (r *Runner) ResetSession(sessionID string) {
	slog.Info("Runner.ResetSession: starting session resource cleanup",
		"sessionID", sessionID)

	// 1. Invalidate session cache so next request re-reads from DB
	if r.sessions != nil {
		r.sessions.Invalidate(sessionID)
		slog.Info("Runner.ResetSession: session cache invalidated",
			"sessionID", sessionID)
	}

	// 2. Reset provider session state (ACP sessions, CLI process mappings)
	r.mu.RLock()
	multiPool := r.multiPool
	r.mu.RUnlock()

	if multiPool != nil {
		multiPool.ResetProviderSession(sessionID)
		slog.Info("Runner.ResetSession: provider sessions reset",
			"sessionID", sessionID)
	}

	slog.Info("Runner.ResetSession: session resource cleanup completed",
		"sessionID", sessionID)
}

// GetProvider returns the provider for the given model.
// If model is empty, uses the default model.
// Supports multi-provider pool for Ollama models (with "ollama:" prefix).
// Falls back to the legacy single provider if no pool is configured.
func (r *Runner) GetProvider(model string) (provider.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	slog.Debug("GetProvider called", "model", model, "hasMultiPool", r.multiPool != nil, "defaultModel", r.defaultModel)

	// Try multi-provider pool first (for any model including empty)
	if r.multiPool != nil {
		effectiveModel := model
		if effectiveModel == "" {
			effectiveModel = r.defaultModel
		}
		slog.Debug("GetProvider trying multiPool", "effectiveModel", effectiveModel)
		if effectiveModel != "" {
			prov, providerName, err := r.multiPool.GetProvider(effectiveModel)
			if err == nil {
				slog.Debug("GetProvider got provider from multiPool", "providerName", providerName, "effectiveModel", effectiveModel)
				return prov, nil
			}
			slog.Warn("GetProvider multiPool error, model may not be registered in any enabled provider",
				"model", effectiveModel, "error", err)
			// When multiPool is configured but the specific model is not found,
			// do NOT silently fall back to the legacy provider — that provider
			// may be a different type (e.g., ACP) that doesn't support this model.
			// Instead, return the error so the caller knows routing failed.
			return nil, fmt.Errorf("model %q not available in any enabled provider: %w", effectiveModel, err)
		}
	}

	// Use provider pool if available
	if r.providerPool != nil {
		if model == "" {
			model = r.defaultModel
		}
		if model == "" {
			return nil, ErrNoProvider
		}
		return r.providerPool.Get(model)
	}

	// Fallback to legacy single provider (only when no multiPool is configured)
	if r.provider == nil {
		return nil, ErrNoProvider
	}
	return r.provider, nil
}

// SetSystemPrompt sets the M04 system prompt builder.
func (r *Runner) SetSystemPrompt(sp *prompt.SystemPromptBuilder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.systemPrompt = sp
}

// SetCompactor sets the optional M04 compactor for history compression.
func (r *Runner) SetCompactor(c *compaction.Compactor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compactor = c
}

// SetContextManager sets the context manager for advanced context compression.
func (r *Runner) SetContextManager(cm *internalContext.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contextManager = cm
}

// SetMemory is a no-op retained for API compatibility.
// Memory is now configured directly on SystemPromptBuilder and message.StandardBuilder.
// Deprecated: configure memory on the prompt builder instead.
func (r *Runner) SetMemory(_ *memory.MemoryIndex) {}

// SetSkillManager sets the optional M07 skill manager.
func (r *Runner) SetSkillManager(sm *skills.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skillManager = sm
}

// SetHookManager sets the optional M07 hook manager.
func (r *Runner) SetHookManager(hm *hooks.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hookManager = hm
}

// triggerHook is a helper that safely triggers hooks with nil checks.
// Returns a default continue result if hookManager is nil.
func (r *Runner) triggerHook(ctx context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	r.mu.RLock()
	hm := r.hookManager
	r.mu.RUnlock()

	if hm == nil {
		return hooks.ContinueResult(), nil
	}
	return hm.Trigger(ctx, hookCtx)
}

// triggerHookWithContinue is like triggerHook but returns a simple continue boolean.
// Returns true if the chain should continue, false if interrupted.
func (r *Runner) triggerHookWithContinue(ctx context.Context, hookCtx *hooks.Context) bool {
	result, _ := r.triggerHook(ctx, hookCtx)
	return result != nil && result.Continue
}

// SetPolicyExecutor sets the optional M08 policy executor.
func (r *Runner) SetPolicyExecutor(pe policy.PolicyChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policyExecutor = pe
}

// SetApprovalManager sets the optional M08 approval manager.
func (r *Runner) SetApprovalManager(am approval.ApprovalHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalManager = am
}

// SetScrubRules compiles and sets custom scrub rules from policy config.
func (r *Runner) SetScrubRules(rules []policy.ScrubRule) error {
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compiledScrubRules = compiled
	return nil
}

// SetBlockMessageTemplate sets the custom block message template.
func (r *Runner) SetBlockMessageTemplate(template string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockMessageTemplate = template
}

// SetCircuitBreakerThreshold sets circuit breaker threshold. 0 disables.
func (r *Runner) SetCircuitBreakerThreshold(threshold int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.circuitBreakerThreshold = threshold
}

// formatBlockMessage formats a blocked tool message using the template or default.
func formatBlockMessage(template, toolName, reason string) string {
	if template == "" {
		return "Tool call blocked by policy: " + reason
	}
	replacer := strings.NewReplacer("{tool}", toolName, "{reason}", reason)
	return replacer.Replace(template)
}

// incrementBlockCount increments and returns the block count for a tool in a session.
func (r *Runner) incrementBlockCount(sessionID, toolName string) int {
	r.blockCountsMu.Lock()
	defer r.blockCountsMu.Unlock()
	if r.blockCounts == nil {
		r.blockCounts = make(map[string]map[string]int)
	}
	if r.blockCounts[sessionID] == nil {
		r.blockCounts[sessionID] = make(map[string]int)
	}
	r.blockCounts[sessionID][toolName]++
	return r.blockCounts[sessionID][toolName]
}

// clearBlockCounts clears block counts for a session.
func (r *Runner) clearBlockCounts(sessionID string) {
	r.blockCountsMu.Lock()
	defer r.blockCountsMu.Unlock()
	delete(r.blockCounts, sessionID)
}

// SetWorkspaceResolver sets the function used to resolve workspace paths for sessions.
func (r *Runner) SetWorkspaceResolver(resolver func(sessionID string) string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceResolver = resolver
}

// SetMCPManager sets the MCP client manager for dynamic tool injection in prompts.
func (r *Runner) SetMCPManager(m *client.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpManager = m
	if r.systemPrompt != nil {
		r.systemPrompt.WithMCPManager(m)
	}
}

// InitDelegateSupport initializes multi-agent delegation if agents are configured.
// It creates the SubRunnerFactory and registers the delegate tool.
// The manage_agents tool is always registered so the LLM can create agents even when none exist.
// Returns the tracker (for API queries) and the factory (for post-init wiring like workspace binder).
func (r *Runner) InitDelegateSupport(appCfg *config.Config, db *sql.DB) (*delegate.DelegationTracker, *delegate.SubRunnerFactory) {
	r.mu.RLock()
	registry := r.registry
	r.mu.RUnlock()

	// Always register manage_agents tool so the LLM can create the first agent
	manageAgentsTool := delegate.NewManageAgentsTool()
	registry.Register(manageAgentsTool)

	r.mu.RLock()
	multiPool := r.multiPool
	sessions := r.sessions
	hookMgr := r.hookManager
	mcpMgr := r.mcpManager
	compactorRef := r.compactor
	sysPr := r.systemPrompt
	ctxMgr := r.contextManager
	skillMgr := r.skillManager
	defModel := r.defaultModel
	cfg := r.config
	r.mu.RUnlock()

	if multiPool == nil {
		slog.Warn("delegate: MultiProviderPool not set, skipping delegate init")
		return nil, nil
	}

	factory := delegate.NewSubRunnerFactory(delegate.SubRunnerFactoryOptions{
		MultiPool:      multiPool,
		Sessions:       sessions,
		ParentRegistry: registry,
		HookManager:    hookMgr,
		MCPManager:     mcpMgr,
		Compactor:      compactorRef,
		SystemPrompt:   sysPr,
		ContextManager: ctxMgr,
		SkillManager:   skillMgr,
		DefaultModel:   defModel,
		MaxIterations:  cfg.MaxIterations,
		MaxTokens:      cfg.MaxTokens,
		Temperature:    cfg.Temperature,
		Timeout:        cfg.Timeout,
	})

	// Always create and wire tracker if DB is available, even when no agents exist yet.
	// Agents may be created dynamically via manage_agents tool at runtime.
	var tracker *delegate.DelegationTracker
	if db != nil {
		tracker = delegate.NewTracker(db)
		factory.SetTracker(tracker)
		slog.Info("delegate: tracker initialized for audit logging")
	}

	globalMaxDepth := appCfg.Delegate.GetMaxDepth()

	// Always register delegate tool — it reads agents dynamically from config
	// so agents created at runtime via manage_agents are immediately available.
	delegateTool := delegate.NewDelegateTool(factory, globalMaxDepth)
	registry.Register(delegateTool)

	// Store factory on runner for direct delegate support (@ mentions)
	r.mu.Lock()
	r.delegateFactory = factory
	r.mu.Unlock()

	numAgents := len(appCfg.Agents)
	slog.Info("delegate: initialized multi-agent support",
		"agents", numAgents,
		"maxDepth", globalMaxDepth)

	return tracker, factory
}

// InitChannels 根据配置初始化渠道系统
func (r *Runner) InitChannels(cfg config.ChannelsConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.channelRegistry = internalChannel.NewRegistry()

	// iMessage
	if cfg.IMessage.Enabled {
		imsgCfg := imessage.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        cfg.IMessage.Trigger.Prefix,
				CaseSensitive: cfg.IMessage.Trigger.CaseSensitive,
				SelfTrigger:   cfg.IMessage.Trigger.SelfTrigger,
				AllowList:     cfg.IMessage.Trigger.AllowList,
			},
			Reply: channel.ReplyConfig{
				Prefix:    cfg.IMessage.Reply.Prefix,
				Separator: cfg.IMessage.Reply.Separator,
			},
			SelfID:    cfg.IMessage.SelfID,
			AllowFrom: cfg.IMessage.AllowFrom,
		}
		imsgCh := imessage.New(imsgCfg)
		imsgCh.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(imsgCh)
		slog.Info("registered iMessage channel", "selfID", cfg.IMessage.SelfID, "allowFrom", cfg.IMessage.AllowFrom)
	}

	// Apple Notes
	if cfg.AppleNotes.Enabled {
		notesCfg := notes.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        cfg.AppleNotes.Trigger.Prefix,
				CaseSensitive: cfg.AppleNotes.Trigger.CaseSensitive,
				SelfTrigger:   cfg.AppleNotes.Trigger.SelfTrigger,
				AllowList:     cfg.AppleNotes.Trigger.AllowList,
			},
			Reply: channel.ReplyConfig{
				Prefix:    cfg.AppleNotes.Reply.Prefix,
				Separator: cfg.AppleNotes.Reply.Separator,
			},
			WatchFolder:   cfg.AppleNotes.WatchFolder,
			ArchiveFolder: cfg.AppleNotes.ArchiveFolder,
			PollInterval:  cfg.AppleNotes.PollInterval,
		}
		notesCh := notes.New(notesCfg)
		notesCh.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(notesCh)
		slog.Info("registered Apple Notes channel", "watchFolder", cfg.AppleNotes.WatchFolder)
	}

	// Apple Reminders
	if cfg.AppleReminders.Enabled {
		remindersCfg := reminders.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        cfg.AppleReminders.Trigger.Prefix,
				CaseSensitive: cfg.AppleReminders.Trigger.CaseSensitive,
				SelfTrigger:   cfg.AppleReminders.Trigger.SelfTrigger,
				AllowList:     cfg.AppleReminders.Trigger.AllowList,
			},
			Reply: channel.ReplyConfig{
				Prefix:    cfg.AppleReminders.Reply.Prefix,
				Separator: cfg.AppleReminders.Reply.Separator,
			},
			WatchList:    cfg.AppleReminders.WatchList,
			PollInterval: cfg.AppleReminders.PollInterval,
		}
		remindersCh := reminders.New(remindersCfg)
		remindersCh.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(remindersCh)
		slog.Info("registered Apple Reminders channel", "watchList", cfg.AppleReminders.WatchList)
	}

	return nil
}

// ChannelRegistry 返回渠道注册表
func (r *Runner) ChannelRegistry() *internalChannel.Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channelRegistry
}

// StartChannel 启动指定类型的渠道
func (r *Runner) StartChannel(ctx context.Context, channelType channel.ChannelType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channelRegistry == nil {
		r.channelRegistry = internalChannel.NewRegistry()
	}

	// 检查渠道是否已注册
	if plugin, exists := r.channelRegistry.Get(channelType); exists {
		// 已注册，直接启动
		return plugin.Start(ctx)
	}

	// 渠道未注册，需要先创建
	switch channelType {
	case channel.ChannelTypeIMessage:
		cfg := imessage.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: false,
				SelfTrigger:   true,
			},
			Reply: channel.ReplyConfig{
				Prefix:    "[Mote]",
				Separator: "\n",
			},
		}
		ch := imessage.New(cfg)
		ch.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(ch)
		slog.Info("registered iMessage channel on-demand")
		return ch.Start(ctx)

	case channel.ChannelTypeNotes:
		cfg := notes.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        "@mote:",
				CaseSensitive: false,
			},
			Reply: channel.ReplyConfig{
				Prefix:    "[Mote 回复]",
				Separator: "\n",
			},
			WatchFolder:   "Mote Inbox",
			ArchiveFolder: "Mote Archive",
			PollInterval:  5 * time.Second,
		}
		ch := notes.New(cfg)
		ch.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(ch)
		slog.Info("registered Apple Notes channel on-demand")
		return ch.Start(ctx)

	case channel.ChannelTypeReminders:
		cfg := reminders.Config{
			Trigger: channel.TriggerConfig{
				Prefix:        "@mote:",
				CaseSensitive: false,
			},
			Reply: channel.ReplyConfig{
				Prefix:    "[Mote]",
				Separator: "\n",
			},
			WatchList:    "Mote",
			PollInterval: 5 * time.Second,
		}
		ch := reminders.New(cfg)
		ch.OnMessage(r.handleChannelMessage)
		r.channelRegistry.Register(ch)
		slog.Info("registered Apple Reminders channel on-demand")
		return ch.Start(ctx)

	default:
		return fmt.Errorf("unsupported channel type: %s", channelType)
	}
}

// StopChannel 停止指定类型的渠道
func (r *Runner) StopChannel(ctx context.Context, channelType channel.ChannelType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.channelRegistry == nil {
		return fmt.Errorf("channel registry not initialized")
	}

	plugin, exists := r.channelRegistry.Get(channelType)
	if !exists {
		return fmt.Errorf("channel not found: %s", channelType)
	}

	return plugin.Stop(ctx)
}

// getChannelModel 获取渠道专属模型配置，返回空字符串则使用默认模型
func (r *Runner) getChannelModel(channelType channel.ChannelType) string {
	cfg := config.GetConfig()
	if cfg == nil {
		return ""
	}
	switch channelType {
	case channel.ChannelTypeIMessage:
		return cfg.Channels.IMessage.Model
	case channel.ChannelTypeNotes:
		return cfg.Channels.AppleNotes.Model
	case channel.ChannelTypeReminders:
		return cfg.Channels.AppleReminders.Model
	default:
		return ""
	}
}

// handleChannelMessage 处理来自渠道的消息
func (r *Runner) handleChannelMessage(ctx context.Context, msg channel.InboundMessage) error {
	// 使用 ChatID 作为 sessionID
	sessionID := fmt.Sprintf("channel:%s:%s", msg.ChannelType, msg.ChatID)

	slog.Info("handling channel message",
		"channelType", msg.ChannelType,
		"chatID", msg.ChatID,
		"senderID", msg.SenderID,
		"contentLen", len(msg.Content),
	)

	// 解析 per-channel 模型配置
	channelModel := r.getChannelModel(msg.ChannelType)

	// 运行 agent（使用渠道专属模型或默认模型）
	var events <-chan Event
	var err error
	if channelModel != "" {
		events, err = r.RunWithModel(ctx, sessionID, msg.Content, channelModel, "channel")
	} else {
		events, err = r.Run(ctx, sessionID, msg.Content)
	}
	if err != nil {
		slog.Error("failed to run agent for channel message", "error", err)
		return fmt.Errorf("run agent: %w", err)
	}

	// 收集响应
	var response string
	for event := range events {
		switch event.Type {
		case EventTypeContent:
			response += event.Content
		case EventTypeError:
			slog.Error("agent error for channel message", "error", event.ErrorMsg)
			return fmt.Errorf("agent error: %s", event.ErrorMsg)
		}
	}

	// 发送回复
	if response != "" {
		r.mu.RLock()
		registry := r.channelRegistry
		r.mu.RUnlock()

		if registry == nil {
			return fmt.Errorf("channel registry not initialized")
		}

		plugin, ok := registry.Get(msg.ChannelType)
		if !ok {
			return fmt.Errorf("channel not found: %s", msg.ChannelType)
		}

		outbound := channel.OutboundMessage{
			ChannelType: msg.ChannelType,
			ChatID:      msg.ChatID,
			Content:     response,
			ReplyToID:   msg.ID,
		}

		if err := plugin.SendMessage(ctx, outbound); err != nil {
			slog.Error("failed to send channel reply", "error", err)
			return fmt.Errorf("send reply: %w", err)
		}

		slog.Info("sent channel reply",
			"channelType", msg.ChannelType,
			"chatID", msg.ChatID,
			"responseLen", len(response),
		)
	}

	return nil
}

// RunDirectDelegate executes a sub-agent directly, bypassing the main agent LLM.
// This is used when the user explicitly selects a sub-agent via @ mention.
// Events are streamed back through the returned channel, identical to Run().
func (r *Runner) RunDirectDelegate(ctx context.Context, parentSessionID, agentName, userPrompt string) (<-chan Event, error) {
	r.mu.RLock()
	factory := r.delegateFactory
	r.mu.RUnlock()

	if factory == nil {
		return nil, fmt.Errorf("delegate support not initialized")
	}

	// Look up agent config
	appCfg := config.GetConfig()
	if appCfg == nil {
		return nil, fmt.Errorf("config not available")
	}
	agentCfg, ok := appCfg.Agents[agentName]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentName)
	}
	if !agentCfg.IsEnabled() {
		return nil, fmt.Errorf("agent %q is disabled", agentName)
	}

	// Build delegation context (depth 0 since this is a user-initiated direct call)
	dc := &delegate.DelegateContext{
		Depth:           0,
		MaxDepth:        appCfg.Delegate.GetMaxDepth(),
		ParentSessionID: parentSessionID,
		AgentName:       agentName,
		Chain:           []string{agentName},
	}

	events := make(chan Event, 100)

	go func() {
		defer close(events)
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("PANIC in direct delegate goroutine", "panic", rec, "agent", agentName)
				events <- NewErrorEvent(fmt.Errorf("internal error: %v", rec))
			}
		}()

		// Forward sub-agent events to the channel
		sink := delegate.ParentEventSink(func(event types.Event) {
			events <- FromTypesEvent(event)
		})

		var result string
		var usage types.Usage
		var err error

		// Check if agent has structured steps → use PDA engine
		if agentCfg.HasSteps() {
			slog.Info("direct delegate: agent has steps, using PDA engine",
				"agent", agentName, "steps", len(agentCfg.Steps))
			result, usage, err = factory.RunPDAWithEvents(ctx, dc, agentCfg, userPrompt, sink)
		} else {
			result, usage, err = factory.RunDelegateWithEvents(ctx, dc, agentCfg, userPrompt, sink)
		}

		if err != nil {
			events <- NewErrorEvent(fmt.Errorf("agent %s: %w", agentName, err))
			return
		}

		// Send done event with summary
		events <- Event{
			Type:      EventTypeDone,
			Content:   result,
			AgentName: agentName,
			Usage:     &Usage{TotalTokens: usage.TotalTokens, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens},
		}
	}()

	return events, nil
}

// buildPDAResumeFn creates a closure that resumes PDA execution from a checkpoint.
// This is called when pda_control("continue") is invoked. The closure runs the
// full PDA pipeline via the SubRunnerFactory and returns the final result.
func (r *Runner) buildPDAResumeFn(
	factory *delegate.SubRunnerFactory,
	cp *cfg.PDACheckpoint,
	sessionID string,
	events chan<- Event,
) delegate.PDAResumeFunc {
	return func(ctx context.Context) (string, error) {
		agentName := cp.AgentName
		prompt := cp.InitialPrompt

		// Look up agent config
		appCfg := config.GetConfig()
		if appCfg == nil {
			return "", fmt.Errorf("config not available")
		}
		agentCfg, ok := appCfg.Agents[agentName]
		if !ok {
			return "", fmt.Errorf("agent %q not found in config", agentName)
		}
		if !agentCfg.IsEnabled() {
			return "", fmt.Errorf("agent %q is disabled", agentName)
		}

		// Rebuild delegate context from checkpoint
		dc := &delegate.DelegateContext{
			Depth:             cp.DelegateInfo.Depth,
			MaxDepth:          cp.DelegateInfo.MaxDepth,
			ParentSessionID:   sessionID,
			AgentName:         agentName,
			Chain:             cp.DelegateInfo.Chain,
			RecursionCounters: cp.DelegateInfo.RecursionCounters,
		}

		// Forward sub-agent events to the runner events channel
		sink := delegate.ParentEventSink(func(event types.Event) {
			re := FromTypesEvent(event)
			select {
			case events <- re:
			case <-ctx.Done():
			}
		})

		slog.Info("pda_control: resuming PDA via factory",
			"agent", agentName,
			"sessionID", sessionID,
			"interruptStep", cp.InterruptStep)

		// RunPDAWithEvents will load the checkpoint from storage and resume
		result, _, err := factory.RunPDAWithEvents(ctx, dc, agentCfg, prompt, sink)
		return result, err
	}
}

// ResumePDA resumes an interrupted PDA in a session from its checkpoint.
// Returns a streaming event channel, identical to Run()/RunDirectDelegate().
func (r *Runner) ResumePDA(ctx context.Context, sessionID string) (<-chan Event, error) {
	r.mu.RLock()
	factory := r.delegateFactory
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("delegate support not initialized")
	}
	if r.sessions == nil || r.sessions.DB() == nil {
		return nil, fmt.Errorf("session store not available")
	}

	cp, err := delegate.LoadPDACheckpoint(r.sessions.DB(), sessionID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}
	if cp == nil {
		return nil, fmt.Errorf("no PDA checkpoint found for session %s", sessionID)
	}

	events := make(chan Event, 100)

	go func() {
		defer close(events)
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("PANIC in PDA resume goroutine", "panic", rec, "sessionID", sessionID)
				events <- NewErrorEvent(fmt.Errorf("internal error: %v", rec))
			}
		}()

		resumeFn := r.buildPDAResumeFn(factory, cp, sessionID, events)
		result, err := resumeFn(ctx)
		if err != nil {
			events <- NewErrorEvent(fmt.Errorf("PDA resume failed for agent %s: %w", cp.AgentName, err))
			return
		}

		events <- Event{
			Type:      EventTypeDone,
			Content:   result,
			AgentName: cp.AgentName,
		}
	}()

	return events, nil
}

// ClearPDACheckpoint removes a PDA checkpoint from a session, enabling a fresh start.
func (r *Runner) ClearPDACheckpoint(sessionID string) error {
	if r.sessions == nil || r.sessions.DB() == nil {
		return fmt.Errorf("session store not available")
	}
	return delegate.ClearPDACheckpoint(r.sessions.DB(), sessionID)
}

// GetPDACheckpointInfo returns basic info about a PDA checkpoint for a session, or nil if none exists.
func (r *Runner) GetPDACheckpointInfo(sessionID string) map[string]any {
	if r.sessions == nil || r.sessions.DB() == nil {
		return nil
	}
	cp, err := delegate.LoadPDACheckpoint(r.sessions.DB(), sessionID)
	if err != nil || cp == nil {
		return nil
	}
	return map[string]any{
		"agent_name":       cp.AgentName,
		"interrupt_step":   cp.InterruptStep,
		"interrupt_agent":  cp.InterruptAgent,
		"interrupt_reason": cp.InterruptReason,
		"executed_steps":   cp.ExecutedSteps,
		"initial_prompt":   cp.InitialPrompt,
		"created_at":       cp.CreatedAt,
	}
}

// Run starts an agent run and returns a channel of events.
func (r *Runner) Run(ctx context.Context, sessionID, userInput string, attachments ...provider.Attachment) (<-chan Event, error) {
	slog.Info("Runner.Run called", "sessionID", sessionID, "hasMultiPool", r.multiPool != nil)

	// Get provider - will be resolved in runLoop based on session model
	if r.provider == nil && r.providerPool == nil && r.multiPool == nil {
		return nil, ErrNoProvider
	}

	// Initialize pause controller if not already done
	r.initPauseController()

	// Apply timeout - cancel must be deferred inside the goroutine, not here
	var cancel context.CancelFunc
	if r.config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
	}

	events := make(chan Event, 100)

	// Enqueue through session-level queue to serialize concurrent requests
	_, enqueueErr := r.runQueue.Enqueue(sessionID, ctx, func(qCtx context.Context) error {
		defer close(events)
		if cancel != nil {
			defer cancel()
		}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("PANIC in runner goroutine", "panic", rec, "sessionID", sessionID)
				events <- NewErrorEvent(fmt.Errorf("internal error: %v", rec))
			}
		}()
		r.runLoop(qCtx, sessionID, userInput, attachments, events)
		return nil
	})
	if enqueueErr != nil {
		close(events)
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("session %s is busy: %w", sessionID, enqueueErr)
	}

	return events, nil
}

// RunWithModel starts an agent run with a specific model and scenario.
// If the session doesn't exist, it will be created with the specified model and scenario.
// CancelSession cancels all pending and running tasks for a session.
// This is used by cron to abort stuck executions so subsequent runs can proceed.
func (r *Runner) CancelSession(sessionID string) {
	r.runQueue.Cancel(sessionID)
}

func (r *Runner) RunWithModel(ctx context.Context, sessionID, userInput, model, scenario string, attachments ...provider.Attachment) (<-chan Event, error) {
	if r.provider == nil && r.providerPool == nil && r.multiPool == nil {
		return nil, ErrNoProvider
	}

	// Initialize pause controller if not already done
	r.initPauseController()

	// Apply timeout - cancel must be deferred inside the goroutine, not here
	var cancel context.CancelFunc
	if r.config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
	}

	events := make(chan Event, 100)

	// Enqueue through session-level queue to serialize concurrent requests
	_, enqueueErr := r.runQueue.Enqueue(sessionID, ctx, func(qCtx context.Context) error {
		defer close(events)
		if cancel != nil {
			defer cancel()
		}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("PANIC in runner goroutine (RunWithModel)", "panic", rec, "sessionID", sessionID)
				events <- NewErrorEvent(fmt.Errorf("internal error: %v", rec))
			}
		}()
		r.runLoopWithModel(qCtx, sessionID, userInput, model, scenario, attachments, events)
		return nil
	})
	if enqueueErr != nil {
		close(events)
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("session %s is busy: %w", sessionID, enqueueErr)
	}

	return events, nil
}

// runLoopWithModel is the main agent execution loop with explicit model/scenario.
func (r *Runner) runLoopWithModel(ctx context.Context, sessionID, userInput, model, scenario string, attachments []provider.Attachment, events chan<- Event) {
	// Get or create session
	cached, err := r.sessions.GetOrCreate(sessionID, nil)
	if err != nil {
		events <- NewErrorEvent(err)
		return
	}

	// Update session model if specified and different
	if model != "" && cached.Session != nil && cached.Session.Model != model {
		if r.sessions != nil && r.sessions.DB() != nil {
			if err := r.sessions.DB().UpdateSessionModel(cached.Session.ID, model); err != nil {
				slog.Warn("failed to update session model", "sessionID", sessionID, "error", err)
			} else {
				cached.Session.Model = model
			}
		}
	}

	// Use the specified model or fall back to session model
	effectiveModel := model
	if effectiveModel == "" && cached.Session != nil {
		effectiveModel = cached.Session.Model
	}

	// Get provider for this model
	prov, err := r.GetProvider(effectiveModel)
	if err != nil {
		events <- NewErrorEvent(fmt.Errorf("failed to get provider: %w", err))
		return
	}

	// Continue with the rest of the run loop (reuse the common logic)
	r.runLoopCoreWithOrchestrator(ctx, cached, sessionID, userInput, attachments, prov, events)
}

// runLoop is the main agent execution loop.
func (r *Runner) runLoop(ctx context.Context, sessionID, userInput string, attachments []provider.Attachment, events chan<- Event) {
	// Get or create session
	cached, err := r.sessions.GetOrCreate(sessionID, nil)
	if err != nil {
		events <- NewErrorEvent(err)
		return
	}

	// Determine which provider to use based on session model
	sessionModel := ""
	if cached.Session != nil {
		sessionModel = cached.Session.Model
	}
	slog.Debug("runLoop getting provider", "sessionID", sessionID, "sessionModel", sessionModel)
	prov, err := r.GetProvider(sessionModel)
	if err != nil {
		events <- NewErrorEvent(fmt.Errorf("failed to get provider: %w", err))
		return
	}
	slog.Debug("runLoop got provider", "sessionID", sessionID, "sessionModel", sessionModel, "providerName", prov.Name())

	// Call the core loop with the resolved provider
	r.runLoopCoreWithOrchestrator(ctx, cached, sessionID, userInput, attachments, prov, events)
}

// runLoopCoreWithOrchestrator 使用新的模块化 Orchestrator 架构执行核心循环
func (r *Runner) runLoopCoreWithOrchestrator(ctx context.Context, cached *scheduler.CachedSession, sessionID, userInput string, attachments []provider.Attachment, prov provider.Provider, events chan<- Event) {
	// M07: Trigger session_create hook for new sessions
	if len(cached.Messages) == 0 {
		hookCtx := hooks.NewContext(hooks.HookSessionCreate)
		hookCtx.Session = &hooks.SessionContext{
			ID:        sessionID,
			CreatedAt: time.Now(),
		}
		_, _ = r.triggerHook(ctx, hookCtx)
	}

	// M07: Trigger before_message hook
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Message = &hooks.MessageContext{
		Content: userInput,
		Role:    string(provider.RoleUser),
		From:    "user",
	}
	hookCtx.Session = &hooks.SessionContext{ID: sessionID}

	result, _ := r.triggerHook(ctx, hookCtx)
	if result != nil && !result.Continue {
		events <- NewErrorEvent(ErrHookInterrupted)
		return
	}
	// Apply modifications
	if result != nil && result.Modified {
		if modified, ok := result.Data["content"].(string); ok {
			userInput = modified
		}
	}

	// MCP JSON preprocessing - detect and handle MCP configurations directly
	if r.mcpManager != nil {
		if result := r.PreprocessMCPInput(ctx, userInput); result != nil && result.Handled {
			// Save user message
			_, _ = r.sessions.AddMessage(sessionID, provider.RoleUser, userInput, nil, "")
			// Save response as assistant message
			_, _ = r.sessions.AddMessage(sessionID, provider.RoleAssistant, result.Response, nil, "")
			// Send response event
			events <- Event{
				Type:    EventTypeContent,
				Content: result.Response,
			}
			events <- Event{Type: EventTypeDone}
			return
		}
	}

	// 创建 Orchestrator builder
	// Check for PDA checkpoint and inject pda_control tool if one exists
	registry := r.registry
	systemPromptExtra := ""
	if r.sessions != nil && r.sessions.DB() != nil {
		if cp, err := delegate.LoadPDACheckpoint(r.sessions.DB(), sessionID); err == nil && cp != nil {
			slog.Info("runLoopCore: PDA checkpoint detected, injecting pda_control tool",
				"agent", cp.AgentName,
				"interruptStep", cp.InterruptStep,
				"sessionID", sessionID)
			registry = r.registry.Clone()

			// Build a resume function that actually executes the PDA pipeline
			var resumeFn delegate.PDAResumeFunc
			r.mu.RLock()
			factory := r.delegateFactory
			r.mu.RUnlock()
			if factory != nil {
				resumeFn = r.buildPDAResumeFn(factory, cp, sessionID, events)
			}

			pdaTool := delegate.NewPDAControlTool(delegate.PDAControlToolOptions{
				Store:      r.sessions.DB(),
				SessionID:  sessionID,
				Checkpoint: cp,
				ResumeFn:   resumeFn,
			})
			if regErr := registry.Register(pdaTool); regErr != nil {
				slog.Warn("runLoopCore: failed to register pda_control tool", "error", regErr)
			} else {
				systemPromptExtra = delegate.PDAResumeHint(cp)
			}
		}
	}

	effectiveSystemPrompt := r.config.SystemPrompt + systemPromptExtra

	orchBuilder := orchestrator.NewBuilder(orchestrator.BuilderOptions{
		Sessions: r.sessions,
		Registry: registry,
		Config: orchestrator.Config{
			MaxIterations: r.config.MaxIterations,
			MaxTokens:     r.config.MaxTokens,
			Temperature:   r.config.Temperature,
			StreamOutput:  true,
			Timeout:       r.config.Timeout,
			SystemPrompt:  effectiveSystemPrompt, // Static fallback + checkpoint hint
		},
		Compactor:      r.compactor,
		SystemPrompt:   r.systemPrompt,
		SkillManager:   r.skillManager,
		HookManager:    r.hookManager,
		MCPManager:     r.mcpManager,
		ContextManager: r.contextManager,
		// Inject full tool executor from Runner (includes policy, hooks, heartbeat, truncation).
		// Use the local `registry` (which may be a clone with pda_control) for tool lookup,
		// falling back to r.registry for everything else handled by executeToolsWithSession.
		ToolExecutor: func(ctx context.Context, toolCalls []provider.ToolCall, sessionID string) ([]provider.Message, int) {
			return r.executeToolsWithSession(ctx, toolCalls, events, sessionID, "", registry)
		},
		WorkspaceResolver: r.workspaceResolver,
	})

	// 构建合适的 orchestrator（根据 provider 类型）
	orch := orchBuilder.Build(prov)

	// 创建运行请求
	req := &orchestrator.RunRequest{
		SessionID:     sessionID,
		UserInput:     userInput,
		Attachments:   attachments,
		Provider:      prov,
		CachedSession: cached,
	}

	// 执行 orchestrator
	slog.Info("runLoopCoreWithOrchestrator: starting orchestrator",
		"sessionID", sessionID,
		"provider", prov.Name(),
		"orchestratorType", fmt.Sprintf("%T", orch))

	orchEvents, err := orch.Run(ctx, req)
	if err != nil {
		events <- NewErrorEvent(err)
		return
	}

	// 转发所有事件，转换为 runner.Event
	// Use context-aware send to prevent goroutine deadlock when
	// the downstream consumer (HTTP handler) disconnects.
	for event := range orchEvents {
		re := FromTypesEvent(event)
		if re.Type == EventTypeThinking {
			slog.Debug("runner: forwarding thinking event to chat handler",
				"sessionID", sessionID,
				"thinkingLen", len(re.Thinking))
		}
		select {
		case events <- re:
		case <-ctx.Done():
			// Context cancelled — drain remaining orchEvents so the
			// orchestrator goroutine doesn't block on its channel sends.
			go func() {
				for range orchEvents {
				}
			}()
			return
		}
	}
}

// executeToolsWithSession executes tool calls with session context for policy checks.
// It sends heartbeat events every 15 seconds during long-running tool executions to keep the connection alive.
// Returns the tool result messages and the count of tool executions that returned errors.
// registryOverride, if non-nil, is tried first for tool lookup (used for session-scoped tools like pda_control).
func (r *Runner) executeToolsWithSession(ctx context.Context, toolCalls []provider.ToolCall, events chan<- Event, sessionID, agentID string, registryOverride ...*tools.Registry) ([]provider.Message, int) {
	// Determine the effective registry for tool execution
	effectiveRegistry := r.registry
	if len(registryOverride) > 0 && registryOverride[0] != nil {
		effectiveRegistry = registryOverride[0]
	}

	var results []provider.Message
	errorCount := 0

	// Start heartbeat goroutine to keep connection alive during tool execution
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				// Send heartbeat to keep connection alive
				select {
				case events <- NewHeartbeatEvent():
					slog.Info("sent heartbeat during tool execution", "sessionID", sessionID)
				default:
					slog.Warn("heartbeat channel full during tool execution", "sessionID", sessionID)
				}
			}
		}
	}()

	// Inject session ID and agent ID into context for skill tools
	if sessionID != "" {
		ctx = tools.WithSessionID(ctx, sessionID)
	}
	if agentID != "" {
		ctx = tools.WithAgentID(ctx, agentID)
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

		// Log raw arguments for debugging
		slog.Info("executeToolsWithSession: processing tool call",
			"tool", toolName,
			"toolCallID", tc.ID,
			"rawArgs", args)

		// Parse arguments
		var argsMap map[string]any
		if args != "" {
			if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
				// JSON parsing failed - likely truncated response
				slog.Warn("Failed to parse tool call arguments",
					"tool", toolName,
					"toolCallID", tc.ID,
					"argsLen", len(args),
					"error", err)
				errMsg := fmt.Sprintf("Error: Your response was truncated and the tool call arguments are incomplete (received %d bytes of invalid JSON). Please try calling the tool again with complete arguments. If writing a large file, consider splitting it into smaller chunks.", len(args))
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    errMsg,
					ToolCallID: tc.ID,
				})
				events <- NewToolResultEvent(tc.ID, toolName, errMsg, true, 0)
				continue
			}
		}

		// M08: Policy check before tool execution
		if r.policyExecutor != nil {
			// Resolve workspace path for $WORKSPACE expansion in PathPrefix rules
			var wsPath string
			if r.workspaceResolver != nil && sessionID != "" {
				wsPath = r.workspaceResolver(sessionID)
			}

			policyResult, err := r.policyExecutor.Check(ctx, &policy.ToolCall{
				Name:          toolName,
				Arguments:     args,
				SessionID:     sessionID,
				AgentID:       agentID,
				WorkspacePath: wsPath,
			})
			if err != nil {
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    "Policy check failed: " + err.Error(),
					ToolCallID: tc.ID,
				})
				events <- NewToolResultEvent(tc.ID, toolName, "Policy error: "+err.Error(), true, 0)
				continue
			}

			if !policyResult.Allowed {
				// Tool call blocked by policy — use template and circuit breaker
				blockMsg := formatBlockMessage(r.blockMessageTemplate, toolName, policyResult.Reason)
				count := r.incrementBlockCount(sessionID, toolName)
				if r.circuitBreakerThreshold > 0 && count >= r.circuitBreakerThreshold {
					blockMsg += fmt.Sprintf("\n[CIRCUIT BREAKER] Tool '%s' has been blocked %d times in this session. Stop attempting to use this tool and find an alternative approach.", toolName, count)
				}
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    blockMsg,
					ToolCallID: tc.ID,
				})
				events <- NewToolResultEvent(tc.ID, toolName, "Blocked: "+policyResult.Reason, true, 0)
				continue
			}

			if policyResult.RequireApproval {
				// Needs approval
				if r.approvalManager == nil {
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Tool call requires approval but no approval manager configured",
						ToolCallID: tc.ID,
					})
					events <- NewToolResultEvent(tc.ID, toolName, "Approval manager not configured", true, 0)
					continue
				}

				// Pre-generate approval ID so we can push it in the SSE event
				// before blocking on RequestApproval. The manager will reuse it.
				approvalID := uuid.New().String()
				approvalExpiresAt := time.Now().Add(5 * time.Minute).Format(time.RFC3339)

				// Request approval — push SSE event so chat page can show approval UI
				approvalCall := &policy.ToolCall{
					Name:      toolName,
					Arguments: args,
					SessionID: sessionID,
					AgentID:   agentID,
					RequestID: approvalID,
				}

				// Push approval_request event to SSE stream before blocking
				// The frontend will show an approval modal. The approval manager
				// also broadcasts via WebSocket for other listeners.
				events <- NewApprovalRequestEvent(
					approvalID,
					toolName,
					args,
					policyResult.ApprovalReason,
					sessionID,
					approvalExpiresAt,
				)

				approvalResult, err := r.approvalManager.RequestApproval(ctx, approvalCall, policyResult.ApprovalReason)
				if err != nil {
					events <- NewApprovalResolvedEvent(approvalID, false, time.Now().Format(time.RFC3339))
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Approval request failed: " + err.Error(),
						ToolCallID: tc.ID,
					})
					events <- NewToolResultEvent(tc.ID, toolName, "Approval failed: "+err.Error(), true, 0)
					continue
				}

				// Push approval resolved event to SSE stream
				events <- NewApprovalResolvedEvent(
					approvalID,
					approvalResult.Approved,
					approvalResult.DecidedAt.Format(time.RFC3339),
				)

				if !approvalResult.Approved {
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Tool call rejected: " + approvalResult.Message,
						ToolCallID: tc.ID,
					})
					events <- NewToolResultEvent(tc.ID, toolName, "Rejected: "+approvalResult.Message, true, 0)
					continue
				}

				// Approval granted — use modified arguments if user edited them
				if approvalResult.ModifiedArguments != "" {
					args = approvalResult.ModifiedArguments
					var newArgsMap map[string]any
					if err := json.Unmarshal([]byte(args), &newArgsMap); err == nil {
						argsMap = newArgsMap
					}
					slog.Info("approval: using modified arguments",
						"tool", toolName,
						"modifiedArgs", args)
				}
				// Proceed with execution
			}
		}

		// M07: Trigger before_tool_call hook
		hookCtx := hooks.NewContext(hooks.HookBeforeToolCall)
		hookCtx.ToolCall = &hooks.ToolCallContext{
			ID:       tc.ID,
			ToolName: toolName,
			Params:   argsMap,
		}
		if !r.triggerHookWithContinue(ctx, hookCtx) {
			// Tool call blocked by hook
			results = append(results, provider.Message{
				Role:       provider.RoleTool,
				Content:    "Tool call blocked by policy",
				ToolCallID: tc.ID,
			})
			events <- NewToolResultEvent(tc.ID, toolName, "Tool call blocked by policy", true, 0)
			continue
		}

		// Execute tool
		start := time.Now()
		result, err := effectiveRegistry.Execute(ctx, toolName, argsMap)
		duration := time.Since(start)

		var output string
		var isError bool
		var toolErr string
		if err != nil {
			output = err.Error()
			toolErr = err.Error()
			isError = true
		} else {
			output = result.Content
			isError = result.IsError
		}

		// Pre-truncate oversized tool results before storing in message history
		maxBytes := DefaultMaxToolResultBytes
		if len(output) > maxBytes {
			before := len(output)
			output = TruncateToolResult(output, maxBytes)
			slog.Info("executeToolsWithSession: truncated oversized tool result",
				"tool", toolName, "beforeBytes", before, "afterBytes", len(output), "maxBytes", maxBytes)
		}

		// M08B: Scrub credentials from tool output before entering LLM context
		output = ScrubCredentials(output, r.compiledScrubRules...)

		// M07: Trigger after_tool_call hook
		afterHookCtx := hooks.NewContext(hooks.HookAfterToolCall)
		afterHookCtx.ToolCall = &hooks.ToolCallContext{
			ID:       tc.ID,
			ToolName: toolName,
			Params:   argsMap,
			Result:   output,
			Error:    toolErr,
			Duration: duration,
		}
		_, _ = r.triggerHook(ctx, afterHookCtx)

		// Emit tool result event
		events <- NewToolResultEvent(tc.ID, toolName, output, isError, duration.Milliseconds())

		// Add tool result message
		results = append(results, provider.Message{
			Role:       provider.RoleTool,
			Content:    output,
			ToolCallID: tc.ID,
		})

		if isError {
			errorCount++
		}
	}

	return results, errorCount
}

// PauseSession 暂停指定会话的执行
func (r *Runner) PauseSession(sessionID string) error {
	r.pauseMu.RLock()
	ctrl := r.pauseController
	r.pauseMu.RUnlock()

	if ctrl == nil {
		return fmt.Errorf("pause controller not initialized")
	}

	return ctrl.Pause(sessionID)
}

// ResumeSession 恢复指定会话的执行
func (r *Runner) ResumeSession(sessionID string, userInput string) error {
	r.pauseMu.RLock()
	ctrl := r.pauseController
	r.pauseMu.RUnlock()

	if ctrl == nil {
		return fmt.Errorf("pause controller not initialized")
	}

	return ctrl.Resume(sessionID, userInput)
}

// GetPauseStatus 获取会话的暂停状态
func (r *Runner) GetPauseStatus(sessionID string) (*PauseStatus, error) {
	r.pauseMu.RLock()
	ctrl := r.pauseController
	r.pauseMu.RUnlock()

	if ctrl == nil {
		return &PauseStatus{Paused: false}, nil
	}

	return ctrl.GetStatus(sessionID)
}

// initPauseController 初始化暂停控制器
// 根据 provider 类型选择合适的控制器实现
func (r *Runner) initPauseController() {
	r.pauseMu.Lock()
	defer r.pauseMu.Unlock()

	// 如果已初始化，直接返回
	if r.pauseController != nil {
		return
	}

	// 检测使用的 Provider 类型
	// 优先检查 multiPool，其次 providerPool，最后 provider
	var prov provider.Provider
	if r.multiPool != nil {
		// 从 multiPool 获取默认 provider
		prov, _ = r.GetProvider("")
	} else if r.providerPool != nil {
		prov, _ = r.GetProvider("")
	} else {
		prov = r.provider
	}

	if prov == nil {
		slog.Warn("cannot initialize pause controller: no provider available")
		return
	}

	// 检查是否为 ACP provider
	if acpCapable, ok := prov.(provider.ACPCapable); ok && acpCapable.IsACPProvider() {
		// 需要获取 ACPProvider 实例来传递给 ACPPauseController
		// 但由于循环依赖问题，这里暂时使用 API 模式控制器
		// TODO: 在 step-09 中解决 ACP 模式的集成
		slog.Info("detected ACP provider, using API pause controller temporarily")
		r.pauseController = NewAPIPauseController(5 * time.Minute)
	} else {
		// API 模式
		slog.Info("using API pause controller")
		r.pauseController = NewAPIPauseController(5 * time.Minute)
	}
}

// SetPauseController 设置暂停控制器（用于外部注入）
func (r *Runner) SetPauseController(ctrl PauseController) {
	r.pauseMu.Lock()
	defer r.pauseMu.Unlock()
	r.pauseController = ctrl
	slog.Info("pause controller set externally")
}
