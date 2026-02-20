package runner

import (
	"context"
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
	"mote/internal/provider/minimax"
	"mote/internal/runner/orchestrator"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/storage"
	"mote/internal/tools"
	"mote/pkg/channel"
)

// SessionTokens tracks token usage for a session.
type SessionTokens struct {
	RequestTokens  int64     `json:"request_tokens"`
	ResponseTokens int64     `json:"response_tokens"`
	TotalTokens    int64     `json:"total_tokens"`
	LastUpdated    time.Time `json:"last_updated"`
}

// memoryFlushState tracks memory flush state per session.
type memoryFlushState struct {
	lastCompactionCount int
}

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
	history      *HistoryManager
	mu           sync.RWMutex

	// M04: Optional advanced components
	compactor      *compaction.Compactor
	memoryIndex    *memory.MemoryIndex
	contextManager *internalContext.Manager // M04-Enhanced: Context compression with persistence

	// M07: Skills and Hooks integration
	skillManager *skills.Manager
	hookManager  *hooks.Manager

	// M08: Policy and Approval integration
	policyExecutor  policy.PolicyChecker
	approvalManager approval.ApprovalHandler

	// MCP integration
	mcpManager *client.Manager

	// Token counting for memory flush
	sessionTokens map[string]*SessionTokens
	tokenMu       sync.RWMutex

	// Memory flush state tracking
	flushStates map[string]*memoryFlushState
	flushMu     sync.RWMutex

	// Compaction config for memory flush settings
	compactionConfig *compaction.CompactionConfig

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
		provider:      prov,
		registry:      registry,
		sessions:      sessions,
		config:        config,
		history:       NewHistoryManager(config.MaxMessages, config.MaxTokens*10),
		sessionTokens: make(map[string]*SessionTokens),
		flushStates:   make(map[string]*memoryFlushState),
		runQueue:      scheduler.NewRunQueue(10, 5*time.Minute),
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
		providerPool:  pool,
		defaultModel:  defaultModel,
		registry:      registry,
		sessions:      sessions,
		config:        config,
		history:       NewHistoryManager(config.MaxMessages, config.MaxTokens*10),
		sessionTokens: make(map[string]*SessionTokens),
		flushStates:   make(map[string]*memoryFlushState),
		runQueue:      scheduler.NewRunQueue(10, 5*time.Minute),
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
func (r *Runner) SetMultiProviderPool(pool *provider.MultiProviderPool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.multiPool = pool
	slog.Info("SetMultiProviderPool called", "pool_not_nil", pool != nil, "providers", func() []string {
		if pool != nil {
			return pool.ListProviders()
		}
		return nil
	}())
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

	// 3. Clear token tracking for this session
	r.tokenMu.Lock()
	delete(r.sessionTokens, sessionID)
	r.tokenMu.Unlock()

	// 4. Clear memory flush state
	r.flushMu.Lock()
	delete(r.flushStates, sessionID)
	r.flushMu.Unlock()

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

// SetMemory sets the optional M04 memory index.
func (r *Runner) SetMemory(m *memory.MemoryIndex) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memoryIndex = m
}

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

// SetMCPManager sets the MCP client manager for dynamic tool injection in prompts.
func (r *Runner) SetMCPManager(m *client.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpManager = m
	if r.systemPrompt != nil {
		r.systemPrompt.WithMCPManager(m)
	}
}

