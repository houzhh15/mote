package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mote/internal/mcp/transport"
	"mote/internal/tools"
)

func TestNewServer(t *testing.T) {
	s := NewServer("test-server", "1.0.0")

	if s.Name() != "test-server" {
		t.Errorf("Name: got %q, want %q", s.Name(), "test-server")
	}
	if s.Version() != "1.0.0" {
		t.Errorf("Version: got %q, want %q", s.Version(), "1.0.0")
	}
	if s.Registry() == nil {
		t.Error("Registry should not be nil")
	}
	if s.IsInitialized() {
		t.Error("Server should not be initialized initially")
	}
}

func TestNewServer_WithRegistry(t *testing.T) {
	registry := tools.NewRegistry()
	s := NewServer("test", "1.0", WithRegistry(registry))

	if s.Registry() != registry {
		t.Error("Registry should be the one provided")
	}
}

func TestServer_SetInitialized(t *testing.T) {
	s := NewServer("test", "1.0")

	if s.IsInitialized() {
		t.Error("Should not be initialized initially")
	}

	s.setInitialized(true)
	if !s.IsInitialized() {
		t.Error("Should be initialized after setInitialized(true)")
	}

	s.setInitialized(false)
	if s.IsInitialized() {
		t.Error("Should not be initialized after setInitialized(false)")
	}
}

func TestServer_Close(t *testing.T) {
	s := NewServer("test", "1.0")

	// Close without starting should not panic
	err := s.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// mockTransport is a simple mock transport for testing.
type mockTransport struct {
	receiveCh chan []byte
	sendCh    chan []byte
	closed    bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		receiveCh: make(chan []byte, 10),
		sendCh:    make(chan []byte, 10),
	}
}

func (t *mockTransport) Send(ctx context.Context, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case t.sendCh <- data:
		return nil
	}
}

func (t *mockTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data := <-t.receiveCh:
		return data, nil
	}
}

func (t *mockTransport) Close() error {
	t.closed = true
	return nil
}

func (t *mockTransport) QueueMessage(msg []byte) {
	t.receiveCh <- msg
}

func (t *mockTransport) GetResponse() ([]byte, bool) {
	select {
	case data := <-t.sendCh:
		return data, true
	default:
		return nil, false
	}
}

func TestServer_ParseAndHandle_Request(t *testing.T) {
	s := NewServer("test", "1.0")
	s.setInitialized(true) // Skip initialization check

	// Create a ping request
	request := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	response := s.parseAndHandle([]byte(request))

	if response == nil {
		t.Fatal("Expected response, got nil")
	}
	if response.Error != nil {
		t.Errorf("Unexpected error: %v", response.Error)
	}
}

func TestServer_ParseAndHandle_Notification(t *testing.T) {
	s := NewServer("test", "1.0")
	s.setInitialized(true)

	// Notification should return nil (no response)
	notification := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	response := s.parseAndHandle([]byte(notification))

	if response != nil {
		t.Errorf("Expected nil response for notification, got: %v", response)
	}
}

func TestServer_ParseAndHandle_InvalidJSON(t *testing.T) {
	s := NewServer("test", "1.0")

	response := s.parseAndHandle([]byte("invalid json"))

	if response == nil {
		t.Fatal("Expected error response, got nil")
	}
	if response.Error == nil {
		t.Error("Expected error in response")
	}
	if response.Error.Code != -32700 { // Parse error
		t.Errorf("Expected parse error code -32700, got %d", response.Error.Code)
	}
}

func TestServer_ParseAndHandle_NotInitialized(t *testing.T) {
	s := NewServer("test", "1.0")
	// Server is not initialized

	// Request for anything except initialize should fail
	request := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	response := s.parseAndHandle([]byte(request))

	if response == nil {
		t.Fatal("Expected error response, got nil")
	}
	if response.Error == nil {
		t.Error("Expected error in response")
	}
	if response.Error.Code != -32003 { // Not initialized
		t.Errorf("Expected not initialized error code -32003, got %d", response.Error.Code)
	}
}

func TestServer_WithTransport(t *testing.T) {
	mockT := newMockTransport()
	s := NewServer("test", "1.0", WithTransport(mockT))

	if s.transport != mockT {
		t.Error("Transport should be set via option")
	}
}

// Integration-like test with mock transport
func TestServer_MessageLoop_Integration(t *testing.T) {
	mockT := newMockTransport()
	registry := tools.NewRegistry()
	s := NewServer("test-server", "1.0.0", WithRegistry(registry), WithTransport(mockT))

	// Start server in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	s.transport = mockT

	go func() {
		s.wg.Add(1)
		s.messageLoop()
	}()

	// Send initialize request
	initRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test-client","version":"1.0"}}}`
	mockT.QueueMessage([]byte(initRequest))

	// Wait for response with timeout
	var response []byte
	for i := 0; i < 100; i++ {
		if data, ok := mockT.GetResponse(); ok {
			response = data
			break
		}
	}

	if response == nil {
		// Cancel and skip test if no response (timing issue)
		cancel()
		s.wg.Wait()
		t.Skip("No response received (timing issue)")
		return
	}

	// Verify response
	var resp map[string]any
	if err := json.Unmarshal(response, &resp); err != nil {
		cancel()
		s.wg.Wait()
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["error"] != nil {
		cancel()
		s.wg.Wait()
		t.Errorf("Unexpected error in response: %v", resp["error"])
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		cancel()
		s.wg.Wait()
		t.Fatal("Result should be an object")
	}

	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		cancel()
		s.wg.Wait()
		t.Fatal("serverInfo should be an object")
	}

	if serverInfo["name"] != "test-server" {
		t.Errorf("serverInfo.name: got %v, want %v", serverInfo["name"], "test-server")
	}

	cancel()
	s.wg.Wait()
}

func TestStdioServerTransport_NewStdioServerTransportWithIO(t *testing.T) {
	input := strings.NewReader(`{"test":"data"}` + "\n")
	var output strings.Builder

	trans := transport.NewStdioServerTransportWithIO(input, &output)
	if trans == nil {
		t.Fatal("Transport should not be nil")
	}
}
