package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	v1 "mote/api/v1"
	"mote/internal/mcp/client"
	"mote/internal/mcp/transport"
	"mote/internal/tools"
)

// mcpManager is the global MCP client manager for tools
var mcpManager *client.Manager

// SetMCPManager sets the MCP client manager for builtin tools
func SetMCPManager(m *client.Manager) {
	mcpManager = m
}

// GetMCPManager returns the MCP client manager
func GetMCPManager() *client.Manager {
	return mcpManager
}

// MCPAddArgs defines the parameters for the mcp_add tool.
type MCPAddArgs struct {
	Name    string            `json:"name" jsonschema:"description=Unique name for the MCP server,required"`
	Type    string            `json:"type" jsonschema:"description=Server type: 'http' or 'stdio',required"`
	URL     string            `json:"url" jsonschema:"description=Server URL (required for http type)"`
	Headers map[string]string `json:"headers" jsonschema:"description=HTTP headers map. CRITICAL: If user provides Authorization token or any headers you MUST include them here. Example: {\"Authorization\": \"Bearer xxx\"}"`
	Command string            `json:"command" jsonschema:"description=Command to run (required for stdio type)"`
	Args    []string          `json:"args" jsonschema:"description=Command arguments (for stdio type)"`
}

// MCPAddTool adds a new MCP server connection
type MCPAddTool struct {
	tools.BaseTool
}

// NewMCPAddTool creates a new mcp_add tool.
func NewMCPAddTool() *MCPAddTool {
	return &MCPAddTool{
		BaseTool: tools.BaseTool{
			ToolName: "mcp_add",
			ToolDescription: `Add and connect to a new MCP server.

CRITICAL: When user provides Authorization header or Bearer token, you MUST extract and include it in the 'headers' parameter!

Example for HTTP server WITH headers:
{
  "name": "my-server",
  "type": "http",
  "url": "http://127.0.0.1:8001/mcp",
  "headers": {"Authorization": "Bearer user_provided_token"}
}

Example for HTTP server WITHOUT headers:
{
  "name": "my-server",
  "type": "http", 
  "url": "http://127.0.0.1:8001/mcp"
}

If user mentions token/bearer/authorization, ALWAYS include 'headers' parameter!`,
			ToolParameters: tools.BuildSchema(MCPAddArgs{}),
		},
	}
}

// Execute adds a new MCP server connection.
func (t *MCPAddTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	if mcpManager == nil {
		return tools.NewErrorResult("MCP manager not initialized"), nil
	}

	name, _ := args["name"].(string)
	serverType, _ := args["type"].(string)

	if name == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "name is required", nil)
	}

	config := client.ClientConfig{
		Command: name, // Use name as identifier for manager
	}

	switch serverType {
	case "http":
		url, _ := args["url"].(string)
		if url == "" {
			return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "url is required for http type", nil)
		}
		config.TransportType = transport.TransportHTTP
		config.URL = url

		// Parse headers - support both map[string]any and map[string]string
		if headersRaw, ok := args["headers"].(map[string]any); ok {
			config.Headers = make(map[string]string)
			for k, v := range headersRaw {
				if s, ok := v.(string); ok {
					config.Headers[k] = s
				}
			}
		} else if headersStr, ok := args["headers"].(map[string]string); ok {
			config.Headers = headersStr
		}

	case "stdio":
		command, _ := args["command"].(string)
		if command == "" {
			return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "command is required for stdio type", nil)
		}
		config.TransportType = transport.TransportStdio
		config.Command = command

		// Parse args
		if argsRaw, ok := args["args"].([]any); ok {
			for _, arg := range argsRaw {
				if s, ok := arg.(string); ok {
					config.Args = append(config.Args, s)
				}
			}
		}

	default:
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), fmt.Sprintf("unsupported server type: %s (use 'http' or 'stdio')", serverType), nil)
	}

	// Persist the server configuration first (so it can be reconnected later even if connection fails now)
	persist := v1.MCPServerPersist{
		Name: name,
		Type: serverType,
	}
	switch serverType {
	case "http":
		persist.URL = config.URL
		persist.Headers = config.Headers
	case "stdio":
		persist.Command = config.Command
		persist.Args = config.Args
	}
	if err := v1.AddMCPServerToConfig(persist); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to save config: %v", err)), nil
	}

	// Try to connect (non-blocking - failures don't prevent adding)
	var connectError string
	if err := mcpManager.Connect(ctx, config); err != nil {
		connectError = err.Error()
	}

	result := map[string]any{
		"success": true,
		"name":    name,
		"type":    serverType,
	}

	// Include headers info for HTTP type
	if serverType == "http" {
		if len(config.Headers) > 0 {
			// Show header keys (not values for security)
			headerKeys := make([]string, 0, len(config.Headers))
			for k := range config.Headers {
				headerKeys = append(headerKeys, k)
			}
			result["headers_configured"] = headerKeys
		} else {
			result["headers_configured"] = nil
			result["warning"] = "No headers configured. If you need Authorization headers, use mcp_update to add them."
		}
	}

	if connectError != "" {
		result["status"] = "disconnected"
		result["connection_error"] = connectError
		result["message"] = fmt.Sprintf("MCP server '%s' configuration saved. Connection failed (will retry on next startup): %s", name, connectError)
	} else {
		result["status"] = "connected"
		result["message"] = fmt.Sprintf("Successfully connected to MCP server '%s'", name)
	}
	content, _ := json.MarshalIndent(result, "", "  ")
	return tools.NewSuccessResult(string(content)), nil
}

