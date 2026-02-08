package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mote/internal/mcp/protocol"
)

// Manager manages multiple MCP client connections.
type Manager struct {
	clients map[string]*Client
	configs []ClientConfig
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// ServerStatus represents the status of a connected MCP server.
type ServerStatus struct {
	Name          string          `json:"name"`
	State         ConnectionState `json:"state"`
	TransportType string          `json:"transport_type"`
	ToolCount     int             `json:"tool_count"`
	PromptCount   int             `json:"prompt_count"`
	LastError     string          `json:"last_error,omitempty"`
	ConnectedAt   *time.Time      `json:"connected_at,omitempty"`
}

// NewManager creates a new MCP client manager.
func NewManager(configs []ClientConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		clients: make(map[string]*Client),
		configs: configs,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start connects to all configured MCP servers.
func (m *Manager) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.configs))

	for _, config := range m.configs {
		wg.Add(1)
		go func(cfg ClientConfig) {
			defer wg.Done()

			client := NewClient(cfg.Command, cfg) // Use command as name if not specified
			if err := client.Connect(ctx); err != nil {
				errCh <- fmt.Errorf("connect to %s: %w", cfg.Command, err)
				return
			}

			m.mu.Lock()
			m.clients[cfg.Command] = client
			m.mu.Unlock()
		}(config)
	}

	wg.Wait()
	close(errCh)

	// Collect errors (non-fatal, just log)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 && len(errs) == len(m.configs) {
		// All connections failed
		return errors.New("all connections failed")
	}

	return nil
}

// Stop disconnects from all MCP servers.
func (m *Manager) Stop() error {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			lastErr = fmt.Errorf("close %s: %w", name, err)
		}
		delete(m.clients, name)
	}

	return lastErr
}

// Connect adds and connects to a new MCP server at runtime.
func (m *Manager) Connect(ctx context.Context, config ClientConfig) error {
	name := config.Command
	if name == "" {
		return errors.New("client name/command is required")
	}

	m.mu.Lock()
	if _, exists := m.clients[name]; exists {
		m.mu.Unlock()
		return fmt.Errorf("client %s already exists", name)
	}
	m.mu.Unlock()

	client := NewClient(name, config)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to %s: %w", name, err)
	}

	m.mu.Lock()
	m.clients[name] = client
	m.mu.Unlock()

	return nil
}

// Disconnect removes and disconnects from an MCP server.
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	client, exists := m.clients[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("client %s not found", name)
	}
	delete(m.clients, name)
	m.mu.Unlock()

	return client.Close()
}

// GetClient returns a client by name.
func (m *Manager) GetClient(name string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.clients[name]
	return client, ok
}

// ListServers returns the status of all connected servers.
func (m *Manager) ListServers() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ServerStatus, 0, len(m.clients))
	for name, client := range m.clients {
		status := ServerStatus{
			Name:          name,
			State:         client.State(),
			TransportType: client.TransportType(),
			ToolCount:     len(client.Tools()),
			PromptCount:   len(client.Prompts()),
		}
		if err := client.LastError(); err != nil {
			status.LastError = err.Error()
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// GetAllTools returns all tools from all connected servers with server name prefix.
func (m *Manager) GetAllTools() []protocol.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allTools []protocol.Tool
	for serverName, client := range m.clients {
		if client.State() != StateConnected {
			continue
		}

		for _, tool := range client.Tools() {
			// Add server name prefix to tool name
			prefixedTool := protocol.Tool{
				Name:        serverName + "_" + tool.Name,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			}
			allTools = append(allTools, prefixedTool)
		}
	}

	return allTools
}

// MCPPrompt represents a prompt from an MCP server with server info.
type MCPPrompt struct {
	ServerName  string              `json:"server_name"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

// MCPPromptArgument represents a prompt argument.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// GetAllPrompts returns all prompts from all connected servers.
func (m *Manager) GetAllPrompts() []MCPPrompt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allPrompts []MCPPrompt
	for serverName, client := range m.clients {
		if client.State() != StateConnected {
			continue
		}

		for _, prompt := range client.Prompts() {
			mcpPrompt := MCPPrompt{
				ServerName:  serverName,
				Name:        prompt.Name,
				Description: prompt.Description,
			}
			for _, arg := range prompt.Arguments {
				mcpPrompt.Arguments = append(mcpPrompt.Arguments, MCPPromptArgument{
					Name:        arg.Name,
					Description: arg.Description,
					Required:    arg.Required,
				})
			}
			allPrompts = append(allPrompts, mcpPrompt)
		}
	}

	return allPrompts
}

// GetPrompt retrieves a specific prompt from an MCP server.
func (m *Manager) GetPrompt(ctx context.Context, serverName, promptName string, args map[string]string) (*protocol.GetPromptResult, error) {
	client, ok := m.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	return client.GetPrompt(ctx, promptName, args)
}

// CallTool calls a tool on the appropriate server based on the prefixed tool name.
func (m *Manager) CallTool(ctx context.Context, prefixedName string, args map[string]any) (*protocol.CallToolResult, error) {
	// Parse server name and tool name from prefixed name
	serverName, toolName, found := parseToolName(prefixedName)
	if !found {
		return nil, fmt.Errorf("invalid tool name format: %s", prefixedName)
	}

	client, ok := m.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	return client.CallTool(ctx, toolName, args)
}

// parseToolName parses a prefixed tool name into server name and tool name.
// Format: servername_toolname
func parseToolName(prefixedName string) (serverName, toolName string, found bool) {
	for i := 0; i < len(prefixedName); i++ {
		if prefixedName[i] == '_' {
			return prefixedName[:i], prefixedName[i+1:], true
		}
	}
	return "", "", false
}

// ClientCount returns the number of connected clients.
func (m *Manager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}
