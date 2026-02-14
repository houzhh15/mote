package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"mote/pkg/logger"
)

// ACP Client errors.
var (
	ErrCopilotCLINotFound = errors.New("copilot CLI not found")
	ErrACPNotInitialized  = errors.New("ACP client not initialized")
	ErrACPClosed          = errors.New("ACP client is closed")
	ErrACPTimeout         = errors.New("ACP request timeout")
	ErrACPProcessDied     = errors.New("ACP process died unexpectedly")
)

// Default timeout for ACP requests.
// ACP requests can involve multiple tool calls and MCP interactions,
// so we use a longer timeout (30 minutes) to accommodate complex tasks.
const DefaultACPTimeout = 30 * time.Minute

// acpMethodPair maps a new (dot-notation) method name to its legacy equivalent.
type acpMethodPair struct {
	newMethod    string
	legacyMethod string
}

// methodName returns the appropriate method name based on the detected protocol version.
func (c *ACPClient) methodName(newMethod, legacyMethod string) string {
	if c.useNewProtocol {
		return newMethod
	}
	return legacyMethod
}

// ACPClient manages communication with the Copilot CLI via ACP protocol.
type ACPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	scanner *bufio.Scanner

	requestID int
	mu        sync.Mutex

	// Response channels indexed by request ID
	responses   map[int]chan *JSONRPCResponse
	responsesMu sync.Mutex

	// Notification handler
	notifyHandler func(method string, params json.RawMessage)
	notifyMu      sync.RWMutex

	// Tool call handler for custom tool execution (tool.call requests from CLI)
	toolCallHandler func(ctx context.Context, req ToolCallRequest) ToolCallResponse
	toolCallMu      sync.RWMutex

	// Hooks handler for hooks.invoke requests from CLI
	hooksHandler func(ctx context.Context, req HooksInvokeRequest) (any, error)
	hooksMu      sync.RWMutex

	// State
	initialized    bool
	useNewProtocol bool // true if CLI supports new dot-notation methods
	closed         bool
	closeMu        sync.RWMutex

	// Configuration
	config ACPConfig

	// For graceful shutdown
	done chan struct{}
}

// NewACPClient creates and starts a new ACP client.
func NewACPClient(cfg ACPConfig) (*ACPClient, error) {
	copilotPath := cfg.CopilotPath
	if copilotPath == "" {
		var err error
		copilotPath, err = findCopilotCLI()
		if err != nil {
			return nil, err
		}
	}

	logger.Info().Str("path", copilotPath).Msg("Starting Copilot CLI for ACP")

	// Sync MCP servers config to CLI's config file before starting
	// The CLI reads MCP config from ~/.copilot/mcp-config.json, not from session/new params
	logger.Info().Int("mcpServersCount", len(cfg.MCPServers)).Msg("ACP: checking MCP servers to sync")
	if len(cfg.MCPServers) > 0 {
		if err := syncMCPConfigToFile(cfg.MCPServers); err != nil {
			logger.Warn().Err(err).Msg("Failed to sync MCP config to CLI config file")
		} else {
			logger.Info().Int("count", len(cfg.MCPServers)).Msg("Synced MCP servers to CLI config file")
		}
	} else {
		logger.Warn().Msg("ACP: no MCP servers configured, skipping sync")
	}

	// Build command arguments
	// Use --allow-all to enable all permissions:
	// - --allow-all-tools: Auto-approve all tool calls without confirmation
	// - --allow-all-paths: Disable file path verification (no permission prompts for paths)
	// - --allow-all-urls: Allow all URL access
	// This is safe because Mote has its own security policy layer (internal/policy)
	args := []string{"--acp", "--stdio", "--allow-all"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	// Note: --allow-all-tools is redundant when using --allow-all, but we keep the config option
	// for backwards compatibility and explicit documentation
	if cfg.AllowAllTools && !containsArg(args, "--allow-all") {
		args = append(args, "--allow-all-tools")
	}

	// Add skill directories to allowed paths
	for _, dir := range cfg.SkillDirectories {
		if dir != "" {
			args = append(args, "--add-dir", dir)
		}
	}

	logger.Info().Strs("args", args).Msg("ACP CLI command arguments")

	// On Windows, .cmd files (npm global installs) must be run via cmd.exe.
	// Direct exec.Command on a .cmd file would fail with "not a valid Win32 application".
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" && (strings.HasSuffix(strings.ToLower(copilotPath), ".cmd") || strings.HasSuffix(strings.ToLower(copilotPath), ".bat")) {
		cmdArgs := append([]string{"/C", copilotPath}, args...)
		cmd = exec.Command("cmd.exe", cmdArgs...)
	} else {
		cmd = exec.Command(copilotPath, args...)
	}

	// Hide the console window on Windows so the CLI runs in the background
	hideProcessWindow(cmd)

	// Inherit parent environment
	// The Copilot CLI uses OAuth tokens from ~/.copilot/ and ~/.config/github-copilot/
	// for authentication. However, if the CLI's own auth is expired or misconfigured
	// (e.g., last_logged_in_user doesn't match available tokens in apps.json),
	// the CLI will fail with 403 when calling GitHub APIs.
	// As a fallback, inject GITHUB_TOKEN from config if it's not already in the
	// environment and a token is available. This allows the CLI to authenticate
	// using the PAT from mote's config when its own OAuth flow fails.
	cmd.Env = os.Environ()
	if cfg.GithubToken != "" {
		hasGithubToken := false
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "GITHUB_TOKEN=") {
				hasGithubToken = true
				break
			}
		}
		if !hasGithubToken {
			cmd.Env = append(cmd.Env, "GITHUB_TOKEN="+cfg.GithubToken)
			logger.Info().Msg("ACP: injecting GITHUB_TOKEN from config as authentication fallback")
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	client := &ACPClient{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		scanner:   bufio.NewScanner(stdout),
		responses: make(map[int]chan *JSONRPCResponse),
		config:    cfg,
		done:      make(chan struct{}),
	}

	// Increase scanner buffer for large responses
	client.scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start copilot CLI: %w", err)
	}

	logger.Info().Int("pid", cmd.Process.Pid).Msg("Copilot ACP server started")

	// Start stderr reader (for debugging)
	go client.readStderr()

	// Start response reader
	go client.readResponses()

	// Monitor process
	go client.monitorProcess()

	return client, nil
}