// MCPListArgs defines the parameters for the mcp_list tool.
type MCPListArgs struct{}

// MCPListTool lists all connected MCP servers.
type MCPListTool struct {
	tools.BaseTool
}

// NewMCPListTool creates a new mcp_list tool.
func NewMCPListTool() *MCPListTool {
	return &MCPListTool{
		BaseTool: tools.BaseTool{
			ToolName:        "mcp_list",
			ToolDescription: "List all connected MCP servers and their available tools. IMPORTANT: Call this first to see what tools are available before calling mcp_call.",
			ToolParameters:  tools.BuildSchema(MCPListArgs{}),
		},
	}
}

// Execute lists all connected MCP servers.
func (t *MCPListTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	if mcpManager == nil {
		return tools.NewErrorResult("MCP manager not initialized"), nil
	}

	servers := mcpManager.ListServers()
	if len(servers) == 0 {
		return tools.NewSuccessResult("No MCP servers connected"), nil
	}

	result := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		serverInfo := map[string]any{
			"name":       s.Name,
			"state":      s.State.String(),
			"tool_count": s.ToolCount,
		}
		if s.LastError != "" {
			serverInfo["last_error"] = s.LastError
		}

		// Get full tool information for this server (including description and parameters)
		if client, ok := mcpManager.GetClient(s.Name); ok {
			toolInfos := make([]map[string]any, 0)
			for _, tool := range client.Tools() {
				toolInfo := map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
				}
				// Parse and include inputSchema for parameters
				if len(tool.InputSchema) > 0 {
					var schema map[string]any
					if err := json.Unmarshal(tool.InputSchema, &schema); err == nil {
						toolInfo["parameters"] = schema
					}
				}
				toolInfos = append(toolInfos, toolInfo)
			}
			serverInfo["tools"] = toolInfos
		}

		result = append(result, serverInfo)
	}

	output := map[string]any{
		"servers": result,
		"count":   len(result),
	}
	content, _ := json.MarshalIndent(output, "", "  ")
	return tools.NewSuccessResult(string(content)), nil
}

// MCPCallTool calls a tool on an MCP server.
type MCPCallTool struct {
	tools.BaseTool
}

// NewMCPCallTool creates a new mcp_call tool.
func NewMCPCallTool() *MCPCallTool {
	// Manually build schema to ensure 'arguments' field is properly defined
	// This is important because map[string]any generates a bare "type": "object"
	// which doesn't give LLM enough information
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Name of the MCP server (e.g. 'local' or 'Aidg')",
			},
			"tool": map[string]any{
				"type":        "string",
				"description": "Name of the tool to call (must match exactly as returned by mcp_list)",
			},
			"arguments": map[string]any{
				"type":        "object",
				"description": "The arguments object to pass to the target tool. This should contain all required parameters for the tool. For example: {\"meeting_id\": \"xxx\", \"slot_key\": \"summary\"}",
				"additionalProperties": map[string]any{
					"description": "Tool parameter value - can be string, number, boolean, array, or object depending on the tool's requirements",
				},
			},
		},
		"required": []string{"server", "tool", "arguments"},
	}

	return &MCPCallTool{
		BaseTool: tools.BaseTool{
			ToolName: "mcp_call",
			ToolDescription: `Call a tool on a connected MCP server.

IMPORTANT: You MUST include the 'arguments' parameter with all required parameters for the target tool!

Usage:
1. First call mcp_list to see available servers, tools, and their required parameters
2. Call mcp_call with server, tool, AND arguments containing all required parameters

Example - calling get_meeting_document:
{
  "server": "Aidg",
  "tool": "get_meeting_document",
  "arguments": {
    "meeting_id": "260129-NIEP-GA讨论会",
    "slot_key": "summary"
  }
}

WARNING: Omitting the 'arguments' field or leaving it empty will cause "missing required parameter" errors!`,
			ToolParameters: schema,
		},
	}
}