// injectMemoryContext adds relevant memory context to a prompt string.
func (r *Runner) injectMemoryContext(ctx context.Context, prompt, userInput string) string {
	if r.memoryIndex == nil || userInput == "" {
		return prompt
	}
	results, err := r.memoryIndex.Search(ctx, userInput, 5)
	if err != nil || len(results) == 0 {
		return prompt
	}
	var memorySection strings.Builder
	memorySection.WriteString("\n\n## Relevant Context From Memory\n\n")
	for _, mem := range results {
		memorySection.WriteString(fmt.Sprintf("- %s\n", mem.Content))
	}
	memorySection.WriteString("\n**Use this context to answer. Only use tools if memory doesn't have the needed information.**\n")
	return prompt + memorySection.String()
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

	// 运行 agent
	events, err := r.Run(ctx, sessionID, msg.Content)
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

// Run starts an agent run and returns a channel of events.
func (r *Runner) Run(ctx context.Context, sessionID, userInput string, attachments ...provider.Attachment) (<-chan Event, error) {
	slog.Info("Runner.Run called", "sessionID", sessionID, "hasMultiPool", r.multiPool != nil)

	// Get provider - will be resolved in runLoop based on session model
	if r.provider == nil && r.providerPool == nil {
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
	if r.provider == nil && r.providerPool == nil {
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
	orchBuilder := orchestrator.NewBuilder(orchestrator.BuilderOptions{
		Sessions:     r.sessions,
		Registry:     r.registry,
		Config: orchestrator.Config{
			MaxIterations: r.config.MaxIterations,
			MaxTokens:     r.config.MaxTokens,
			Temperature:   r.config.Temperature,
			StreamOutput:  true,
			Timeout:       r.config.Timeout,
		},
		Compactor:    r.compactor,
		SystemPrompt: r.systemPrompt,
		SkillManager: r.skillManager,
		HookManager:  r.hookManager,
		MCPManager:   r.mcpManager,
		// Inject full tool executor from Runner (includes policy, hooks, heartbeat, truncation)
		ToolExecutor: func(ctx context.Context, toolCalls []provider.ToolCall, sessionID string) ([]provider.Message, int) {
			return r.executeToolsWithSession(ctx, toolCalls, events, sessionID, "")
		},
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
	for event := range orchEvents {
		events <- FromTypesEvent(event)
	}
}

// buildMessages constructs the message list for the provider.
func (r *Runner) buildMessages(ctx context.Context, cached *scheduler.CachedSession, userInput string) ([]provider.Message, error) {
	// Build system prompt using SystemPromptBuilder (primary) or static config (fallback)
	var sysPromptContent string
	var err error

	if r.systemPrompt != nil {
		// SystemPromptBuilder handles: memory search, MCP injection, tool listing, slots
		sysPromptContent, err = r.systemPrompt.Build(ctx, userInput)
		if err != nil {
			return nil, fmt.Errorf("build system prompt: %w", err)
		}
	} else if r.config.SystemPrompt != "" {
		// Static config fallback - manually inject memory if available
		sysPromptContent = r.config.SystemPrompt
		sysPromptContent = r.injectMemoryContext(ctx, sysPromptContent, userInput)
	} else {
		// No prompt configured - use minimal default
		sysPromptContent = "You are a helpful AI assistant."
		sysPromptContent = r.injectMemoryContext(ctx, sysPromptContent, userInput)
	}

	// Inject skills section if skillManager is available
	if r.skillManager != nil {
		skillsSection := skills.NewPromptSection(r.skillManager)
		// Apply session-level skill selection filter
		if cached.Session != nil && len(cached.Session.SelectedSkills) > 0 {
			skillsSection.WithSelectedSkills(cached.Session.SelectedSkills)
		}
		if section := skillsSection.Build(); section != "" {
			sysPromptContent += "\n\n" + section
		}
		if activePrompts := skillsSection.BuildActivePrompts(); activePrompts != "" {
			sysPromptContent += "\n" + activePrompts
		}
	}

	// Use Context Manager if available (preferred for advanced compression)
	if r.contextManager != nil {
		messages, err := r.contextManager.BuildContext(ctx, cached.Session.ID, sysPromptContent, userInput)
		if err != nil {
			slog.Warn("context manager failed, falling back to legacy method",
				"sessionID", cached.Session.ID,
				"error", err)
			// Fall through to legacy method
		} else {
			return messages, nil
		}
	}

	// Legacy method: manual message loading
	var messages []provider.Message

	// Add history messages
	for _, msg := range cached.Messages {
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
		Content: userInput,
	})

	return messages, nil
}

// buildChatRequest creates a ChatRequest from messages.
// sessionID is used as the conversation ID to help providers track multi-turn tool call conversations.
func (r *Runner) buildChatRequest(messages []provider.Message, model string, sessionID string, attachments []provider.Attachment) provider.ChatRequest {
	tools, _ := r.registry.ToProviderTools()

	// For Ollama provider, strip the "ollama:" prefix from model name
	model = strings.TrimPrefix(model, "ollama:")

	// Sanitize messages to remove corrupted tool call data (e.g., truncated arguments)
	messages = provider.SanitizeMessages(messages)

	req := provider.ChatRequest{
		Model:          model,
		Messages:       messages,
		Attachments:    attachments,
		Tools:          tools,
		Temperature:    r.config.Temperature,
		MaxTokens:      r.config.MaxTokens,
		Stream:         r.config.StreamOutput,
		ConversationID: sessionID, // Pass session ID as conversation ID for quota tracking
	}
	return req
}

// callProvider calls the LLM provider and processes the response.
// Deprecated: Use callProviderWith for multi-model support.
//
//nolint:unused // Reserved for backward compatibility
func (r *Runner) callProvider(ctx context.Context, req provider.ChatRequest, events chan<- Event, iteration int) (*provider.ChatResponse, error) {
	if r.config.StreamOutput {
		return r.callProviderStream(ctx, req, events, iteration)
	}
	return r.callProviderChat(ctx, req, events)
}

// callProviderWith calls the specified provider and processes the response.
func (r *Runner) callProviderWith(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- Event, iteration int) (*provider.ChatResponse, error) {
	if r.config.StreamOutput {
		return r.callProviderStreamWith(ctx, prov, req, events, iteration)
	}
	return r.callProviderChatWith(ctx, prov, req, events)
}

// callProviderChat calls the provider without streaming.
// Deprecated: Use callProviderChatWith for multi-model support.
//
//nolint:unused // Reserved for backward compatibility
func (r *Runner) callProviderChat(ctx context.Context, req provider.ChatRequest, events chan<- Event) (*provider.ChatResponse, error) {
	resp, err := r.provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// Emit content event
	if resp.Content != "" {
		events <- NewContentEvent(resp.Content)
	}

	// Emit tool call events
	for _, tc := range resp.ToolCalls {
		storageTc := providerToStorageToolCall(tc)
		events <- NewToolCallEvent(storageTc)
	}

	return resp, nil
}

// callProviderChatWith calls the specified provider without streaming.
func (r *Runner) callProviderChatWith(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- Event) (*provider.ChatResponse, error) {
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// Emit content event
	if resp.Content != "" {
		events <- NewContentEvent(resp.Content)
	}

	// Emit tool call events
	for _, tc := range resp.ToolCalls {
		storageTc := providerToStorageToolCall(tc)
		events <- NewToolCallEvent(storageTc)
	}

	return resp, nil
}

// callProviderStream calls the provider with streaming.
// Deprecated: Use callProviderStreamWith for multi-model support.
//
//nolint:unused // Reserved for backward compatibility
func (r *Runner) callProviderStream(ctx context.Context, req provider.ChatRequest, events chan<- Event, iteration int) (*provider.ChatResponse, error) {
	streamCh, err := r.provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	return r.processStreamResponse(ctx, streamCh, events, iteration)
}

// callProviderStreamWith calls the specified provider with streaming.
func (r *Runner) callProviderStreamWith(ctx context.Context, prov provider.Provider, req provider.ChatRequest, events chan<- Event, iteration int) (*provider.ChatResponse, error) {
	streamCh, err := prov.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	return r.processStreamResponse(ctx, streamCh, events, iteration)
}

// processStreamResponse processes the streaming response from a provider.
// It sends heartbeat events periodically to keep the connection alive during long model responses.
func (r *Runner) processStreamResponse(ctx context.Context, streamCh <-chan provider.ChatEvent, events chan<- Event, iteration int) (*provider.ChatResponse, error) {
	resp := &provider.ChatResponse{
		FinishReason: provider.FinishReasonStop, // Default to stop
	}
	var contentBuilder string
	var thinkingBuilder string // Track thinking content for fallback when content is empty
	pendingToolCalls := make(map[int]*provider.ToolCall)

	// Start heartbeat for streaming - model may take a long time to generate tool call arguments
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
				select {
				case events <- NewHeartbeatEvent():
					slog.Info("sent heartbeat during stream processing", "iteration", iteration)
				default:
					slog.Warn("heartbeat channel full during stream", "iteration", iteration)
				}
			}
		}
	}()

	for streamEvent := range streamCh {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch streamEvent.Type {
		case provider.EventTypeContent:
			contentBuilder += streamEvent.Delta
			events <- Event{
				Type:      EventTypeContent,
				Content:   streamEvent.Delta,
				Iteration: iteration,
			}

		case provider.EventTypeThinking:
			// Forward thinking events for temporary display and accumulate for fallback
			if streamEvent.Thinking != "" {
				thinkingBuilder += streamEvent.Thinking
				events <- Event{
					Type:      EventTypeThinking,
					Thinking:  streamEvent.Thinking,
					Iteration: iteration,
				}
			}

		case provider.EventTypeToolCall:
			if streamEvent.ToolCall != nil {
				tc := streamEvent.ToolCall
				if existing, ok := pendingToolCalls[tc.Index]; ok {
					// Accumulate arguments
					existing.Arguments += tc.Arguments
					if tc.Function != nil {
						if existing.Function == nil {
							existing.Function = tc.Function
						} else {
							existing.Function.Arguments += tc.Function.Arguments
						}
					}
				} else {
					// New tool call
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

		case provider.EventTypeToolCallUpdate:
			// Forward tool call update events
			if streamEvent.ToolCallUpdate != nil {
				events <- Event{
					Type: EventTypeToolCallUpdate,
					ToolCallUpdate: &ToolCallUpdateEvent{
						ToolCallID: streamEvent.ToolCallUpdate.ID,
						ToolName:   streamEvent.ToolCallUpdate.Name,
						Status:     streamEvent.ToolCallUpdate.Status,
						Arguments:  streamEvent.ToolCallUpdate.Arguments,
					},
					Iteration: iteration,
				}
			}

		case provider.EventTypeDone:
			if streamEvent.Usage != nil {
				resp.Usage = streamEvent.Usage
			}
			// Capture finish reason from LLM - this is the authoritative signal
			if streamEvent.FinishReason != "" {
				resp.FinishReason = streamEvent.FinishReason
			}

		case provider.EventTypeError:
			if streamEvent.Error != nil {
				return nil, streamEvent.Error
			}
		}
	}

	resp.Content = contentBuilder

	// MiniMax-M2.5 sometimes outputs tool calls as raw XML in content
	// (e.g. <invoke name="read_file">...</invoke>) instead of structured
	// tool_calls. After accumulating all content, check for this pattern
	// and extract them as proper tool calls.
	if len(pendingToolCalls) == 0 && resp.Content != "" && strings.Contains(resp.Content, "<invoke ") {
		if xmlCalls, cleanContent := minimax.ExtractXMLToolCalls(resp.Content); len(xmlCalls) > 0 {
			slog.Warn("processStreamResponse: extracted XML tool calls from content",
				"count", len(xmlCalls),
				"originalLen", len(resp.Content),
				"cleanLen", len(cleanContent))
			for i := range xmlCalls {
				tc := xmlCalls[i]
				resp.ToolCalls = append(resp.ToolCalls, tc)
				storageTc := providerToStorageToolCall(tc)
				events <- NewToolCallEvent(storageTc)
			}
			resp.Content = cleanContent
			resp.FinishReason = provider.FinishReasonToolCalls
		}
	}

	// Fallback: if content is empty but thinking content was received,
	// use thinking as the response content. This handles the case where
	// models with reasoning_split put all output into reasoning_content
	// but return empty content (observed with MiniMax after compaction).
	if resp.Content == "" && thinkingBuilder != "" && len(pendingToolCalls) == 0 {
		slog.Warn("processStreamResponse: content empty but thinking received, using thinking as fallback",
			"thinkingLen", len(thinkingBuilder),
			"finishReason", resp.FinishReason)
		resp.Content = thinkingBuilder
		// Also emit the thinking content as regular content so frontend displays it
		events <- Event{
			Type:      EventTypeContent,
			Content:   thinkingBuilder,
			Iteration: iteration,
		}
	}

	slog.Info("processStreamResponse: stream ended",
		"contentLen", len(resp.Content),
		"thinkingLen", len(thinkingBuilder),
		"pendingToolCallsCount", len(pendingToolCalls),
		"finishReason", resp.FinishReason)

	// Convert pending tool calls to slice
	for _, tc := range pendingToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, *tc)
		// Emit tool call event
		storageTc := providerToStorageToolCall(*tc)
		events <- NewToolCallEvent(storageTc)
	}

	// Adjust FinishReason based on actual tool calls if needed
	// (Some providers may not set FinishReason correctly in stream mode)
	if len(resp.ToolCalls) > 0 && resp.FinishReason == provider.FinishReasonStop {
		resp.FinishReason = provider.FinishReasonToolCalls
	}

	return resp, nil
}

// executeTools executes the tool calls and returns tool result messages.
// Deprecated: Use executeToolsWithSession for session-aware execution.
//
//nolint:unused // Reserved for backward compatibility
func (r *Runner) executeTools(ctx context.Context, toolCalls []provider.ToolCall, events chan<- Event) ([]provider.Message, int) {
	return r.executeToolsWithSession(ctx, toolCalls, events, "", "")
}

// executeToolsWithSession executes tool calls with session context for policy checks.
// It sends heartbeat events every 15 seconds during long-running tool executions to keep the connection alive.
// Returns the tool result messages and the count of tool executions that returned errors.
func (r *Runner) executeToolsWithSession(ctx context.Context, toolCalls []provider.ToolCall, events chan<- Event, sessionID, agentID string) ([]provider.Message, int) {
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
			policyResult, err := r.policyExecutor.Check(ctx, &policy.ToolCall{
				Name:      toolName,
				Arguments: args,
				SessionID: sessionID,
				AgentID:   agentID,
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
				// Tool call blocked by policy
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    "Tool call blocked by policy: " + policyResult.Reason,
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

				// Request approval
				approvalResult, err := r.approvalManager.RequestApproval(ctx, &policy.ToolCall{
					Name:      toolName,
					Arguments: args,
					SessionID: sessionID,
					AgentID:   agentID,
				}, policyResult.ApprovalReason)
				if err != nil {
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Approval request failed: " + err.Error(),
						ToolCallID: tc.ID,
					})
					events <- NewToolResultEvent(tc.ID, toolName, "Approval failed: "+err.Error(), true, 0)
					continue
				}

				if !approvalResult.Approved {
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Tool call rejected: " + approvalResult.Message,
						ToolCallID: tc.ID,
					})
					events <- NewToolResultEvent(tc.ID, toolName, "Rejected: "+approvalResult.Message, true, 0)
					continue
				}
				// Approval granted, proceed with execution
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
		result, err := r.registry.Execute(ctx, toolName, argsMap)
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
		if r.compactionConfig != nil && r.compactionConfig.ToolResultMaxBytes > 0 {
			maxBytes = r.compactionConfig.ToolResultMaxBytes
		}
		if len(output) > maxBytes {
			before := len(output)
			output = TruncateToolResult(output, maxBytes)
			slog.Info("executeToolsWithSession: truncated oversized tool result",
				"tool", toolName, "beforeBytes", before, "afterBytes", len(output), "maxBytes", maxBytes)
		}

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

