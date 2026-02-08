package client

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"mote/internal/mcp/protocol"
)

func TestNewManager(t *testing.T) {
	configs := []ClientConfig{
		{TransportType: "stdio", Command: "server1"},
		{TransportType: "stdio", Command: "server2"},
	}

	manager := NewManager(configs)
	if manager == nil {
		t.Fatal("Manager should not be nil")
	}
	if len(manager.configs) != 2 {
		t.Errorf("Expected 2 configs, got %d", len(manager.configs))
	}
}

func TestManager_GetClient_NotFound(t *testing.T) {
	manager := NewManager(nil)

	_, ok := manager.GetClient("nonexistent")
	if ok {
		t.Error("Should not find nonexistent client")
	}
}

func TestManager_Disconnect_NotFound(t *testing.T) {
	manager := NewManager(nil)

	err := manager.Disconnect("nonexistent")
	if err == nil {
		t.Error("Should return error for nonexistent client")
	}
}

func TestManager_Connect_EmptyName(t *testing.T) {
	manager := NewManager(nil)

	config := ClientConfig{
		TransportType: "stdio",
		Command:       "",
	}

	err := manager.Connect(context.Background(), config)
	if err == nil {
		t.Error("Should return error for empty name")
	}
}

func TestManager_Connect_Duplicate(t *testing.T) {
	manager := NewManager(nil)

	// Add a mock client
	manager.mu.Lock()
	manager.clients["test"] = &Client{name: "test"}
	manager.mu.Unlock()

	config := ClientConfig{
		TransportType: "stdio",
		Command:       "test",
	}

	err := manager.Connect(context.Background(), config)
	if err == nil {
		t.Error("Should return error for duplicate client")
	}
}

func TestManager_ListServers(t *testing.T) {
	manager := NewManager(nil)

	// Add mock clients
	manager.mu.Lock()
	manager.clients["server1"] = &Client{
		name:  "server1",
		state: StateConnected,
		tools: []protocol.Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}
	manager.clients["server2"] = &Client{
		name:  "server2",
		state: StateError,
	}
	manager.mu.Unlock()

	statuses := manager.ListServers()
	if len(statuses) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statuses))
	}

	// Check statuses (order may vary)
	statusMap := make(map[string]ServerStatus)
	for _, s := range statuses {
		statusMap[s.Name] = s
	}

	if statusMap["server1"].ToolCount != 2 {
		t.Errorf("server1 tool count: got %d, want 2", statusMap["server1"].ToolCount)
	}
	if statusMap["server2"].State != StateError {
		t.Errorf("server2 state: got %v, want StateError", statusMap["server2"].State)
	}
}

func TestManager_GetAllTools(t *testing.T) {
	manager := NewManager(nil)

	// Add mock clients
	manager.mu.Lock()
	manager.clients["server1"] = &Client{
		name:  "server1",
		state: StateConnected,
		tools: []protocol.Tool{
			{Name: "tool1", Description: "Tool 1"},
			{Name: "tool2", Description: "Tool 2"},
		},
	}
	manager.clients["server2"] = &Client{
		name:  "server2",
		state: StateConnected,
		tools: []protocol.Tool{
			{Name: "tool3", Description: "Tool 3"},
		},
	}
	// This client is not connected, should be skipped
	manager.clients["server3"] = &Client{
		name:  "server3",
		state: StateDisconnected,
		tools: []protocol.Tool{
			{Name: "tool4", Description: "Tool 4"},
		},
	}
	manager.mu.Unlock()

	tools := manager.GetAllTools()

	// Should only include tools from connected servers
	if len(tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(tools))
	}

	// Check prefixed names
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedNames := []string{"server1_tool1", "server1_tool2", "server2_tool3"}
	for _, name := range expectedNames {
		if !toolNames[name] {
			t.Errorf("Expected tool %s not found", name)
		}
	}
}