// containsArg checks if args slice contains the specified argument.
func containsArg(args []string, arg string) bool {
	for _, a := range args {
		if a == arg {
			return true
		}
	}
	return false
}

// findCopilotCLI searches for the copilot CLI in known locations.
// Prioritizes the latest Node.js version (v22+) for full model support.
// Supports macOS, Linux, and Windows.
func findCopilotCLI() (string, error) {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		// Fallback for edge cases
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			homeDir = os.Getenv("USERPROFILE") // Windows fallback
		}
	}

	var possiblePaths []string

	if runtime.GOOS == "windows" {
		// Windows: nvm-windows stores versions under %APPDATA%\nvm
		appData := os.Getenv("APPDATA")
		if appData != "" {
			nvmPattern := filepath.Join(appData, "nvm", "*", "copilot.cmd")
			if nvmMatches, _ := filepath.Glob(nvmPattern); len(nvmMatches) > 0 {
				sort.Slice(nvmMatches, func(i, j int) bool {
					return nvmMatches[i] > nvmMatches[j]
				})
				for _, path := range nvmMatches {
					if _, err := os.Stat(path); err == nil {
						return path, nil
					}
				}
			}
		}

		// Windows: npm global install, VS Code bundled locations
		if appData != "" {
			possiblePaths = append(possiblePaths,
				// npm global (standard npm prefix on Windows)
				filepath.Join(appData, "npm", "copilot.cmd"),
				// VS Code bundled
				filepath.Join(appData, "Code", "User", "globalStorage", "github.copilot-chat", "copilotCli", "copilot.exe"),
				// VS Code Insiders
				filepath.Join(appData, "Code - Insiders", "User", "globalStorage", "github.copilot-chat", "copilotCli", "copilot.exe"),
			)
		}
		if homeDir != "" {
			possiblePaths = append(possiblePaths,
				filepath.Join(homeDir, "AppData", "Roaming", "npm", "copilot.cmd"),
			)
		}
	} else {
		// macOS / Linux: nvm installations
		if homeDir != "" {
			nvmPattern := filepath.Join(homeDir, ".nvm", "versions", "node", "*", "bin", "copilot")
			if nvmMatches, _ := filepath.Glob(nvmPattern); len(nvmMatches) > 0 {
				sort.Slice(nvmMatches, func(i, j int) bool {
					return nvmMatches[i] > nvmMatches[j]
				})
				for _, path := range nvmMatches {
					if _, err := os.Stat(path); err == nil {
						return path, nil
					}
				}
			}
		}

		// macOS / Linux: standard paths
		possiblePaths = append(possiblePaths,
			"/usr/local/bin/copilot",
			"/usr/bin/copilot",
		)
		if homeDir != "" {
			possiblePaths = append(possiblePaths,
				// VS Code bundled (macOS)
				filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "globalStorage", "github.copilot-chat", "copilotCli", "copilot"),
				// VS Code Insiders (macOS)
				filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "globalStorage", "github.copilot-chat", "copilotCli", "copilot"),
				// VS Code bundled (Linux)
				filepath.Join(homeDir, ".vscode", "extensions", "github.copilot-chat", "copilotCli", "copilot"),
			)
		}
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try PATH lookup as last resort — works on all platforms
	// On Windows, LookPath checks for .exe, .cmd, .bat etc. automatically
	if path, err := exec.LookPath("copilot"); err == nil {
		return path, nil
	}

	return "", ErrCopilotCLINotFound
}

