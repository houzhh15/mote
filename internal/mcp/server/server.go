// Package server implements the MCP server that exposes tools via the MCP protocol.
package server

import (
	"context"
	"encoding/json"
	"sync"

	"mote/internal/mcp/protocol"
	"mote/internal/mcp/transport"
	"mote/internal/tools"
)

// Server is an MCP server that exposes tools via the MCP protocol.
type Server struct {
	name    string
	version string

	transport transport.Transport
	registry  *tools.Registry
	mapper    *ToolMapper
	handler   *MethodHandler

	initialized bool
	initMu      sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ServerOption is a functional option for configuring a Server.
type ServerOption func(*Server)

// WithRegistry sets the tool registry for the server.
func WithRegistry(registry *tools.Registry) ServerOption {
	return func(s *Server) {
		s.registry = registry
	}
}

// WithTransport sets the transport for the server.
func WithTransport(t transport.Transport) ServerOption {
	return func(s *Server) {
		s.transport = t
	}
}

// WithToolPrefix sets a prefix for tool names in MCP.
func WithToolPrefix(prefix string) ServerOption {
	return func(s *Server) {
		s.mapper = NewToolMapper(s.registry, prefix)
	}
}

// NewServer creates a new MCP server.
func NewServer(name, version string, opts ...ServerOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		name:     name,
		version:  version,
		registry: tools.NewRegistry(),
		ctx:      ctx,
		cancel:   cancel,
	}

	for _, opt := range opts {
		opt(s)
	}

	// Create mapper after options are applied (in case registry was set)
	if s.mapper == nil {
		s.mapper = NewToolMapper(s.registry, "")
	}

	// Create handler
	s.handler = NewMethodHandler(s)

	return s
}

// Name returns the server name.
func (s *Server) Name() string {
	return s.name
}

// Version returns the server version.
func (s *Server) Version() string {
	return s.version
}

// Registry returns the tool registry.
func (s *Server) Registry() *tools.Registry {
	return s.registry
}

// IsInitialized returns whether the server has been initialized by a client.
func (s *Server) IsInitialized() bool {
	s.initMu.RLock()
	defer s.initMu.RUnlock()
	return s.initialized
}

// setInitialized marks the server as initialized.
func (s *Server) setInitialized(v bool) {
	s.initMu.Lock()
	defer s.initMu.Unlock()
	s.initialized = v
}

// Serve starts the server with the default stdio transport.
func (s *Server) Serve() error {
	t := transport.NewStdioServerTransport()
	return s.ServeTransport(t)
}

// ServeTransport starts the server with the specified transport.
func (s *Server) ServeTransport(t transport.Transport) error {
	s.transport = t
	s.wg.Add(1)
	go s.messageLoop()
	s.wg.Wait()
	return nil
}

// Close shuts down the server.
func (s *Server) Close() error {
	s.cancel()
	s.wg.Wait()
	if s.transport != nil {
		return s.transport.Close()
	}
	return nil
}

// messageLoop is the main message processing loop.
func (s *Server) messageLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Receive message
		data, err := s.transport.Receive(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // Context cancelled, normal shutdown
			}
			// Send error response for receive errors
			s.sendError(nil, protocol.NewParseError(err.Error()))
			continue
		}

		// Parse and handle message
		response := s.parseAndHandle(data)
		if response == nil {
			continue // Notification, no response needed
		}

		// Send response
		responseData, err := json.Marshal(response)
		if err != nil {
			s.sendError(response.ID, protocol.NewInternalError(err.Error()))
			continue
		}

		if err := s.transport.Send(s.ctx, responseData); err != nil {
			// Log error but continue
			continue
		}
	}
}

// parseAndHandle parses a message and handles it.
func (s *Server) parseAndHandle(data []byte) *protocol.Response {
	msg, err := protocol.ParseMessage(data)
	if err != nil {
		return protocol.NewErrorResponse(nil, protocol.NewParseError(err.Error()))
	}

	if msg.IsNotification() {
		notif := &protocol.Notification{
			Jsonrpc: msg.Jsonrpc,
			Method:  msg.Method,
			Params:  msg.Params,
		}
		s.handler.HandleNotification(s.ctx, notif)
		return nil
	}

	if msg.IsRequest() {
		req := &protocol.Request{
			Jsonrpc: msg.Jsonrpc,
			ID:      msg.ID,
			Method:  msg.Method,
			Params:  msg.Params,
		}
		return s.handler.HandleRequest(s.ctx, req)
	}

	// Unexpected message type
	return protocol.NewErrorResponse(nil, protocol.NewInvalidRequestError("unexpected message type"))
}

// sendError sends an error response.
func (s *Server) sendError(id any, rpcErr *protocol.RPCError) {
	response := protocol.NewErrorResponse(id, rpcErr)
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	_ = s.transport.Send(s.ctx, data)
}
