package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gorilla/mux"
)

func setupTestHandler() *Handler {
	registry := NewRegistry("")
	hub := newMockBroadcaster()
	state := NewStateManager(hub)
	static := NewStaticServer("", fstest.MapFS{
		"index.html": {Data: []byte("<html>test</html>")},
	})

	return NewHandler(registry, state, static, nil)
}

func TestHandler_HandleComponents(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ui/components", nil)
	rec := httptest.NewRecorder()

	h.HandleComponents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var resp ComponentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Empty registry should return empty list
	if resp.Components == nil {
		t.Error("Components should not be nil")
	}
}

func TestHandler_HandleGetState(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/ui/state", nil)
	rec := httptest.NewRecorder()

	h.HandleGetState(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var state UIState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check default state
	if state.Theme != "system" {
		t.Errorf("Theme = %q, want 'system'", state.Theme)
	}
	if state.CurrentPage != "chat" {
		t.Errorf("CurrentPage = %q, want 'chat'", state.CurrentPage)
	}
}

func TestHandler_HandleUpdateState(t *testing.T) {
	h := setupTestHandler()

	// Update state
	body := bytes.NewReader([]byte(`{"theme":"dark","current_page":"settings"}`))
	req := httptest.NewRequest(http.MethodPut, "/api/ui/state", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleUpdateState(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var state UIState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if state.Theme != "dark" {
		t.Errorf("Theme = %q, want 'dark'", state.Theme)
	}
	if state.CurrentPage != "settings" {
		t.Errorf("CurrentPage = %q, want 'settings'", state.CurrentPage)
	}
}

func TestHandler_HandleUpdateState_InvalidJSON(t *testing.T) {
	h := setupTestHandler()

	body := bytes.NewReader([]byte(`invalid json`))
	req := httptest.NewRequest(http.MethodPut, "/api/ui/state", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleUpdateState(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandler_HandleListSessions_NoDB(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()

	h.HandleListSessions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestHandler_HandleGetSession_NoDB(t *testing.T) {
	h := setupTestHandler()

	r := mux.NewRouter()
	r.HandleFunc("/api/sessions/{id}", h.HandleGetSession).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/test-id", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestHandler_HandleDeleteSession_NoDB(t *testing.T) {
	h := setupTestHandler()

	r := mux.NewRouter()
	r.HandleFunc("/api/sessions/{id}", h.HandleDeleteSession).Methods(http.MethodDelete)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/test-id", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestHandler_HandleGetConfig(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	h.HandleGetConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var config ConfigView
	if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if config.Gateway.Port != 8080 {
		t.Errorf("Gateway.Port = %d, want 8080", config.Gateway.Port)
	}
}

func TestHandler_HandleUpdateConfig_Disabled(t *testing.T) {
	h := setupTestHandler()

	body := bytes.NewReader([]byte(`{"gateway":{"port":9090}}`))
	req := httptest.NewRequest(http.MethodPut, "/api/config", body)
	rec := httptest.NewRecorder()

	h.HandleUpdateConfig(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	h := setupTestHandler()
	r := mux.NewRouter()
	h.RegisterRoutes(r)

	// Test that routes are registered
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/ui/components"},
		{http.MethodGet, "/api/ui/state"},
		{http.MethodPut, "/api/ui/state"},
		{http.MethodGet, "/api/sessions"},
		{http.MethodGet, "/api/sessions/test-id"},
		{http.MethodDelete, "/api/sessions/test-id"},
		{http.MethodGet, "/api/config"},
		{http.MethodPut, "/api/config"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/index.html"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			// Should not be 404 (route not found)
			// May be other status codes like 503 (no db) or 200
			if rec.Code == http.StatusNotFound && !isAPIPath(tt.path) {
				// Static files may return 404 if not found
			} else if rec.Code == http.StatusNotFound {
				t.Errorf("%s %s returned 404, route may not be registered", tt.method, tt.path)
			}
		})
	}
}

func isAPIPath(path string) bool {
	return len(path) >= 4 && path[:4] == "/api"
}

func TestNewHandler(t *testing.T) {
	registry := NewRegistry("")
	hub := newMockBroadcaster()
	state := NewStateManager(hub)
	static := NewStaticServer("", nil)

	h := NewHandler(registry, state, static, nil)

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if h.registry != registry {
		t.Error("registry not set correctly")
	}
	if h.state != state {
		t.Error("state not set correctly")
	}
	if h.static != static {
		t.Error("static not set correctly")
	}
}
