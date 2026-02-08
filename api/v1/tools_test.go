package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestRouter_HandleListTools_NoRegistry(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/tools", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp ToolsListResponse
	if err := _ = json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Tools == nil {
		t.Error("Tools should not be nil")
	}
	if resp.Count != 0 {
		t.Errorf("Expected count 0, got %d", resp.Count)
	}
}

func TestRouter_HandleGetTool_NoRegistry(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/tools/test_tool", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestRouter_HandleExecuteTool_NoRegistry(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	body := ToolExecuteRequest{
		Params: map[string]any{"key": "value"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/tools/test_tool/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestRouter_HandleExecuteTool_InvalidJSON(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	router.RegisterRoutes(m)

	// First we need to handle the case where tools is nil
	req := httptest.NewRequest("POST", "/api/v1/tools/test_tool/execute", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	// Without a registry, it returns 404 before checking JSON
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestToolInfo_Structure(t *testing.T) {
	tool := ToolInfo{
		Name:        "test_tool",
		Description: "A test tool",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
		Type: "builtin",
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"name", "description", "schema", "type"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestToolExecuteResponse_Success(t *testing.T) {
	resp := ToolExecuteResponse{
		Success: true,
		Result:  "operation completed",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result ToolExecuteResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}
	if result.Error != "" {
		t.Error("Expected error to be empty for successful response")
	}
}

func TestToolExecuteResponse_Error(t *testing.T) {
	resp := ToolExecuteResponse{
		Success: false,
		Error:   "tool execution failed",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result ToolExecuteResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.Success {
		t.Error("Expected success to be false")
	}
	if result.Error == "" {
		t.Error("Expected error message for failed response")
	}
}

func TestGetToolType_Default(t *testing.T) {
	// Test with a type that doesn't implement toolTyper
	result := getToolType(struct{}{})
	if result != "builtin" {
		t.Errorf("Expected 'builtin', got %q", result)
	}
}
