package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mote/internal/mcp/protocol"
	"mote/internal/tools"
)

// mockMCPClient is a mock implementation of MCPClient for testing.
type mockMCPClient struct {
	name      string
	tools     []protocol.Tool
	callFunc  func(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error)
	callCalls []callRecord
}

type callRecord struct {
	Name string
	Args map[string]any
}

func (m *mockMCPClient) Name() string {
	return m.name
}

func (m *mockMCPClient) ListTools() []protocol.Tool {
	return m.tools
}

func (m *mockMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	m.callCalls = append(m.callCalls, callRecord{Name: name, Args: args})
	if m.callFunc != nil {
		return m.callFunc(ctx, name, args)
	}
	return &protocol.CallToolResult{
		Content: []protocol.Content{{Type: "text", Text: "default result"}},
		IsError: false,
	}, nil
}

func TestNewBridge(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	if bridge == nil {
		t.Fatal("Bridge should not be nil")
	}
	if bridge.registry != registry {
		t.Error("Registry not set correctly")
	}
}

func TestBridge_Register(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name: "test-server",
		tools: []protocol.Tool{
			{Name: "tool1", Description: "Tool 1"},
			{Name: "tool2", Description: "Tool 2"},
		},
	}

	err := bridge.Register(client)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Check tools are registered in the registry
	if registry.Len() != 2 {
		t.Errorf("Expected 2 tools in registry, got %d", registry.Len())
	}

	// Check tool names are prefixed
	if _, ok := registry.Get("test-server_tool1"); !ok {
		t.Error("tool1 not found with prefix")
	}
	if _, ok := registry.Get("test-server_tool2"); !ok {
		t.Error("tool2 not found with prefix")
	}

	// Check adapters are tracked
	adapters := bridge.GetAdapters("test-server")
	if len(adapters) != 2 {
		t.Errorf("Expected 2 adapters, got %d", len(adapters))
	}
}

func TestBridge_Register_NilClient(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	err := bridge.Register(nil)
	if err == nil {
		t.Error("Should return error for nil client")
	}
}

func TestBridge_Register_EmptyName(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name: "",
		tools: []protocol.Tool{
			{Name: "tool1"},
		},
	}

	err := bridge.Register(client)
	if err == nil {
		t.Error("Should return error for empty client name")
	}
}

func TestBridge_Register_NoTools(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name:  "empty-server",
		tools: nil,
	}

	err := bridge.Register(client)
	if err != nil {
		t.Errorf("Register with no tools should not fail: %v", err)
	}
}

func TestBridge_Register_Duplicate(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name: "test-server",
		tools: []protocol.Tool{
			{Name: "tool1"},
		},
	}

	// First registration
	if err := bridge.Register(client); err != nil {
		t.Fatalf("First register failed: %v", err)
	}

	// Second registration should fail
	err := bridge.Register(client)
	if err == nil {
		t.Error("Should return error for duplicate registration")
	}
}

func TestBridge_Unregister(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name: "test-server",
		tools: []protocol.Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}

	_ = bridge.Register(client)

	err := bridge.Unregister("test-server")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Check tools are removed from registry
	if registry.Len() != 0 {
		t.Errorf("Expected 0 tools in registry, got %d", registry.Len())
	}

	// Check adapters are removed
	adapters := bridge.GetAdapters("test-server")
	if len(adapters) != 0 {
		t.Errorf("Expected 0 adapters, got %d", len(adapters))
	}
}

func TestBridge_Unregister_NotFound(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	err := bridge.Unregister("nonexistent")
	if err == nil {
		t.Error("Should return error for nonexistent client")
	}
}

func TestBridge_Refresh(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	client := &mockMCPClient{
		name: "test-server",
		tools: []protocol.Tool{
			{Name: "tool1"},
		},
	}

	_ = bridge.Register(client)

	// Update tools
	client.tools = []protocol.Tool{
		{Name: "tool2"},
		{Name: "tool3"},
	}

	err := bridge.Refresh(client)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Check new tools are registered
	if registry.Len() != 2 {
		t.Errorf("Expected 2 tools in registry, got %d", registry.Len())
	}

	if _, ok := registry.Get("test-server_tool2"); !ok {
		t.Error("tool2 not found")
	}
	if _, ok := registry.Get("test-server_tool3"); !ok {
		t.Error("tool3 not found")
	}
	if _, ok := registry.Get("test-server_tool1"); ok {
		t.Error("tool1 should be removed")
	}
}

func TestBridge_ListClients(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	_ = bridge.Register(&mockMCPClient{name: "server1", tools: []protocol.Tool{{Name: "t1"}}})
	_ = bridge.Register(&mockMCPClient{name: "server2", tools: []protocol.Tool{{Name: "t2"}}})

	clients := bridge.ListClients()
	if len(clients) != 2 {
		t.Errorf("Expected 2 clients, got %d", len(clients))
	}
}

func TestBridge_ToolCount(t *testing.T) {
	registry := tools.NewRegistry()
	bridge := NewBridge(registry)

	if bridge.ToolCount() != 0 {
		t.Errorf("Initial tool count: got %d, want 0", bridge.ToolCount())
	}

	_ = bridge.Register(&mockMCPClient{
		name: "server1",
		tools: []protocol.Tool{
			{Name: "t1"},
			{Name: "t2"},
		},
	})

	if bridge.ToolCount() != 2 {
		t.Errorf("Tool count after register: got %d, want 2", bridge.ToolCount())
	}
}