// CLIMCPConfig represents the MCP config file format for Copilot CLI (~/.copilot/mcp-config.json)
type CLIMCPConfig struct {
	MCPServers map[string]CLIMCPServerConfig `json:"mcpServers"`
}

// CLIMCPServerConfig represents an MCP server in CLI config format
type CLIMCPServerConfig struct {
	Type    string            `json:"type"`              // "stdio", "http", "sse", "local"
	Command string            `json:"command,omitempty"` // for stdio/local
	Args    []string          `json:"args,omitempty"`    // for stdio/local
	URL     string            `json:"url,omitempty"`     // for http/sse
	Headers map[string]string `json:"headers,omitempty"` // for http/sse
	Tools   []string          `json:"tools"`             // must be present, e.g., ["*"]
}

// syncMCPConfigToFile writes MCP server config to ~/.copilot/mcp-config.json
// This is necessary because CLI reads MCP config from file, not from session/new params
func syncMCPConfigToFile(servers map[string]MCPServerConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return fmt.Errorf("failed to determine user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".copilot")
	configPath := filepath.Join(configDir, "mcp-config.json")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Convert to CLI format
	cliConfig := CLIMCPConfig{
		MCPServers: make(map[string]CLIMCPServerConfig),
	}

	for name, srv := range servers {
		cliSrv := CLIMCPServerConfig{
			Type:    srv.Type,
			Command: srv.Command,
			Args:    srv.Args,
			URL:     srv.URL,
			Headers: srv.Headers,
			Tools:   srv.Tools,
		}
		// Ensure tools is not nil
		if cliSrv.Tools == nil {
			cliSrv.Tools = []string{"*"}
		}
		cliConfig.MCPServers[name] = cliSrv
	}

	// Write to file with pretty formatting
	data, err := json.MarshalIndent(cliConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write MCP config file: %w", err)
	}

	logger.Debug().Str("path", configPath).Int("servers", len(servers)).Msg("Wrote MCP config to CLI config file")
	return nil
}

// readStderr reads and logs stderr output from the CLI.
func (c *ACPClient) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		// Log as info level so CLI errors are more visible
		logger.Info().Str("source", "copilot-stderr").Msg(line)
	}
}

