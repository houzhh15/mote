// Package ipc provides IPC server for receiving commands from the tray.
package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Command action constants
const (
	ActionShowWindow     = "show_window"
	ActionHideWindow     = "hide_window"
	ActionRestartService = "restart_service"
	ActionQuit           = "quit"
	ActionGetStatus      = "get_status"
)

// Command represents a command from the tray
type Command struct {
	Action    string         `json:"action"`
	Timestamp int64          `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

// Response represents a response to the tray
type Response struct {
	Success bool           `json:"success"`
	Error   string         `json:"error"`
	Data    map[string]any `json:"data"`
}

// CommandHandler handles incoming commands
type CommandHandler func(cmd *Command) *Response

// Server is an IPC server for receiving commands from the tray
type Server struct {
	socketPath string
	listener   net.Listener
	handler    CommandHandler
	running    bool
	mu         sync.Mutex
}

// NewServer creates a new IPC server
func NewServer(handler CommandHandler) *Server {
	return &Server{
		socketPath: getSocketPath(),
		handler:    handler,
	}
}

// Start starts the IPC server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// Remove existing socket file
	if runtime.GOOS != "windows" {
		_ = os.Remove(s.socketPath)
	}

	// Ensure directory exists
	if runtime.GOOS != "windows" {
		dir := filepath.Dir(s.socketPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create socket directory: %w", err)
		}
	}

	// Create listener
	var err error
	if runtime.GOOS == "windows" {
		s.listener, err = net.Listen("tcp", "127.0.0.1:18789")
	} else {
		s.listener, err = net.Listen("unix", s.socketPath)
		// Set socket permissions
		if err == nil {
			_ = os.Chmod(s.socketPath, 0600)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	s.running = true

	// Accept connections in background
	go s.acceptLoop()

	return nil
}

// Stop stops the IPC server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
	}

	// Clean up socket file
	if runtime.GOOS != "windows" {
		_ = os.Remove(s.socketPath)
	}

	return nil
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				continue
			}
			return
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read command
	var cmd Command
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&cmd); err != nil {
		s.sendError(conn, "failed to decode command")
		return
	}

	// Handle command
	var resp *Response
	if s.handler != nil {
		resp = s.handler(&cmd)
	} else {
		resp = &Response{Success: true}
	}

	// Send response
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

// sendError sends an error response
func (s *Server) sendError(conn net.Conn, msg string) {
	resp := &Response{
		Success: false,
		Error:   msg,
	}
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

// getSocketPath returns the platform-specific socket path
func getSocketPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\mote-gui`
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".mote", "gui.sock")
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
