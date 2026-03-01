package delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"mote/internal/compaction"
	"mote/internal/config"
	internalContext "mote/internal/context"
	"mote/internal/hooks"
	"mote/internal/mcp/client"
	"mote/internal/prompt"
	"mote/internal/provider"
	"mote/internal/runner/delegate/cfg"
	"mote/internal/runner/orchestrator"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/tools"
)

// ParentEventSink is a callback to transparently forward sub-agent events to the parent.
type ParentEventSink func(event types.Event)

// parentSinkKey is the context key for threading ParentEventSink through
// the tool execution chain so that nested delegate calls (DelegateTool) can
// forward content events to the top-level SSE stream.
type parentSinkKeyType struct{}

var parentSinkKey = parentSinkKeyType{}

// WithParentEventSink stores a ParentEventSink in the context.
func WithParentEventSink(ctx context.Context, sink ParentEventSink) context.Context {
	return context.WithValue(ctx, parentSinkKey, sink)
}

// ParentEventSinkFromContext retrieves the ParentEventSink from the context, if any.
func ParentEventSinkFromContext(ctx context.Context) ParentEventSink {
	if v, ok := ctx.Value(parentSinkKey).(ParentEventSink); ok {
		return v
	}
	return nil
}

// friendlyErrorMessage wraps well-known provider errors with user-friendly
// Chinese messages.  Unknown errors are returned as-is.
func friendlyErrorMessage(agentName string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// MiniMax content policy (output new_sensitive / unprocessable_entity_error)
	if strings.Contains(msg, "sensitive") ||
		strings.Contains(msg, "content_filter") ||
		strings.Contains(msg, "content_policy") {
		return fmt.Sprintf("🚫 %s 触发了内容安全审核，被禁言了", agentName)
	}
	return msg
}

// SubRunnerFactory creates lightweight sub-agent execution environments.
// It holds shared dependencies from the main Runner and constructs
// filtered Orchestrators for sub-agents.
type SubRunnerFactory struct {
	mu             sync.RWMutex
	multiPool      *provider.MultiProviderPool
	sessions       *scheduler.SessionManager
	parentRegistry *tools.Registry
	hookManager    *hooks.Manager
	mcpManager     *client.Manager
	compactor      *compaction.Compactor
	systemPrompt   *prompt.SystemPromptBuilder
	contextManager *internalContext.Manager
	skillManager   *skills.Manager
	defaultModel   string

	// Default RunConfig values inherited from main Runner.
	maxIterations int
	maxTokens     int
	temperature   float64
	timeout       time.Duration

	// Optional tracker for audit persistence (nil = no tracking).
	tracker *DelegationTracker

	// Optional workspace binder: copies parent session workspace to child session.
	// Set via SetWorkspaceBinder after creation (server layer provides implementation).
	workspaceBinder func(parentSessionID, childSessionID string)

	// Optional workspace resolver: returns the workspace path for a session.
	// Set via SetWorkspaceResolver after creation.
	workspaceResolver func(sessionID string) string
}

// SubRunnerFactoryOptions contains all dependencies needed to construct
// a SubRunnerFactory, avoiding direct dependency on runner.Runner.
type SubRunnerFactoryOptions struct {
	MultiPool      *provider.MultiProviderPool
	Sessions       *scheduler.SessionManager
	ParentRegistry *tools.Registry
	HookManager    *hooks.Manager
	MCPManager     *client.Manager
	Compactor      *compaction.Compactor
	SystemPrompt   *prompt.SystemPromptBuilder
	ContextManager *internalContext.Manager
	SkillManager   *skills.Manager
	DefaultModel   string
	MaxIterations  int
	MaxTokens      int
	Temperature    float64
	Timeout        time.Duration
}

// NewSubRunnerFactory creates a new sub-runner factory.
func NewSubRunnerFactory(opts SubRunnerFactoryOptions) *SubRunnerFactory {
	return &SubRunnerFactory{
		multiPool:      opts.MultiPool,
		sessions:       opts.Sessions,
		parentRegistry: opts.ParentRegistry,
		hookManager:    opts.HookManager,
		mcpManager:     opts.MCPManager,
		compactor:      opts.Compactor,
		systemPrompt:   opts.SystemPrompt,
		contextManager: opts.ContextManager,
		skillManager:   opts.SkillManager,
		defaultModel:   opts.DefaultModel,
		maxIterations:  opts.MaxIterations,
		maxTokens:      opts.MaxTokens,
		temperature:    opts.Temperature,
		timeout:        opts.Timeout,
	}
}

