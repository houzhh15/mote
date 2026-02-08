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
