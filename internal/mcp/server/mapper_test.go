package server

import (
	"context"
	"encoding/json"
	"testing"

	"mote/internal/mcp/protocol"
	"mote/internal/tools"
)

func TestToolMapper_ListTools(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{name: "tool1", description: "Tool 1"})
	registry.MustRegister(&mockTool{name: "tool2", description: "Tool 2"})

	mapper := NewToolMapper(registry, "")
	mcpTools := mapper.ListTools()

	if len(mcpTools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(mcpTools))
	}

	// Check that tool names are preserved
	names := make(map[string]bool)
	for _, tool := range mcpTools {
		names[tool.Name] = true
	}
	if !names["tool1"] || !names["tool2"] {
		t.Error("Tool names not preserved")
	}
}

func TestToolMapper_ListTools_WithPrefix(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{name: "tool1", description: "Tool 1"})

	mapper := NewToolMapper(registry, "mote")
	mcpTools := mapper.ListTools()

	if len(mcpTools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(mcpTools))
	}

	if mcpTools[0].Name != "mote_tool1" {
		t.Errorf("Tool name: got %q, want %q", mcpTools[0].Name, "mote_tool1")
	}
}

func TestToolMapper_GetTool(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{name: "echo", description: "Echo tool"})

	mapper := NewToolMapper(registry, "")

	tool, ok := mapper.GetTool("echo")
	if !ok {
		t.Fatal("Tool should be found")
	}
	if tool.Name() != "echo" {
		t.Errorf("Tool name: got %q, want %q", tool.Name(), "echo")
	}

	_, ok = mapper.GetTool("nonexistent")
	if ok {
		t.Error("Nonexistent tool should not be found")
	}
}

func TestToolMapper_GetTool_WithPrefix(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{name: "echo", description: "Echo tool"})

	mapper := NewToolMapper(registry, "mote")

	// Should find tool using prefixed name
	tool, ok := mapper.GetTool("mote_echo")
	if !ok {
		t.Fatal("Tool should be found with prefixed name")
	}
	if tool.Name() != "echo" {
		t.Errorf("Tool name: got %q, want %q", tool.Name(), "echo")
	}

	// Should also find tool using internal name (for backward compatibility)
	_, ok = mapper.GetTool("echo")
	if !ok {
		t.Fatal("Tool should be found with internal name")
	}
}

func TestToolMapper_Execute(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{
		name:        "echo",
		description: "Echo tool",
		response:    "Hello!",
	})

	mapper := NewToolMapper(registry, "")

	result, err := mapper.Execute(context.Background(), "echo", map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Hello!" {
		t.Errorf("Content text: got %q, want %q", result.Content[0].Text, "Hello!")
	}
	if result.IsError {
		t.Error("Result should not be an error")
	}
}

func TestToolMapper_Execute_NotFound(t *testing.T) {
	registry := tools.NewRegistry()
	mapper := NewToolMapper(registry, "")

	_, err := mapper.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("Expected error for nonexistent tool")
	}

	rpcErr, ok := err.(*protocol.RPCError)
	if !ok {
		t.Fatalf("Expected RPCError, got %T", err)
	}
	if rpcErr.Code != protocol.ErrCodeToolNotFound {
		t.Errorf("Error code: got %d, want %d", rpcErr.Code, protocol.ErrCodeToolNotFound)
	}
}

func TestToolMapper_Execute_WithPrefix(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&mockTool{
		name:        "echo",
		description: "Echo tool",
		response:    "Prefixed!",
	})

	mapper := NewToolMapper(registry, "mote")

	// Execute using prefixed name
	result, err := mapper.Execute(context.Background(), "mote_echo", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Content[0].Text != "Prefixed!" {
		t.Errorf("Content text: got %q, want %q", result.Content[0].Text, "Prefixed!")
	}
}

func TestToolMapper_ToMCPTool(t *testing.T) {
	tool := &mockTool{
		name:        "test",
		description: "Test tool",
	}

	mapper := NewToolMapper(tools.NewRegistry(), "")
	mcpTool := mapper.ToMCPTool(tool)

	if mcpTool.Name != "test" {
		t.Errorf("Name: got %q, want %q", mcpTool.Name, "test")
	}
	if mcpTool.Description != "Test tool" {
		t.Errorf("Description: got %q, want %q", mcpTool.Description, "Test tool")
	}
	if mcpTool.InputSchema == nil {
		t.Error("InputSchema should not be nil")
	}

	// Parse InputSchema to verify content
	var schema map[string]any
	if err := json.Unmarshal(mcpTool.InputSchema, &schema); err != nil {
		t.Fatalf("Failed to parse InputSchema: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("InputSchema type: got %v, want %q", schema["type"], "object")
	}
}

func TestToolMapper_ToMCPResult(t *testing.T) {
	mapper := NewToolMapper(tools.NewRegistry(), "")

	// Test success result
	successResult := tools.NewSuccessResult("Success message")
	mcpResult := mapper.ToMCPResult(successResult)

	if len(mcpResult.Content) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(mcpResult.Content))
	}
	if mcpResult.Content[0].Type != "text" {
		t.Errorf("Content type: got %q, want %q", mcpResult.Content[0].Type, "text")
	}
	if mcpResult.Content[0].Text != "Success message" {
		t.Errorf("Content text: got %q, want %q", mcpResult.Content[0].Text, "Success message")
	}
	if mcpResult.IsError {
		t.Error("Result should not be an error")
	}

	// Test error result
	errorResult := tools.NewErrorResult("Error message")
	mcpResult = mapper.ToMCPResult(errorResult)

	if !mcpResult.IsError {
		t.Error("Result should be an error")
	}
	if mcpResult.Content[0].Text != "Error message" {
		t.Errorf("Content text: got %q, want %q", mcpResult.Content[0].Text, "Error message")
	}
}

func TestToolMapper_ToInternalName(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		mcpName  string
		expected string
	}{
		{"no prefix", "", "tool", "tool"},
		{"with prefix", "mote", "mote_tool", "tool"},
		{"prefix not matching", "mote", "other_tool", "other_tool"},
		{"prefix partial match", "mote", "motetool", "motetool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewToolMapper(tools.NewRegistry(), tt.prefix)
			result := mapper.toInternalName(tt.mcpName)
			if result != tt.expected {
				t.Errorf("toInternalName(%q): got %q, want %q", tt.mcpName, result, tt.expected)
			}
		})
	}
}

func TestToolMapper_ToMCPName(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		internalName string
		expected     string
	}{
		{"no prefix", "", "tool", "tool"},
		{"with prefix", "mote", "tool", "mote_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewToolMapper(tools.NewRegistry(), tt.prefix)
			result := mapper.toMCPName(tt.internalName)
			if result != tt.expected {
				t.Errorf("toMCPName(%q): got %q, want %q", tt.internalName, result, tt.expected)
			}
		})
	}
}