// Sessions returns the session manager for external wiring (e.g. PDA engine).
func (f *SubRunnerFactory) Sessions() *scheduler.SessionManager {
	return f.sessions
}

// SetTracker enables delegation tracking for audit and monitoring.
func (f *SubRunnerFactory) SetTracker(t *DelegationTracker) {
	f.tracker = t
}

// SetDefaultModel updates the fallback model used when an agent has no explicit model.
// This should be called whenever the user changes the active model via the API.
func (f *SubRunnerFactory) SetDefaultModel(model string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defaultModel = model
	slog.Info("delegate: default model updated", "model", model)
}

// getDefaultModel returns the current fallback model (thread-safe).
func (f *SubRunnerFactory) getDefaultModel() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.defaultModel
}

// SetWorkspaceBinder sets the function used to copy workspace bindings from parent to child sessions.
func (f *SubRunnerFactory) SetWorkspaceBinder(binder func(parentSessionID, childSessionID string)) {
	f.workspaceBinder = binder
}

// SetWorkspaceResolver sets the function used to resolve workspace paths for sessions.
func (f *SubRunnerFactory) SetWorkspaceResolver(resolver func(sessionID string) string) {
	f.workspaceResolver = resolver
}

// RunDelegate executes a sub-agent with the given configuration and prompt.
// It returns the aggregated text result, token usage, and any error.
// This is a convenience wrapper around RunDelegateWithEvents with no event sink.
func (f *SubRunnerFactory) RunDelegate(
	ctx context.Context,
	delegateCtx *DelegateContext,
	agentCfg config.AgentConfig,
	userPrompt string,
) (string, types.Usage, error) {
	return f.RunDelegateWithEvents(ctx, delegateCtx, agentCfg, userPrompt, nil)
}

