package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mote/internal/config"
	"mote/internal/gateway/handlers"
	"mote/internal/mcp/client"
	"mote/internal/mcp/transport"

	"github.com/gorilla/mux"
)

// MCPServerPersist represents a persisted MCP server configuration.
type MCPServerPersist struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
}

var mcpConfigMu sync.Mutex

// getMCPConfigPath returns the path to the MCP servers config file.
func getMCPConfigPath() (string, error) {
	dir, err := config.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp_servers.json"), nil
}

// loadMCPServersConfig loads MCP servers from the config file.
func loadMCPServersConfig() ([]MCPServerPersist, error) {
	path, err := getMCPConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No config file yet
		}
		return nil, err
	}

	var servers []MCPServerPersist
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, err
	}

	return servers, nil
}

// saveMCPServersConfig saves MCP servers to the config file.
func saveMCPServersConfig(servers []MCPServerPersist) error {
	mcpConfigMu.Lock()
	defer mcpConfigMu.Unlock()

	path, err := getMCPConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadMCPServersConfigPublic loads MCP servers config (public wrapper for tools).
func LoadMCPServersConfigPublic() ([]MCPServerPersist, error) {
	return loadMCPServersConfig()
}

// AddMCPServerToConfig adds a server to the persisted config.
// Exported for use by mcp_add tool.
func AddMCPServerToConfig(server MCPServerPersist) error {
	servers, err := loadMCPServersConfig()
	if err != nil {
		servers = []MCPServerPersist{}
	}

	// Check if already exists, update if so
	found := false
	for i, s := range servers {
		if s.Name == server.Name {
			servers[i] = server
			found = true
			break
		}
	}
	if !found {
		servers = append(servers, server)
	}

	return saveMCPServersConfig(servers)
}

// RemoveMCPServerFromConfig removes a server from the persisted config.
// Exported for use by mcp_remove tool.
func RemoveMCPServerFromConfig(name string) error {
	servers, err := loadMCPServersConfig()
	if err != nil {
		return nil // No config to remove from
	}

	// Filter out the server
	filtered := make([]MCPServerPersist, 0, len(servers))
	for _, s := range servers {
		if s.Name != name {
			filtered = append(filtered, s)
		}
	}

	return saveMCPServersConfig(filtered)
}

// LoadSavedMCPServers loads and connects to saved MCP servers.
// This should be called at startup after the MCP manager is initialized.
func LoadSavedMCPServers(ctx context.Context, manager *client.Manager) error {
	servers, err := loadMCPServersConfig()
	if err != nil {
		return err
	}

	for _, server := range servers {
		cfg := client.ClientConfig{
			Command: server.Name,
		}

		switch server.Type {
		case "http":
			cfg.TransportType = transport.TransportHTTP
			cfg.URL = server.URL
			cfg.Headers = server.Headers
		case "stdio":
			cfg.TransportType = transport.TransportStdio
			cfg.Command = server.Command
			cfg.Args = server.Args
		default:
			continue
		}

		// Use a short timeout for each server
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = manager.Connect(connectCtx, cfg) // Ignore errors, just try to connect
		cancel()
	}

	return nil
}

// HandleListMCPServers returns a list of connected MCP servers.
func (r *Router) HandleListMCPServers(w http.ResponseWriter, req *http.Request) {
	// Load configured servers from file
	configuredServers, _ := loadMCPServersConfig()
	cfgMap := make(map[string]MCPServerPersist)
	for _, cfg := range configuredServers {
		cfgMap[cfg.Name] = cfg
	}

	// Get connected servers from manager
	connectedServers := make(map[string]MCPServerInfo)
	if r.mcpClient != nil {
		statuses := r.mcpClient.ListServers()
		for _, status := range statuses {
			info := MCPServerInfo{
				Name:        status.Name,
				Status:      status.State.String(),
				Transport:   status.TransportType,
				ToolCount:   status.ToolCount,
				PromptCount: status.PromptCount,
				Error:       status.LastError,
			}
			// Add config info if available
			if cfg, ok := cfgMap[status.Name]; ok {
				info.URL = cfg.URL
				info.Headers = cfg.Headers
				info.Command = cfg.Command
				info.Args = cfg.Args
			}
			connectedServers[status.Name] = info
		}
	}

	// Merge: prefer connected server info, add disconnected ones as "disconnected"
	var servers []MCPServerInfo
	seenNames := make(map[string]bool)

	// Add connected servers first
	for name, server := range connectedServers {
		servers = append(servers, server)
		seenNames[name] = true
	}

	// Add configured but not connected servers
	for _, cfg := range configuredServers {
		if !seenNames[cfg.Name] {
			servers = append(servers, MCPServerInfo{
				Name:      cfg.Name,
				Status:    "disconnected",
				Transport: cfg.Type,
				URL:       cfg.URL,
				Headers:   cfg.Headers,
				Command:   cfg.Command,
				Args:      cfg.Args,
				Error:     "未连接 - 服务可能未启动",
			})
		}
	}

	if servers == nil {
		servers = []MCPServerInfo{}
	}

	handlers.SendJSON(w, http.StatusOK, MCPServersResponse{
		Servers: servers,
	})
}

// HandleListMCPTools returns a list of all MCP tools across all servers.
func (r *Router) HandleListMCPTools(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendJSON(w, http.StatusOK, MCPToolsResponse{
			Tools: []MCPToolInfo{},
		})
		return
	}

	// Optional filter by server name
	serverFilter := req.URL.Query().Get("server")

	// Get all tools from all servers
	allTools := r.mcpClient.GetAllTools()

	var tools []MCPToolInfo
	for _, tool := range allTools {
		// Parse server name from prefixed tool name (format: serverName_toolName)
		serverName := ""
		if idx := strings.Index(tool.Name, "_"); idx > 0 {
			serverName = tool.Name[:idx]
		}

		// Apply server filter if provided
		if serverFilter != "" && serverName != serverFilter {
			continue
		}

		tools = append(tools, MCPToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Server:      serverName,
			Schema:      tool.InputSchema,
		})
	}

	if tools == nil {
		tools = []MCPToolInfo{}
	}

	handlers.SendJSON(w, http.StatusOK, MCPToolsResponse{
		Tools: tools,
	})
}

