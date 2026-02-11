package copilot

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		req      JSONRPCRequest
		expected string
	}{
		{
			name: "simple request",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "initialize",
			},
			expected: `{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		},
		{
			name: "request with params",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      2,
				Method:  "session/new",
				Params: map[string]interface{}{
					"cwd": "/tmp",
				},
			},
			expected: `{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp"}}`,
		},
		{
			name: "notification (no ID)",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "initialized",
			},
			expected: `{"jsonrpc":"2.0","method":"initialized"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(data))
			}
		})
	}
}

func TestJSONRPCResponse_IsNotification(t *testing.T) {
	tests := []struct {
		name           string
		resp           JSONRPCResponse
		isNotification bool
	}{
		{
			name: "regular response",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      intPtr(1),
				Result:  json.RawMessage(`{}`),
			},
			isNotification: false,
		},
		{
			name: "notification",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				Method:  "session/update",
				Params:  json.RawMessage(`{}`),
			},
			isNotification: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.resp.IsNotification() != tt.isNotification {
				t.Errorf("Expected IsNotification=%v, got %v", tt.isNotification, tt.resp.IsNotification())
			}
		})
	}
}

func TestInitializeParams_Marshal(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion:    1,
		ClientCapabilities: ClientCapabilities{},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	expected := `{"protocolVersion":1,"clientCapabilities":{}}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}

func TestInitializeResult_Unmarshal(t *testing.T) {
	input := `{
		"protocolVersion": 1,
		"agentInfo": {
			"name": "copilot",
			"version": "1.0.0"
		},
		"agentCapabilities": {
			"loadSession": true,
			"setMode": false,
			"modes": ["code", "chat"]
		}
	}`

	var result InitializeResult
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if result.ProtocolVersion != 1 {
		t.Errorf("Expected protocol version 1, got %d", result.ProtocolVersion)
	}
	if result.AgentInfo.Name != "copilot" {
		t.Errorf("Expected agent name 'copilot', got '%s'", result.AgentInfo.Name)
	}
	if result.AgentInfo.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", result.AgentInfo.Version)
	}
	if !result.AgentCapabilities.LoadSession {
		t.Error("Expected LoadSession to be true")
	}
	if len(result.AgentCapabilities.Modes) != 2 {
		t.Errorf("Expected 2 modes, got %d", len(result.AgentCapabilities.Modes))
	}
}