// RunDelegateWithEvents executes a sub-agent, forwarding events to parentSink in real-time.
// It also records delegation tracking if a tracker is configured.
func (f *SubRunnerFactory) RunDelegateWithEvents(
	ctx context.Context,
	delegateCtx *DelegateContext,
	agentCfg config.AgentConfig,
	userPrompt string,
	parentSink ParentEventSink,
) (string, types.Usage, error) {
	// 1. Build session ID
	sessionID := fmt.Sprintf("delegate:%s:%s:%d",
		delegateCtx.ParentSessionID,
		delegateCtx.AgentName,
		time.Now().UnixMilli())

	// 2. Record start (optional tracking)
	var invocationID string
	if f.tracker != nil {
		invocationID = GenerateInvocationID(delegateCtx.ParentSessionID, delegateCtx.AgentName)
		_ = f.tracker.StartDelegation(DelegationRecord{
			ID:              invocationID,
			ParentSessionID: delegateCtx.ParentSessionID,
			ChildSessionID:  sessionID,
			AgentName:       delegateCtx.AgentName,
			Depth:           delegateCtx.Depth,
			Chain:           ChainToJSON(delegateCtx.Chain),
			Prompt:          userPrompt,
			StartedAt:       time.Now(),
		})
	}

	// 3. Apply timeout (0 = no timeout, inherit parent context cancellation)
	timeout := agentCfg.GetTimeout()
	var subCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		subCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		subCtx = ctx
		cancel = func() {} // no-op
	}

	// 4. Resolve provider
	model := agentCfg.Model
	if model == "" {
		model = f.getDefaultModel()
	}
	prov, _, err := f.multiPool.GetProvider(model)
	if err != nil {
		f.completeTracking(invocationID, "failed", 0, 0, err)
		return "", types.Usage{}, fmt.Errorf("get provider for model %s: %w", model, err)
	}

	// 5. Build filtered registry
	subRegistry := f.buildFilteredRegistry(agentCfg.Tools, delegateCtx)

	// 6. Determine agent system prompt
	agentSysPrompt := agentCfg.SystemPrompt
	if agentSysPrompt == "" {
		agentSysPrompt = "You are a helpful AI assistant."
	}

	// 7. Build a dedicated SystemPromptBuilder for this sub-agent.
	// Previously we shared the main agent's builder, which meant the sub-agent's
	// custom system prompt was silently overridden by the main agent's identity.
	// Now we create a per-agent builder that:
	//   - Uses the agent's own name and system prompt as ExtraPrompt
	//   - Inherits MCP injection from the parent (so sub-agents can use MCP tools)
	//   - Uses the sub-agent's filtered registry (not the parent's full toolset)
	subPromptCfg := prompt.PromptConfig{
		AgentName:   delegateCtx.AgentName,
		ExtraPrompt: agentSysPrompt,
	}
	// Copy timezone/workspace from parent builder config if available
	if f.systemPrompt != nil {
		parentCfg := f.systemPrompt.GetConfig()
		subPromptCfg.Timezone = parentCfg.Timezone
		subPromptCfg.WorkspaceDir = parentCfg.WorkspaceDir
	}
	// Resolve workspace from session binding (overrides parent config if available)
	if f.workspaceResolver != nil && delegateCtx.ParentSessionID != "" {
		if wsPath := f.workspaceResolver(delegateCtx.ParentSessionID); wsPath != "" {
			subPromptCfg.WorkspaceDir = wsPath
		}
	}
	subPromptBuilder := prompt.NewSystemPromptBuilder(subPromptCfg, subRegistry)
	if f.mcpManager != nil {
		subPromptBuilder.WithMCPManager(f.mcpManager)
	}

	slog.Info("delegate: sub-agent system prompt config",
		"agent", delegateCtx.AgentName,
		"agentSystemPrompt", agentSysPrompt,
		"hasOwnBuilder", true,
	)

	// 8. Config overrides
	maxIter := f.maxIterations
	if agentCfg.MaxIterations > 0 {
		maxIter = agentCfg.MaxIterations
	}
	temp := f.temperature
	if agentCfg.Temperature > 0 {
		temp = agentCfg.Temperature
	}
	maxTok := f.maxTokens
	if agentCfg.MaxTokens > 0 {
		maxTok = agentCfg.MaxTokens
	}

	// 9. Build orchestrator with the sub-agent's own SystemPromptBuilder
	orchBuilder := orchestrator.NewBuilder(orchestrator.BuilderOptions{
		Sessions: f.sessions,
		Registry: subRegistry,
		Config: orchestrator.Config{
			MaxIterations: maxIter,
			MaxTokens:     maxTok,
			Temperature:   temp,
			StreamOutput:  true,
			Timeout:       timeout,
			SystemPrompt:  agentSysPrompt, // fallback if builder fails
		},
		Compactor:      f.compactor,
		SystemPrompt:   subPromptBuilder, // sub-agent's OWN builder, not the parent's
		SkillManager:   f.skillManager,
		HookManager:    f.hookManager,
		MCPManager:     f.mcpManager,
		ContextManager: f.contextManager,
		ToolExecutor:   f.makeToolExecutor(subRegistry, sessionID),
	})

	orch := orchBuilder.Build(prov)

	// 10. Session creation + metadata setup
	cached, _ := f.sessions.GetOrCreate(sessionID, nil)

	// Set the model on the session so the ChatPage shows the correct model
	if db := f.sessions.DB(); db != nil {
		if err := db.UpdateSessionModel(sessionID, model); err != nil {
			slog.Warn("delegate: failed to set session model", "sessionID", sessionID, "model", model, "error", err)
		} else if cached.Session != nil {
			cached.Session.Model = model
		}
	}

	// Copy parent session's workspace binding to child session
	if f.workspaceBinder != nil && delegateCtx.ParentSessionID != "" {
		f.workspaceBinder(delegateCtx.ParentSessionID, sessionID)
	}

	slog.Info("delegate: starting sub-agent (with events)",
		"agent", delegateCtx.AgentName,
		"depth", delegateCtx.Depth,
		"sessionID", sessionID,
		"model", model,
		"parentSessionID", delegateCtx.ParentSessionID)

	orchEvents, err := orch.Run(subCtx, &orchestrator.RunRequest{
		SessionID:     sessionID,
		UserInput:     userPrompt,
		Provider:      prov,
		CachedSession: cached,
	})
	if err != nil {
		f.completeTracking(invocationID, "failed", 0, 0, err)
		return "", types.Usage{}, fmt.Errorf("sub-agent run: %w", err)
	}

	// 10. Heartbeat goroutine
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	go func() {
		for {
			select {
			case <-subCtx.Done():
				return
			case <-heartbeatTicker.C:
				if parentSink != nil {
					parentSink(types.Event{
						Type:       types.EventTypeHeartbeat,
						AgentName:  delegateCtx.AgentName,
						AgentDepth: delegateCtx.Depth,
						Content:    fmt.Sprintf("[%s] still executing...", delegateCtx.AgentName),
					})
				}
			}
		}
	}()

	// 11. Aggregate + forward events
	var content strings.Builder
	var totalUsage types.Usage
	for event := range orchEvents {
		// Tag every event with agent identity
		event.AgentName = delegateCtx.AgentName
		event.AgentDepth = delegateCtx.Depth

		switch event.Type {
		case types.EventTypeContent:
			content.WriteString(event.Content)
			if parentSink != nil {
				parentSink(event)
			}
		case types.EventTypeToolCall, types.EventTypeToolResult, types.EventTypeToolCallUpdate:
			if parentSink != nil {
				parentSink(event)
			}
		case types.EventTypeDone:
			if event.Usage != nil {
				totalUsage = *event.Usage
			}
		case types.EventTypeError:
			errMsg := event.ErrorMsg
			if event.Error != nil {
				errMsg = event.Error.Error()
			}
			friendly := friendlyErrorMessage(delegateCtx.AgentName, fmt.Errorf("%s", errMsg))
			f.completeTracking(invocationID, "failed", content.Len(), totalUsage.TotalTokens, fmt.Errorf("%s", errMsg))
			return content.String(), totalUsage,
				fmt.Errorf("sub-agent error: %s", friendly)
		}
	}

	f.completeTracking(invocationID, "completed", content.Len(), totalUsage.TotalTokens, nil)
	return content.String(), totalUsage, nil
}

