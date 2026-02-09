package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mote/internal/config"
	"mote/internal/gateway/websocket"
)

func TestNewServer(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.router == nil { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Error("router is nil")
	}

	if server.hub != hub { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Error("hub not set correctly")
	}

	// Check UI components are initialized
	if server.uiRegistry == nil { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Error("uiRegistry is nil")
	}

	if server.uiState == nil { //nolint:staticcheck // SA5011: server checked above
		t.Error("uiState is nil")
	}

	if server.uiHandler == nil { //nolint:staticcheck // SA5011: server checked above
		t.Error("uiHandler is nil")
	}
}

func TestServerHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", resp["status"])
	}
}

func TestServerShutdown(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 0, // Random port
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	// Start server in background
	go func() {
		_ = server.Start()
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	ctx := context.Background()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestServerRouter(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	if server.Router() == nil {
		t.Error("Router() returned nil")
	}
}

func TestServerHub(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	if server.Hub() != hub {
		t.Error("Hub() returned wrong hub")
	}
}

func TestServerUIRegistry(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	if server.UIRegistry() == nil {
		t.Error("UIRegistry() returned nil")
	}
}

func TestServerUIState(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	if server.UIState() == nil {
		t.Error("UIState() returned nil")
	}

	// Test default state
	state := server.UIState().Get()
	if state.Theme != "system" {
		t.Errorf("default theme = %q, want 'system'", state.Theme)
	}
}

func TestServerUIEndpoints(t *testing.T) {
	cfg := &config.Config{
		Version: "v1.0.0-test",
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
	}

	hub := websocket.NewHub()
	server := NewServer(cfg, hub, nil)

	// Test UI components endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/ui/components", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/ui/components status = %d, want 200", w.Code)
	}

	// Test UI state endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/ui/state", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/ui/state status = %d, want 200", w.Code)
	}
}
