package delegate

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"mote/internal/config"
	"mote/internal/provider"
	"mote/internal/runner/delegate/cfg"
	"mote/internal/runner/orchestrator"
	"mote/internal/runner/types"
	"mote/internal/tools"
)

// DelegateTool implements tools.Tool to delegate tasks to sub-agents.
// It reads agents dynamically from config so agents created at runtime are
// immediately available for delegation.
type DelegateTool struct {
	factory        *SubRunnerFactory
	globalMaxDepth int
}

// NewDelegateTool creates a new delegate tool.
func NewDelegateTool(factory *SubRunnerFactory, globalMaxDepth int) *DelegateTool {
	return &DelegateTool{
		factory:        factory,
		globalMaxDepth: globalMaxDepth,
	}
}

// enabledAgents returns the currently enabled agents from live config.
func (t *DelegateTool) enabledAgents() map[string]config.AgentConfig {
	appCfg := config.GetConfig()
	if appCfg == nil {
		return nil
	}
	agents := make(map[string]config.AgentConfig)
	for name, ac := range appCfg.Agents {
		if ac.IsEnabled() {
			agents[name] = ac
		}
	}
	return agents
}

// Name returns the tool name.
func (t *DelegateTool) Name() string { return "delegate" }

// Description returns a dynamic description listing available agents.
func (t *DelegateTool) Description() string {
	return "Delegate a task to a specialized sub-agent. Available agents: " + t.listAgents()
}

// Parameters returns the JSON Schema for the delegate tool.
func (t *DelegateTool) Parameters() map[string]any {
	names := t.agentNames()
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "Name of the sub-agent to delegate to: " + t.listAgents(),
				"enum":        names,
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Task description for the sub-agent",
			},
		},
		"required": []string{"agent", "prompt"},
	}
}

