package server

import (
	"context"
	"encoding/json"
	"strings"

	"mote/internal/mcp/protocol"
	"mote/internal/tools"
)

// ToolMapper maps between internal tools and MCP tools.
type ToolMapper struct {
	registry *tools.Registry
	prefix   string
}

// NewToolMapper creates a new ToolMapper.
func NewToolMapper(registry *tools.Registry, prefix string) *ToolMapper {
	return &ToolMapper{
		registry: registry,
		prefix:   prefix,
	}
}

// ListTools returns all tools as MCP protocol tools.
func (m *ToolMapper) ListTools() []protocol.Tool {
	internalTools := m.registry.List()
	mcpTools := make([]protocol.Tool, 0, len(internalTools))

	for _, t := range internalTools {
		mcpTools = append(mcpTools, m.ToMCPTool(t))
	}

	return mcpTools
}

// GetTool retrieves an internal tool by its MCP name.
func (m *ToolMapper) GetTool(mcpName string) (tools.Tool, bool) {
	internalName := m.toInternalName(mcpName)
	return m.registry.Get(internalName)
}

// Execute executes a tool by its MCP name and returns an MCP CallToolResult.
func (m *ToolMapper) Execute(ctx context.Context, mcpName string, args map[string]any) (*protocol.CallToolResult, error) {
	tool, ok := m.GetTool(mcpName)
	if !ok {
		return nil, protocol.NewToolNotFoundError(mcpName)
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		return nil, protocol.NewToolExecutionError(mcpName, err)
	}

	return m.ToMCPResult(result), nil
}

// ToMCPTool converts an internal tool to an MCP protocol tool.
func (m *ToolMapper) ToMCPTool(t tools.Tool) protocol.Tool {
	schema := m.toMCPInputSchema(t.Parameters())
	schemaJSON, _ := json.Marshal(schema)
	return protocol.Tool{
		Name:        m.toMCPName(t.Name()),
		Description: t.Description(),
		InputSchema: schemaJSON,
	}
}

// ToMCPResult converts a ToolResult to an MCP CallToolResult.
func (m *ToolMapper) ToMCPResult(result tools.ToolResult) *protocol.CallToolResult {
	content := []protocol.Content{
		protocol.NewTextContent(result.Content),
	}

	return &protocol.CallToolResult{
		Content: content,
		IsError: result.IsError,
	}
}

// toMCPName converts an internal tool name to an MCP tool name.
func (m *ToolMapper) toMCPName(internalName string) string {
	if m.prefix == "" {
		return internalName
	}
	return m.prefix + "_" + internalName
}

// toInternalName converts an MCP tool name to an internal tool name.
func (m *ToolMapper) toInternalName(mcpName string) string {
	if m.prefix == "" {
		return mcpName
	}
	prefix := m.prefix + "_"
	if strings.HasPrefix(mcpName, prefix) {
		return strings.TrimPrefix(mcpName, prefix)
	}
	return mcpName
}

// toMCPInputSchema converts internal parameters to MCP input schema format.
func (m *ToolMapper) toMCPInputSchema(params map[string]any) map[string]any {
	// Ensure we have a valid JSON Schema structure
	schema := make(map[string]any)

	// Copy all fields from params
	for k, v := range params {
		schema[k] = v
	}

	// Ensure type is set to "object"
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}

	return schema
}
