// Package client implements the MCP client for connecting to external MCP servers.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"mote/internal/mcp/protocol"
	"mote/internal/mcp/transport"
)

// ConnectionState represents the state of the client connection.
type ConnectionState int

const (
	// StateDisconnected means the client is not connected.
	StateDisconnected ConnectionState = iota
	// StateConnecting means the client is in the process of connecting.
	StateConnecting
	// StateConnected means the client is connected and ready.
	StateConnected
	// StateError means the client encountered an error.
	StateError
)

// String returns a string representation of the connection state.
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for ConnectionState.
func (s ConnectionState) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// ClientConfig holds configuration for an MCP client.
type ClientConfig struct {
	// TransportType specifies the transport type ("stdio" or "http+sse").
	TransportType transport.TransportType

	// Command is the command to run for stdio transport.
	Command string
	// Args are the arguments for the command.
	Args []string
	// Env are environment variables for the subprocess.
	Env map[string]string
	// WorkDir is the working directory for the subprocess.
	WorkDir string

	// URL is the server URL for HTTP+SSE transport.
	URL string
	// Headers are HTTP headers for HTTP+SSE transport (e.g., Authorization).
	Headers map[string]string

	// Timeout is the connection timeout.
	Timeout time.Duration
}

// Client is an MCP client that connects to external MCP servers.
type Client struct {
	name   string
	config ClientConfig

	transport  transport.Transport
	serverInfo protocol.ServerInfo
	tools      []protocol.Tool
	prompts    []protocol.Prompt

	pending   map[int64]chan *protocol.Response
	pendingMu sync.Mutex
	nextID    int64

	state   ConnectionState
	stateMu sync.RWMutex
	lastErr error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewClient creates a new MCP client.
func NewClient(name string, config ClientConfig) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &Client{
		name:    name,
		config:  config,
		pending: make(map[int64]chan *protocol.Response),
		state:   StateDisconnected,
	}
}

// Name returns the client name.
func (c *Client) Name() string {
	return c.name
}

// State returns the current connection state.
func (c *Client) State() ConnectionState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// LastError returns the last error encountered.
func (c *Client) LastError() error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.lastErr
}

// ServerInfo returns the server info from the initialize response.
func (c *Client) ServerInfo() protocol.ServerInfo {
	return c.serverInfo
}

// Tools returns the list of tools available from the server.
func (c *Client) Tools() []protocol.Tool {
	return c.tools
}

// Prompts returns the list of prompts available from the server.
func (c *Client) Prompts() []protocol.Prompt {
	return c.prompts
}

// GetConfig returns the client configuration.
func (c *Client) GetConfig() ClientConfig {
	return c.config
}

// TransportType returns the transport type of this client ("stdio" or "http").
func (c *Client) TransportType() string {
	return string(c.config.TransportType)
}

func (c *Client) setState(state ConnectionState, err error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.state = state
	c.lastErr = err
}

// Connect establishes a connection to the MCP server.
func (c *Client) Connect(ctx context.Context) error {
	c.setState(StateConnecting, nil)

	// Create transport based on config
	var t transport.Transport
	switch c.config.TransportType {
	case transport.TransportStdio:
		stdioT := transport.NewStdioClientTransport(
			c.config.Command,
			c.config.Args,
			transport.WithEnv(c.config.Env),
			transport.WithWorkDir(c.config.WorkDir),
		)
		if err := stdioT.Start(); err != nil {
			c.setState(StateError, err)
			return fmt.Errorf("start transport: %w", err)
		}
		t = stdioT
	case transport.TransportHTTPSSE:
		httpT := transport.NewHTTPClientTransport(c.config.URL, c.config.Headers)
		if err := httpT.Start(); err != nil {
			c.setState(StateError, err)
			return fmt.Errorf("start HTTP transport: %w", err)
		}
		t = httpT
	case transport.TransportHTTP:
		httpT := transport.NewSimpleHTTPClientTransport(c.config.URL, c.config.Headers)
		if err := httpT.Start(); err != nil {
			c.setState(StateError, err)
			return fmt.Errorf("start HTTP transport: %w", err)
		}
		t = httpT
	default:
		err := fmt.Errorf("unknown transport type: %s", c.config.TransportType)
		c.setState(StateError, err)
		return err
	}

	c.transport = t
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Perform initialization
	if err := c.initialize(ctx); err != nil {
		c.Close()
		c.setState(StateError, err)
		return fmt.Errorf("initialize: %w", err)
	}

	// List available tools
	if err := c.refreshTools(ctx); err != nil {
		c.Close()
		c.setState(StateError, err)
		return fmt.Errorf("list tools: %w", err)
	}

	// List available prompts (optional - ignore errors for servers that don't support prompts)
	_ = c.refreshPrompts(ctx)

	c.setState(StateConnected, nil)
	return nil
}