// readResponses reads JSON-RPC responses from stdout.
func (c *ACPClient) readResponses() {
	defer func() {
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()
		close(c.done)
	}()

	lineNum := 0
	for c.scanner.Scan() {
		lineNum++
		line := c.scanner.Text()
		if line == "" {
			continue
		}

		// Log all raw messages from CLI for debugging
		logger.Debug().
			Int("lineNum", lineNum).
			Str("rawLine", line[:min(500, len(line))]).
			Msg("ACP CLI raw message")

		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			logger.Warn().Err(err).Str("line", line[:min(100, len(line))]).Msg("Failed to parse ACP response")
			continue
		}

		// Handle notifications (no ID, has Method)
		if resp.IsNotification() {
			c.notifyMu.RLock()
			handler := c.notifyHandler
			c.notifyMu.RUnlock()

			if handler != nil {
				// Process synchronously to ensure permission responses are sent
				// back before the next stdout line is read. This prevents a race
				// where the CLI blocks waiting for permission while we block
				// reading the next line.
				handler(resp.Method, resp.Params)
			} else {
				logger.Warn().Str("method", resp.Method).Msg("ACP notification received but no handler set")
			}
			continue
		}

		// Handle JSON-RPC requests from CLI (has both ID and Method).
		// This is used for tool.call — the CLI calls us to execute a custom tool.
		if resp.ID != nil && resp.Method != "" {
			c.handleIncomingRequest(resp)
			continue
		}

		// Handle responses (has ID)
		if resp.ID != nil {
			// Log response details
			if resp.Result != nil {
				logger.Debug().
					Int("id", *resp.ID).
					Int("resultLen", len(resp.Result)).
					Str("resultPreview", string(resp.Result)[:min(200, len(resp.Result))]).
					Msg("ACP response received")
			}
			if resp.Error != nil {
				logger.Warn().
					Int("id", *resp.ID).
					Int("errorCode", resp.Error.Code).
					Str("errorMsg", resp.Error.Message).
					Msg("ACP error response received")
			}

			// Skip ID 0 - the CLI sometimes sends a spurious response with
			// id:0 as an ack to our notifications (which have no ID, so the
			// CLI's JSON parser defaults to 0). This is harmless.
			if *resp.ID == 0 {
				logger.Debug().Msg("Skipping ID 0 response (spurious ack)")
				continue
			}
			c.responsesMu.Lock()
			if ch, ok := c.responses[*resp.ID]; ok {
				select {
				case ch <- &resp:
				default:
					logger.Warn().Int("id", *resp.ID).Msg("Response channel full, dropping response")
				}
				delete(c.responses, *resp.ID)
			} else {
				logger.Warn().Int("id", *resp.ID).Msg("ACP response received but no channel registered")
			}
			c.responsesMu.Unlock()
		}
	}

	if err := c.scanner.Err(); err != nil {
		logger.Error().Err(err).Msg("ACP scanner error")
	}
	logger.Info().Int("totalLines", lineNum).Msg("ACP readResponses loop ended")
}

// monitorProcess monitors the copilot CLI process.
func (c *ACPClient) monitorProcess() {
	err := c.cmd.Wait()
	if err != nil {
		logger.Warn().Err(err).Msg("Copilot CLI process exited with error")
	} else {
		logger.Info().Msg("Copilot CLI process exited normally")
	}
}

// sendRequest sends a JSON-RPC request and waits for a response.
func (c *ACPClient) sendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return nil, ErrACPClosed
	}
	c.closeMu.RUnlock()

	// Generate request ID
	c.mu.Lock()
	c.requestID++
	id := c.requestID
	c.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Log the full request for debugging session/new issues
	if method == LegacyMethodSessionNew || method == MethodSessionCreate {
		logger.Debug().
			Str("method", method).
			Int("id", id).
			RawJSON("payload", data).
			Msg("ACP session create request payload")
	}

	// Create response channel
	respCh := make(chan *JSONRPCResponse, 1)
	c.responsesMu.Lock()
	c.responses[id] = respCh
	c.responsesMu.Unlock()

	// deferCleanup controls whether the deferred cleanup removes the response channel.
	// When ctx is cancelled, a drain goroutine takes ownership of cleanup instead.
	deferCleanup := true
	defer func() {
		if deferCleanup {
			c.responsesMu.Lock()
			delete(c.responses, id)
			c.responsesMu.Unlock()
		}
	}()

	// Send request (NDJSON format: JSON + newline)
	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	logger.Debug().Str("method", method).Int("id", id).Msg("Sent ACP request")

	// Determine timeout
	timeout := DefaultACPTimeout
	if c.config.Timeout > 0 {
		timeout = time.Duration(c.config.Timeout) * time.Second
	}

	// Wait for response
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("ACP error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		// Context cancelled (user clicked Stop). The CLI may still be processing.
		// Transfer channel cleanup to a drain goroutine that waits for the CLI's
		// eventual response, preventing "no channel registered" warnings.
		deferCleanup = false // Prevent deferred cleanup — drain goroutine takes over
		go func() {
			drainTimer := time.NewTimer(30 * time.Second)
			defer drainTimer.Stop()
			defer func() {
				c.responsesMu.Lock()
				delete(c.responses, id)
				c.responsesMu.Unlock()
			}()
			select {
			case resp := <-respCh:
				if resp != nil {
					logger.Info().Int("id", id).Str("method", method).Msg("ACP response drained after context cancellation")
				}
			case <-drainTimer.C:
				logger.Warn().Int("id", id).Str("method", method).Msg("ACP drain timeout after context cancellation")
			case <-c.done:
				// CLI process exited
			}
		}()
		return nil, ctx.Err()
	case <-time.After(timeout):
		logger.Error().Str("method", method).Int("id", id).Dur("timeout", timeout).Msg("ACP request timeout")
		return nil, ErrACPTimeout
	case <-c.done:
		return nil, ErrACPProcessDied
	}
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (c *ACPClient) sendNotification(method string, params interface{}) error {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return ErrACPClosed
	}
	c.closeMu.RUnlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		// No ID for notifications
		Method: method,
		Params: params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	return nil
}

