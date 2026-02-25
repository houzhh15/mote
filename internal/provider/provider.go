// Package provider defines the LLM provider interface and types.
package provider

import "context"

// Provider defines the interface for LLM providers.
type Provider interface {
	// Name returns the provider name.
	Name() string

	// Models returns the list of supported models.
	Models() []string

	// Chat sends a chat request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Stream sends a chat request and returns a channel of streaming events.
	Stream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
}

// ACPCapable indicates a provider that uses ACP protocol.
// ACP providers handle tool call loops internally, so the runner
// should not perform external tool call iterations.
type ACPCapable interface {
	Provider
	// IsACPProvider returns true if this provider uses ACP protocol.
	IsACPProvider() bool
}

// SessionResettable indicates a provider that maintains per-session state
// (e.g., ACP sessions, CLI processes) and can reset it on demand.
// This is used when model/workspace changes require a full session rebuild.
type SessionResettable interface {
	// ResetSession clears all runtime state for the given conversationID,
	// including ACP session mappings, cached models, and CLI processes if stuck.
	// The next request for this conversationID will create fresh resources.
	ResetSession(conversationID string)
}

// ConnectionResettable indicates a provider that can reset its HTTP connection
// pools.  This is used after sustained API usage (e.g., compaction) to force
// new TCP connections and avoid stale ALB session affinity.
type ConnectionResettable interface {
	// ResetConnections closes all idle connections in the provider's HTTP
	// client pools, forcing fresh TCP connections on the next request.
	ResetConnections()
}

// ContextWindowProvider indicates a provider that can report the context
// window size for a given model.  The compaction system uses this to
// adapt MaxContextTokens and ReserveTokens proportionally instead of
// relying on a fixed default (48K).
type ContextWindowProvider interface {
	// ContextWindow returns the maximum context window in tokens for the
	// given model name.  Returns 0 if the model is unknown.
	ContextWindow(model string) int
}

// MaxOutputProvider indicates a provider that can report the maximum output
// token limit for a given model.  This is used to inject the limit into the
// system prompt so the LLM can self-regulate its output size and avoid
// truncation of large tool call arguments.
type MaxOutputProvider interface {
	// MaxOutput returns the maximum output tokens the model can generate
	// in a single response.  Returns 0 if unknown.
	MaxOutput(model string) int
}
