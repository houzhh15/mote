// Package transport provides transport layer implementations for MCP protocol.
package transport

import (
	"context"
)

// TransportType represents the type of transport.
type TransportType string

const (
	// TransportStdio represents stdio-based transport.
	TransportStdio TransportType = "stdio"
	// TransportHTTPSSE represents HTTP+SSE-based transport.
	TransportHTTPSSE TransportType = "http+sse"
	// TransportHTTP represents simple HTTP transport (no SSE).
	TransportHTTP TransportType = "http"
)

// Transport defines the interface for MCP message transport.
type Transport interface {
	// Send sends data through the transport.
	// The data should be a complete JSON-RPC message.
	Send(ctx context.Context, data []byte) error

	// Receive receives data from the transport.
	// Returns a complete JSON-RPC message.
	Receive(ctx context.Context) ([]byte, error)

	// Close closes the transport and releases resources.
	Close() error
}

// ServerTransport is a transport used by MCP servers.
// It typically reads from stdin and writes to stdout.
type ServerTransport interface {
	Transport
}

// ClientTransport is a transport used by MCP clients.
// It may need additional lifecycle management like starting a subprocess.
type ClientTransport interface {
	Transport

	// Start starts the transport (e.g., launches subprocess).
	Start() error
}