// RunPDAWithEvents executes a sub-agent that has structured steps (PDA engine),
// forwarding events to parentSink in real-time. This is used for direct delegate
// calls (@ mention) on agents with steps.
func (f *SubRunnerFactory) RunPDAWithEvents(
	ctx context.Context,
	delegateCtx *DelegateContext,
	agentCfg config.AgentConfig,
	userPrompt string,
	parentSink ParentEventSink,
) (string, types.Usage, error) {
	agentName := delegateCtx.AgentName

	// 0. Apply timeout (consistent with RunDelegateWithEvents)
	timeout := agentCfg.GetTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		slog.Info("delegate: PDA timeout set",
			"agent", agentName,
			"timeout", timeout)
	} else {
		slog.Info("delegate: PDA running without timeout",
			"agent", agentName)
	}

	// 1. Use parent session ID for orchestrator context (no separate PDA session needed)
	sessionID := delegateCtx.ParentSessionID

	// Transcript accumulator — collects per-step sub-agent outputs with agent markers
	// so the full PDA conversation can be persisted to the session.
	var pdaTranscript strings.Builder

	// 2. RunPromptWithContext callback — unified callback for all PDA step types.
	// Builds orchestrator per agent, sets InjectedMessages for frame-local context.
	runPromptWithContextFn := func(ctx context.Context, callAgentName string, messages []provider.Message, userInput string) (string, cfg.Usage, []provider.Message, error) {
		// Look up agent config
		appCfg := config.GetConfig()
		if appCfg == nil {
			return "", cfg.Usage{}, nil, fmt.Errorf("config not available")
		}
		callAgentCfg, ok := appCfg.Agents[callAgentName]
		if !ok || !callAgentCfg.IsEnabled() {
			return "", cfg.Usage{}, nil, fmt.Errorf("agent %q not found or disabled", callAgentName)
		}

		// Build orchestrator for this agent
		callDC := &DelegateContext{
			Depth:           delegateCtx.Depth + 1,
			MaxDepth:        delegateCtx.MaxDepth,
			ParentSessionID: sessionID,
			AgentName:       callAgentName,
			Chain:           append(delegateCtx.Chain, callAgentName),
		}
		callOrch, callProv, cached, err := f.BuildOrchestratorForAgent(callAgentCfg, callDC, sessionID, cfg.IsRouteOnly(ctx), cfg.IsPDAManaged(ctx))
		if err != nil {
			return "", cfg.Usage{}, nil, fmt.Errorf("build orchestrator for %q: %w", callAgentName, err)
		}

		// Build InjectedMessages: frame context + current step's user input.
		// The orchestrator's InjectedMessages path does NOT auto-append UserInput,
		// so the caller must include it explicitly.
		injectedMsgs := make([]provider.Message, len(messages), len(messages)+1)
		copy(injectedMsgs, messages)
		injectedMsgs = append(injectedMsgs, provider.Message{Role: provider.RoleUser, Content: userInput})

		// Run with InjectedMessages (frame-local context)
		// Inject parentSink into context so nested DelegateTool calls can
		// forward content events to the top-level SSE stream.
		orchCtx := ctx
		if parentSink != nil {
			orchCtx = WithParentEventSink(orchCtx, parentSink)
		}
		events, err := callOrch.Run(orchCtx, &orchestrator.RunRequest{
			SessionID:        sessionID,
			UserInput:        userInput,
			Provider:         callProv,
			CachedSession:    cached,
			InjectedMessages: injectedMsgs,
		})
		if err != nil {
			return "", cfg.Usage{}, nil, fmt.Errorf("orchestrator run for %q: %w", callAgentName, err)
		}

		// Route-only calls are internal decision mechanics — suppress
		// their content from the user-visible transcript and message history.
		isRouteDecision := cfg.IsRouteOnly(ctx)

		// Drain events, forward to parent sink, aggregate result
		var content strings.Builder
		var totalUsage cfg.Usage
		var eventErr error
		for event := range events {
			event.AgentName = callAgentName
			event.AgentDepth = callDC.Depth
			switch event.Type {
			case types.EventTypeContent:
				content.WriteString(event.Content)
				if parentSink != nil && !isRouteDecision {
					parentSink(event)
				}
			case types.EventTypeToolCall, types.EventTypeToolResult, types.EventTypeToolCallUpdate:
				if parentSink != nil {
					parentSink(event)
				}
			case types.EventTypeDone:
				if event.Usage != nil {
					totalUsage = cfg.Usage{
						PromptTokens:     event.Usage.PromptTokens,
						CompletionTokens: event.Usage.CompletionTokens,
						TotalTokens:      event.Usage.TotalTokens,
					}
				}
			case types.EventTypeError:
				if parentSink != nil {
					parentSink(event)
				}
				if event.Error != nil {
					friendly := friendlyErrorMessage(callAgentName, event.Error)
					eventErr = fmt.Errorf("event error: %s", friendly)
				} else if event.ErrorMsg != "" {
					friendly := friendlyErrorMessage(callAgentName, fmt.Errorf("%s", event.ErrorMsg))
					eventErr = fmt.Errorf("event error: %s", friendly)
				}
				// Don't return early — let the loop drain remaining events
				// so the channel doesn't block, and then fall through to
				// transcript-append below.
			}
		}

		// Append this step's output to the PDA transcript with agent markers.
		// Skip route-decision steps — their content (e.g. "心理学专家") is
		// internal routing metadata that must not appear in the conversation.
		stepContent := content.String()
		if stepContent != "" && !isRouteDecision {
			depth := delegateCtx.Depth + 1
			if pdaTranscript.Len() > 0 {
				pdaTranscript.WriteString("\n")
			}
			pdaTranscript.WriteString(fmt.Sprintf("<<AGENT:%s:%d>>", callAgentName, depth))
			pdaTranscript.WriteString(stepContent)

			// Persist this step's content as a separate assistant message so
			// the session history shows each sub-agent's output individually.
			markedContent := fmt.Sprintf("<<AGENT:%s:%d>>", callAgentName, depth) + stepContent
			if _, addErr := f.sessions.AddMessage(sessionID, "assistant", markedContent, nil, ""); addErr != nil {
				slog.Warn("delegate: failed to persist PDA step message",
					"sessionID", sessionID, "agent", callAgentName, "error", addErr)
			}
		}

		// If an error event was received, return it after transcript capture
		if eventErr != nil {
			return content.String(), totalUsage, nil, eventErr
		}

		// Return new messages for frame context accumulation
		newMsgs := []provider.Message{
			{Role: provider.RoleUser, Content: userInput},
			{Role: provider.RoleAssistant, Content: content.String()},
		}
		return content.String(), totalUsage, newMsgs, nil
	}

	agentProvider := func(name string) (*cfg.AgentCFG, bool) {
		appCfg := config.GetConfig()
		if appCfg == nil {
			return nil, false
		}
		ac, ok := appCfg.Agents[name]
		if !ok {
			return nil, false
		}
		return &cfg.AgentCFG{
			Steps:        ac.Steps,
			MaxRecursion: ac.MaxRecursion,
		}, true
	}

	// 3. Create PDA engine
	maxStackDepth := config.GetConfig().Delegate.GetMaxStackDepth()
	engine := cfg.NewPDAEngine(cfg.PDAEngineOptions{
		RunPromptWithContext: runPromptWithContextFn,
		AgentProvider:        agentProvider,
		MaxStackDepth:        maxStackDepth,
	})

	// 3.1. Set checkpoint persistence callback
	store := f.sessions.DB()
	engine.OnCheckpoint = func(cp *cfg.PDACheckpoint) error {
		return SavePDACheckpoint(store, sessionID, cp)
	}

	// 3.2. Try loading saved checkpoint for resume
	savedCheckpoint, err := LoadPDACheckpoint(store, sessionID)
	if err != nil {
		slog.Warn("delegate: failed to load PDA checkpoint, starting fresh",
			"sessionID", sessionID, "error", err)
		savedCheckpoint = nil
	}

	// 3.3. Set PDA progress event callbacks
	// Resolve model name for progress events
	pdaModel := agentCfg.Model
	if pdaModel == "" {
		pdaModel = f.getDefaultModel()
	}

	// Helper to build parent step info from engine's parent frames
	buildParentSteps := func() []types.ParentStepInfo {
		parentFrames := engine.ParentFrames()
		if len(parentFrames) == 0 {
			return nil
		}
		ps := make([]types.ParentStepInfo, len(parentFrames))
		for i, pf := range parentFrames {
			// Derive the step label for the parent's current step
			stepLabel := ""
			if pf.StepIndex < len(pf.Steps) {
				stepLabel = pf.Steps[pf.StepIndex].Label
			}
			ps[i] = types.ParentStepInfo{
				AgentName:  pf.AgentName,
				StepIndex:  pf.StepIndex,
				TotalSteps: pf.TotalSteps,
				StepLabel:  stepLabel,
			}
		}
		return ps
	}

	if parentSink != nil {
		engine.OnStepStart = func(frame cfg.StackFrame, step cfg.Step) {
			parentSink(types.Event{
				Type: types.EventTypePDAProgress,
				PDAProgress: &types.PDAProgressEvent{
					AgentName:   frame.AgentName,
					StepIndex:   frame.StepIndex,
					TotalSteps:  frame.TotalSteps,
					StepLabel:   step.Label,
					StepType:    string(step.Type),
					Phase:       "started",
					StackDepth:  engine.CurrentStackDepth(),
					Model:       pdaModel,
					ParentSteps: buildParentSteps(),
				},
			})
		}
		engine.OnStepComplete = func(frame cfg.StackFrame, step cfg.Step, result string) {
			parentSink(types.Event{
				Type: types.EventTypePDAProgress,
				PDAProgress: &types.PDAProgressEvent{
					AgentName:     frame.AgentName,
					StepIndex:     frame.StepIndex,
					TotalSteps:    frame.TotalSteps,
					StepLabel:     step.Label,
					StepType:      string(step.Type),
					Phase:         "completed",
					StackDepth:    engine.CurrentStackDepth(),
					ExecutedSteps: engine.ExecutedSteps(),
					Model:         pdaModel,
					ParentSteps:   buildParentSteps(),
				},
			})
		}
	}

	// 4. Build DelegateInfo from DelegateContext
	delegateInfo := delegateInfoFromContext(delegateCtx)

	// 5. Build AgentCFG
	agentCFG := cfg.AgentCFG{
		Steps:        agentCfg.Steps,
		MaxRecursion: agentCfg.MaxRecursion,
	}

	// 6. Execute PDA
	slog.Info("delegate: starting PDA execution (direct)",
		"agent", agentName,
		"steps", len(agentCfg.Steps),
		"depth", delegateCtx.Depth,
		"sessionID", sessionID,
		"resuming", savedCheckpoint != nil)

	// 5.1. Mark session as PDA before execution starts.
	// This is a permanent flag that survives checkpoint clear/error.
	if markErr := MarkPDASession(store, sessionID, agentName); markErr != nil {
		slog.Warn("delegate: failed to mark session as PDA",
			"sessionID", sessionID, "error", markErr)
	}

	// Save user message BEFORE PDA execution so per-step assistant messages
	// appear after the user prompt in chronological order.
	if _, addErr := f.sessions.AddMessage(sessionID, "user", userPrompt, nil, ""); addErr != nil {
		slog.Warn("delegate: failed to persist PDA user message",
			"sessionID", sessionID, "error", addErr)
	}

	result, usage, err := engine.Execute(ctx, delegateInfo, agentCFG, userPrompt, savedCheckpoint)
	if err != nil {
		// Persist an interruption message so the session's conversation history contains
		// PDA context. Without this, when the user returns to the session, the LLM has
		// no memory of the PDA execution and cannot meaningfully help resume it.
		// Include the accumulated transcript (with agent markers) so the user can see
		// what sub-agents produced before the interruption.
		executedSteps := engine.ExecutedSteps()
		interruptMsg := fmt.Sprintf("[PDA Agent %q interrupted at step %d/%d]\nCompleted steps: %s\nReason: %s\n\nThe workflow has been checkpointed and can be resumed.",
			agentName, len(executedSteps), len(agentCfg.Steps),
			strings.Join(executedSteps, ", "),
			err.Error(),
		)
		// Per-step messages are already saved individually in runPromptWithContextFn.
		// User message was already saved before engine.Execute.
		// Only save the interrupt metadata message here.
		slog.Info("delegate: PDA interrupted",
			"sessionID", sessionID,
			"transcriptLen", pdaTranscript.Len(),
			"executedSteps", len(executedSteps),
			"error", err.Error())
		if _, addErr := f.sessions.AddMessage(sessionID, "assistant", interruptMsg, nil, ""); addErr != nil {
			slog.Warn("delegate: failed to persist PDA interrupt message",
				"sessionID", sessionID, "error", addErr)
		}
		return "", types.Usage{}, fmt.Errorf("PDA execution failed: %w", err)
	}

	// 6.1. Clear checkpoint on success
	if clearErr := ClearPDACheckpoint(store, sessionID); clearErr != nil {
		slog.Warn("delegate: failed to clear PDA checkpoint after success",
			"sessionID", sessionID, "error", clearErr)
	}

	// 6.2. Per-step messages are already saved individually in runPromptWithContextFn.
	// User message was already saved before engine.Execute.
	// Just log the completion. No additional transcript save needed.
	slog.Info("delegate: PDA completed, per-step messages already saved",
		"sessionID", sessionID,
		"transcriptLen", pdaTranscript.Len(),
		"resultLen", len(result),
		"executedSteps", len(engine.ExecutedSteps()))

	slog.Info("delegate: PDA completed (direct)",
		"agent", agentName,
		"depth", delegateCtx.Depth,
		"steps", len(engine.ExecutedSteps()),
		"tokens", usage.TotalTokens)

	return result, types.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}, nil
}