func TestManager_ClientCount(t *testing.T) {
	manager := NewManager(nil)

	if manager.ClientCount() != 0 {
		t.Errorf("Initial count: got %d, want 0", manager.ClientCount())
	}

	manager.mu.Lock()
	manager.clients["server1"] = &Client{name: "server1"}
	manager.clients["server2"] = &Client{name: "server2"}
	manager.mu.Unlock()

	if manager.ClientCount() != 2 {
		t.Errorf("Count after adding: got %d, want 2", manager.ClientCount())
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantFound  bool
	}{
		{"normal", "server_tool", "server", "tool", true},
		{"with underscore in tool", "server_my_tool", "server", "my_tool", true},
		{"no underscore", "servertool", "", "", false},
		{"empty", "", "", "", false},
		{"starts with underscore", "_tool", "", "tool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, tool, found := parseToolName(tt.input)
			if server != tt.wantServer || tool != tt.wantTool || found != tt.wantFound {
				t.Errorf("parseToolName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, server, tool, found, tt.wantServer, tt.wantTool, tt.wantFound)
			}
		})
	}
}

func TestManager_CallTool_InvalidFormat(t *testing.T) {
	manager := NewManager(nil)

	_, err := manager.CallTool(context.Background(), "invalidname", nil)
	if err == nil {
		t.Error("Should return error for invalid tool name format")
	}
}

func TestManager_CallTool_ServerNotFound(t *testing.T) {
	manager := NewManager(nil)

	_, err := manager.CallTool(context.Background(), "server_tool", nil)
	if err == nil {
		t.Error("Should return error for nonexistent server")
	}
}

// mockManagerTransport for manager tests
type mockManagerTransport struct {
	mu        sync.Mutex
	sendCh    chan []byte
	receiveCh chan []byte
}

func (t *mockManagerTransport) Send(ctx context.Context, data []byte) error {
	select {
	case t.sendCh <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *mockManagerTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.receiveCh:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *mockManagerTransport) Close() error {
	return nil
}

func (t *mockManagerTransport) getSent() ([]byte, bool) {
	select {
	case data := <-t.sendCh:
		return data, true
	case <-time.After(1 * time.Second):
		return nil, false
	}
}

func TestManager_CallTool_Success(t *testing.T) {
	manager := NewManager(nil)

	// Create a mock transport and client
	mockT := &mockManagerTransport{
		sendCh:    make(chan []byte, 10),
		receiveCh: make(chan []byte, 10),
	}

	client := &Client{
		name:      "myserver",
		transport: mockT,
		pending:   make(map[int64]chan *protocol.Response),
		config: ClientConfig{
			Timeout: 5 * time.Second,
		},
		state: StateConnected,
	}

	ctx, cancel := context.WithCancel(context.Background())
	client.ctx = ctx
	client.cancel = cancel
	client.wg.Add(1)
	go client.receiveLoop()

	manager.mu.Lock()
	manager.clients["myserver"] = client
	manager.mu.Unlock()

	// Handle the request
	go func() {
		data, ok := mockT.getSent()
		if !ok {
			return
		}

		var req protocol.Request
		if err := json.Unmarshal(data, &req); err != nil {
			return
		}

		respData, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "result"},
				},
				"isError": false,
			},
		})
		mockT.receiveCh <- respData
	}()

	result, err := manager.CallTool(context.Background(), "myserver_mytool", nil)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content, got %d", len(result.Content))
	}

	client.Close()
}

func TestManager_Stop(t *testing.T) {
	manager := NewManager(nil)

	// Add mock clients
	mockT1 := &mockManagerTransport{sendCh: make(chan []byte, 10), receiveCh: make(chan []byte, 10)}
	mockT2 := &mockManagerTransport{sendCh: make(chan []byte, 10), receiveCh: make(chan []byte, 10)}

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	manager.mu.Lock()
	manager.clients["server1"] = &Client{
		name:      "server1",
		transport: mockT1,
		ctx:       ctx1,
		cancel:    cancel1,
	}
	manager.clients["server2"] = &Client{
		name:      "server2",
		transport: mockT2,
		ctx:       ctx2,
		cancel:    cancel2,
	}
	manager.mu.Unlock()

	err := manager.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if manager.ClientCount() != 0 {
		t.Errorf("Client count after stop: got %d, want 0", manager.ClientCount())
	}
}
