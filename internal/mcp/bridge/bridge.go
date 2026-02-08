// Package bridge implements the MCP tool bridge that connects external MCP servers
// to the internal tool registry.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"mote/internal/mcp/protocol"
	"mote/internal/tools"
)

// MCPClient defines the interface required for an MCP client to be used with the bridge.
type MCPClient interface {
	// Name returns the client's name.
	Name() string
	// ListTools returns the tools available from the server.
	ListTools() []protocol.Tool
	// CallTool calls a tool on the remote server.
	CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error)
}

// Bridge manages the registration of external MCP tools into the internal registry.
type Bridge struct {
	registry *tools.Registry
	adapters map[string][]*ToolAdapter
	mu       sync.RWMutex
}

// NewBridge creates a new MCP bridge.
func NewBridge(registry *tools.Registry) *Bridge {
	return &Bridge{
		registry: registry,
		adapters: make(map[string][]*ToolAdapter),
	}
}

// Register registers all tools from an MCP client into the registry.
func (b *Bridge) Register(client MCPClient) error {
	if client == nil {
		return fmt.Errorf("client cannot be nil")
	}

	clientName := client.Name()
	if clientName == "" {
		return fmt.Errorf("client name cannot be empty")
	}

	mcpTools := client.ListTools()
	if len(mcpTools) == 0 {
		return nil // No tools to register
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check for existing registration
	if _, exists := b.adapters[clientName]; exists {
		return fmt.Errorf("client %q already registered", clientName)
	}

	adapters := make([]*ToolAdapter, 0, len(mcpTools))
	var registeredNames []string

	for _, tool := range mcpTools {
		adapter := NewToolAdapter(client, tool)
		if err := b.registry.Register(adapter); err != nil {
			// Rollback already registered tools
			for _, name := range registeredNames {
				_ = b.registry.Unregister(name)
			}
			return fmt.Errorf("register tool %q: %w", adapter.Name(), err)
		}
		adapters = append(adapters, adapter)
		registeredNames = append(registeredNames, adapter.Name())
	}

	b.adapters[clientName] = adapters
	return nil
}

// Unregister removes all tools from an MCP client from the registry.
func (b *Bridge) Unregister(clientName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	adapters, exists := b.adapters[clientName]
	if !exists {
		return fmt.Errorf("client %q not registered", clientName)
	}

	for _, adapter := range adapters {
		_ = b.registry.Unregister(adapter.Name())
	}

	delete(b.adapters, clientName)
	return nil
}

// Refresh updates the tools registered for an MCP client.
// It first unregisters existing tools, then registers the new set.
func (b *Bridge) Refresh(client MCPClient) error {
	clientName := client.Name()

	// Try to unregister (ignore error if not registered)
	_ = b.Unregister(clientName)

	// Register new tools
	return b.Register(client)
}

// GetAdapters returns the tool adapters for a client.
func (b *Bridge) GetAdapters(clientName string) []*ToolAdapter {
	b.mu.RLock()
	defer b.mu.RUnlock()

	adapters := b.adapters[clientName]
	result := make([]*ToolAdapter, len(adapters))
	copy(result, adapters)
	return result
}

// ListClients returns the names of all registered clients.
func (b *Bridge) ListClients() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, 0, len(b.adapters))
	for name := range b.adapters {
		result = append(result, name)
	}
	return result
}

// ToolCount returns the total number of registered tools.
func (b *Bridge) ToolCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, adapters := range b.adapters {
		count += len(adapters)
	}
	return count
}

// ToolAdapter adapts an MCP tool to the internal Tool interface.
type ToolAdapter struct {
	client       MCPClient
	info         protocol.Tool
	prefixedName string
	description  string
	parameters   map[string]any
}

// NewToolAdapter creates a new tool adapter.
func NewToolAdapter(client MCPClient, info protocol.Tool) *ToolAdapter {
	prefixedName := fmt.Sprintf("%s_%s", client.Name(), info.Name)

	// Parse InputSchema into parameters map
	var params map[string]any
	if len(info.InputSchema) > 0 {
		_ = json.Unmarshal(info.InputSchema, &params)
	}
	if params == nil {
		params = make(map[string]any)
	}

	return &ToolAdapter{
		client:       client,
		info:         info,
		prefixedName: prefixedName,
		description:  info.Description,
		parameters:   params,
	}
}

// Name returns the prefixed tool name (format: clientName_toolName).
func (a *ToolAdapter) Name() string {
	return a.prefixedName
}

// Description returns the tool's description.
func (a *ToolAdapter) Description() string {
	return a.description
}

// Parameters returns the tool's input schema as a map.
func (a *ToolAdapter) Parameters() map[string]any {
	return a.parameters
}

// OriginalName returns the original tool name without the client prefix.
func (a *ToolAdapter) OriginalName() string {
	return a.info.Name
}

// ClientName returns the name of the client this tool belongs to.
func (a *ToolAdapter) ClientName() string {
	return a.client.Name()
}

// Execute calls the tool on the remote MCP server.
func (a *ToolAdapter) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	result, err := a.client.CallTool(ctx, a.info.Name, args)
	if err != nil {
		return tools.NewErrorResult(err.Error()), nil
	}

	if result.IsError {
		// Tool execution returned an error
		content := extractErrorContent(result.Content)
		return tools.NewErrorResult(content), nil
	}

	// Convert MCP content to tool result
	content := extractContent(result.Content)
	return tools.NewSuccessResult(content), nil
}

// extractContent extracts text content from MCP Content slice.
func extractContent(contents []protocol.Content) string {
	if len(contents) == 0 {
		return ""
	}

	// Prefer text content
	for _, c := range contents {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}

	// Try other content types
	for _, c := range contents {
		switch c.Type {
		case "image":
			if c.Data != "" {
				return fmt.Sprintf("[image: %s]", c.MimeType)
			}
		case "resource":
			if c.URI != "" {
				return fmt.Sprintf("[resource: %s]", c.URI)
			}
		}
	}

	// Fallback: JSON encode the first content
	if len(contents) > 0 {
		data, _ := json.Marshal(contents[0])
		return string(data)
	}

	return ""
}

// extractErrorContent extracts error content from MCP Content slice.
func extractErrorContent(contents []protocol.Content) string {
	content := extractContent(contents)
	if content == "" {
		return "unknown error"
	}
	return content
}
