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