// completeTracking records the completion of a delegation invocation.
func (f *SubRunnerFactory) completeTracking(invocationID, status string, resultLen, tokens int, err error) {
	if f.tracker == nil || invocationID == "" {
		return
	}
	var errMsg *string
	if err != nil {
		s := err.Error()
		errMsg = &s
	}
	_ = f.tracker.CompleteDelegation(invocationID, status, resultLen, tokens, errMsg)
}

// buildFilteredRegistry creates a filtered clone of the parent registry.
func (f *SubRunnerFactory) buildFilteredRegistry(
	allowedTools []string,
	delegateCtx *DelegateContext,
) *tools.Registry {
	subRegistry := f.parentRegistry.Clone()

	// Filter by whitelist
	if len(allowedTools) > 0 {
		subRegistry.Filter(allowedTools)
	}

	// Remove delegate tool if depth limit reached
	if !delegateCtx.CanDelegate() {
		subRegistry.Remove("delegate")
	}

	// Set agent identity
	subRegistry.SetAgentID(delegateCtx.AgentName)

	return subRegistry
}

// makeToolExecutor creates a ToolExecutorFunc for sub-agent tool execution.
func (f *SubRunnerFactory) makeToolExecutor(
	subRegistry *tools.Registry,
	sessionID string,
) orchestrator.ToolExecutorFunc {
	return func(ctx context.Context, toolCalls []provider.ToolCall, sid string) ([]provider.Message, int) {
		var results []provider.Message
		errorCount := 0

		// Inject session context
		ctx = tools.WithSessionID(ctx, sessionID)
		agentID := subRegistry.GetAgentID()
		if agentID != "" {
			ctx = tools.WithAgentID(ctx, agentID)
		}

		for _, tc := range toolCalls {
			toolName := tc.Name
			if tc.Function != nil {
				toolName = tc.Function.Name
			}
			rawArgs := tc.Arguments
			if tc.Function != nil {
				rawArgs = tc.Function.Arguments
			}

			// Parse arguments
			var argsMap map[string]any
			if rawArgs != "" {
				if err := json.Unmarshal([]byte(rawArgs), &argsMap); err != nil {
					results = append(results, provider.Message{
						Role:       provider.RoleTool,
						Content:    fmt.Sprintf("failed to parse arguments: %v", err),
						ToolCallID: tc.ID,
					})
					errorCount++
					continue
				}
			}

			// Execute tool
			result, err := subRegistry.Execute(ctx, toolName, argsMap)
			if err != nil {
				results = append(results, provider.Message{
					Role:       provider.RoleTool,
					Content:    fmt.Sprintf("tool error: %v", err),
					ToolCallID: tc.ID,
				})
				errorCount++
				continue
			}

			if result.IsError {
				errorCount++
			}

			results = append(results, provider.Message{
				Role:       provider.RoleTool,
				Content:    result.Content,
				ToolCallID: tc.ID,
			})
		}

		return results, errorCount
	}
}

