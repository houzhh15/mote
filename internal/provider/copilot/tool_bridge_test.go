package copilot

import (
	"context"
	"fmt"
	"testing"
)

// mockToolRegistry is a mock implementation of ToolRegistryInterface for testing.
type mockToolRegistry struct {
	tools    []ToolInfo
	execFunc func(ctx context.Context, name string, args map[string]any) (ToolExecResult, error)
}

func (m *mockToolRegistry) ListToolInfo() []ToolInfo {
	return m.tools
}

func (m *mockToolRegistry) ExecuteTool(ctx context.Context, name string, args map[string]any) (ToolExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, name, args)
	}
	return ToolExecResult{Content: "ok"}, nil
}

func TestToolBridge_GetBridgeTools_ExcludesBuiltins(t *testing.T) {
	registry := &mockToolRegistry{
		tools: []ToolInfo{
			{Name: "shell", Description: "Run shell command"},
			{Name: "read_file", Description: "Read a file"},
			{Name: "write_file", Description: "Write a file"},
			{Name: "edit_file", Description: "Edit a file"},
			{Name: "list_dir", Description: "List directory"},
			{Name: "http", Description: "HTTP request"},
			{Name: "mcp_list", Description: "List MCP servers"},
		},
	}

	bridge := NewToolBridge(registry)
	tools := bridge.GetBridgeTools()

	// Should only include http and mcp_list (not the 5 builtins)
	if len(tools) != 2 {
		t.Fatalf("expected 2 bridge tools, got %d", len(tools))
	}

	// Verify mote_ prefix
	expectedNames := map[string]bool{"mote_http": true, "mote_mcp_list": true}
	for _, tool := range tools {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected tool name: %s", tool.Name)
		}
	}
}

func TestToolBridge_GetBridgeTools_AddsMotePrefix(t *testing.T) {
	registry := &mockToolRegistry{
		tools: []ToolInfo{
			{Name: "custom_tool", Description: "A custom tool", Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string"},
				},
			}},
		},
	}

	bridge := NewToolBridge(registry)
	tools := bridge.GetBridgeTools()

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "mote_custom_tool" {
		t.Errorf("expected name mote_custom_tool, got %s", tools[0].Name)
	}
	if tools[0].Description != "A custom tool" {
		t.Errorf("expected description 'A custom tool', got %s", tools[0].Description)
	}
	if tools[0].Parameters == nil {
		t.Error("expected parameters to be preserved")
	}
}

func TestToolBridge_GetBridgeTools_EmptyRegistry(t *testing.T) {
	registry := &mockToolRegistry{tools: []ToolInfo{}}
	bridge := NewToolBridge(registry)
	tools := bridge.GetBridgeTools()

	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
}

func TestToolBridge_GetBridgeTools_NilBridge(t *testing.T) {
	var bridge *ToolBridge
	tools := bridge.GetBridgeTools()
	if tools != nil {
		t.Errorf("expected nil, got %v", tools)
	}
}