// initialize performs the MCP initialization handshake.
func (c *Client) initialize(ctx context.Context) error {
	params := protocol.InitializeParams{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo: protocol.ClientInfo{
			Name:    c.name,
			Version: "1.0.0",
		},
		Capabilities: protocol.Capabilities{},
	}

	var result protocol.InitializeResult
	if err := c.call(ctx, protocol.MethodInitialize, params, &result); err != nil {
		return err
	}

	// Allow newer protocol versions - MCP is backward compatible
	// We only reject if the server version is older than what we support
	// For now, just log and continue with any version
	_ = result.ProtocolVersion // Server version (may be newer)

	c.serverInfo = result.ServerInfo

	// Send initialized notification
	notif, err := protocol.NewNotification(protocol.MethodInitialized, nil)
	if err != nil {
		return fmt.Errorf("create initialized notification: %w", err)
	}
	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal initialized notification: %w", err)
	}
	if err := c.transport.Send(ctx, data); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

// refreshTools retrieves the list of available tools from the server.
func (c *Client) refreshTools(ctx context.Context) error {
	var result protocol.ListToolsResult
	if err := c.call(ctx, protocol.MethodToolsList, nil, &result); err != nil {
		return err
	}
	c.tools = result.Tools
	return nil
}

// refreshPrompts retrieves the list of available prompts from the server.
func (c *Client) refreshPrompts(ctx context.Context) error {
	var result protocol.ListPromptsResult
	if err := c.call(ctx, protocol.MethodPromptsList, nil, &result); err != nil {
		return err
	}
	c.prompts = result.Prompts
	return nil
}

// ListTools returns the cached list of tools.
func (c *Client) ListTools() []protocol.Tool {
	return c.tools
}

// ListPrompts returns the cached list of prompts.
func (c *Client) ListPrompts() []protocol.Prompt {
	return c.prompts
}

// GetPrompt retrieves a specific prompt with arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*protocol.GetPromptResult, error) {
	params := protocol.GetPromptParams{
		Name:      name,
		Arguments: args,
	}

	var result protocol.GetPromptResult
	if err := c.call(ctx, protocol.MethodPromptsGet, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CallTool calls a tool on the remote MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	params := protocol.CallToolParams{
		Name:      name,
		Arguments: args,
	}

	var result protocol.CallToolResult
	if err := c.call(ctx, protocol.MethodToolsCall, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// call sends a request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params, result any) error {
	id := atomic.AddInt64(&c.nextID, 1)

	// Create request
	req, err := protocol.NewRequestWithID(id, method, params)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Create response channel
	respCh := make(chan *protocol.Response, 1)

	// Register pending request
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	// Cleanup on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if err := c.transport.Send(ctx, data); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.config.Timeout):
		return errors.New("request timeout")
	case resp := <-respCh:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && resp.Result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("unmarshal result: %w", err)
			}
		}
		return nil
	}
}

// receiveLoop reads responses from the transport and dispatches them.
func (c *Client) receiveLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		data, err := c.transport.Receive(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return // Normal shutdown
			}
			// Log error and continue or handle reconnection
			continue
		}

		// Parse message
		msg, err := protocol.ParseMessage(data)
		if err != nil {
			continue
		}

		// Handle response
		if msg.IsResponse() {
			c.handleResponse(msg)
		}
		// Ignore notifications and other messages for now
	}
}

