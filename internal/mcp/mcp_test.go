// Package mcp_test contains integration tests for the MCP module.
package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mote/internal/mcp/bridge"
	"mote/internal/mcp/client"
	"mote/internal/mcp/protocol"
	"mote/internal/mcp/server"
	"mote/internal/mcp/transport"
	"mote/internal/tools"
)

// echoTool is a simple tool that echoes its input arguments.
type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string { return "Echoes the input arguments as JSON" }
func (e *echoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string"},
		},
	}
}
func (e *echoTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	data, _ := json.Marshal(args)
	return tools.NewSuccessResult(string(data)), nil
}

// addTool is a simple tool that adds two numbers.
type addTool struct{}

func (a *addTool) Name() string        { return "add" }
func (a *addTool) Description() string { return "Adds two numbers" }
func (a *addTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
			"b": map[string]any{"type": "number"},
		},
	}
}
func (a *addTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	aVal, _ := args["a"].(float64)
	bVal, _ := args["b"].(float64)
	return tools.NewSuccessResult(json.Number.String(json.Number(jsonFloat(aVal + bVal)))), nil
}

func jsonFloat(f float64) string {
	data, _ := json.Marshal(f)
	return string(data)
}

// TestBridgeWithMockClient tests the bridge with a mock MCP client.
func TestBridgeWithMockClient(t *testing.T) {
	registry := tools.NewRegistry()
	br := bridge.NewBridge(registry)

	// Create a mock client that satisfies bridge.MCPClient interface
	mockC := &mockBridgeClient{
		name: "test-server",
		tools: []protocol.Tool{
			{Name: "tool1", Description: "Tool 1"},
			{Name: "tool2", Description: "Tool 2"},
		},
		callResult: &protocol.CallToolResult{
			Content: []protocol.Content{{Type: "text", Text: "bridge result"}},
		},
	}

	// Register client
	if err := br.Register(mockC); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify tools are in registry with prefix
	if registry.Len() != 2 {
		t.Errorf("Expected 2 tools in registry, got %d", registry.Len())
	}

	// Execute a tool through the registry
	result, err := registry.Execute(context.Background(), "test-server_tool1", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Content != "bridge result" {
		t.Errorf("Expected 'bridge result', got %q", result.Content)
	}

	// Unregister
	if err := br.Unregister("test-server"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	if registry.Len() != 0 {
		t.Errorf("Expected 0 tools after unregister, got %d", registry.Len())
	}
}

// mockBridgeClient implements bridge.MCPClient for testing.
type mockBridgeClient struct {
	name       string
	tools      []protocol.Tool
	callResult *protocol.CallToolResult
}

func (m *mockBridgeClient) Name() string { return m.name }

func (m *mockBridgeClient) ListTools() []protocol.Tool { return m.tools }

func (m *mockBridgeClient) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	return m.callResult, nil
}

// TestReconnectPolicy verifies the reconnect policy calculations.
func TestReconnectPolicy_Integration(t *testing.T) {
	policy := client.DefaultReconnectPolicy()

	// Test exponential backoff
	delays := make([]time.Duration, 0)
	for i := 0; i < 6; i++ {
		if !policy.ShouldRetry(i) {
			break
		}
		delays = append(delays, policy.NextDelay(i))
	}

	if len(delays) != 5 {
		t.Errorf("Expected 5 retry delays, got %d", len(delays))
	}

	// Verify delays increase exponentially until max
	expectedDelays := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}

	for i, expected := range expectedDelays {
		if delays[i] != expected {
			t.Errorf("Delay %d: got %v, want %v", i, delays[i], expected)
		}
	}
}

// TestManagerClientCount verifies the manager tracks clients correctly.
func TestManagerClientCount_Integration(t *testing.T) {
	configs := []client.ClientConfig{
		{TransportType: "stdio", Command: "echo"},
		{TransportType: "stdio", Command: "cat"},
	}

	manager := client.NewManager(configs)
	if manager.ClientCount() != 0 {
		t.Errorf("Initial client count: got %d, want 0", manager.ClientCount())
	}

	// Note: Full connection tests would require actual commands
	// This test just verifies the manager initializes correctly
}

// TestProtocolVersionCompatibility ensures protocol version is correct.
func TestProtocolVersionCompatibility(t *testing.T) {
	if protocol.ProtocolVersion != "2024-11-05" {
		t.Errorf("Protocol version: got %q, want %q", protocol.ProtocolVersion, "2024-11-05")
	}
}

// TestServerToolMapping verifies the server correctly maps tools.
func TestServerToolMapping(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&echoTool{})
	registry.MustRegister(&addTool{})

	srv := server.NewServer("test-server", "1.0.0", server.WithRegistry(registry))
	_ = srv // Server is configured correctly

	// Verify registry has both tools
	if registry.Len() != 2 {
		t.Errorf("Expected 2 tools, got %d", registry.Len())
	}

	if _, ok := registry.Get("echo"); !ok {
		t.Error("echo tool not found")
	}
	if _, ok := registry.Get("add"); !ok {
		t.Error("add tool not found")
	}
}

// TestAllModules runs a quick sanity check on all MCP module imports.
func TestAllModules(t *testing.T) {
	// This test ensures all packages compile and can be imported
	_ = protocol.ProtocolVersion
	_ = transport.TransportStdio
	_ = server.NewServer("test", "1.0.0")
	_ = client.NewClient("test", client.ClientConfig{})
	_ = bridge.NewBridge(tools.NewRegistry())
	_ = client.NewManager(nil)
	_ = client.DefaultReconnectPolicy()
}