// Execute delegates a task to a sub-agent.
func (t *DelegateTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	// Extract parameters
	agentName, ok := args["agent"].(string)
	if !ok {
		return tools.NewErrorResult("parameter 'agent' must be a string"), nil
	}
	prompt, ok := args["prompt"].(string)
	if !ok {
		return tools.NewErrorResult("parameter 'prompt' must be a string"), nil
	}

	// Get delegation context
	dc := GetDelegateContext(ctx)

	// Ensure ParentSessionID is set from the current session context.
	// For root-level delegation (depth 0), the default DelegateContext has
	// an empty ParentSessionID. We populate it from the orchestrator's session
	// ID so that the child factory can: (a) build proper child session IDs,
	// and (b) copy the parent workspace binding to the child session.
	if dc.ParentSessionID == "" {
		if sid, ok := tools.SessionIDFromContext(ctx); ok && sid != "" {
			dc.ParentSessionID = sid
		}
	}

	// Dynamic hard limit check (0 = unlimited)
	maxStackDepth := config.GetConfig().Delegate.GetMaxStackDepth()
	if maxStackDepth > 0 && dc.Depth >= maxStackDepth {
		return tools.NewErrorResult(fmt.Sprintf(
			"delegation depth %d exceeds max stack depth %d",
			dc.Depth, maxStackDepth)), nil
	}

	// Configured depth check
	if !dc.CanDelegate() {
		return tools.NewErrorResult(fmt.Sprintf(
			"delegation depth %d exceeds configured maximum %d (chain: %s)",
			dc.Depth, dc.MaxDepth,
			strings.Join(dc.Chain, " -> "))), nil
	}

	// Read agents dynamically from live config
	agents := t.enabledAgents()

	// Agent existence check
	agentCfg, exists := agents[agentName]
	if !exists {
		available := make([]string, 0, len(agents))
		for name := range agents {
			available = append(available, name)
		}
		sort.Strings(available)
		return tools.NewErrorResult(fmt.Sprintf(
			"unknown agent: %q (available: %s)",
			agentName, strings.Join(available, ", "))), nil
	}

	// Circular delegation detection
	for _, ancestor := range dc.Chain {
		if ancestor == agentName {
			return tools.NewErrorResult(fmt.Sprintf(
				"circular delegation detected: %s -> %s (chain: %s)",
				dc.Chain[len(dc.Chain)-1], agentName,
				strings.Join(dc.Chain, " -> "))), nil
		}
	}

	// Inject contexts
	childDC := dc.ForChild(agentName)

	// Agent-level depth constraint: MaxDepth means how many more levels
	// this agent can delegate downward. E.g. MaxDepth=2 means the agent
	// (at childDC.Depth) can spawn children up to childDC.Depth+2.
	// Take the MIN with the inherited ceiling so a child can never
	// escalate beyond the parent's limit.
	if agentCfg.MaxDepth > 0 {
		agentCeiling := childDC.Depth + agentCfg.MaxDepth
		if childDC.MaxDepth == 0 || agentCeiling < childDC.MaxDepth {
			childDC.MaxDepth = agentCeiling
		}
	}

	ctx = WithDelegateContext(ctx, childDC)
	ctx = tools.WithAgentID(ctx, agentName)

	// PDA structured execution path
	if agentCfg.HasSteps() {
		return t.executeStructured(ctx, childDC, agentCfg, agentName, prompt)
	}

	// Execute delegation (legacy single-prompt path)
	startTime := time.Now()

	// If a parentSink is threaded through the context, forward content events
	// (but NOT tool events) from the nested sub-agent so the user sees output
	// in real-time without exposing internal tool activity.
	var result string
	var usage types.Usage
	var err error
	if sink := ParentEventSinkFromContext(ctx); sink != nil {
		contentOnlySink := ParentEventSink(func(event types.Event) {
			switch event.Type {
			case types.EventTypeContent, types.EventTypeError, types.EventTypeHeartbeat:
				sink(event)
			}
		})
		result, usage, err = t.factory.RunDelegateWithEvents(ctx, childDC, agentCfg, prompt, contentOnlySink)
	} else {
		result, usage, err = t.factory.RunDelegate(ctx, childDC, agentCfg, prompt)
	}
	duration := time.Since(startTime)

	if err != nil {
		slog.Warn("delegate execution failed",
			"agent", agentName,
			"depth", childDC.Depth,
			"duration", duration,
			"error", err)
		friendly := friendlyErrorMessage(agentName, err)
		return tools.NewErrorResult(fmt.Sprintf(
			"delegate(%s) failed: %s", agentName, friendly)), nil
	}

	slog.Info("delegate completed",
		"agent", agentName,
		"depth", childDC.Depth,
		"duration", duration,
		"tokens", usage.TotalTokens)

	output := fmt.Sprintf("[Agent: %s | Duration: %s | Tokens: %d]\n\n%s",
		agentName, duration.Round(time.Millisecond), usage.TotalTokens, result)

	return tools.ToolResult{
		Content: output,
		Metadata: map[string]any{
			"agent":    agentName,
			"duration": duration.Milliseconds(),
			"tokens":   usage.TotalTokens,
			"depth":    childDC.Depth,
		},
	}, nil
}