func TestNewSessionParams_Marshal(t *testing.T) {
	params := NewSessionParams{
		Cwd: "/home/user/project",
		McpServers: []MCPServer{
			{
				Name:    "test-server",
				Type:    "stdio",
				Command: "node",
				Args:    []string{"server.js"},
				Env:     []MCPEnvVar{},
			},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify it contains expected fields
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["cwd"] != "/home/user/project" {
		t.Errorf("Expected cwd '/home/user/project', got '%v'", parsed["cwd"])
	}

	mcpServers, ok := parsed["mcpServers"].([]interface{})
	if !ok || len(mcpServers) != 1 {
		t.Errorf("Expected 1 MCP server, got %v", mcpServers)
	}
}

func TestPromptParams_Marshal(t *testing.T) {
	params := PromptParams{
		SessionID: "test-session-123",
		Prompt: []PromptContent{
			{Type: "text", Text: "Hello, world!"},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	expected := `{"sessionId":"test-session-123","prompt":[{"type":"text","text":"Hello, world!"}]}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}

func TestSessionUpdate_Unmarshal(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect func(t *testing.T, params SessionUpdateParams)
	}{
		{
			name: "agent message chunk",
			input: `{
				"sessionId": "test-123",
				"update": {
					"sessionUpdate": "agent_message_chunk",
					"content": {
						"type": "text",
						"text": "Hello"
					}
				}
			}`,
			expect: func(t *testing.T, params SessionUpdateParams) {
				if params.SessionID != "test-123" {
					t.Errorf("Expected sessionId 'test-123', got '%s'", params.SessionID)
				}
				if params.Update.SessionUpdate != UpdateTypeAgentMessageChunk {
					t.Errorf("Expected update type '%s', got '%s'", UpdateTypeAgentMessageChunk, params.Update.SessionUpdate)
				}
				if params.Update.Content == nil {
					t.Fatal("Expected content to be non-nil")
				}
				if params.Update.Content.Text != "Hello" {
					t.Errorf("Expected text 'Hello', got '%s'", params.Update.Content.Text)
				}
			},
		},
		{
			name: "tool call start",
			input: `{
				"sessionId": "test-456",
				"update": {
					"sessionUpdate": "tool_call_start",
					"toolCall": {
						"id": "call-1",
						"name": "shell",
						"arguments": "{\"command\": \"ls\"}"
					}
				}
			}`,
			expect: func(t *testing.T, params SessionUpdateParams) {
				if params.Update.SessionUpdate != UpdateTypeToolCallStart {
					t.Errorf("Expected update type '%s', got '%s'", UpdateTypeToolCallStart, params.Update.SessionUpdate)
				}
				if params.Update.ToolCall == nil {
					t.Fatal("Expected toolCall to be non-nil")
				}
				if params.Update.ToolCall.ID != "call-1" {
					t.Errorf("Expected tool call ID 'call-1', got '%s'", params.Update.ToolCall.ID)
				}
				if params.Update.ToolCall.Name != "shell" {
					t.Errorf("Expected tool name 'shell', got '%s'", params.Update.ToolCall.Name)
				}
			},
		},
		{
			name: "content as array",
			input: `{
				"sessionId": "test-789",
				"update": {
					"sessionUpdate": "agent_message_chunk",
					"content": [
						{"type": "text", "text": "Hello "},
						{"type": "text", "text": "World"}
					]
				}
			}`,
			expect: func(t *testing.T, params SessionUpdateParams) {
				if params.SessionID != "test-789" {
					t.Errorf("Expected sessionId 'test-789', got '%s'", params.SessionID)
				}
				if params.Update.Content == nil {
					t.Fatal("Expected content to be non-nil")
				}
				if params.Update.Content.Type != "text" {
					t.Errorf("Expected type 'text', got '%s'", params.Update.Content.Type)
				}
				if params.Update.Content.Text != "Hello World" {
					t.Errorf("Expected text 'Hello World', got '%s'", params.Update.Content.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params SessionUpdateParams
			if err := json.Unmarshal([]byte(tt.input), &params); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			tt.expect(t, params)
		})
	}
}

func TestConvertStopReason(t *testing.T) {
	tests := []struct {
		acpReason string
		expected  string
	}{
		{StopReasonEndTurn, "stop"},
		{StopReasonToolUse, "tool_calls"},
		{StopReasonMaxTokens, "length"},
		{StopReasonStopSequence, "stop"},
		{"unknown", "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.acpReason, func(t *testing.T) {
			result := convertStopReason(tt.acpReason)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function
func intPtr(i int) *int {
	return &i
}

// ========== Phase 1 新增类型和函数测试 ==========

func TestConvertMCPServers(t *testing.T) {
	tests := []struct {
		name     string
		input    []MCPServerPersistInfo
		expected map[string]MCPServerConfig
	}{
		{
			name:     "empty input",
			input:    nil,
			expected: map[string]MCPServerConfig{},
		},
		{
			name: "stdio server",
			input: []MCPServerPersistInfo{
				{Name: "fs", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}},
			},
			expected: map[string]MCPServerConfig{
				"fs": {Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, Tools: []string{"*"}},
			},
		},
		{
			name: "local server",
			input: []MCPServerPersistInfo{
				{Name: "fs", Type: "local", Command: "npx", Args: []string{"-y", "server"}},
			},
			expected: map[string]MCPServerConfig{
				"fs": {Type: "local", Command: "npx", Args: []string{"-y", "server"}, Tools: []string{"*"}},
			},
		},
		{
			name: "http server",
			input: []MCPServerPersistInfo{
				{Name: "api", Type: "http", URL: "http://localhost:8080/mcp", Headers: map[string]string{"Authorization": "Bearer token"}},
			},
			expected: map[string]MCPServerConfig{
				"api": {Type: "http", URL: "http://localhost:8080/mcp", Headers: map[string]string{"Authorization": "Bearer token"}, Tools: []string{"*"}},
			},
		},
		{
			name: "sse server keeps type",
			input: []MCPServerPersistInfo{
				{Name: "sse-api", Type: "sse", URL: "http://localhost:9090/sse"},
			},
			expected: map[string]MCPServerConfig{
				"sse-api": {Type: "sse", URL: "http://localhost:9090/sse", Tools: []string{"*"}},
			},
		},
		{
			name: "mixed types",
			input: []MCPServerPersistInfo{
				{Name: "fs", Type: "stdio", Command: "npx", Args: []string{"server"}},
				{Name: "api", Type: "http", URL: "http://localhost:8080"},
				{Name: "events", Type: "sse", URL: "http://localhost:9090"},
			},
			expected: map[string]MCPServerConfig{
				"fs":     {Type: "stdio", Command: "npx", Args: []string{"server"}, Tools: []string{"*"}},
				"api":    {Type: "http", URL: "http://localhost:8080", Tools: []string{"*"}},
				"events": {Type: "sse", URL: "http://localhost:9090", Tools: []string{"*"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertMCPServers(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d servers, got %d", len(tt.expected), len(result))
			}
			for name, expected := range tt.expected {
				got, ok := result[name]
				if !ok {
					t.Fatalf("missing server %q", name)
				}
				if got.Type != expected.Type {
					t.Errorf("server %q: type = %q, want %q", name, got.Type, expected.Type)
				}
				if got.Command != expected.Command {
					t.Errorf("server %q: command = %q, want %q", name, got.Command, expected.Command)
				}
				if got.URL != expected.URL {
					t.Errorf("server %q: url = %q, want %q", name, got.URL, expected.URL)
				}
				// Check tools field
				if len(got.Tools) != len(expected.Tools) {
					t.Errorf("server %q: tools count = %d, want %d", name, len(got.Tools), len(expected.Tools))
				} else if len(got.Tools) > 0 && got.Tools[0] != expected.Tools[0] {
					t.Errorf("server %q: tools[0] = %q, want %q", name, got.Tools[0], expected.Tools[0])
				}
			}
		})
	}
}

func TestBuildACPSystemMessage(t *testing.T) {
	tests := []struct {
		name           string
		customPrompt   string
		workspaceRules string
		mcpServers     []string
		expectNil      bool
		expectContains []string
	}{
		{
			name:      "both empty returns nil",
			expectNil: true,
		},
		{
			name:           "custom prompt only",
			customPrompt:   "You are a Go expert.",
			expectContains: []string{"You are a Go expert."},
		},
		{
			name:           "workspace rules only",
			workspaceRules: "Use gofmt.",
			expectContains: []string{"## Workspace Rules", "Use gofmt."},
		},
		{
			name:           "both present joined with separator",
			customPrompt:   "You are a Go expert.",
			workspaceRules: "Use gofmt.",
			expectContains: []string{"You are a Go expert.", "---", "## Workspace Rules", "Use gofmt."},
		},
		{
			name:           "with MCP servers",
			mcpServers:     []string{"SmartTest", "Aidg"},
			expectContains: []string{"## Available MCP Servers", "SmartTest", "Aidg", "mcp_list_tools"},
		},
		{
			name:           "all options combined",
			customPrompt:   "You are a Go expert.",
			workspaceRules: "Use gofmt.",
			mcpServers:     []string{"MyMCP"},
			expectContains: []string{"You are a Go expert.", "## Workspace Rules", "## Available MCP Servers", "MyMCP"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildACPSystemMessage(tt.customPrompt, tt.workspaceRules, tt.mcpServers, "")
			if tt.expectNil {
				if result != nil {
					t.Fatalf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			for _, s := range tt.expectContains {
				if !contains(result.Content, s) {
					t.Errorf("expected content to contain %q, got %q", s, result.Content)
				}
			}
		})
	}
}

func TestCreateSessionParams_JSONMarshal(t *testing.T) {
	streaming := true
	reqPerm := true
	params := CreateSessionParams{
		Model:            "claude-sonnet-4",
		WorkingDirectory: "/home/user/project",
		MCPServers: map[string]MCPServerConfig{
			"fs": {Type: "stdio", Command: "npx", Args: []string{"server"}},
		},
		SystemMessage: &SystemMessageConfig{
			Content: "You are helpful.",
		},
		Streaming:         &streaming,
		RequestPermission: &reqPerm,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify it's valid JSON and has expected structure
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// MCPServers should be an object, not an array
	mcpServers, ok := decoded["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers should be a map, got %T", decoded["mcpServers"])
	}
	if _, ok := mcpServers["fs"]; !ok {
		t.Error("mcpServers should contain 'fs' key")
	}

	// SystemMessage should be nested object
	sysMsg, ok := decoded["systemMessage"].(map[string]interface{})
	if !ok {
		t.Fatalf("systemMessage should be a map, got %T", decoded["systemMessage"])
	}
	if sysMsg["content"] != "You are helpful." {
		t.Errorf("systemMessage.content = %q, want %q", sysMsg["content"], "You are helpful.")
	}

	// model and workingDirectory
	if decoded["model"] != "claude-sonnet-4" {
		t.Errorf("model = %q, want %q", decoded["model"], "claude-sonnet-4")
	}
	if decoded["workingDirectory"] != "/home/user/project" {
		t.Errorf("workingDirectory = %q, want %q", decoded["workingDirectory"], "/home/user/project")
	}
}

func TestCreateSessionParams_OmitEmpty(t *testing.T) {
	params := CreateSessionParams{
		WorkingDirectory: "/tmp",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Empty fields should be omitted
	for _, key := range []string{"model", "mcpServers", "tools", "systemMessage", "streaming"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("expected %q to be omitted when empty, but it was present", key)
		}
	}

	// workingDirectory should be present
	if _, ok := decoded["workingDirectory"]; !ok {
		t.Error("workingDirectory should be present")
	}
}

func TestToolCallRequest_JSONMarshal(t *testing.T) {
	req := ToolCallRequest{
		SessionID:  "sess-123",
		ToolCallID: "tc-456",
		ToolName:   "mote_http",
		Arguments:  map[string]interface{}{"url": "https://example.com"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded["sessionId"] != "sess-123" {
		t.Errorf("sessionId = %q, want %q", decoded["sessionId"], "sess-123")
	}
	if decoded["toolName"] != "mote_http" {
		t.Errorf("toolName = %q, want %q", decoded["toolName"], "mote_http")
	}
}

func TestToolResult_JSONMarshal(t *testing.T) {
	result := ToolResult{
		TextResultForLLM: "Success: status 200",
		ResultType:       "success",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if !contains(string(data), `"textResultForLlm"`) {
		t.Error("expected textResultForLlm JSON key")
	}
	if !contains(string(data), `"resultType"`) {
		t.Error("expected resultType JSON key")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchContains(s, substr)
}

func searchContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