// SetNotificationHandler sets the handler for incoming notifications.
func (c *ACPClient) SetNotificationHandler(handler func(method string, params json.RawMessage)) {
	c.notifyMu.Lock()
	c.notifyHandler = handler
	c.notifyMu.Unlock()
}

// SetToolCallHandler sets the handler for incoming tool.call requests from CLI.
func (c *ACPClient) SetToolCallHandler(handler func(ctx context.Context, req ToolCallRequest) ToolCallResponse) {
	c.toolCallMu.Lock()
	c.toolCallHandler = handler
	c.toolCallMu.Unlock()
}

// SetHooksHandler sets the handler for incoming hooks.invoke requests from CLI.
func (c *ACPClient) SetHooksHandler(handler func(ctx context.Context, req HooksInvokeRequest) (any, error)) {
	c.hooksMu.Lock()
	c.hooksHandler = handler
	c.hooksMu.Unlock()
}

// handleIncomingRequest handles a JSON-RPC request from CLI (e.g., tool.call, hooks.invoke, session/request_permission).
func (c *ACPClient) handleIncomingRequest(resp JSONRPCResponse) {
	switch resp.Method {
	case "tool.call":
		c.toolCallMu.RLock()
		handler := c.toolCallHandler
		c.toolCallMu.RUnlock()

		if handler == nil {
			logger.Warn().Str("method", resp.Method).Msg("tool.call received but no handler set")
			c.sendErrorResponse(*resp.ID, -32601, "tool.call handler not registered")
			return
		}

		var req ToolCallRequest
		if err := json.Unmarshal(resp.Params, &req); err != nil {
			logger.Warn().Err(err).Msg("Failed to parse tool.call request")
			c.sendErrorResponse(*resp.ID, -32602, "invalid tool.call params: "+err.Error())
			return
		}

		logger.Debug().Str("tool", req.ToolName).Str("callId", req.ToolCallID).
			Msg("Handling tool.call request")

		// Execute synchronously — the CLI is waiting for the response
		result := handler(context.Background(), req)

		// Send response back to CLI
		resultBytes, err := json.Marshal(result)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to marshal tool.call response")
			c.sendErrorResponse(*resp.ID, -32603, "failed to marshal result")
			return
		}

		c.sendResultResponse(*resp.ID, resultBytes)

	case LegacyMethodRequestPermission, MethodPermissionRequest:
		// Handle permission request from CLI (e.g., for MCP server connections, file access)
		c.handlePermissionRequest(resp)

	case "hooks.invoke":
		c.hooksMu.RLock()
		handler := c.hooksHandler
		c.hooksMu.RUnlock()

		if handler == nil {
			logger.Debug().Msg("hooks.invoke received but no handler set, returning empty result")
			c.sendResultResponse(*resp.ID, []byte("{}"))
			return
		}

		var req HooksInvokeRequest
		if err := json.Unmarshal(resp.Params, &req); err != nil {
			logger.Warn().Err(err).Msg("Failed to parse hooks.invoke request")
			c.sendErrorResponse(*resp.ID, -32602, "invalid hooks.invoke params: "+err.Error())
			return
		}

		logger.Debug().Str("hookType", req.HookType).Msg("Handling hooks.invoke request")

		result, err := handler(context.Background(), req)
		if err != nil {
			logger.Warn().Err(err).Str("hookType", req.HookType).Msg("Hooks handler error")
			c.sendErrorResponse(*resp.ID, -32603, "hooks error: "+err.Error())
			return
		}

		resultBytes, err := json.Marshal(result)
		if err != nil {
			c.sendErrorResponse(*resp.ID, -32603, "failed to marshal hooks result")
			return
		}
		c.sendResultResponse(*resp.ID, resultBytes)

	default:
		logger.Warn().Str("method", resp.Method).Int("id", *resp.ID).
			Msg("Unhandled JSON-RPC request from CLI")
		c.sendErrorResponse(*resp.ID, -32601, "method not found: "+resp.Method)
	}
}

