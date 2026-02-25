package delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mote/internal/compaction"
	"mote/internal/config"
	internalContext "mote/internal/context"
	"mote/internal/hooks"
	"mote/internal/mcp/client"
	"mote/internal/prompt"
	"mote/internal/provider"
	"mote/internal/runner/orchestrator"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/tools"
)

// ParentEventSink is a callback to transparently forward sub-agent events to the parent.
type ParentEventSink func(event types.Event)

// SubRunnerFactory creates lightweight sub-agent execution environments.
// It holds shared dependencies from the main Runner and constructs
// filtered Orchestrators for sub-agents.
type SubRunnerFactory struct {
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

// SetTracker enables delegation tracking for audit and monitoring.
func (f *SubRunnerFactory) SetTracker(t *DelegationTracker) {
	f.tracker = t
}

// SetWorkspaceBinder sets the function used to copy workspace bindings from parent to child sessions.
func (f *SubRunnerFactory) SetWorkspaceBinder(binder func(parentSessionID, childSessionID string)) {
	f.workspaceBinder = binder
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

	// 3. Apply timeout
	timeout := agentCfg.GetTimeout()
	subCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 4. Resolve provider
	model := agentCfg.Model
	if model == "" {
		model = f.defaultModel
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
			f.completeTracking(invocationID, "failed", content.Len(), totalUsage.TotalTokens, fmt.Errorf("%s", errMsg))
			return content.String(), totalUsage,
				fmt.Errorf("sub-agent error: %s", errMsg)
		}
	}

	f.completeTracking(invocationID, "completed", content.Len(), totalUsage.TotalTokens, nil)
	return content.String(), totalUsage, nil
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