// HandleListMCPPrompts returns a list of all MCP prompts across all servers.
func (r *Router) HandleListMCPPrompts(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendJSON(w, http.StatusOK, MCPPromptsResponse{
			Prompts: []MCPPromptInfo{},
		})
		return
	}

	// Optional filter by server name
	serverFilter := req.URL.Query().Get("server")

	// Get all prompts from all servers
	allPrompts := r.mcpClient.GetAllPrompts()

	var prompts []MCPPromptInfo
	for _, prompt := range allPrompts {
		// Apply server filter if provided
		if serverFilter != "" && prompt.ServerName != serverFilter {
			continue
		}

		info := MCPPromptInfo{
			Name:        prompt.Name,
			Description: prompt.Description,
			Server:      prompt.ServerName,
		}
		for _, arg := range prompt.Arguments {
			info.Arguments = append(info.Arguments, MCPPromptArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}
		prompts = append(prompts, info)
	}

	if prompts == nil {
		prompts = []MCPPromptInfo{}
	}

	handlers.SendJSON(w, http.StatusOK, MCPPromptsResponse{
		Prompts: prompts,
	})
}

// MCPGetPromptRequest is the request body for getting an MCP prompt.
type MCPGetPromptRequest struct {
	Arguments map[string]string `json:"arguments,omitempty"`
}

// MCPGetPromptResponse is the response for getting an MCP prompt.
type MCPGetPromptResponse struct {
	Description string             `json:"description,omitempty"`
	Messages    []MCPPromptMessage `json:"messages"`
}

