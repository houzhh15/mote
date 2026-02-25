package delegate

import "context"

// MaxAbsoluteDepth is the hard limit on delegation recursion depth.
// This cannot be overridden by configuration.
const MaxAbsoluteDepth = 5

// DelegateContext carries delegation chain metadata through context.
type DelegateContext struct {
	// Depth is the current recursion depth (0 = main agent).
	Depth int
	// MaxDepth is the maximum allowed depth (from AgentConfig or global default).
	MaxDepth int
	// ParentSessionID is the parent session ID (used to build child session IDs).
	ParentSessionID string
	// AgentName is the current sub-agent name (the AgentConfig map key).
	AgentName string
	// Chain is the delegation path, e.g. ["main", "researcher", "summarizer"].
	Chain []string
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
	return &DelegateContext{Depth: 0, MaxDepth: 3}
}

// CanDelegate returns true if the current depth allows further delegation.
func (dc *DelegateContext) CanDelegate() bool {
	return dc.Depth < dc.MaxDepth
}

// ForChild creates a child delegation context for the given agent name.
func (dc *DelegateContext) ForChild(agentName string) *DelegateContext {
	newChain := make([]string, len(dc.Chain)+1)
	copy(newChain, dc.Chain)
	newChain[len(dc.Chain)] = agentName
	return &DelegateContext{
		Depth:           dc.Depth + 1,
		MaxDepth:        dc.MaxDepth,
		ParentSessionID: dc.ParentSessionID,
		AgentName:       agentName,
		Chain:           newChain,
	}
}
