package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"mote/pkg/logger"
)

// Server is the IPC server that runs in the main process
type Server struct {
	listener   net.Listener
	socketPath string

	clients   map[ProcessRole]*clientConn
	clientsMu sync.RWMutex

	handlers   map[MessageType][]Handler
	handlersMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	onClientConnect    func(role ProcessRole)
	onClientDisconnect func(role ProcessRole)
}

// clientConn represents a connected client
type clientConn struct {
	conn    net.Conn
	role    ProcessRole
	encoder *Encoder
	mu      sync.Mutex
}

// ServerOption is a functional option for Server
type ServerOption func(*Server)

// WithOnClientConnect sets a callback for when a client connects
func WithOnClientConnect(fn func(role ProcessRole)) ServerOption {
	return func(s *Server) {
		s.onClientConnect = fn
	}
}

// WithOnClientDisconnect sets a callback for when a client disconnects
func WithOnClientDisconnect(fn func(role ProcessRole)) ServerOption {
	return func(s *Server) {
		s.onClientDisconnect = fn
	}
}

// NewServer creates a new IPC server
func NewServer(opts ...ServerOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		clients:  make(map[ProcessRole]*clientConn),
		handlers: make(map[MessageType][]Handler),
		ctx:      ctx,
		cancel:   cancel,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.RegisterHandler(MsgPing, HandlerFunc(s.handlePing))
	s.RegisterHandler(MsgPong, HandlerFunc(s.handlePong))

	return s
}

// Start starts the IPC server
func (s *Server) Start() error {
	socketPath := s.getSocketPath()
	s.socketPath = socketPath

	if runtime.GOOS != "windows" {
		os.Remove(socketPath)
	}

	listener, err := s.listen(socketPath)
	if err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	s.listener = listener

	if runtime.GOOS != "windows" {
		if err := os.Chmod(socketPath, 0600); err != nil {
			logger.Warnf("failed to set socket permissions: %v", err)
		}
	}

	logger.Infof("IPC server listening on %s", socketPath)

	go s.acceptLoop()

	return nil
}

// Stop stops the IPC server and disconnects all clients
func (s *Server) Stop() error {
	s.cancel()

	s.clientsMu.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clients = make(map[ProcessRole]*clientConn)
	s.clientsMu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}

	if runtime.GOOS != "windows" && s.socketPath != "" {
		os.Remove(s.socketPath)
	}

	logger.Info().Msg("IPC server stopped")
	return nil
}

// SocketPath returns the socket path being used
func (s *Server) SocketPath() string {
	return s.socketPath
}

// RegisterHandler registers a handler for a message type
func (s *Server) RegisterHandler(msgType MessageType, handler Handler) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.handlers[msgType] = append(s.handlers[msgType], handler)
}

// Send sends a message to a specific client
func (s *Server) Send(role ProcessRole, msg *Message) error {
	s.clientsMu.RLock()
	client, ok := s.clients[role]
	s.clientsMu.RUnlock()

	if !ok {
		return fmt.Errorf("client not found: %s", role)
	}

	return s.sendToClient(client, msg)
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(msg *Message) error {
	s.clientsMu.RLock()
	clients := make([]*clientConn, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	var lastErr error
	for _, client := range clients {
		if err := s.sendToClient(client, msg); err != nil {
			lastErr = err
			logger.Warnf("failed to send to client %s: %v", client.role, err)
		}
	}
	return lastErr
}

// IsClientConnected checks if a client with the given role is connected
func (s *Server) IsClientConnected(role ProcessRole) bool {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	_, ok := s.clients[role]
	return ok
}

// ConnectedClients returns a list of connected client roles
func (s *Server) ConnectedClients() []ProcessRole {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	roles := make([]ProcessRole, 0, len(s.clients))
	for role := range s.clients {
		roles = append(roles, role)
	}
	return roles
}

func (s *Server) getSocketPath() string {
	if runtime.GOOS == "windows" {
		return WindowsPipeName
	}
	return UnixSocketPath
}

func (s *Server) listen(path string) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return listenPipe(path)
	}
	return net.Listen("unix", path)
}

func (s *Server) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				logger.Warnf("failed to accept connection: %v", err)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	decoder := NewDecoder(conn)
	client := &clientConn{
		conn:    conn,
		encoder: NewEncoder(conn),
	}

	defer func() {
		conn.Close()
		if client.role != "" {
			s.clientsMu.Lock()
			delete(s.clients, client.role)
			s.clientsMu.Unlock()

			if s.onClientDisconnect != nil {
				s.onClientDisconnect(client.role)
			}
			logger.Infof("IPC client disconnected: %s", client.role)
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		msg, err := decoder.Decode()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if client.role != "" {
					pingMsg := NewMessage(MsgPing, RoleMain).WithTarget(client.role)
					if err := s.sendToClient(client, pingMsg); err != nil {
						logger.Warnf("failed to ping client %s: %v", client.role, err)
						return
					}
				}
				continue
			}
			if err.Error() != "EOF" {
				logger.Warnf("failed to decode message: %v", err)
			}
			return
		}

		if msg.Type == MsgRegister {
			s.handleRegister(client, msg)
		} else {
			s.handleMessage(msg)
		}
	}
}

func (s *Server) handleRegister(client *clientConn, msg *Message) {
	var payload RegisterPayload
	if err := msg.ParsePayload(&payload); err != nil {
		logger.Warnf("invalid register payload: %v", err)
		return
	}

	s.clientsMu.Lock()
	if existing, exists := s.clients[payload.Role]; exists {
		existing.conn.Close()
	}
	client.role = payload.Role
	s.clients[payload.Role] = client
	s.clientsMu.Unlock()

	logger.Infof("IPC client registered: %s (pid: %d)", payload.Role, payload.PID)

	if s.onClientConnect != nil {
		s.onClientConnect(payload.Role)
	}
}

func (s *Server) handleMessage(msg *Message) {
	s.handlersMu.RLock()
	handlers, ok := s.handlers[msg.Type]
	s.handlersMu.RUnlock()

	if !ok || len(handlers) == 0 {
		logger.Warnf("no handler for message type: %s", msg.Type)
		return
	}

	for _, handler := range handlers {
		if err := handler.Handle(msg); err != nil {
			logger.Warnf("handler error for %s: %v", msg.Type, err)
		}
	}
}

func (s *Server) handlePing(msg *Message) error {
	pong := NewMessage(MsgPong, RoleMain).
		WithTarget(msg.Source).
		WithReplyTo(msg.ID)

	return s.Send(msg.Source, pong)
}

// handlePong handles pong responses from clients (heartbeat acknowledgment).
func (s *Server) handlePong(_ *Message) error {
	// Pong is just an acknowledgment — no action needed.
	return nil
}

func (s *Server) sendToClient(client *clientConn, msg *Message) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	_ = client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	return client.encoder.Encode(msg)
}