// BuildOrchestratorForAgent creates a fully configured orchestrator for a specific agent,
// along with the provider and cached session. This is used by the PDA engine to run
// prompts on a shared session.
// When routeOnly is true, the orchestrator is built with an empty tool registry and
// maxIterations=1 so the LLM produces a simple text decision without tool calls.
func (f *SubRunnerFactory) BuildOrchestratorForAgent(
	agentCfg config.AgentConfig,
	delegateCtx *DelegateContext,
	sessionID string,
	routeOnly ...bool,
) (orchestrator.Orchestrator, provider.Provider, *scheduler.CachedSession, error) {
	isRoute := len(routeOnly) > 0 && routeOnly[0]
	isPDAManaged := len(routeOnly) > 1 && routeOnly[1]
	// 1. Resolve provider
	model := agentCfg.Model
	if model == "" {
		model = f.getDefaultModel()
	}
	prov, _, err := f.multiPool.GetProvider(model)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get provider for model %s: %w", model, err)
	}

	// 2. Build filtered registry (empty for route-only mode, no delegate for PDA-managed)
	var subRegistry *tools.Registry
	if isRoute {
		subRegistry = tools.NewRegistry()
	} else {
		subRegistry = f.buildFilteredRegistry(agentCfg.Tools, delegateCtx)
		// PDA-managed steps should not have access to the delegate tool —
		// the PDA engine controls delegation via route/agent_ref steps.
		if isPDAManaged {
			subRegistry.Remove("delegate")
		}
	}

	// 3. Build sub-agent system prompt
	agentSysPrompt := agentCfg.SystemPrompt
	if agentSysPrompt == "" {
		agentSysPrompt = "You are a helpful AI assistant."
	}
	subPromptCfg := prompt.PromptConfig{
		AgentName:   delegateCtx.AgentName,
		ExtraPrompt: agentSysPrompt,
	}
	if f.systemPrompt != nil {
		parentCfg := f.systemPrompt.GetConfig()
		subPromptCfg.Timezone = parentCfg.Timezone
		subPromptCfg.WorkspaceDir = parentCfg.WorkspaceDir
	}
	// Resolve workspace from session binding (overrides parent config if available)
	if f.workspaceResolver != nil && sessionID != "" {
		if wsPath := f.workspaceResolver(sessionID); wsPath != "" {
			subPromptCfg.WorkspaceDir = wsPath
		}
	}
	subPromptBuilder := prompt.NewSystemPromptBuilder(subPromptCfg, subRegistry)
	if f.mcpManager != nil {
		subPromptBuilder.WithMCPManager(f.mcpManager)
	}

	// 4. Config overrides
	maxIter := f.maxIterations
	if isRoute {
		maxIter = 1 // Route decisions need exactly one LLM call, no tool loop
	} else if agentCfg.MaxIterations > 0 {
		maxIter = agentCfg.MaxIterations
	}
	temp := f.temperature
	if agentCfg.Temperature > 0 {
		temp = agentCfg.Temperature
	}
	maxTok := f.maxTokens
	if agentCfg.MaxTokens > 0 {
		maxTok = agentCfg.MaxTokens
	}
	timeout := agentCfg.GetTimeout()

	// 5. Build orchestrator
	orchBuilder := orchestrator.NewBuilder(orchestrator.BuilderOptions{
		Sessions: f.sessions,
		Registry: subRegistry,
		Config: orchestrator.Config{
			MaxIterations: maxIter,
			MaxTokens:     maxTok,
			Temperature:   temp,
			StreamOutput:  true,
			Timeout:       timeout,
			SystemPrompt:  agentSysPrompt,
		},
		Compactor:      f.compactor,
		SystemPrompt:   subPromptBuilder,
		SkillManager:   f.skillManager,
		HookManager:    f.hookManager,
		MCPManager:     f.mcpManager,
		ContextManager: f.contextManager,
		ToolExecutor:   f.makeToolExecutor(subRegistry, sessionID),
	})
	orch := orchBuilder.Build(prov)

	// 6. Session creation + metadata
	cached, _ := f.sessions.GetOrCreate(sessionID, nil)

	if db := f.sessions.DB(); db != nil {
		if err := db.UpdateSessionModel(sessionID, model); err != nil {
			slog.Warn("delegate: failed to set session model",
				"sessionID", sessionID, "model", model, "error", err)
		} else if cached.Session != nil {
			cached.Session.Model = model
		}
	}

	// Copy parent workspace binding to child session
	if f.workspaceBinder != nil && delegateCtx.ParentSessionID != "" {
		f.workspaceBinder(delegateCtx.ParentSessionID, sessionID)
	}

	return orch, prov, cached, nil
}