func TestToolBridge_ExecuteTool_StripsMotePrefix(t *testing.T) {
	var calledName string
	var calledArgs map[string]any

	registry := &mockToolRegistry{
		execFunc: func(ctx context.Context, name string, args map[string]any) (ToolExecResult, error) {
			calledName = name
			calledArgs = args
			return ToolExecResult{Content: "result content"}, nil
		},
	}

	bridge := NewToolBridge(registry)
	result, err := bridge.ExecuteTool(context.Background(), "mote_http", map[string]any{"url": "https://example.com"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledName != "http" {
		t.Errorf("expected stripped name 'http', got '%s'", calledName)
	}
	if calledArgs["url"] != "https://example.com" {
		t.Errorf("expected url arg, got %v", calledArgs)
	}
	if result.ResultType != "success" {
		t.Errorf("expected success, got %s", result.ResultType)
	}
	if result.TextResultForLLM != "result content" {
		t.Errorf("expected 'result content', got '%s'", result.TextResultForLLM)
	}
}

func TestToolBridge_ExecuteTool_HandlesError(t *testing.T) {
	registry := &mockToolRegistry{
		execFunc: func(ctx context.Context, name string, args map[string]any) (ToolExecResult, error) {
			return ToolExecResult{}, fmt.Errorf("connection timeout")
		},
	}

	bridge := NewToolBridge(registry)
	result, err := bridge.ExecuteTool(context.Background(), "mote_http", nil)

	// ExecuteTool should NOT return error — it wraps errors in ToolResult
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultType != "failure" {
		t.Errorf("expected failure, got %s", result.ResultType)
	}
	if result.Error != "connection timeout" {
		t.Errorf("expected error message, got '%s'", result.Error)
	}
}

func TestToolBridge_ExecuteTool_IsErrorResult(t *testing.T) {
	registry := &mockToolRegistry{
		execFunc: func(ctx context.Context, name string, args map[string]any) (ToolExecResult, error) {
			return ToolExecResult{Content: "file not found", IsError: true}, nil
		},
	}

	bridge := NewToolBridge(registry)
	result, err := bridge.ExecuteTool(context.Background(), "mote_custom", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultType != "failure" {
		t.Errorf("expected failure for IsError=true, got %s", result.ResultType)
	}
}

func TestToolBridge_ExecuteTool_WithoutPrefix(t *testing.T) {
	var calledName string
	registry := &mockToolRegistry{
		execFunc: func(ctx context.Context, name string, args map[string]any) (ToolExecResult, error) {
			calledName = name
			return ToolExecResult{Content: "ok"}, nil
		},
	}

	bridge := NewToolBridge(registry)
	_, _ = bridge.ExecuteTool(context.Background(), "direct_tool", nil)

	// Without mote_ prefix, TrimPrefix is a no-op, so original name is used
	if calledName != "direct_tool" {
		t.Errorf("expected 'direct_tool', got '%s'", calledName)
	}
}

func TestToolBridge_CustomExcludes(t *testing.T) {
	registry := &mockToolRegistry{
		tools: []ToolInfo{
			{Name: "http", Description: "HTTP"},
			{Name: "mcp_list", Description: "MCP List"},
			{Name: "custom", Description: "Custom"},
		},
	}

	bridge := NewToolBridgeWithExcludes(registry, []string{"http"})
	tools := bridge.GetBridgeTools()

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool.Name == "mote_http" {
			t.Error("http should be excluded")
		}
	}
}

func TestProtocolMethodConstants(t *testing.T) {
	// Verify new protocol method names follow dot notation
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"SessionCreate", MethodSessionCreate, "session.create"},
		{"SessionSend", MethodSessionSend, "session.send"},
		{"SessionEvent", MethodSessionEvent, "session.event"},
		{"PermissionRequest", MethodPermissionRequest, "permission.request"},
		{"PermissionRespond", MethodPermissionRespond, "permission.response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}

func TestProtocolLegacyMethodConstants(t *testing.T) {
	// Verify legacy protocol method names follow slash notation
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"SessionNew", LegacyMethodSessionNew, "session/new"},
		{"SessionPrompt", LegacyMethodSessionPrompt, "session/prompt"},
		{"SessionUpdate", LegacyMethodSessionUpdate, "session/update"},
		{"RequestPermission", LegacyMethodRequestPermission, "session/request_permission"},
		{"PermissionResponse", LegacyMethodPermissionResponse, "session/permission_response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}

func TestACPClient_MethodName_NewProtocol(t *testing.T) {
	// Simulate new protocol client
	c := &ACPClient{useNewProtocol: true}
	method := c.methodName(MethodSessionCreate, LegacyMethodSessionNew)
	if method != "session.create" {
		t.Errorf("expected session.create, got %s", method)
	}
}

func TestACPClient_MethodName_LegacyProtocol(t *testing.T) {
	// Simulate legacy protocol client
	c := &ACPClient{useNewProtocol: false}
	method := c.methodName(MethodSessionCreate, LegacyMethodSessionNew)
	if method != "session/new" {
		t.Errorf("expected session/new, got %s", method)
	}
}
