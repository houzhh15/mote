package delegate

import "context"

// DelegateContext carries delegation chain metadata through context.
type DelegateContext struct {
	// Depth is the current recursion depth (0 = main agent).
	Depth int
	// MaxDepth is the absolute depth ceiling.
	// Delegation is blocked when Depth >= MaxDepth.
	// 0 = unlimited. This ceiling can be tightened (never loosened)
	// by per-agent MaxDepth settings which mean "how many more levels
	// this agent can delegate downward".
	MaxDepth int
	// ParentSessionID is the parent session ID (used to build child session IDs).
	ParentSessionID string
	// AgentName is the current sub-agent name (the AgentConfig map key).
	AgentName string
	// Chain is the delegation path, e.g. ["main", "researcher", "summarizer"].
	Chain []string
	// RecursionCounters tracks per-agent recursion counts for PDA engine.
	RecursionCounters map[string]int
}

type delegateContextKey struct{}

// WithDelegateContext injects the delegation context into a Go context.
func WithDelegateContext(ctx context.Context, dc *DelegateContext) context.Context {
	return context.WithValue(ctx, delegateContextKey{}, dc)
}

// GetDelegateContext extracts the delegation context from a Go context.
// Returns a default (Depth=0, MaxDepth=3) if not present.
func GetDelegateContext(ctx context.Context) *DelegateContext {
	if dc, ok := ctx.Value(delegateContextKey{}).(*DelegateContext); ok {
		return dc
	}
	return &DelegateContext{Depth: 0, MaxDepth: 3, RecursionCounters: map[string]int{}}
}

// CanDelegate returns true if the current depth allows further delegation.
// MaxDepth == 0 means unlimited.
func (dc *DelegateContext) CanDelegate() bool {
	if dc.MaxDepth == 0 {
		return true
	}
	return dc.Depth < dc.MaxDepth
}

// ForChild creates a child delegation context for the given agent name.
func (dc *DelegateContext) ForChild(agentName string) *DelegateContext {
	newChain := make([]string, len(dc.Chain)+1)
	copy(newChain, dc.Chain)
	newChain[len(dc.Chain)] = agentName

	// Copy recursion counters
	counters := make(map[string]int, len(dc.RecursionCounters))
	for k, v := range dc.RecursionCounters {
		counters[k] = v
	}

	return &DelegateContext{
		Depth:             dc.Depth + 1,
		MaxDepth:          dc.MaxDepth,
		ParentSessionID:   dc.ParentSessionID,
		AgentName:         agentName,
		Chain:             newChain,
		RecursionCounters: counters,
	}
}
