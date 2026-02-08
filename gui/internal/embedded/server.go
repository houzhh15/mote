// Package embedded provides a thin wrapper around internal/server for GUI mode.
// The actual server implementation is in internal/server and shared with CLI.
package embedded

import (
	"mote/internal/server"
)

// Re-export types for backward compatibility
type (
	Server       = server.Server
	ServerConfig = server.ServerConfig
)

// NewServer creates a new server instance (wrapper for backward compatibility)
var NewServer = server.NewServer
