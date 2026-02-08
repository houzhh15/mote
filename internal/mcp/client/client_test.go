package client

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"mote/internal/mcp/protocol"
	"mote/internal/mcp/transport"
)

func TestNewClient(t *testing.T) {
	config := ClientConfig{
		TransportType: transport.TransportStdio,
		Command:       "echo",
		Args:          []string{"test"},
	}

	c := NewClient("test-client", config)

	if c.Name() != "test-client" {
		t.Errorf("Name: got %q, want %q", c.Name(), "test-client")
	}
	if c.State() != StateDisconnected {
		t.Errorf("Initial state: got %v, want %v", c.State(), StateDisconnected)
	}
}

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
		{StateError, "error"},
		{ConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String(): got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClient_DefaultTimeout(t *testing.T) {
	config := ClientConfig{
		TransportType: transport.TransportStdio,
		Command:       "echo",
	}

	c := NewClient("test", config)
	if c.config.Timeout != 30*time.Second {
		t.Errorf("Default timeout: got %v, want %v", c.config.Timeout, 30*time.Second)
	}
}

func TestClient_CustomTimeout(t *testing.T) {
	config := ClientConfig{
		TransportType: transport.TransportStdio,
		Command:       "echo",
		Timeout:       5 * time.Second,
	}

	c := NewClient("test", config)
	if c.config.Timeout != 5*time.Second {
		t.Errorf("Custom timeout: got %v, want %v", c.config.Timeout, 5*time.Second)
	}
}

// mockClientTransport is a mock transport for testing the client.
type mockClientTransport struct {
	mu        sync.Mutex
	sendCh    chan []byte
	receiveCh chan []byte
	started   bool
	closed    bool
}

func newMockClientTransport() *mockClientTransport {
	return &mockClientTransport{
		sendCh:    make(chan []byte, 10),
		receiveCh: make(chan []byte, 10),
	}
}

func (t *mockClientTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	return nil
}

func (t *mockClientTransport) Send(ctx context.Context, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case t.sendCh <- data:
		return nil
	}
}

func (t *mockClientTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data := <-t.receiveCh:
		return data, nil
	}
}

func (t *mockClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	close(t.receiveCh)
	return nil
}

func (t *mockClientTransport) QueueResponse(data []byte) {
	t.receiveCh <- data
}

func (t *mockClientTransport) GetSent() ([]byte, bool) {
	select {
	case data := <-t.sendCh:
		return data, true
	case <-time.After(1 * time.Second):
		return nil, false
	}
}

func TestClient_Call_Success(t *testing.T) {
	mockT := newMockClientTransport()

	c := &Client{
		name:      "test",
		transport: mockT,
		pending:   make(map[int64]chan *protocol.Response),
		config: ClientConfig{
			Timeout: 5 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.cancel = cancel

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Start a goroutine to handle the request
	go func() {
		// Wait for request
		data, ok := mockT.GetSent()
		if !ok {
			return
		}

		// Parse request to get ID
		var req protocol.Request
		if err := json.Unmarshal(data, &req); err != nil {
			return
		}

		// Send response
		resp := &protocol.Response{
			Jsonrpc: "2.0",
			ID:      req.ID,
		}
		respData, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{"data": "test"},
		})
		mockT.QueueResponse(respData)
		_ = resp // avoid unused warning
	}()

	// Make call
	var result map[string]any
	err := c.call(context.Background(), "test/method", nil, &result)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if result["data"] != "test" {
		t.Errorf("result.data: got %v, want %v", result["data"], "test")
	}

	c.Close()
}

func TestClient_Call_Error(t *testing.T) {
	mockT := newMockClientTransport()

	c := &Client{
		name:      "test",
		transport: mockT,
		pending:   make(map[int64]chan *protocol.Response),
		config: ClientConfig{
			Timeout: 5 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.cancel = cancel

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Start a goroutine to handle the request
	go func() {
		// Wait for request
		data, ok := mockT.GetSent()
		if !ok {
			return
		}

		// Parse request to get ID
		var req protocol.Request
		if err := json.Unmarshal(data, &req); err != nil {
			return
		}

		// Send error response
		respData, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": "Method not found",
			},
		})
		mockT.QueueResponse(respData)
	}()

	// Make call
	err := c.call(context.Background(), "unknown/method", nil, nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	c.Close()
}

func TestClient_Call_Timeout(t *testing.T) {
	mockT := newMockClientTransport()

	c := &Client{
		name:      "test",
		transport: mockT,
		pending:   make(map[int64]chan *protocol.Response),
		config: ClientConfig{
			Timeout: 100 * time.Millisecond, // Short timeout
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	c.cancel = cancel

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Don't send any response, let it timeout
	err := c.call(context.Background(), "test/method", nil, nil)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
	if err.Error() != "request timeout" {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	c.Close()
}

func TestClient_Call_ContextCancelled(t *testing.T) {
	mockT := newMockClientTransport()

	c := &Client{
		name:      "test",
		transport: mockT,
		pending:   make(map[int64]chan *protocol.Response),
		config: ClientConfig{
			Timeout: 5 * time.Second,
		},
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	c.ctx = clientCtx
	c.cancel = clientCancel

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Create a cancellable context for the call
	callCtx, callCancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		callCancel()
	}()

	err := c.call(callCtx, "test/method", nil, nil)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	c.Close()
}

func TestClient_HandleResponse_UnknownID(t *testing.T) {
	c := &Client{
		pending: make(map[int64]chan *protocol.Response),
	}

	// Handle response with unknown ID should not panic
	msg := &protocol.Message{
		Jsonrpc: "2.0",
		ID:      int64(999),
		Result:  json.RawMessage(`{}`),
	}

	c.handleResponse(msg) // Should not panic
}

func TestClient_Close_NotConnected(t *testing.T) {
	c := NewClient("test", ClientConfig{})

	// Close should not panic when not connected
	err := c.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestClient_ServerInfo(t *testing.T) {
	c := &Client{
		serverInfo: protocol.ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	info := c.ServerInfo()
	if info.Name != "test-server" {
		t.Errorf("ServerInfo.Name: got %q, want %q", info.Name, "test-server")
	}
	if info.Version != "1.0.0" {
		t.Errorf("ServerInfo.Version: got %q, want %q", info.Version, "1.0.0")
	}
}

func TestClient_Tools(t *testing.T) {
	c := &Client{
		tools: []protocol.Tool{
			{Name: "tool1", Description: "Tool 1"},
			{Name: "tool2", Description: "Tool 2"},
		},
	}

	tools := c.Tools()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
}

func TestClient_Connect_UnknownTransport(t *testing.T) {
	config := ClientConfig{
		TransportType: "unknown",
	}

	c := NewClient("test", config)
	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("Expected error for unknown transport")
	}
	if c.State() != StateError {
		t.Errorf("State should be StateError, got %v", c.State())
	}
}

func TestClient_Connect_HTTPSSENotImplemented(t *testing.T) {
	config := ClientConfig{
		TransportType: transport.TransportHTTPSSE,
	}

	c := NewClient("test", config)
	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("Expected error for HTTP+SSE transport")
	}
	if c.State() != StateError {
		t.Errorf("State should be StateError, got %v", c.State())
	}
}
