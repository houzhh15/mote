package protocol

import (
	"encoding/json"
	"testing"
)

func TestToolSerialization(t *testing.T) {
	schema := ToolInputSchema(
		map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
		},
		[]string{"query"},
	)

	tool := Tool{
		Name:        "search",
		Description: "Search for information",
		InputSchema: schema,
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal tool failed: %v", err)
	}

	var parsed Tool
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal tool failed: %v", err)
	}

	if parsed.Name != "search" {
		t.Errorf("expected name search, got %s", parsed.Name)
	}
	if parsed.Description != "Search for information" {
		t.Errorf("expected description, got %s", parsed.Description)
	}
}

func TestInitializeParams(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo: ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
		Capabilities: Capabilities{
			Tools: &ToolsCapability{
				ListChanged: true,
			},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed InitializeParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.ProtocolVersion != ProtocolVersion {
		t.Errorf("expected version %s, got %s", ProtocolVersion, parsed.ProtocolVersion)
	}
	if parsed.ClientInfo.Name != "test-client" {
		t.Errorf("expected client name test-client, got %s", parsed.ClientInfo.Name)
	}
}

func TestInitializeResult(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: ServerInfo{
			Name:    "mote",
			Version: "1.0.0",
		},
		Capabilities: Capabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed InitializeResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.ServerInfo.Name != "mote" {
		t.Errorf("expected server name mote, got %s", parsed.ServerInfo.Name)
	}
}

func TestCallToolParams(t *testing.T) {
	params := CallToolParams{
		Name: "read_file",
		Arguments: map[string]any{
			"path": "/tmp/test.txt",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed CallToolParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", parsed.Name)
	}
	if parsed.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("expected path /tmp/test.txt, got %v", parsed.Arguments["path"])
	}
}

func TestCallToolResult(t *testing.T) {
	result := CallToolResult{
		Content: []Content{
			NewTextContent("Hello, World!"),
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed CallToolResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(parsed.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(parsed.Content))
	}
	if parsed.Content[0].Type != ContentTypeText {
		t.Errorf("expected type text, got %s", parsed.Content[0].Type)
	}
	if parsed.Content[0].Text != "Hello, World!" {
		t.Errorf("expected text, got %s", parsed.Content[0].Text)
	}
}

func TestContentTypes(t *testing.T) {
	t.Run("TextContent", func(t *testing.T) {
		content := NewTextContent("test text")
		if content.Type != ContentTypeText {
			t.Errorf("expected type %s, got %s", ContentTypeText, content.Type)
		}
		if content.Text != "test text" {
			t.Errorf("expected text, got %s", content.Text)
		}
	})

	t.Run("ImageContent", func(t *testing.T) {
		content := NewImageContent("base64data", "image/png")
		if content.Type != ContentTypeImage {
			t.Errorf("expected type %s, got %s", ContentTypeImage, content.Type)
		}
		if content.Data != "base64data" {
			t.Errorf("expected data, got %s", content.Data)
		}
		if content.MimeType != "image/png" {
			t.Errorf("expected mimeType, got %s", content.MimeType)
		}
	})

	t.Run("ResourceContent", func(t *testing.T) {
		content := NewResourceContent("file:///tmp/test.txt", "text/plain")
		if content.Type != ContentTypeResource {
			t.Errorf("expected type %s, got %s", ContentTypeResource, content.Type)
		}
		if content.URI != "file:///tmp/test.txt" {
			t.Errorf("expected uri, got %s", content.URI)
		}
	})
}

func TestListToolsResult(t *testing.T) {
	result := ListToolsResult{
		Tools: []Tool{
			{
				Name:        "tool1",
				Description: "First tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
			{
				Name:        "tool2",
				Description: "Second tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed ListToolsResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(parsed.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(parsed.Tools))
	}
}

func TestToolInputSchema(t *testing.T) {
	schema := ToolInputSchema(
		map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"age": map[string]any{
				"type": "integer",
			},
		},
		[]string{"name"},
	)

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("Unmarshal schema failed: %v", err)
	}

	if parsed["type"] != "object" {
		t.Errorf("expected type object, got %v", parsed["type"])
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	if _, ok := props["name"]; !ok {
		t.Error("expected properties to contain name")
	}

	required, ok := parsed["required"].([]any)
	if !ok {
		t.Fatal("expected required to be an array")
	}
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("expected required to be [name], got %v", required)
	}
}

func TestMethodConstants(t *testing.T) {
	if MethodInitialize != "initialize" {
		t.Errorf("MethodInitialize should be initialize, got %s", MethodInitialize)
	}
	if MethodInitialized != "notifications/initialized" {
		t.Errorf("MethodInitialized should be notifications/initialized, got %s", MethodInitialized)
	}
	if MethodToolsList != "tools/list" {
		t.Errorf("MethodToolsList should be tools/list, got %s", MethodToolsList)
	}
	if MethodToolsCall != "tools/call" {
		t.Errorf("MethodToolsCall should be tools/call, got %s", MethodToolsCall)
	}
	if MethodPing != "ping" {
		t.Errorf("MethodPing should be ping, got %s", MethodPing)
	}
}

func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion should be 2024-11-05, got %s", ProtocolVersion)
	}
}