// providerToStorageToolCall converts a provider ToolCall to a storage ToolCall.
func providerToStorageToolCall(tc provider.ToolCall) *storage.ToolCall {
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

	return &storage.ToolCall{
		ID:       tc.ID,
		Type:     "function",
		Function: json.RawMessage(fnData),
	}
}

// convertToolCalls converts provider tool calls to storage tool calls.
func convertToolCalls(tcs []provider.ToolCall) []storage.ToolCall {
	var result []storage.ToolCall
	for _, tc := range tcs {
		stc := providerToStorageToolCall(tc)
		result = append(result, *stc)
	}
	return result
}

// SetCompactionConfig sets the compaction configuration for memory flush settings.
func (r *Runner) SetCompactionConfig(cfg *compaction.CompactionConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compactionConfig = cfg
}

// UpdateTokens updates the token count for a session.
func (r *Runner) UpdateTokens(sessionID string, reqTokens, respTokens int64) {
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()

	if r.sessionTokens == nil {
		r.sessionTokens = make(map[string]*SessionTokens)
	}

	st, ok := r.sessionTokens[sessionID]
	if !ok {
		st = &SessionTokens{}
		r.sessionTokens[sessionID] = st
	}

	st.RequestTokens += reqTokens
	st.ResponseTokens += respTokens
	st.TotalTokens = st.RequestTokens + st.ResponseTokens
	st.LastUpdated = time.Now()
}