// MCPPromptMessage represents a message in a prompt.
type MCPPromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HandleGetMCPPrompt retrieves a specific prompt from an MCP server.
func (r *Router) HandleGetMCPPrompt(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	serverName := vars["server"]
	promptName := vars["name"]

	// Parse arguments from request body (optional)
	var args map[string]string
	if req.ContentLength > 0 {
		var getReq MCPGetPromptRequest
		if err := json.NewDecoder(req.Body).Decode(&getReq); err == nil {
			args = getReq.Arguments
		}
	}

	// Call MCP server to get prompt
	result, err := r.mcpClient.GetPrompt(req.Context(), serverName, promptName, args)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, err.Error())
		return
	}

	// Convert to response format
	var messages []MCPPromptMessage
	for _, msg := range result.Messages {
		messages = append(messages, MCPPromptMessage{
			Role:    msg.Role,
			Content: msg.Content.Text,
		})
	}

	handlers.SendJSON(w, http.StatusOK, MCPGetPromptResponse{
		Description: result.Description,
		Messages:    messages,
	})
}

// MCPAddRequest is the request body for adding a new MCP server.
// Supports both simple format and full JSON config format.
type MCPAddRequest struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
}

// MCPImportRequest is the request body for importing MCP servers from JSON config.
// Format: {"server_name": {"type": "http", "url": "...", ...}, ...}
type MCPImportRequest map[string]MCPServerConfig

