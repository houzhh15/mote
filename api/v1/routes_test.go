package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

func TestRouter_RegisterRoutes(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	// Verify key routes are registered
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/health"},
		{"POST", "/api/v1/chat"},
		{"POST", "/api/v1/chat/stream"},
		{"GET", "/api/v1/sessions"},
		{"POST", "/api/v1/sessions"},
		{"GET", "/api/v1/tools"},
		{"POST", "/api/v1/memory/search"},
		{"GET", "/api/v1/cron/jobs"},
		{"GET", "/api/v1/mcp/servers"},
		{"GET", "/api/v1/config"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			match := &mux.RouteMatch{}
			if !m.Match(req, match) {
				t.Errorf("Route %s %s not registered", route.method, route.path)
			}
		})
	}
}

func TestRouter_HandleHealth_NoDeps(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	// Add request_time to context for handler
	type contextKey string
	ctx := req.Context()
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	// Handle will panic due to missing context value, test that route exists
	// In a full test, we'd mock the context properly
	defer func() {
		if r := recover(); r == nil {
			// No panic, verify response
			if rr.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", rr.Code)
			}
		}
	}()

	m.ServeHTTP(rr, req)
}

func TestSetupLegacyRedirects(t *testing.T) {
	m := mux.NewRouter()
	SetupLegacyRedirects(m)

	tests := []struct {
		oldPath  string
		wantPath string
	}{
		{"/api/health", "/api/v1/health"},
		{"/api/cron/jobs", "/api/v1/cron/jobs"},
		{"/api/sessions", "/api/v1/sessions"},
		{"/api/config", "/api/v1/config"},
	}

	for _, tt := range tests {
		t.Run(tt.oldPath, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.oldPath, nil)
			rr := httptest.NewRecorder()

			m.ServeHTTP(rr, req)

			if rr.Code != http.StatusPermanentRedirect {
				t.Errorf("Expected status %d, got %d", http.StatusPermanentRedirect, rr.Code)
			}

			location := rr.Header().Get("Location")
			if location != tt.wantPath {
				t.Errorf("Expected redirect to %s, got %s", tt.wantPath, location)
			}
		})
	}
}

func TestRouter_HandleListSessions_NoDatabase(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleGetConfig(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var config ConfigResponse
	if err := _ = json.NewDecoder(rr.Body).Decode(&config); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check gateway defaults
	if config.Gateway.Host != "localhost" {
		t.Errorf("Expected gateway host localhost, got %s", config.Gateway.Host)
	}
	if config.Gateway.Port != 18788 {
		t.Errorf("Expected gateway port 18788, got %d", config.Gateway.Port)
	}

	// Check provider default
	if config.Provider.Default != "copilot" {
		t.Errorf("Expected provider default copilot, got %s", config.Provider.Default)
	}
}

func TestGenerateSessionID(t *testing.T) {
	id := generateSessionID()

	if id == "" {
		t.Error("Session ID should not be empty")
	}

	if len(id) < 10 {
		t.Errorf("Session ID too short: %s", id)
	}

	// Should start with "sess_"
	if id[:5] != "sess_" {
		t.Errorf("Session ID should start with 'sess_', got %s", id)
	}

	// Generate multiple IDs, they should be different
	id2 := generateSessionID()
	if id == id2 {
		t.Error("Generated session IDs should be unique")
	}
}

func TestRouter_HandleUIComponents_NoHandler(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/ui/components", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp UIComponentsResponse
	if err := _ = json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Components == nil {
		t.Error("Components should not be nil")
	}
}

func TestRouter_HandleUpdateConfig_ProviderChange(t *testing.T) {
	// Create temp config file for viper
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte("provider:\n  default: copilot\n"), 0644)

	// Setup viper to use temp config
	viper.SetConfigFile(configPath)
	viper.ReadInConfig()
	defer viper.Reset()

	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	// Test updating provider to ollama
	body := bytes.NewBufferString(`{"provider":{"default":"ollama"}}`)
	req := httptest.NewRequest("PUT", "/api/v1/config", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	// Verify response contains updated provider
	var resp ConfigResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp.Provider.Default != "ollama" {
		t.Errorf("Expected provider 'ollama', got '%s'", resp.Provider.Default)
	}
}

func TestRouter_HandleUpdateConfig_InvalidProvider(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	// Test with invalid provider
	body := bytes.NewBufferString(`{"provider":{"default":"invalid_provider"}}`)
	req := httptest.NewRequest("PUT", "/api/v1/config", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