// Execute calls a tool on an MCP server.
func (t *MCPCallTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	if mcpManager == nil {
		return tools.NewErrorResult("MCP manager not initialized"), nil
	}

	server, _ := args["server"].(string)
	toolName, _ := args["tool"].(string)

	if server == "" || toolName == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "server and tool are required", nil)
	}

	// Get arguments - handle both map and JSON string formats
	toolArgs := make(map[string]any)
	if rawArgs, exists := args["arguments"]; exists && rawArgs != nil {
		switch v := rawArgs.(type) {
		case map[string]any:
			// Already parsed as map
			toolArgs = v
		case string:
			// LLM may pass arguments as JSON string, try to parse it
			if v != "" {
				if err := json.Unmarshal([]byte(v), &toolArgs); err != nil {
					slog.Warn("mcp_call: failed to parse arguments string as JSON",
						"server", server,
						"tool", toolName,
						"arguments", v,
						"error", err)
					// Keep toolArgs empty, let the MCP server report missing params
				}
			}
		default:
			slog.Warn("mcp_call: unexpected arguments type",
				"server", server,
				"tool", toolName,
				"type", fmt.Sprintf("%T", rawArgs))
		}
	}

	slog.Info("mcp_call: executing tool",
		"server", server,
		"tool", toolName,
		"arguments", toolArgs)

	// Call tool using prefixed name format
	prefixedName := server + "_" + toolName
	result, err := mcpManager.CallTool(ctx, prefixedName, toolArgs)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("tool call failed: %v", err)), nil
	}

	// Parse result content
	if len(result.Content) > 0 {
		content := result.Content[0]
		if content.Type == "text" {
			return tools.NewSuccessResult(content.Text), nil
		}
	}

	// Fallback: marshal entire result
	content, _ := json.MarshalIndent(result, "", "  ")
	return tools.NewSuccessResult(string(content)), nil
}

// MCPRemoveArgs defines the parameters for the mcp_remove tool.
type MCPRemoveArgs struct {
	Name string `json:"name" jsonschema:"description=Name of the MCP server to remove,required"`
}

// MCPRemoveTool removes an MCP server connection.
type MCPRemoveTool struct {
	tools.BaseTool
}

// NewMCPRemoveTool creates a new mcp_remove tool.
func NewMCPRemoveTool() *MCPRemoveTool {
	return &MCPRemoveTool{
		BaseTool: tools.BaseTool{
			ToolName:        "mcp_remove",
			ToolDescription: "Remove and disconnect from an MCP server.",
			ToolParameters:  tools.BuildSchema(MCPRemoveArgs{}),
		},
	}
}

// Execute removes an MCP server connection.
func (t *MCPRemoveTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	if mcpManager == nil {
		return tools.NewErrorResult("MCP manager not initialized"), nil
	}

	name, _ := args["name"].(string)
	if name == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "name is required", nil)
	}

	if err := mcpManager.Disconnect(name); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to disconnect: %v", err)), nil
	}

	// Remove from persisted config
	_ = v1.RemoveMCPServerFromConfig(name)

	result := map[string]any{
		"success": true,
		"message": fmt.Sprintf("Successfully disconnected from MCP server '%s'", name),
	}
	content, _ := json.MarshalIndent(result, "", "  ")
	return tools.NewSuccessResult(string(content)), nil
}

// MCPUpdateArgs defines the parameters for the mcp_update tool.
type MCPUpdateArgs struct {
	Name    string            `json:"name" jsonschema:"description=Name of the existing MCP server to update,required"`
	URL     string            `json:"url" jsonschema:"description=New server URL (optional - only update if provided)"`
	Headers map[string]string `json:"headers" jsonschema:"description=HTTP headers to add/update. CRITICAL: You MUST include this parameter when user provides Authorization token. Copy headers exactly from user input. Example: {\"Authorization\": \"Bearer eyJhbGc...\"},required"`
}

