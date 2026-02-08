package protocol

import "encoding/json"

// MCP method constants.
const (
	MethodInitialize  = "initialize"
	MethodInitialized = "notifications/initialized"
	MethodToolsList   = "tools/list"
	MethodToolsCall   = "tools/call"
	MethodPromptsList = "prompts/list"
	MethodPromptsGet  = "prompts/get"
	MethodPing        = "ping"
	MethodCancelled   = "notifications/cancelled"
)

// MCP protocol version.
const ProtocolVersion = "2024-11-05"

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ClientInfo      ClientInfo   `json:"clientInfo"`
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo contains information about the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities declares the capabilities supported by client or server.
type Capabilities struct {
	// Tools capability indicates support for tool-related operations.
	Tools *ToolsCapability `json:"tools,omitempty"`

	// Experimental contains experimental capabilities.
	Experimental map[string]any `json:"experimental,omitempty"`
}

// ToolsCapability declares tool-related capabilities.
type ToolsCapability struct {
	// ListChanged indicates the server will send notifications when tools change.
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListToolsParams represents parameters for tools/list request.
type ListToolsParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListToolsResult represents the result of tools/list request.
type ListToolsResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// CallToolParams represents parameters for tools/call request.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallToolResult represents the result of tools/call request.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents a content item in tool results.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// Content type constants.
const (
	ContentTypeText     = "text"
	ContentTypeImage    = "image"
	ContentTypeResource = "resource"
)

// NewTextContent creates a text content item.
func NewTextContent(text string) Content {
	return Content{
		Type: ContentTypeText,
		Text: text,
	}
}

// NewImageContent creates an image content item.
func NewImageContent(data, mimeType string) Content {
	return Content{
		Type:     ContentTypeImage,
		Data:     data,
		MimeType: mimeType,
	}
}

// NewResourceContent creates a resource content item.
func NewResourceContent(uri, mimeType string) Content {
	return Content{
		Type:     ContentTypeResource,
		URI:      uri,
		MimeType: mimeType,
	}
}

// PingParams represents parameters for ping request.
type PingParams struct{}

// PingResult represents the result of ping request.
type PingResult struct{}

// Prompt represents an MCP prompt definition.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents a prompt argument definition.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsParams represents parameters for prompts/list request.
type ListPromptsParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListPromptsResult represents the result of prompts/list request.
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor *string  `json:"nextCursor,omitempty"`
}

// GetPromptParams represents parameters for prompts/get request.
type GetPromptParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent represents the content of a prompt message.
type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// GetPromptResult represents the result of prompts/get request.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// CancelledParams represents parameters for notifications/cancelled.
type CancelledParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

// ToolInputSchema creates a JSON Schema for tool parameters.
func ToolInputSchema(properties map[string]any, required []string) json.RawMessage {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	data, _ := json.Marshal(schema)
	return data
}