// GetSessionTokens returns the token statistics for a session.
func (r *Runner) GetSessionTokens(sessionID string) *SessionTokens {
	r.tokenMu.RLock()
	defer r.tokenMu.RUnlock()
	if r.sessionTokens == nil {
		return nil
	}
	return r.sessionTokens[sessionID]
}

// EstimateTokens estimates token count from text (fallback when Usage is unavailable).
// Rough estimate: 1 token ≈ 3 characters for mixed Chinese/English text.
func EstimateTokens(text string) int64 {
	if len(text) == 0 {
		return 0
	}
	return int64((len(text) + 2) / 3)
}

// getMemoryFlushState returns the memory flush state for a session.
func (r *Runner) getMemoryFlushState(sessionID string) *memoryFlushState {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()

	if r.flushStates == nil {
		r.flushStates = make(map[string]*memoryFlushState)
	}

	state, ok := r.flushStates[sessionID]
	if !ok {
		state = &memoryFlushState{}
		r.flushStates[sessionID] = state
	}
	return state
}

// shouldRunMemoryFlush checks if memory flush should run before processing.
func (r *Runner) shouldRunMemoryFlush(sessionID string) bool {
	// Check if compaction config is available
	if r.compactionConfig == nil {
		return false
	}

	cfg := r.compactionConfig.MemoryFlush
	if !cfg.Enabled {
		return false
	}

	// Check token threshold
	tokens := r.GetSessionTokens(sessionID)
	if tokens == nil {
		return false
	}

	// Calculate threshold: contextWindow - reserveTokens - softThreshold
	contextWindow := int64(r.compactionConfig.MaxContextTokens)
	threshold := contextWindow - cfg.ReserveTokens - cfg.SoftThresholdTokens
	if tokens.TotalTokens < threshold {
		return false
	}

	// Check if we already flushed in this compaction cycle
	state := r.getMemoryFlushState(sessionID)
	compactionCount := 0
	if r.compactor != nil {
		compactionCount = r.compactor.GetCompactionCount(sessionID)
	}
	if state.lastCompactionCount >= compactionCount && compactionCount > 0 {
		return false // Already flushed in this cycle
	}

	return true
}

