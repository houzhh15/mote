package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPServerTransport(t *testing.T) {
	transport := NewHTTPServerTransport(":0")
	if transport == nil {
		t.Fatal("Transport should not be nil")
	}
	if transport.addr != ":0" {
		t.Errorf("addr: got %q, want %q", transport.addr, ":0")
	}
}

func TestNewHTTPClientTransport(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer token"}
	transport := NewHTTPClientTransport("http://localhost:8080", headers)

	if transport == nil {
		t.Fatal("Transport should not be nil")
	}
	if transport.endpoint != "http://localhost:8080" {
		t.Errorf("endpoint: got %q, want %q", transport.endpoint, "http://localhost:8080")
	}
}

func TestHTTPClientTransport_EndpointTrailingSlash(t *testing.T) {
	transport := NewHTTPClientTransport("http://localhost:8080/", nil)
	if transport.endpoint != "http://localhost:8080" {
		t.Errorf("endpoint should strip trailing slash: got %q", transport.endpoint)
	}
}

func TestHTTPClientTransport_NotStarted(t *testing.T) {
	transport := NewHTTPClientTransport("http://localhost:8080", nil)

	ctx := context.Background()

	err := transport.Send(ctx, []byte("test"))
	if err != ErrNotStarted {
		t.Errorf("Send before start: got %v, want ErrNotStarted", err)
	}

	_, err = transport.Receive(ctx)
	if err != ErrNotStarted {
		t.Errorf("Receive before start: got %v, want ErrNotStarted", err)
	}
}

func TestHTTPClientTransport_Close(t *testing.T) {
	transport := NewHTTPClientTransport("http://localhost:8080", nil)

	// Close without starting should not panic
	err := transport.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	ctx := context.Background()
	err = transport.Send(ctx, []byte("test"))
	if err != ErrTransportClosed {
		t.Errorf("Send after close: got %v, want ErrTransportClosed", err)
	}
}

func TestHTTPClientTransport_DoubleClose(t *testing.T) {
	transport := NewHTTPClientTransport("http://localhost:8080", nil)

	err := transport.Close()
	if err != nil {
		t.Errorf("First close failed: %v", err)
	}

	err = transport.Close()
	if err != nil {
		t.Errorf("Second close should not fail: %v", err)
	}
}

// TestHTTPServerTransport_HandleMCP tests the MCP endpoint handler.
func TestHTTPServerTransport_HandleMCP(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	// Create a test session
	sessionCh := make(chan []byte, 1)
	transport.sessionsMu.Lock()
	transport.sessions["test-session"] = sessionCh
	transport.sessionsMu.Unlock()

	// Start a goroutine to receive and respond
	go func() {
		<-transport.incoming
		sessionCh <- []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	}()

	// Create test request
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("X-Session-ID", "test-session")
	w := httptest.NewRecorder()

	transport.handleMCP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHTTPServerTransport_HandleMCP_MethodNotAllowed(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()

	transport.handleMCP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Status code: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestHTTPServerTransport_HandleMCP_MissingSessionID(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	transport.handleMCP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Status code: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHTTPServerTransport_HandleMCP_SessionNotFound(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("X-Session-ID", "nonexistent")
	w := httptest.NewRecorder()

	transport.handleMCP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Status code: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHTTPServerTransport_SendReceive(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	ctx := context.Background()

	// Send incoming message
	go func() {
		transport.incoming <- []byte(`{"test":"data"}`)
	}()

	// Receive
	data, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	var msg map[string]string
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg["test"] != "data" {
		t.Errorf("Message data: got %q, want %q", msg["test"], "data")
	}
}

func TestHTTPServerTransport_SendToSession(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	// Create session
	sessionCh := make(chan []byte, 1)
	transport.sessionsMu.Lock()
	transport.sessions["session1"] = sessionCh
	transport.sessionsMu.Unlock()

	ctx := context.Background()

	// Send to session
	err := transport.SendToSession(ctx, "session1", []byte("test"))
	if err != nil {
		t.Fatalf("SendToSession failed: %v", err)
	}

	// Verify message received
	select {
	case data := <-sessionCh:
		if string(data) != "test" {
			t.Errorf("Data: got %q, want %q", string(data), "test")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestHTTPServerTransport_SendToSession_NotFound(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	ctx := context.Background()
	err := transport.SendToSession(ctx, "nonexistent", []byte("test"))
	if err != ErrSessionNotFound {
		t.Errorf("SendToSession: got %v, want ErrSessionNotFound", err)
	}
}

func TestHTTPServerTransport_Close(t *testing.T) {
	transport := NewHTTPServerTransport(":0")

	err := transport.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Context should be cancelled
	select {
	case <-transport.ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled after Close")
	}
}

func TestErrSessionNotFound(t *testing.T) {
	if ErrSessionNotFound.Error() != "session not found" {
		t.Errorf("ErrSessionNotFound: got %q, want %q", ErrSessionNotFound.Error(), "session not found")
	}
}

func TestErrSSEConnectionClosed(t *testing.T) {
	if ErrSSEConnectionClosed.Error() != "SSE connection closed" {
		t.Errorf("ErrSSEConnectionClosed: got %q, want %q", ErrSSEConnectionClosed.Error(), "SSE connection closed")
	}
}
