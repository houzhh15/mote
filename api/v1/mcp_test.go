package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestRouter_HandleListMCPServers_NoClient(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/mcp/servers", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp MCPServersResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Servers == nil {
		t.Error("Servers should not be nil")
	}
	// Note: may return configured but disconnected servers from mcp_servers.json
	// The important thing is the API doesn't error
}

func TestRouter_HandleListMCPTools_NoClient(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/mcp/tools", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp MCPToolsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Tools == nil {
		t.Error("Tools should not be nil")
	}
	if len(resp.Tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(resp.Tools))
	}
}

func TestMCPServerInfo_Structure(t *testing.T) {
	server := MCPServerInfo{
		Name:      "test-server",
		Status:    "connected",
		Transport: "stdio",
		Tools:     []string{"tool1", "tool2"},
		Error:     "",
	}

	data, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"name", "status", "transport", "tools"}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestMCPServerInfo_WithError(t *testing.T) {
	server := MCPServerInfo{
		Name:      "test-server",
		Status:    "error",
		Transport: "stdio",
		Tools:     []string{},
		Error:     "connection failed",
	}

	data, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result MCPServerInfo
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("Expected error message for error status")
	}
}

func TestMCPToolInfo_Structure(t *testing.T) {
	tool := MCPToolInfo{
		Name:        "mcp_tool",
		Description: "An MCP tool",
		Server:      "test-server",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"name", "description", "server", "schema"}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestMCPServersResponse_EmptyServers(t *testing.T) {
	resp := MCPServersResponse{
		Servers: []MCPServerInfo{},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result MCPServersResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Servers == nil {
		t.Error("Servers should not be nil")
	}
	if len(result.Servers) != 0 {
		t.Errorf("Expected 0 servers, got %d", len(result.Servers))
	}
}

func TestMCPToolsResponse_EmptyTools(t *testing.T) {
	resp := MCPToolsResponse{
		Tools: []MCPToolInfo{},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result MCPToolsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Tools == nil {
		t.Error("Tools should not be nil")
	}
	if len(result.Tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(result.Tools))
	}
}