func TestToolAdapter_Name(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}
	tool := protocol.Tool{Name: "mytool", Description: "My Tool"}
	adapter := NewToolAdapter(client, tool)

	if adapter.Name() != "myserver_mytool" {
		t.Errorf("Name: got %q, want %q", adapter.Name(), "myserver_mytool")
	}
}

func TestToolAdapter_Description(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}
	tool := protocol.Tool{Name: "mytool", Description: "My Tool Description"}
	adapter := NewToolAdapter(client, tool)

	if adapter.Description() != "My Tool Description" {
		t.Errorf("Description: got %q, want %q", adapter.Description(), "My Tool Description")
	}
}

func TestToolAdapter_Parameters(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	schemaBytes, _ := json.Marshal(schema)

	tool := protocol.Tool{
		Name:        "mytool",
		InputSchema: schemaBytes,
	}
	adapter := NewToolAdapter(client, tool)

	params := adapter.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type: got %v, want 'object'", params["type"])
	}
}

func TestToolAdapter_Parameters_Empty(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}
	tool := protocol.Tool{Name: "mytool", InputSchema: nil}
	adapter := NewToolAdapter(client, tool)

	params := adapter.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestToolAdapter_OriginalName(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}
	tool := protocol.Tool{Name: "mytool"}
	adapter := NewToolAdapter(client, tool)

	if adapter.OriginalName() != "mytool" {
		t.Errorf("OriginalName: got %q, want %q", adapter.OriginalName(), "mytool")
	}
}

func TestToolAdapter_ClientName(t *testing.T) {
	client := &mockMCPClient{name: "myserver"}
	tool := protocol.Tool{Name: "mytool"}
	adapter := NewToolAdapter(client, tool)

	if adapter.ClientName() != "myserver" {
		t.Errorf("ClientName: got %q, want %q", adapter.ClientName(), "myserver")
	}
}

func TestToolAdapter_Execute_Success(t *testing.T) {
	client := &mockMCPClient{
		name: "myserver",
		callFunc: func(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
			return &protocol.CallToolResult{
				Content: []protocol.Content{{Type: "text", Text: "result text"}},
				IsError: false,
			}, nil
		},
	}
	tool := protocol.Tool{Name: "mytool"}
	adapter := NewToolAdapter(client, tool)

	result, err := adapter.Execute(context.Background(), map[string]any{"arg1": "value1"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.IsError {
		t.Error("Result should not be an error")
	}
	if result.Content != "result text" {
		t.Errorf("Content: got %q, want %q", result.Content, "result text")
	}

	// Verify call was made with original name (not prefixed)
	if len(client.callCalls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(client.callCalls))
	}
	if client.callCalls[0].Name != "mytool" {
		t.Errorf("Call name: got %q, want %q", client.callCalls[0].Name, "mytool")
	}
}

func TestToolAdapter_Execute_Error(t *testing.T) {
	client := &mockMCPClient{
		name: "myserver",
		callFunc: func(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
			return nil, errors.New("connection failed")
		},
	}
	tool := protocol.Tool{Name: "mytool"}
	adapter := NewToolAdapter(client, tool)

	result, err := adapter.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute should not return error directly: %v", err)
	}

	if !result.IsError {
		t.Error("Result should be an error")
	}
	if result.Content != "connection failed" {
		t.Errorf("Content: got %q, want %q", result.Content, "connection failed")
	}
}

func TestToolAdapter_Execute_ToolError(t *testing.T) {
	client := &mockMCPClient{
		name: "myserver",
		callFunc: func(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
			return &protocol.CallToolResult{
				Content: []protocol.Content{{Type: "text", Text: "tool error message"}},
				IsError: true,
			}, nil
		},
	}
	tool := protocol.Tool{Name: "mytool"}
	adapter := NewToolAdapter(client, tool)

	result, err := adapter.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}

	if !result.IsError {
		t.Error("Result should be an error")
	}
	if result.Content != "tool error message" {
		t.Errorf("Content: got %q, want %q", result.Content, "tool error message")
	}
}

func TestExtractContent_Text(t *testing.T) {
	contents := []protocol.Content{
		{Type: "text", Text: "hello world"},
	}
	result := extractContent(contents)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestExtractContent_Empty(t *testing.T) {
	result := extractContent(nil)
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}

func TestExtractContent_Image(t *testing.T) {
	contents := []protocol.Content{
		{Type: "image", Data: "base64data", MimeType: "image/png"},
	}
	result := extractContent(contents)
	if result != "[image: image/png]" {
		t.Errorf("got %q, want %q", result, "[image: image/png]")
	}
}

func TestExtractContent_Resource(t *testing.T) {
	contents := []protocol.Content{
		{Type: "resource", URI: "file:///path/to/file"},
	}
	result := extractContent(contents)
	if result != "[resource: file:///path/to/file]" {
		t.Errorf("got %q, want %q", result, "[resource: file:///path/to/file]")
	}
}

func TestExtractContent_PreferText(t *testing.T) {
	contents := []protocol.Content{
		{Type: "image", Data: "base64data", MimeType: "image/png"},
		{Type: "text", Text: "caption"},
	}
	result := extractContent(contents)
	if result != "caption" {
		t.Errorf("got %q, want %q", result, "caption")
	}
}

func TestExtractErrorContent_Empty(t *testing.T) {
	result := extractErrorContent(nil)
	if result != "unknown error" {
		t.Errorf("got %q, want %q", result, "unknown error")
	}
}

func TestExtractErrorContent_WithText(t *testing.T) {
	contents := []protocol.Content{
		{Type: "text", Text: "specific error"},
	}
	result := extractErrorContent(contents)
	if result != "specific error" {
		t.Errorf("got %q, want %q", result, "specific error")
	}
}