// MCPServerConfig represents a single MCP server configuration for import.
type MCPServerConfig struct {
	Type        string            `json:"type"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
}

// HandleAddMCPServer adds a new MCP server connection.
func (r *Router) HandleAddMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	var reqBody MCPAddRequest
	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if reqBody.Name == "" || reqBody.Type == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "name and type are required")
		return
	}

	// Build client config
	config := client.ClientConfig{
		Command: reqBody.Name,
	}

	switch reqBody.Type {
	case "http":
		if reqBody.URL == "" {
			handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "url is required for http type")
			return
		}
		config.TransportType = transport.TransportHTTP
		config.URL = reqBody.URL
		config.Headers = reqBody.Headers

	case "stdio":
		if reqBody.Command == "" {
			handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "command is required for stdio type")
			return
		}
		config.TransportType = transport.TransportStdio
		config.Command = reqBody.Command
		config.Args = reqBody.Args

	default:
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "type must be 'http' or 'stdio'")
		return
	}

	// Persist the server configuration first (so it can be reconnected later even if connection fails now)
	persist := MCPServerPersist{
		Name:    reqBody.Name,
		Type:    reqBody.Type,
		URL:     reqBody.URL,
		Headers: reqBody.Headers,
		Command: reqBody.Command,
		Args:    reqBody.Args,
	}
	if err := AddMCPServerToConfig(persist); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to save config: "+err.Error())
		return
	}

	// Try to connect to the server (non-blocking - failures don't prevent adding)
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	var connectError string
	if err := r.mcpClient.Connect(ctx, config); err != nil {
		// Connection failed, but config is saved - server can be reconnected later
		connectError = err.Error()
	}

	// Get the new server status
	servers := r.mcpClient.ListServers()
	for _, s := range servers {
		if s.Name == reqBody.Name {
			handlers.SendJSON(w, http.StatusCreated, MCPServerInfo{
				Name:      s.Name,
				Status:    s.State.String(),
				Transport: reqBody.Type,
				ToolCount: s.ToolCount,
				Error:     connectError,
			})
			return
		}
	}

	// Server not in list (connection failed), return saved status
	status := "disconnected"
	if connectError == "" {
		status = "connected"
	}
	handlers.SendJSON(w, http.StatusCreated, MCPServerInfo{
		Name:      reqBody.Name,
		Status:    status,
		Transport: reqBody.Type,
		Error:     connectError,
	})
}

// MCPUpdateRequest is the request body for updating an MCP server.
type MCPUpdateRequest struct {
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
}

// HandleUpdateMCPServer updates an existing MCP server configuration.
func (r *Router) HandleUpdateMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	// Load existing config
	servers, err := loadMCPServersConfig()
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to load config: "+err.Error())
		return
	}

	// Find existing server
	var existing *MCPServerPersist
	for i := range servers {
		if servers[i].Name == name {
			existing = &servers[i]
			break
		}
	}
	if existing == nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Server not found: "+name)
		return
	}

	// Parse update request
	var reqBody MCPUpdateRequest
	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Apply updates
	if reqBody.Type != "" {
		existing.Type = reqBody.Type
	}
	if reqBody.URL != "" {
		existing.URL = reqBody.URL
	}
	if reqBody.Headers != nil {
		existing.Headers = reqBody.Headers
	}
	if reqBody.Command != "" {
		existing.Command = reqBody.Command
	}
	if reqBody.Args != nil {
		existing.Args = reqBody.Args
	}

	// Save updated config
	if err := saveMCPServersConfig(servers); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to save config: "+err.Error())
		return
	}

	// Reconnect the server with new config
	config := client.ClientConfig{
		Command: name,
	}
	switch existing.Type {
	case "http":
		config.TransportType = transport.TransportHTTP
		config.URL = existing.URL
		config.Headers = existing.Headers
	case "stdio":
		config.TransportType = transport.TransportStdio
		config.Command = existing.Command
		config.Args = existing.Args
	}

	// First disconnect if connected
	_ = r.mcpClient.Disconnect(name)

	// Reconnect
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	var connectError string
	if err := r.mcpClient.Connect(ctx, config); err != nil {
		connectError = err.Error()
	}

	// Return updated server info
	serverList := r.mcpClient.ListServers()
	for _, s := range serverList {
		if s.Name == name {
			handlers.SendJSON(w, http.StatusOK, MCPServerInfo{
				Name:      s.Name,
				Status:    s.State.String(),
				Transport: existing.Type,
				ToolCount: s.ToolCount,
				Error:     connectError,
			})
			return
		}
	}

	status := "disconnected"
	if connectError == "" {
		status = "connected"
	}
	handlers.SendJSON(w, http.StatusOK, MCPServerInfo{
		Name:      name,
		Status:    status,
		Transport: existing.Type,
		Error:     connectError,
	})
}

// HandleImportMCPServers imports MCP servers from JSON config.
// Supports format: {"server_name": {"type": "http", "url": "...", "headers": {...}}, ...}
func (r *Router) HandleImportMCPServers(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	var importReq MCPImportRequest
	if err := json.NewDecoder(req.Body).Decode(&importReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid JSON format: "+err.Error())
		return
	}

	if len(importReq) == 0 {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "No servers provided")
		return
	}

	results := make([]MCPServerInfo, 0, len(importReq))
	errors := make([]string, 0)

	for name, serverConfig := range importReq {
		config := client.ClientConfig{
			Command: name,
		}

		switch serverConfig.Type {
		case "http":
			if serverConfig.URL == "" {
				errors = append(errors, name+": url is required for http type")
				continue
			}
			config.TransportType = transport.TransportHTTP
			config.URL = serverConfig.URL
			config.Headers = serverConfig.Headers

		case "stdio":
			if serverConfig.Command == "" {
				errors = append(errors, name+": command is required for stdio type")
				continue
			}
			config.TransportType = transport.TransportStdio
			config.Command = serverConfig.Command
			config.Args = serverConfig.Args

		default:
			errors = append(errors, name+": type must be 'http' or 'stdio'")
			continue
		}

		// Persist config first (so it can be reconnected later even if connection fails now)
		persist := MCPServerPersist{
			Name:    name,
			Type:    serverConfig.Type,
			URL:     serverConfig.URL,
			Headers: serverConfig.Headers,
			Command: serverConfig.Command,
			Args:    serverConfig.Args,
		}
		if err := AddMCPServerToConfig(persist); err != nil {
			errors = append(errors, name+": failed to save config: "+err.Error())
			continue
		}

		// Try to connect to the server with a shorter timeout for batch operations
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		connectErr := r.mcpClient.Connect(ctx, config)
		cancel()

		serverInfo := MCPServerInfo{
			Name:      name,
			Transport: serverConfig.Type,
		}
		if connectErr != nil {
			// Connection failed, but config is saved - server can be reconnected later
			serverInfo.Status = "disconnected"
			serverInfo.Error = connectErr.Error()
		} else {
			serverInfo.Status = "connected"
		}
		results = append(results, serverInfo)
	}

	response := map[string]any{
		"imported": results,
		"count":    len(results),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	handlers.SendJSON(w, http.StatusOK, response)
}

// HandleStopMCPServer stops an MCP server connection without removing it from config.
// The server can be started again later.
func (r *Router) HandleStopMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	if name == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "server name is required")
		return
	}

	if err := r.mcpClient.Disconnect(name); err != nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Server not found: "+err.Error())
		return
	}

	// Note: We do NOT remove from persisted config, so it can be started again
	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Server stopped: " + name,
	})
}

// HandleDeleteMCPServer removes an MCP server connection.
func (r *Router) HandleDeleteMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	if name == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "server name is required")
		return
	}

	// Try to disconnect (ignore error if not connected)
	_ = r.mcpClient.Disconnect(name)

	// Remove from persisted config (this is the important part)
	if err := RemoveMCPServerFromConfig(name); err != nil {
		// Check if the server exists in config
		servers, _ := loadMCPServersConfig()
		found := false
		for _, s := range servers {
			if s.Name == name {
				found = true
				break
			}
		}
		if !found {
			handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Server not found in config: "+name)
			return
		}
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Server removed: " + name,
	})
}

// HandleRestartMCPServer restarts an MCP server connection.
// If the server is not yet connected, it will be connected from the config file.
func (r *Router) HandleRestartMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	if name == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "server name is required")
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	// Try to get the existing client
	cli, ok := r.mcpClient.GetClient(name)
	if ok {
		// Server is connected, do a restart (disconnect then reconnect)
		config := cli.GetConfig()

		if err := r.mcpClient.Disconnect(name); err != nil {
			handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to disconnect: "+err.Error())
			return
		}

		if err := r.mcpClient.Connect(ctx, config); err != nil {
			handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to reconnect: "+err.Error())
			return
		}

		handlers.SendJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Server restarted: " + name,
		})
		return
	}

	// Server not connected, try to load from config and connect
	servers, err := loadMCPServersConfig()
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to load config: "+err.Error())
		return
	}

	var serverConfig *MCPServerPersist
	for i := range servers {
		if servers[i].Name == name {
			serverConfig = &servers[i]
			break
		}
	}

	if serverConfig == nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Server not found in config: "+name)
		return
	}

	// Build client config and connect
	config := client.ClientConfig{
		Command: serverConfig.Name, // Command is used as the client name
	}

	switch serverConfig.Type {
	case "stdio":
		config.TransportType = transport.TransportStdio
		config.Command = serverConfig.Command
		config.Args = serverConfig.Args
	case "http":
		config.TransportType = transport.TransportHTTP
		config.URL = serverConfig.URL
		config.Headers = serverConfig.Headers
	default:
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Unknown transport type: "+serverConfig.Type)
		return
	}

	if err := r.mcpClient.Connect(ctx, config); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to connect: "+err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Server started: " + name,
	})
}

// HandleGetMCPServer returns detailed info about a specific MCP server.
func (r *Router) HandleGetMCPServer(w http.ResponseWriter, req *http.Request) {
	if r.mcpClient == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "MCP manager not initialized")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	if name == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "server name is required")
		return
	}

	cli, ok := r.mcpClient.GetClient(name)
	if !ok {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Server not found")
		return
	}

	tools := cli.Tools()
	prompts := cli.Prompts()

	var toolInfos []MCPToolInfo
	for _, t := range tools {
		toolInfos = append(toolInfos, MCPToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Server:      name,
			Schema:      t.InputSchema,
		})
	}

	var promptInfos []MCPPromptInfo
	for _, p := range prompts {
		promptInfos = append(promptInfos, MCPPromptInfo{
			Name:        p.Name,
			Description: p.Description,
		})
	}

	config := cli.GetConfig()
	serverType := "stdio"
	if config.TransportType == transport.TransportHTTP {
		serverType = "http"
	}

	handlers.SendJSON(w, http.StatusOK, MCPServerDetail{
		Name:        name,
		Status:      cli.State().String(),
		Transport:   serverType,
		URL:         config.URL,
		ToolCount:   len(tools),
		PromptCount: len(prompts),
		Tools:       toolInfos,
		Prompts:     promptInfos,
	})
}