// handleResponse dispatches a response to the waiting caller.
func (c *Client) handleResponse(msg *protocol.Message) {
	// Get request ID
	id := protocol.GetRequestID(msg.ID)
	if id == 0 {
		return
	}

	// Find and notify waiting caller
	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	c.pendingMu.Unlock()

	if ok {
		resp := &protocol.Response{
			Jsonrpc: msg.Jsonrpc,
			ID:      msg.ID,
			Result:  msg.Result,
			Error:   msg.Error,
		}
		select {
		case ch <- resp:
		default:
			// Channel full or closed, drop response
		}
	}
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()

	var err error
	if c.transport != nil {
		err = c.transport.Close()
	}

	c.setState(StateDisconnected, nil)
	return err
}

// Reconnect attempts to reconnect the client using exponential backoff.
// It will try up to maxAttempts times, with increasing delays between attempts.
func (c *Client) Reconnect(ctx context.Context) error {
	return c.ReconnectWithOptions(ctx, ReconnectOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
	})
}

// ReconnectOptions configures the reconnection behavior.
type ReconnectOptions struct {
	MaxAttempts    int           // Maximum number of reconnection attempts
	InitialBackoff time.Duration // Initial backoff duration
	MaxBackoff     time.Duration // Maximum backoff duration
}

// ReconnectWithOptions attempts to reconnect with custom options.
func (c *Client) ReconnectWithOptions(ctx context.Context, opts ReconnectOptions) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 5
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 1 * time.Second
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 30 * time.Second
	}

	var lastErr error
	backoff := opts.InitialBackoff

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		// Close existing connection
		if c.transport != nil {
			c.Close()
		}

		// Wait before reconnecting (except for first attempt)
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			// Exponential backoff
			backoff *= 2
			if backoff > opts.MaxBackoff {
				backoff = opts.MaxBackoff
			}
		}

		c.setState(StateConnecting, nil)

		// Attempt to connect
		err := c.Connect(ctx)
		if err == nil {
			return nil // Success
		}

		lastErr = err
	}

	c.setState(StateError, lastErr)
	return fmt.Errorf("max reconnect attempts (%d) exceeded: %w", opts.MaxAttempts, lastErr)
}

// IsConnected returns true if the client is currently connected.
func (c *Client) IsConnected() bool {
	return c.State() == StateConnected
}

// CallToolWithReconnect calls a tool, attempting to reconnect if the connection is lost.
func (c *Client) CallToolWithReconnect(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	// First, try the call directly
	result, err := c.CallTool(ctx, name, args)
	if err == nil {
		return result, nil
	}

	// Check if it's a connection error
	if !isConnectionError(err) {
		return nil, err
	}

	// Try to reconnect
	if reconnErr := c.Reconnect(ctx); reconnErr != nil {
		return nil, fmt.Errorf("tool call failed, reconnect failed: %w (original: %v)", reconnErr, err)
	}

	// Retry the call after reconnection
	return c.CallTool(ctx, name, args)
}

// isConnectionError checks if an error indicates a connection problem.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Common connection error patterns
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		containsAny(errStr, "connection", "closed", "EOF", "broken pipe", "reset by peer", "transport")
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// GetStateInfo returns detailed state information for the client.
type ClientStateInfo struct {
	Name       string          `json:"name"`
	Status     ConnectionState `json:"status"`
	ToolsCount int             `json:"tools_count"`
	LastError  string          `json:"last_error,omitempty"`
}

// GetStateInfo returns the current state information.
func (c *Client) GetStateInfo() ClientStateInfo {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	info := ClientStateInfo{
		Name:       c.name,
		Status:     c.state,
		ToolsCount: len(c.tools),
	}
	if c.lastErr != nil {
		info.LastError = c.lastErr.Error()
	}
	return info
}