// sendResultResponse sends a successful JSON-RPC response to CLI.
func (c *ACPClient) sendResultResponse(id int, result json.RawMessage) {
	resp := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal JSON-RPC response")
		return
	}

	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()

	if err != nil {
		logger.Error().Err(err).Msg("Failed to send JSON-RPC response")
	}
}

// sendErrorResponse sends a JSON-RPC error response to CLI.
func (c *ACPClient) sendErrorResponse(id int, code int, message string) {
	resp := struct {
		JSONRPC string       `json:"jsonrpc"`
		ID      int          `json:"id"`
		Error   JSONRPCError `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error:   JSONRPCError{Code: code, Message: message},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal JSON-RPC error response")
		return
	}

	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()

	if err != nil {
		logger.Error().Err(err).Msg("Failed to send JSON-RPC error response")
	}
}

// handlePermissionRequest handles permission requests from CLI.
// This is called when CLI needs permission for operations like connecting to MCP servers,
// accessing files, or executing tools.
// By default, we approve all permissions since the user has configured these capabilities.
func (c *ACPClient) handlePermissionRequest(resp JSONRPCResponse) {
	// Log raw params for debugging
	logger.Debug().
		RawJSON("rawParams", resp.Params).
		Msg("Permission request raw params")

	var params RequestPermissionParams
	if err := json.Unmarshal(resp.Params, &params); err != nil {
		logger.Warn().Err(err).
			RawJSON("rawParams", resp.Params).
			Msg("Failed to parse permission request params")
		// Try to approve anyway with empty result
	}

	// Log the permission request details
	if len(params.Permissions) > 0 {
		tools := make([]string, len(params.Permissions))
		for i, p := range params.Permissions {
			tools[i] = p.Tool
		}
		logger.Info().
			Str("sessionId", params.SessionID).
			Strs("tools", tools).
			Msg("Auto-approving permission request from CLI")
	} else {
		logger.Info().
			Str("sessionId", params.SessionID).
			RawJSON("rawParams", resp.Params).
			Msg("Auto-approving permission request from CLI (no specific tools)")
	}

	// Determine the optionId to use
	optionID := "allow_once" // Default
	if len(params.Options) > 0 {
		// Prefer allow_always, then allow_once
		for _, opt := range params.Options {
			if opt.Kind == "allow_always" {
				optionID = opt.OptionID
				break
			}
			if opt.Kind == "allow_once" {
				optionID = opt.OptionID
			}
		}
	}

	logger.Debug().
		Str("optionId", optionID).
		Int("optionsCount", len(params.Options)).
		Msg("Selected permission option")

	// Build the permission response result
	result := RequestPermissionResult{
		OptionID:         optionID,
		SelectedOptionID: optionID,
		Outcome: &PermissionOutcome{
			Outcome: "approved",
		},
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal permission response")
		if resp.ID != nil {
			c.sendErrorResponse(*resp.ID, -32603, "failed to marshal permission response")
		}
		return
	}

	// Log the actual response being sent for debugging
	logger.Info().
		Str("sessionId", params.SessionID).
		Str("responseBody", string(resultBytes)).
		Msg("Sending permission approval")

	// Send response via JSON-RPC response (if request had an ID)
	if resp.ID != nil {
		c.sendResultResponse(*resp.ID, resultBytes)
		logger.Debug().Int("id", *resp.ID).Msg("Sent JSON-RPC permission response")
	}

	// Also send as a notification using permission.response/session/permission_response
	// This is how some CLI versions expect to receive the response
	notifyPayload := map[string]interface{}{
		"sessionId":        params.SessionID,
		"optionId":         optionID,
		"selectedOptionId": optionID,
		"outcome": map[string]string{
			"outcome": "approved",
		},
	}

	// Try new method first, then legacy
	method := c.methodName(MethodPermissionRespond, LegacyMethodPermissionResponse)
	if err := c.sendNotification(method, notifyPayload); err != nil {
		logger.Warn().Err(err).Str("method", method).Msg("Failed to send permission notification")
	} else {
		logger.Info().Str("method", method).Msg("Sent permission notification")
	}
}

// ========== ACP Protocol Methods ==========

// Initialize performs the ACP initialize handshake.
func (c *ACPClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion:    ACPProtocolVersion,
		ClientCapabilities: ClientCapabilities{},
	}

	resp, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	// Detect protocol version to decide method name format.
	// CLI versions supporting protocolVersion >= MinNewProtocolVersion use
	// dot-notation method names (session.create, session.send, etc.).
	c.useNewProtocol = result.ProtocolVersion >= MinNewProtocolVersion
	c.mu.Unlock()

	logger.Info().
		Str("agent", result.AgentInfo.Name).
		Str("version", result.AgentInfo.Version).
		Int("protocol", result.ProtocolVersion).
		Bool("newProtocol", c.useNewProtocol).
		Msg("ACP initialized")

	return &result, nil
}

// Deprecated: NewSession creates a new ACP session using legacy array-style MCP config.
// Use CreateSession instead.
func (c *ACPClient) NewSession(ctx context.Context, cwd string, mcpServers []MCPServer) (*NewSessionResult, error) {
	c.mu.Lock()
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized {
		return nil, ErrACPNotInitialized
	}

	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "/"
		}
	}

	// Ensure mcpServers is not nil (ACP requires an array, not null/undefined)
	if mcpServers == nil {
		mcpServers = []MCPServer{}
	}

	params := NewSessionParams{
		Cwd:        cwd,
		McpServers: mcpServers,
	}

	resp, err := c.sendRequest(ctx, "session/new", params)
	if err != nil {
		return nil, fmt.Errorf("session/new failed: %w", err)
	}

	var result NewSessionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse session/new result: %w", err)
	}

	logger.Info().Str("sessionId", result.SessionID).Msg("ACP session created")

	return &result, nil
}

// CreateSession creates a new ACP session with SDK-aligned parameters.
// Uses session.create on new CLI versions, falls back to session/new on older ones.
// For legacy protocol, it converts CreateSessionParams into NewSessionParams
// (cwd + array-style mcpServers) since the old CLI doesn't understand the new format.
func (c *ACPClient) CreateSession(ctx context.Context, params CreateSessionParams) (*NewSessionResult, error) {
	c.mu.Lock()
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized {
		return nil, ErrACPNotInitialized
	}

	method := c.methodName(MethodSessionCreate, LegacyMethodSessionNew)

	// Build the actual request payload — for legacy protocol, convert to old format
	var payload interface{}
	if c.useNewProtocol {
		payload = params
	} else {
		// Legacy CLI expects: { cwd: string, mcpServers: MCPServer[] }
		// with args/env as arrays, not nil or object
		legacyParams := NewSessionParams{
			Cwd: params.WorkingDirectory,
		}
		// Convert map-style MCPServers to array-style with proper format
		if len(params.MCPServers) > 0 {
			servers := make([]MCPServer, 0, len(params.MCPServers))
			for name, cfg := range params.MCPServers {
				srv := MCPServer{
					Name:    name,
					Type:    cfg.Type, // Field is "type" in JSON, not "transport"
					Command: cfg.Command,
					URL:     cfg.URL,
				}
				// Set tools - default to ["*"] if not specified
				if len(cfg.Tools) > 0 {
					srv.Tools = cfg.Tools
				} else {
					srv.Tools = []string{"*"} // 默认启用所有工具
				}
				// Ensure Args is non-nil array
				if cfg.Args != nil {
					srv.Args = cfg.Args
				} else {
					srv.Args = []string{}
				}
				// Convert Env map to array of {name, value}
				if len(cfg.Env) > 0 {
					srv.Env = make([]MCPEnvVar, 0, len(cfg.Env))
					for k, v := range cfg.Env {
						srv.Env = append(srv.Env, MCPEnvVar{Name: k, Value: v})
					}
				} else {
					srv.Env = []MCPEnvVar{}
				}
				// Convert Headers map to array of {name, value}
				if len(cfg.Headers) > 0 {
					srv.Headers = make([]MCPEnvVar, 0, len(cfg.Headers))
					for k, v := range cfg.Headers {
						srv.Headers = append(srv.Headers, MCPEnvVar{Name: k, Value: v})
					}
				}
				servers = append(servers, srv)
			}
			legacyParams.McpServers = servers
		} else {
			legacyParams.McpServers = []MCPServer{} // Must be array, not null
		}
		payload = legacyParams
		logger.Debug().
			Str("cwd", legacyParams.Cwd).
			Int("mcpServers", len(legacyParams.McpServers)).
			Msg("ACP: using legacy session/new params")
	}

	resp, err := c.sendRequest(ctx, method, payload)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", method, err)
	}

	var result NewSessionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse %s result: %w", method, err)
	}

	logger.Info().Str("sessionId", result.SessionID).Str("method", method).Msg("ACP session created")

	return &result, nil
}

// Prompt sends a prompt to the session and waits for completion.
// The actual streaming content and tool calls are delivered via notifications.
func (c *ACPClient) Prompt(ctx context.Context, sessionID, text string) (*PromptResult, error) {
	c.mu.Lock()
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized {
		return nil, ErrACPNotInitialized
	}

	params := PromptParams{
		SessionID: sessionID,
		Prompt: []PromptContent{
			{Type: "text", Text: text},
		},
	}

	logger.Debug().
		Str("sessionId", sessionID).
		Int("promptLen", len(text)).
		Msg("Sending ACP prompt")

	method := c.methodName(MethodSessionSend, LegacyMethodSessionPrompt)
	resp, err := c.sendRequest(ctx, method, params)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", method, err)
	}

	var result PromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse %s result: %w", method, err)
	}

	logger.Debug().
		Str("sessionId", sessionID).
		Str("stopReason", result.StopReason).
		Msg("ACP prompt completed")

	return &result, nil
}

// PromptWithContent sends a prompt with multimodal content (text + images).
func (c *ACPClient) PromptWithContent(ctx context.Context, sessionID string, content []PromptContent) (*PromptResult, error) {
	c.mu.Lock()
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized {
		return nil, ErrACPNotInitialized
	}

	params := PromptParams{
		SessionID: sessionID,
		Prompt:    content,
	}

	logger.Debug().
		Str("sessionId", sessionID).
		Int("contentCount", len(content)).
		Msg("Sending ACP prompt with multimodal content")

	method := c.methodName(MethodSessionSend, LegacyMethodSessionPrompt)
	resp, err := c.sendRequest(ctx, method, params)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", method, err)
	}

	var result PromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse %s result: %w", method, err)
	}

	logger.Debug().
		Str("sessionId", sessionID).
		Str("stopReason", result.StopReason).
		Msg("ACP prompt with multimodal content completed")

	return &result, nil
}

// RespondToPermission responds to a permission request.
func (c *ACPClient) RespondToPermission(sessionID string, approved bool) error {
	outcome := "denied"
	if approved {
		outcome = "approved"
	}

	result := RequestPermissionResult{
		Outcome: &PermissionOutcome{
			Outcome: outcome,
		},
	}

	return c.sendNotification(c.methodName(MethodPermissionRespond, LegacyMethodPermissionResponse), map[string]interface{}{
		"sessionId": sessionID,
		"result":    result,
	})
}

// Close closes the ACP client and terminates the CLI process.
func (c *ACPClient) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	logger.Info().Msg("Closing ACP client")

	// Close stdin to signal EOF to the process
	if c.stdin != nil {
		c.stdin.Close()
	}

	// Give the process a moment to exit gracefully
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(5 * time.Second):
		// Force kill
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
	}

	// Drain all pending response channels so blocked callers can wake up
	c.responsesMu.Lock()
	for id, ch := range c.responses {
		close(ch)
		delete(c.responses, id)
	}
	c.responsesMu.Unlock()

	return nil
}

// IsClosed returns true if the client has been closed.
func (c *ACPClient) IsClosed() bool {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.closed
}

// IsInitialized returns true if the client has been initialized.
func (c *ACPClient) IsInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