// agentNames returns sorted agent names from live config.
func (t *DelegateTool) agentNames() []string {
	agents := t.enabledAgents()
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// listAgents returns a comma-separated list of agent names with descriptions from live config.
func (t *DelegateTool) listAgents() string {
	agents := t.enabledAgents()
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		a := agents[name]
		mode := "standard"
		if len(a.Steps) > 0 {
			mode = fmt.Sprintf("PDA/%d steps", len(a.Steps))
		}
		if a.Description != "" {
			parts = append(parts, fmt.Sprintf("%s [%s] (%s)", name, mode, a.Description))
		} else {
			parts = append(parts, fmt.Sprintf("%s [%s]", name, mode))
		}
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// PDA (structured) execution support
// ---------------------------------------------------------------------------

// executeStructured runs an agent through the PDA engine when it has structured steps.
func (t *DelegateTool) executeStructured(
	ctx context.Context,
	childDC *DelegateContext,
	agentCfg config.AgentConfig,
	agentName string,
	prompt string,
) (tools.ToolResult, error) {
	startTime := time.Now()

	// Accumulate sub-agent content across all PDA steps with agent markers.
	// This transcript is persisted to the session on both success and error
	// so the history view can show each sub-agent's contribution.
	var pdaTranscript strings.Builder

	// 0. Apply timeout (consistent with non-PDA delegate path)
	timeout := agentCfg.GetTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		slog.Info("delegate: PDA (tool) timeout set",
			"agent", agentName,
			"timeout", timeout)
	}

	// 1. Use parent session ID (no separate PDA session needed)
	sessionID := childDC.ParentSessionID

	// 2. RunPromptWithContext callback — unified for all PDA step types.
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
			Depth:           childDC.Depth + 1,
			MaxDepth:        childDC.MaxDepth,
			ParentSessionID: sessionID,
			AgentName:       callAgentName,
			Chain:           append(childDC.Chain, callAgentName),
		}
		callOrch, callProv, cached, err := t.factory.BuildOrchestratorForAgent(callAgentCfg, callDC, sessionID, cfg.IsRouteOnly(ctx), cfg.IsPDAManaged(ctx))
		if err != nil {
			return "", cfg.Usage{}, nil, fmt.Errorf("build orchestrator for %q: %w", callAgentName, err)
		}

		// Build InjectedMessages: frame context + current step's user input.
		injectedMsgs := make([]provider.Message, len(messages), len(messages)+1)
		copy(injectedMsgs, messages)
		injectedMsgs = append(injectedMsgs, provider.Message{Role: provider.RoleUser, Content: userInput})

		// Run with InjectedMessages (frame-local context)
		// Thread parentSink into context for deeper nested delegate calls.
		orchCtx := ctx
		if sink := ParentEventSinkFromContext(ctx); sink != nil {
			orchCtx = WithParentEventSink(orchCtx, sink)
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

		// Drain events, aggregate result.
		// Forward content events to parent sink so the user sees sub-agent
		// output in real-time, but do NOT forward tool events (user's tools
		// should remain invisible for nested sub-agents).
		// Route-decision steps are suppressed from transcript/persistence.
		isRouteDecision := cfg.IsRouteOnly(ctx)
		sink := ParentEventSinkFromContext(ctx)
		var result string
		var totalUsage cfg.Usage
		var evtErr error
		if isRouteDecision {
			// Drain without forwarding to sink
			result, totalUsage, evtErr = collectPDAEventsWithSink(events, nil, callAgentName, callDC.Depth)
		} else {
			result, totalUsage, evtErr = collectPDAEventsWithSink(events, sink, callAgentName, callDC.Depth)
		}

		// Append this step's output to the PDA transcript with agent markers.
		// Skip route-decision steps — their content is internal routing metadata.
		stepContent := result
		if stepContent != "" && !isRouteDecision {
			depth := childDC.Depth + 1
			if pdaTranscript.Len() > 0 {
				pdaTranscript.WriteString("\n")
			}
			pdaTranscript.WriteString(fmt.Sprintf("<<AGENT:%s:%d>>", callAgentName, depth))
			pdaTranscript.WriteString(stepContent)

			// Persist this step's content as a separate assistant message so
			// the session history shows each sub-agent's output individually.
			if t.factory != nil && t.factory.sessions != nil {
				markedContent := fmt.Sprintf("<<AGENT:%s:%d>>", callAgentName, depth) + stepContent
				if _, addErr := t.factory.sessions.AddMessage(sessionID, "assistant", markedContent, nil, ""); addErr != nil {
					slog.Warn("delegate: failed to persist PDA step message",
						"sessionID", sessionID, "agent", callAgentName, "error", addErr)
				}
			}
		}

		if evtErr != nil {
			return result, totalUsage, nil, evtErr
		}

		// Return new messages for frame context accumulation
		newMsgs := []provider.Message{
			{Role: provider.RoleUser, Content: userInput},
			{Role: provider.RoleAssistant, Content: result},
		}
		return result, totalUsage, newMsgs, nil
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
	store := t.factory.sessions.DB()
	engine.OnCheckpoint = func(cp *cfg.PDACheckpoint) error {
		return SavePDACheckpoint(store, sessionID, cp)
	}

	// 3.2. Try loading saved checkpoint for resume
	savedCheckpoint, loadErr := LoadPDACheckpoint(store, sessionID)
	if loadErr != nil {
		slog.Warn("delegate: failed to load PDA checkpoint, starting fresh",
			"sessionID", sessionID, "error", loadErr)
		savedCheckpoint = nil
	}

	// 4. Build DelegateInfo from DelegateContext
	delegateInfo := delegateInfoFromContext(childDC)

	// 5. Build AgentCFG
	agentCFG := cfg.AgentCFG{
		Steps:        agentCfg.Steps,
		MaxRecursion: agentCfg.MaxRecursion,
	}

	// 5.1. Mark session as PDA before execution starts.
	// This is a permanent flag that survives checkpoint clear/error.
	if markErr := MarkPDASession(store, sessionID, agentName); markErr != nil {
		slog.Warn("delegate: failed to mark session as PDA",
			"sessionID", sessionID, "error", markErr)
	}

	// 6. Execute PDA
	slog.Info("delegate: starting PDA execution (tool)",
		"agent", agentName,
		"steps", len(agentCfg.Steps),
		"depth", childDC.Depth,
		"sessionID", sessionID,
		"resuming", savedCheckpoint != nil)

	result, usage, err := engine.Execute(ctx, delegateInfo, agentCFG, prompt, savedCheckpoint)
	duration := time.Since(startTime)

	if err != nil {
		// Persist interruption message to session history.
		// Include the accumulated transcript (with agent markers) so the user
		// can see what sub-agents produced before the interruption.
		executedSteps := engine.ExecutedSteps()
		interruptMsg := fmt.Sprintf("[PDA Agent %q interrupted at step %d/%d after %s]\nCompleted: %s\nReason: %s",
			agentName, len(executedSteps), len(agentCfg.Steps),
			duration.Round(time.Millisecond),
			strings.Join(executedSteps, ", "),
			err.Error(),
		)
		// Per-step messages are already saved individually in runPromptWithContextFn.
		// Only save the interrupt metadata message here.
		slog.Info("delegate: PDA (tool) interrupted",
			"agent", agentName,
			"depth", childDC.Depth,
			"duration", duration,
			"sessionID", sessionID,
			"transcriptLen", pdaTranscript.Len(),
			"executedSteps", len(executedSteps),
			"error", err.Error())
		if t.factory != nil && t.factory.sessions != nil {
			if _, addErr := t.factory.sessions.AddMessage(sessionID, "assistant", interruptMsg, nil, ""); addErr != nil {
				slog.Warn("delegate: failed to persist PDA interrupt message",
					"sessionID", sessionID, "error", addErr)
			}
		}
		return tools.NewErrorResult(fmt.Sprintf(
			"delegate(%s) PDA failed: %v", agentName, err)), nil
	}

	// 6.1. Clear checkpoint on success
	if clearErr := ClearPDACheckpoint(store, sessionID); clearErr != nil {
		slog.Warn("delegate: failed to clear PDA checkpoint after success",
			"sessionID", sessionID, "error", clearErr)
	}

	// Per-step messages are already saved individually in runPromptWithContextFn.
	// Just log the completion. No additional transcript save needed.
	slog.Info("delegate: PDA completed, per-step messages already saved",
		"agent", agentName,
		"depth", childDC.Depth,
		"duration", duration,
		"tokens", usage.TotalTokens,
		"steps", len(engine.ExecutedSteps()),
		"transcriptLen", pdaTranscript.Len())

	output := fmt.Sprintf("[Agent: %s | Mode: structured | Steps: %d | Duration: %s | Tokens: %d]\n\n%s",
		agentName, len(engine.ExecutedSteps()), duration.Round(time.Millisecond),
		usage.TotalTokens, result)

	return tools.ToolResult{
		Content: output,
		Metadata: map[string]any{
			"agent":    agentName,
			"mode":     "structured",
			"steps":    len(engine.ExecutedSteps()),
			"duration": duration.Milliseconds(),
			"tokens":   usage.TotalTokens,
			"depth":    childDC.Depth,
		},
	}, nil
}

// collectPDAEvents drains an event channel, aggregating content text and usage.
func collectPDAEvents(events <-chan types.Event) (string, cfg.Usage, error) {
	return collectPDAEventsWithSink(events, nil, "", 0)
}

// collectPDAEventsWithSink drains an event channel, aggregating content text and usage.
// If sink is non-nil, content events are forwarded to the parent SSE stream so that
// nested sub-agent output is visible in real-time. Tool events are intentionally NOT
// forwarded — nested sub-agent tool calls should remain invisible to the user.
func collectPDAEventsWithSink(events <-chan types.Event, sink ParentEventSink, agentName string, agentDepth int) (string, cfg.Usage, error) {
	var content strings.Builder
	var totalUsage cfg.Usage
	var eventErr error

	for event := range events {
		switch event.Type {
		case types.EventTypeContent:
			content.WriteString(event.Content)
			if sink != nil {
				fwd := event
				fwd.AgentName = agentName
				fwd.AgentDepth = agentDepth
				sink(fwd)
			}
		case types.EventTypeToolCall, types.EventTypeToolResult, types.EventTypeToolCallUpdate:
			if sink != nil {
				fwd := event
				fwd.AgentName = agentName
				fwd.AgentDepth = agentDepth
				sink(fwd)
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
			if sink != nil {
				fwd := event
				fwd.AgentName = agentName
				fwd.AgentDepth = agentDepth
				sink(fwd)
			}
			if event.Error != nil {
				friendly := friendlyErrorMessage(agentName, event.Error)
				eventErr = fmt.Errorf("event error: %s", friendly)
			} else if event.ErrorMsg != "" {
				friendly := friendlyErrorMessage(agentName, fmt.Errorf("%s", event.ErrorMsg))
				eventErr = fmt.Errorf("event error: %s", friendly)
			}
			// Don't return early — let the loop drain remaining events
			// so the sending goroutine doesn't block, and partial content
			// before the error is captured for transcript persistence.
		}
	}
	return content.String(), totalUsage, eventErr
}

// delegateInfoFromContext converts a DelegateContext to a cfg.DelegateInfo.
func delegateInfoFromContext(dc *DelegateContext) *cfg.DelegateInfo {
	return &cfg.DelegateInfo{
		Depth:             dc.Depth,
		MaxDepth:          dc.MaxDepth,
		ParentSessionID:   dc.ParentSessionID,
		AgentName:         dc.AgentName,
		Chain:             dc.Chain,
		RecursionCounters: dc.RecursionCounters,
	}
}

// delegateContextFromInfo converts a cfg.DelegateInfo back to a DelegateContext.
func delegateContextFromInfo(di *cfg.DelegateInfo) *DelegateContext {
	return &DelegateContext{
		Depth:             di.Depth,
		MaxDepth:          di.MaxDepth,
		ParentSessionID:   di.ParentSessionID,
		AgentName:         di.AgentName,
		Chain:             di.Chain,
		RecursionCounters: di.RecursionCounters,
	}
}

// cfgUsageFromTypes converts types.Usage to cfg.Usage.
func cfgUsageFromTypes(u types.Usage) cfg.Usage {
	return cfg.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
