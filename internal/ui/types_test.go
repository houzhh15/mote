package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestComponent_JSONSerialization(t *testing.T) {
	comp := Component{
		Name:        "weather",
		Description: "Weather widget",
		File:        "components/weather.js",
		Version:     "1.0.0",
		Props:       map[string]any{"location": "default"},
	}

	data, err := json.Marshal(comp)
	if err != nil {
		t.Fatalf("failed to marshal Component: %v", err)
	}

	var decoded Component
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Component: %v", err)
	}

	if decoded.Name != comp.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, comp.Name)
	}
	if decoded.File != comp.File {
		t.Errorf("File mismatch: got %q, want %q", decoded.File, comp.File)
	}
}

func TestPage_JSONSerialization(t *testing.T) {
	page := Page{
		Name:        "chat",
		File:        "pages/Chat.js",
		Route:       "/chat",
		Description: "Chat page",
	}

	data, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("failed to marshal Page: %v", err)
	}

	var decoded Page
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Page: %v", err)
	}

	if decoded.Route != page.Route {
		t.Errorf("Route mismatch: got %q, want %q", decoded.Route, page.Route)
	}
}

func TestManifest_JSONSerialization(t *testing.T) {
	manifest := Manifest{
		Version: "1.0",
		Components: []Component{
			{Name: "comp1", File: "comp1.js"},
		},
		Pages: []Page{
			{Name: "page1", File: "page1.js", Route: "/page1"},
		},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal Manifest: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Manifest: %v", err)
	}

	if decoded.Version != manifest.Version {
		t.Errorf("Version mismatch: got %q, want %q", decoded.Version, manifest.Version)
	}
	if len(decoded.Components) != 1 {
		t.Errorf("Components count mismatch: got %d, want 1", len(decoded.Components))
	}
	if len(decoded.Pages) != 1 {
		t.Errorf("Pages count mismatch: got %d, want 1", len(decoded.Pages))
	}
}

func TestUIState_JSONSerialization(t *testing.T) {
	state := UIState{
		ActiveSession: "session-123",
		Theme:         "dark",
		SidebarOpen:   true,
		CurrentPage:   "chat",
		Custom:        map[string]any{"key": "value"},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal UIState: %v", err)
	}

	var decoded UIState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal UIState: %v", err)
	}

	if decoded.Theme != state.Theme {
		t.Errorf("Theme mismatch: got %q, want %q", decoded.Theme, state.Theme)
	}
	if decoded.SidebarOpen != state.SidebarOpen {
		t.Errorf("SidebarOpen mismatch: got %v, want %v", decoded.SidebarOpen, state.SidebarOpen)
	}
}

func TestUIState_OmitEmpty(t *testing.T) {
	state := UIState{
		SidebarOpen: false,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal UIState: %v", err)
	}

	str := string(data)
	if strings.Contains(str, "active_session") {
		t.Error("empty active_session should be omitted")
	}
	if strings.Contains(str, "theme") {
		t.Error("empty theme should be omitted")
	}
}

func TestSessionSummary_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	summary := SessionSummary{
		ID:           "sess-001",
		CreatedAt:    now,
		UpdatedAt:    now,
		MessageCount: 5,
		Preview:      "Hello...",
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal SessionSummary: %v", err)
	}

	var decoded SessionSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SessionSummary: %v", err)
	}

	if decoded.ID != summary.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, summary.ID)
	}
	if decoded.MessageCount != summary.MessageCount {
		t.Errorf("MessageCount mismatch: got %d, want %d", decoded.MessageCount, summary.MessageCount)
	}
}

func TestConfigView_JSONSerialization(t *testing.T) {
	config := ConfigView{
		Gateway: GatewayConfigView{Port: 8080, Host: "localhost"},
		Memory:  MemoryConfigView{Enabled: true},
		Cron:    CronConfigView{Enabled: false},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal ConfigView: %v", err)
	}

	var decoded ConfigView
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ConfigView: %v", err)
	}

	if decoded.Gateway.Port != config.Gateway.Port {
		t.Errorf("Gateway.Port mismatch: got %d, want %d", decoded.Gateway.Port, config.Gateway.Port)
	}
	if decoded.Memory.Enabled != config.Memory.Enabled {
		t.Errorf("Memory.Enabled mismatch: got %v, want %v", decoded.Memory.Enabled, config.Memory.Enabled)
	}
}

func TestErrorResponse_JSONSerialization(t *testing.T) {
	errResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "ui_component_not_found",
			Message: "Component 'weather' not found",
		},
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("failed to marshal ErrorResponse: %v", err)
	}

	var decoded ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ErrorResponse: %v", err)
	}

	if decoded.Error.Code != errResp.Error.Code {
		t.Errorf("Error.Code mismatch: got %q, want %q", decoded.Error.Code, errResp.Error.Code)
	}
}