// executeMemoryFlush executes the pre-compaction memory flush.
func (r *Runner) executeMemoryFlush(ctx context.Context, sessionID string, events chan<- Event) error {
	if r.compactionConfig == nil {
		return nil
	}

	// Get provider for memory flush (use default model)
	prov, err := r.GetProvider("")
	if err != nil {
		return nil
	}

	cfg := r.compactionConfig.MemoryFlush
	slog.Info("executing memory flush",
		"sessionID", sessionID,
		"tokens", r.GetSessionTokens(sessionID).TotalTokens)

	// Build flush request with memory flush prompts
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: cfg.SystemPrompt},
	}

	// Inject recent conversation history so the LLM has context for what to save
	if histSnippet := r.buildFlushHistory(sessionID, 16000, 2000); histSnippet != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleUser,
			Content: "Here is the recent conversation for context:\n\n" + histSnippet,
		})
		messages = append(messages, provider.Message{
			Role:    provider.RoleAssistant,
			Content: "Understood. I will review the conversation and save important information to memory.",
		})
	}

	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: cfg.Prompt,
	})

	// Build tools for memory flush (only memory-related tools)
	var tools []provider.Tool
	if r.registry != nil {
		for _, t := range r.registry.List() {
			// Only include memory-related tools for flush
			name := t.Name()
			if name == "mote_memory_add" || name == "mote_memory_search" {
				params, _ := json.Marshal(t.Parameters())
				tools = append(tools, provider.Tool{
					Type: "function",
					Function: provider.ToolFunction{
						Name:        name,
						Description: t.Description(),
						Parameters:  params,
					},
				})
			}
		}
	}

	// Call provider
	req := provider.ChatRequest{
		Messages:  messages,
		MaxTokens: r.config.MaxTokens,
		Tools:     tools,
	}

	resp, err := prov.Chat(ctx, req)
	if err != nil {
		slog.Warn("memory flush LLM call failed", "error", err)
		return err
	}

	// Process any tool calls (e.g., mote_memory_add) with session context
	if len(resp.ToolCalls) > 0 {
		r.executeToolsWithSession(ctx, resp.ToolCalls, events, sessionID, "") //nolint:errcheck // memory flush doesn't need error tracking
	}

	// Update flush state
	state := r.getMemoryFlushState(sessionID)
	if r.compactor != nil {
		state.lastCompactionCount = r.compactor.GetCompactionCount(sessionID)
	}

	slog.Info("memory flush completed",
		"sessionID", sessionID,
		"toolCalls", len(resp.ToolCalls))

	return nil
}

// buildFlushHistory returns a compact text snippet of recent session messages
// for injection into the memory flush prompt. maxTotalChars limits the overall
// output, maxPerMsg caps individual message content.
func (r *Runner) buildFlushHistory(sessionID string, maxTotalChars, maxPerMsg int) string {
	msgs, err := r.sessions.GetMessages(sessionID)
	if err != nil || len(msgs) == 0 {
		return ""
	}

	// Walk backwards, collecting messages until budget is exhausted
	var lines []string
	total := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "system" {
			continue
		}
		content := m.Content
		if len(content) > maxPerMsg {
			content = content[:maxPerMsg] + "…[truncated]"
		}
		line := fmt.Sprintf("[%s]: %s", m.Role, content)
		if total+len(line) > maxTotalChars {
			break
		}
		lines = append(lines, line)
		total += len(line)
	}
	if len(lines) == 0 {
		return ""
	}
	// Reverse to chronological order
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n")
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
