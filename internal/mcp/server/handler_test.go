package server

import (
	"context"
	"encoding/json"
	"testing"

	"mote/internal/mcp/protocol"
	"mote/internal/tools"
)

func TestMethodHandler_HandleRequest_Initialize(t *testing.T) {
	s := NewServer("test-server", "1.0.0")

	params := protocol.InitializeParams{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo: protocol.ClientInfo{
			Name:    "test-client",
			Version: "1.0",
		},
	}
	paramsJSON, _ := json.Marshal(params)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodInitialize,
		Params:  paramsJSON,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	if !s.IsInitialized() {
		t.Error("Server should be initialized after initialize request")
	}

	// Verify result structure
	resultJSON, _ := json.Marshal(resp.Result)
	var result protocol.InitializeResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result.ProtocolVersion != protocol.ProtocolVersion {
		t.Errorf("ProtocolVersion: got %q, want %q", result.ProtocolVersion, protocol.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name: got %q, want %q", result.ServerInfo.Name, "test-server")
	}
}

func TestMethodHandler_HandleRequest_Initialize_WrongVersion(t *testing.T) {
	s := NewServer("test-server", "1.0.0")

	params := protocol.InitializeParams{
		ProtocolVersion: "1999-01-01", // Wrong version
		ClientInfo: protocol.ClientInfo{
			Name: "test-client",
		},
	}
	paramsJSON, _ := json.Marshal(params)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodInitialize,
		Params:  paramsJSON,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected error for wrong protocol version")
	}
	if resp.Error.Code != protocol.ErrCodeInvalidParams {
		t.Errorf("Error code: got %d, want %d", resp.Error.Code, protocol.ErrCodeInvalidParams)
	}
}

func TestMethodHandler_HandleRequest_NotInitialized(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	// Not calling initialize first

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodToolsList,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected not initialized error")
	}
	if resp.Error.Code != protocol.ErrCodeNotInitialized {
		t.Errorf("Error code: got %d, want %d", resp.Error.Code, protocol.ErrCodeNotInitialized)
	}
}

func TestMethodHandler_HandleRequest_MethodNotFound(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	s.setInitialized(true)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "unknown/method",
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected method not found error")
	}
	if resp.Error.Code != protocol.ErrCodeMethodNotFound {
		t.Errorf("Error code: got %d, want %d", resp.Error.Code, protocol.ErrCodeMethodNotFound)
	}
}

func TestMethodHandler_HandleRequest_ToolsList(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{name: "tool1", description: "Tool 1"})
	registry.MustRegister(&mockTool{name: "tool2", description: "Tool 2"})

	s := NewServer("test-server", "1.0.0", WithRegistry(registry))
	s.setInitialized(true)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodToolsList,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result protocol.ListToolsResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(result.Tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(result.Tools))
	}
}

func TestMethodHandler_HandleRequest_ToolsCall(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{
		name:        "echo",
		description: "Echo tool",
		response:    "Hello, World!",
	})

	s := NewServer("test-server", "1.0.0", WithRegistry(registry))
	s.setInitialized(true)

	params := protocol.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"message": "Hello"},
	}
	paramsJSON, _ := json.Marshal(params)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodToolsCall,
		Params:  paramsJSON,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result protocol.CallToolResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Hello, World!" {
		t.Errorf("Content text: got %q, want %q", result.Content[0].Text, "Hello, World!")
	}
}

func TestMethodHandler_HandleRequest_ToolsCall_NotFound(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	s.setInitialized(true)

	params := protocol.CallToolParams{
		Name: "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodToolsCall,
		Params:  paramsJSON,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected tool not found error")
	}
	if resp.Error.Code != protocol.ErrCodeToolNotFound {
		t.Errorf("Error code: got %d, want %d", resp.Error.Code, protocol.ErrCodeToolNotFound)
	}
}

func TestMethodHandler_HandleRequest_ToolsCall_MissingName(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	s.setInitialized(true)

	params := protocol.CallToolParams{
		Name: "", // Empty name
	}
	paramsJSON, _ := json.Marshal(params)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodToolsCall,
		Params:  paramsJSON,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected error for missing tool name")
	}
	if resp.Error.Code != protocol.ErrCodeInvalidParams {
		t.Errorf("Error code: got %d, want %d", resp.Error.Code, protocol.ErrCodeInvalidParams)
	}
}

func TestMethodHandler_HandleRequest_Ping(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	s.setInitialized(true)

	req := &protocol.Request{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  protocol.MethodPing,
	}

	resp := s.handler.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}
}

func TestMethodHandler_HandleNotification_Initialized(t *testing.T) {
	s := NewServer("test-server", "1.0.0")

	notif := &protocol.Notification{
		Jsonrpc: "2.0",
		Method:  protocol.MethodInitialized,
	}

	// Should not panic
	s.handler.HandleNotification(context.Background(), notif)
}

func TestMethodHandler_HandleNotification_Cancelled(t *testing.T) {
	s := NewServer("test-server", "1.0.0")

	notif := &protocol.Notification{
		Jsonrpc: "2.0",
		Method:  protocol.MethodCancelled,
	}

	// Should not panic
	s.handler.HandleNotification(context.Background(), notif)
}

// mockTool is a simple tool for testing.
type mockTool struct {
	name        string
	description string
	response    string
	shouldError bool
}

func (t *mockTool) Name() string {
	return t.name
}

func (t *mockTool) Description() string {
	return t.description
}

func (t *mockTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *mockTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	if t.shouldError {
		return tools.NewErrorResult("mock error"), nil
	}
	return tools.NewSuccessResult(t.response), nil
}