// MCPUpdateTool updates an existing MCP server configuration.
type MCPUpdateTool struct {
	tools.BaseTool
}

// NewMCPUpdateTool creates a new mcp_update tool.
func NewMCPUpdateTool() *MCPUpdateTool {
	return &MCPUpdateTool{
		BaseTool: tools.BaseTool{
			ToolName:        "mcp_update",
			ToolDescription: "Update an existing MCP server configuration. CRITICAL: When user provides headers/Authorization in their request, you MUST pass them in the 'headers' parameter. Example call: {\"name\": \"server\", \"headers\": {\"Authorization\": \"Bearer token\"}}",
			ToolParameters:  tools.BuildSchema(MCPUpdateArgs{}),
		},
	}
}

// Execute updates an existing MCP server configuration.
func (t *MCPUpdateTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "name is required", nil)
	}

	// Debug: log received args
	fmt.Printf("[mcp_update] Received args: %+v\n", args)

	// Load existing config
	servers, err := v1.LoadMCPServersConfigPublic()
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to load config: %v", err)), nil
	}

	// Find the server
	var found *v1.MCPServerPersist
	for i := range servers {
		if servers[i].Name == name {
			found = &servers[i]
			break
		}
	}

	if found == nil {
		return tools.NewErrorResult(fmt.Sprintf("MCP server '%s' not found", name)), nil
	}

	// Update URL if provided
	if newURL, ok := args["url"].(string); ok && newURL != "" {
		found.URL = newURL
	}

	// Update/merge headers
	if headersRaw, ok := args["headers"].(map[string]any); ok && len(headersRaw) > 0 {
		if found.Headers == nil {
			found.Headers = make(map[string]string)
		}
		for k, v := range headersRaw {
			if s, ok := v.(string); ok {
				found.Headers[k] = s
			}
		}
	} else if headersStr, ok := args["headers"].(map[string]string); ok && len(headersStr) > 0 {
		if found.Headers == nil {
			found.Headers = make(map[string]string)
		}
		for k, v := range headersStr {
			found.Headers[k] = v
		}
	}

	// Save updated config
	if err := v1.AddMCPServerToConfig(*found); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to save config: %v", err)), nil
	}

	// Disconnect and reconnect with new config
	if mcpManager != nil {
		_ = mcpManager.Disconnect(name) // Ignore error if not connected

		// Reconnect with updated config
		config := client.ClientConfig{
			Command:       name,
			TransportType: transport.TransportHTTP,
			URL:           found.URL,
			Headers:       found.Headers,
		}
		if found.Type == "stdio" {
			config.TransportType = transport.TransportStdio
			config.Command = found.Command
			config.Args = found.Args
		}

		var connectError string
		if err := mcpManager.Connect(ctx, config); err != nil {
			connectError = err.Error()
		}

		result := map[string]any{
			"success":     true,
			"name":        name,
			"url":         found.URL,
			"headers_set": len(found.Headers) > 0,
		}
		if connectError != "" {
			result["status"] = "disconnected"
			result["connection_error"] = connectError
			result["message"] = fmt.Sprintf("MCP server '%s' configuration updated. Reconnection failed: %s", name, connectError)
		} else {
			result["status"] = "connected"
			result["message"] = fmt.Sprintf("MCP server '%s' configuration updated and reconnected successfully", name)
		}
		content, _ := json.MarshalIndent(result, "", "  ")
		return tools.NewSuccessResult(string(content)), nil
	}

	result := map[string]any{
		"success":     true,
		"name":        name,
		"url":         found.URL,
		"headers_set": len(found.Headers) > 0,
		"message":     fmt.Sprintf("MCP server '%s' configuration updated", name),
	}
	content, _ := json.MarshalIndent(result, "", "  ")
	return tools.NewSuccessResult(string(content)), nil
}

// RegisterMCPTools registers all MCP-related tools with the registry.
func RegisterMCPTools(registry *tools.Registry) error {
	mcpTools := []tools.Tool{
		NewMCPAddTool(),
		NewMCPListTool(),
		NewMCPCallTool(),
		NewMCPRemoveTool(),
		NewMCPUpdateTool(),
	}

	for _, tool := range mcpTools {
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("failed to register %s: %w", tool.Name(), err)
		}
	}

	return nil
}
