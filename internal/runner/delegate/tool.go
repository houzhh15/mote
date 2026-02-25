package delegate

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"mote/internal/config"
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

	// Hard limit check
	if dc.Depth >= MaxAbsoluteDepth {
		return tools.NewErrorResult(fmt.Sprintf(
			"delegation depth %d exceeds absolute maximum %d",
			dc.Depth, MaxAbsoluteDepth)), nil
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

	// Agent-level depth check
	if agentCfg.MaxDepth > 0 && dc.Depth >= agentCfg.MaxDepth {
		return tools.NewErrorResult(fmt.Sprintf(
			"agent %q depth limit reached (%d)",
			agentName, agentCfg.MaxDepth)), nil
	}

	// Inject contexts
	childDC := dc.ForChild(agentName)
	ctx = WithDelegateContext(ctx, childDC)
	ctx = tools.WithAgentID(ctx, agentName)

	// Execute delegation
	startTime := time.Now()
	result, usage, err := t.factory.RunDelegate(ctx, childDC, agentCfg, prompt)
	duration := time.Since(startTime)

	if err != nil {
		slog.Warn("delegate execution failed",
			"agent", agentName,
			"depth", childDC.Depth,
			"duration", duration,
			"error", err)
		return tools.NewErrorResult(fmt.Sprintf(
			"delegate(%s) failed: %v", agentName, err)), nil
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
		desc := agents[name].Description
		if desc != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", name, desc))
		} else {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}
