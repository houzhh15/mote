// Package main contains the application lifecycle management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"mote/gui/internal/backend"
	"mote/gui/internal/embedded"
	"mote/gui/internal/ipcbridge"
	"mote/internal/cli/defaults"
	"mote/internal/storage"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// CopilotConfig represents the copilot section in config.
type CopilotConfig struct {
	Token string `yaml:"token"`
}

// GatewayConfig represents the gateway section in config.
type GatewayConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// MoteConfig represents the mote configuration file.
type MoteConfig struct {
	Gateway GatewayConfig `yaml:"gateway"`
	Copilot CopilotConfig `yaml:"copilot"`
}

// Default port for mote server
const defaultServerPort = 18788

// App struct holds the application state and dependencies.
type App struct {
	ctx            context.Context
	apiClient      *backend.APIClient
	embeddedServer *embedded.Server
	ipcBridge      *ipcbridge.Bridge
	logger         zerolog.Logger
	deviceCode     string
	serverPort     int      // Configured server port
	chatCancels    sync.Map // map[string]context.CancelFunc — per-session cancel for ChatStream
}

// NewApp creates a new App application struct.
func NewApp() *App {
	homeDir, _ := os.UserHomeDir()
	logDir := filepath.Join(homeDir, ".mote", "logs")
	_ = os.MkdirAll(logDir, 0755)

	logFile, err := os.OpenFile(
		filepath.Join(logDir, "gui.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	var logger zerolog.Logger
	if err != nil {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(logFile).With().Timestamp().Logger()
	}

	app := &App{
		apiClient:  backend.NewAPIClient(fmt.Sprintf("http://localhost:%d", defaultServerPort), 30*time.Second),
		logger:     logger,
		serverPort: defaultServerPort, // Will be updated from config in startup
	}

	// Create IPC bridge with callbacks
	app.ipcBridge = ipcbridge.NewBridge(
		ipcbridge.WithLogger(logger),
		ipcbridge.WithOnTrayReady(func() {
			logger.Info().Msg("Tray process is ready")
		}),
		ipcbridge.WithOnBubbleReady(func() {
			logger.Info().Msg("Bubble process is ready")
		}),
		ipcbridge.WithOnNotification(func(title, message string) {
			logger.Info().Str("title", title).Str("message", message).Msg("Received notification")
		}),
		ipcbridge.WithOnShowBubble(func(query string) {
			logger.Info().Str("query", query).Msg("Show bubble requested")
		}),
		ipcbridge.WithOnServiceStatus(func(running bool) {
			logger.Info().Bool("running", running).Msg("Service status changed")
		}),
		ipcbridge.WithOnQuit(func() {
			logger.Info().Msg("Quit request from tray, triggering application quit")
			if app.ctx != nil {
				runtime.Quit(app.ctx)
			}
		}),
		ipcbridge.WithOnShowWindow(func() {
			logger.Info().Msg("Show window request from tray")
			if app.ctx != nil {
				runtime.WindowShow(app.ctx)
				runtime.WindowUnminimise(app.ctx)
			}
		}),
		ipcbridge.WithOnHideWindow(func() {
			logger.Info().Msg("Hide window request from tray")
			if app.ctx != nil {
				runtime.WindowMinimise(app.ctx)
			}
		}),
		ipcbridge.WithOnRestartService(func() {
			logger.Info().Msg("Restart service request from tray")
			if err := app.RestartService(); err != nil {
				logger.Error().Err(err).Msg("Failed to restart service")
			}
		}),
	)

	return app
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logger.Info().Msg("Mote GUI starting")

	// Start IPC server first
	if err := a.ipcBridge.Start(); err != nil {
		a.logger.Error().Err(err).Msg("Failed to start IPC server")
	} else {
		a.logger.Info().Msg("IPC server started")
	}

	// Check if mote needs initialization (first run)
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".mote")
	configPath := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		a.logger.Info().Msg("First run detected, initializing mote configuration...")
		if err := a.initializeMote(configDir, configPath); err != nil {
			a.logger.Error().Err(err).Msg("Failed to initialize mote configuration")
			// Continue anyway - embedded server will report more specific errors
		} else {
			a.logger.Info().Str("path", configDir).Msg("Mote configuration initialized successfully")
		}
	}

	// Load configuration to get server port
	cfg, err := a.loadConfig()
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to load config, using default port")
	} else if cfg.Gateway.Port > 0 && cfg.Gateway.Port != a.serverPort {
		// Port configured differently, update and recreate API client
		a.serverPort = cfg.Gateway.Port
		apiURL := fmt.Sprintf("http://localhost:%d", a.serverPort)
		a.apiClient = backend.NewAPIClient(apiURL, 30*time.Second)
		a.logger.Info().Int("port", a.serverPort).Msg("API client updated with configured port")
	}

	a.embeddedServer, err = embedded.NewServer(embedded.ServerConfig{
		ConfigPath: configPath,
		Logger:     a.logger,
		OnStateChange: func(running bool) {
			a.logger.Info().Bool("running", running).Msg("Embedded server state changed")
			_ = a.ipcBridge.BroadcastServiceStatus(running)
		},
	})
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to create embedded server")
		return
	}

	// Start embedded server in goroutine
	a.logger.Info().Msg("Starting embedded mote server...")
	if err := a.embeddedServer.Start(); err != nil {
		a.logger.Error().Err(err).Msg("Failed to start embedded server")
		return
	}
	a.logger.Info().Msg("Embedded mote server started successfully")

	// Broadcast service status via IPC
	_ = a.ipcBridge.BroadcastServiceStatus(true)

	// Start system tray via IPC
	a.startSystemTray()
}

// initializeMote creates the initial mote configuration directory and files.
// This is equivalent to running "mote init" for first-time setup.
func (a *App) initializeMote(configDir, configPath string) error {
	// Create directory structure
	dirs := []string{
		configDir,
		filepath.Join(configDir, "logs"),
		filepath.Join(configDir, "ui"),
		filepath.Join(configDir, "tools"),
		filepath.Join(configDir, "skills"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Generate default configuration (using same defaults as mote init)
	defaultConfig := map[string]any{
		"gateway": map[string]any{
			"port": 18788,
			"host": "127.0.0.1",
		},
		"provider": map[string]any{
			"default": "copilot",
		},
		"ollama": map[string]any{
			"endpoint":   "http://localhost:11434",
			"model":      "llama3.2",
			"timeout":    "5m",
			"keep_alive": "5m",
		},
		"log": map[string]any{
			"level":  "info",
			"format": "console",
		},
		"memory": map[string]any{
			"enabled": true,
		},
		"cron": map[string]any{
			"enabled": true,
		},
		"mcp": map[string]any{
			"server": map[string]any{
				"enabled":   true,
				"transport": "stdio",
			},
		},
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Initialize database (same as mote init)
	dataPath := filepath.Join(configDir, "data.db")
	db, err := storage.Open(dataPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	db.Close()
	a.logger.Info().Str("path", dataPath).Msg("Database initialized")

	// Copy default skills (same as mote init)
	if err := a.copyDefaultSkills(configDir); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to copy default skills")
		// Continue anyway - not critical
	}

	return nil
}

// copyDefaultSkills copies embedded default skills to the user's skills directory.
// This is equivalent to the CLI's copyDefaultSkills function.
func (a *App) copyDefaultSkills(configDir string) error {
	skillsDir := filepath.Join(configDir, "skills")
	defaultsFS := defaults.GetDefaultsFS()

	// Walk the embedded skills directory
	return fs.WalkDir(defaultsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root skills directory itself
		if path == "skills" {
			return nil
		}

		// Calculate the relative path and destination
		relPath, err := filepath.Rel("skills", path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(skillsDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Skip if file already exists (don't overwrite user modifications)
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}

		// Read embedded file
		data, err := defaultsFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Write file
		a.logger.Debug().Str("path", destPath).Msg("Copied default skill")
		return os.WriteFile(destPath, data, 0644)
	})
}

// startSystemTray starts the mote-tray application via IPC.
func (a *App) startSystemTray() {
	// Find mote-tray executable
	var trayPath string
	trayName := "mote-tray"
	if goruntime.GOOS == "windows" {
		trayName = "mote-tray.exe"
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)

		// macOS: Look in Contents/Helpers/Mote Tray.app/Contents/MacOS/mote-tray
		if goruntime.GOOS == "darwin" {
			bundlePath := filepath.Join(exeDir, "..", "Helpers", "Mote Tray.app", "Contents", "MacOS", "mote-tray")
			if _, err := os.Stat(bundlePath); err == nil {
				trayPath = bundlePath
			}
		}

		// Fallback: Resources directory (development mode / macOS legacy)
		if trayPath == "" {
			resourcePath := filepath.Join(exeDir, "..", "Resources", trayName)
			if _, err := os.Stat(resourcePath); err == nil {
				trayPath = resourcePath
			}
		}

		// Fallback: same directory (development mode / Windows / Linux)
		if trayPath == "" {
			sameDirPath := filepath.Join(exeDir, trayName)
			if _, err := os.Stat(sameDirPath); err == nil {
				trayPath = sameDirPath
			}
		}
	}

	if trayPath == "" {
		a.logger.Debug().Msg("mote-tray not found, skipping system tray")
		return
	}

	// Start tray via IPC bridge (managed subprocess)
	if err := a.ipcBridge.StartTray(trayPath); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to start system tray via IPC")
		return
	}

	a.logger.Info().Str("path", trayPath).Msg("System tray started via IPC")
}

// shutdown is called when the app terminates.
func (a *App) shutdown(ctx context.Context) {
	a.logger.Info().Msg("Mote GUI shutting down")

	// Stop IPC bridge and subprocesses (tray, bubble)
	if a.ipcBridge != nil {
		if err := a.ipcBridge.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("Failed to stop IPC bridge")
		}
	}

	// Stop embedded server
	if a.embeddedServer != nil && a.embeddedServer.IsRunning() {
		if err := a.embeddedServer.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("Failed to stop embedded server")
		}
	}

	a.logger.Info().Msg("Mote GUI shutdown complete")
}

// beforeClose is called when the user tries to close the window.
// Returns false to allow closing, true to prevent it.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// Allow the window to close normally
	// The shutdown callback will handle cleanup
	return false
}

// GetVersion returns the application version.
func (a *App) GetVersion() string {
	return "1.0.0"
}

// ShowWindow shows the main window.
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// HideWindow hides the main window.
func (a *App) HideWindow() {
	runtime.WindowMinimise(a.ctx)
}

// RestartService restarts the mote service.
func (a *App) RestartService() error {
	a.logger.Info().Msg("Restarting embedded mote server")

	// Stop embedded server
	if a.embeddedServer != nil {
		if err := a.embeddedServer.Stop(); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	}

	// Broadcast service stopped via IPC
	_ = a.ipcBridge.BroadcastServiceStatus(false)

	// Restart embedded server
	if err := a.embeddedServer.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Broadcast service started via IPC
	_ = a.ipcBridge.BroadcastServiceStatus(true)

	a.logger.Info().Msg("Embedded mote server restarted successfully")
	return nil
}

// Quit closes the application completely.
func (a *App) Quit() {
	a.logger.Info().Msg("User requested quit")
	runtime.Quit(a.ctx)
}

// =============================================
// API methods exposed to Wails frontend
// =============================================

// =============================================
// Unified API Proxy - Single entry point for all API calls
// =============================================

// CallAPI executes an HTTP API call to the mote backend service.
// This is the unified entry point for all API calls from the frontend.
// method: HTTP method (GET, POST, PUT, DELETE, PATCH)
// path: Full API path (e.g., "/api/v1/sessions")
// bodyJSON: Request body as JSON string (empty string for no body)
// Returns: JSON response as byte array
func (a *App) CallAPI(method, path, bodyJSON string) ([]byte, error) {
	return a.apiClient.CallAPI(method, path, bodyJSON)
}

// ChatStream sends a chat message and streams the response via Wails events.
// Events are emitted with the name "chat:stream:{sessionID}" containing JSON event data.
// This method returns when the stream is complete.
func (a *App) ChatStream(requestJSON string) error {
	apiURL := fmt.Sprintf("http://localhost:%d/api/v1/chat/stream", a.serverPort)

	// Parse the incoming JSON to inject stream:true, then re-marshal
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(requestJSON), &reqBody); err != nil {
		return fmt.Errorf("failed to parse request JSON: %w", err)
	}
	reqBody["stream"] = true

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create cancellable context so CancelChat can abort this stream.
	streamCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Extract session ID and store the cancel function.
	sessionID, _ := reqBody["session_id"].(string)
	if sessionID != "" {
		// Cancel any previous ChatStream for this session.
		if prev, loaded := a.chatCancels.LoadAndDelete(sessionID); loaded {
			prev.(context.CancelFunc)()
		}
		a.chatCancels.Store(sessionID, cancel)
		defer a.chatCancels.Delete(sessionID)
	}

	req, err := http.NewRequestWithContext(streamCtx, "POST", apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{} // Timeout controlled by streamCtx
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: %s", string(body))
	}

	// Use session-specific event name for isolation between concurrent chats
	eventName := fmt.Sprintf("chat:stream:%s", sessionID)

	// Read SSE events and emit them via Wails events
	reader := resp.Body
	buffer := make([]byte, 0, 4096)
	chunk := make([]byte, 1024)

	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)

			// Process complete lines
			for {
				idx := -1
				for i := 0; i < len(buffer)-1; i++ {
					if buffer[i] == '\n' && buffer[i+1] == '\n' {
						idx = i
						break
					}
				}
				if idx == -1 {
					break
				}

				line := string(buffer[:idx])
				buffer = buffer[idx+2:]

				if strings.HasPrefix(line, "data: ") {
					eventData := line[6:]
					runtime.EventsEmit(a.ctx, eventName, eventData)
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading stream: %w", err)
		}
	}

	return nil
}

// CancelChat cancels a running chat stream for the given session.
// It aborts the HTTP request to the embedded server and also calls
// the cancel API endpoint to stop the runner execution.
func (a *App) CancelChat(sessionID string) error {
	// 1. Cancel the ChatStream HTTP request (if still running)
	if cancel, loaded := a.chatCancels.LoadAndDelete(sessionID); loaded {
		cancel.(context.CancelFunc)()
	}

	// 2. Call the backend cancel API to stop the runner task
	apiURL := fmt.Sprintf("http://localhost:%d/api/v1/sessions/%s/cancel", a.serverPort, url.PathEscape(sessionID))
	ctx, cancelReq := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelReq()
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.logger.Warn().Err(err).Str("sessionID", sessionID).Msg("cancel API call failed")
		return nil // Non-fatal: ChatStream cancel already done above
	}
	defer resp.Body.Close()
	a.logger.Info().Str("sessionID", sessionID).Msg("CancelChat completed")
	return nil
}

// =============================================
// Service Status (GUI-specific, not proxied)
// =============================================

// GetServiceStatus returns the current service status.
// This is GUI-specific as it includes local process and IPC info.
func (a *App) GetServiceStatus() map[string]interface{} {
	isRunning := a.embeddedServer != nil && a.embeddedServer.IsRunning()
	return map[string]interface{}{
		"running":          isRunning,
		"port":             a.serverPort,
		"url":              fmt.Sprintf("http://localhost:%d", a.serverPort),
		"tray_connected":   a.ipcBridge.IsTrayConnected(),
		"bubble_connected": a.ipcBridge.IsBubbleConnected(),
	}
}

// =============================================
// IPC methods (GUI-specific)
// =============================================

// ShowBubble shows the bubble window with optional query
func (a *App) ShowBubble(query string) error {
	// Find bubble executable
	var bubblePath string
	bubbleName := "mote-bubble"
	if goruntime.GOOS == "windows" {
		bubbleName = "mote-bubble.exe"
	}

	if exePath, err := os.Executable(); err == nil {
		// First try Helpers directory (macOS standard)
		if goruntime.GOOS == "darwin" {
			helpersPath := filepath.Join(filepath.Dir(exePath), "..", "Helpers", "Mote Bubble.app", "Contents", "MacOS", "mote-bubble")
			if _, err := os.Stat(helpersPath); err == nil {
				bubblePath = helpersPath
			}
		}

		// Fallback to Resources directory (legacy / macOS)
		if bubblePath == "" {
			resourcePath := filepath.Join(filepath.Dir(exePath), "..", "Resources", bubbleName)
			if _, err := os.Stat(resourcePath); err == nil {
				bubblePath = resourcePath
			}
		}

		// Fallback to same directory as executable (Windows / Linux / dev mode)
		if bubblePath == "" {
			sameDirPath := filepath.Join(filepath.Dir(exePath), bubbleName)
			if _, err := os.Stat(sameDirPath); err == nil {
				bubblePath = sameDirPath
			}
		}
	}

	if bubblePath == "" {
		return fmt.Errorf("mote-bubble not found")
	}

	// Start bubble if not connected
	if !a.ipcBridge.IsBubbleConnected() {
		if err := a.ipcBridge.StartBubble(bubblePath); err != nil {
			return fmt.Errorf("failed to start bubble: %w", err)
		}
	}

	// Send show command
	return a.ipcBridge.SendShowBubble(query)
}

// HideBubble hides the bubble window
func (a *App) HideBubble() error {
	return a.ipcBridge.SendHideBubble()
}

// SendNotification sends a notification via tray
func (a *App) SendNotification(title, message string) error {
	if !a.ipcBridge.IsTrayConnected() {
		return fmt.Errorf("tray not connected")
	}
	return a.ipcBridge.SendNotificationToTray(title, message)
}

// GetIPCStatus returns IPC connection status
func (a *App) GetIPCStatus() map[string]interface{} {
	return map[string]interface{}{
		"tray_connected":   a.ipcBridge.IsTrayConnected(),
		"bubble_connected": a.ipcBridge.IsBubbleConnected(),
	}
}

// =============================================
// API methods exposed to Wails frontend
// =============================================

// AuthStatus represents the authentication status response.
type AuthStatus struct {
	Authenticated   bool   `json:"authenticated"`
	TokenMasked     string `json:"token_masked,omitempty"`
	CopilotVerified bool   `json:"copilot_verified,omitempty"`
	Error           string `json:"error,omitempty"`
}

// DeviceCodeResponse represents the device code for OAuth flow.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// AuthResult represents the result of authentication.
type AuthResult struct {
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	Interval int    `json:"interval,omitempty"` // Suggested polling interval in seconds
}

// GetAuthStatus returns the current authentication status.
func (a *App) GetAuthStatus() (*AuthStatus, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}

	status := &AuthStatus{
		Authenticated: cfg.Copilot.Token != "",
	}

	if status.Authenticated {
		status.TokenMasked = maskToken(cfg.Copilot.Token)
	}

	return status, nil
}

// StartDeviceLogin initiates the OAuth device flow for GitHub Copilot.
func (a *App) StartDeviceLogin() (*DeviceCodeResponse, error) {
	a.logger.Info().Msg("StartDeviceLogin called")

	// Import and use copilot auth manager
	authMgr := newCopilotAuthManager()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deviceResp, err := authMgr.RequestDeviceCode(ctx)
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to request device code")
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}

	a.logger.Info().
		Str("user_code", deviceResp.UserCode).
		Str("verification_uri", deviceResp.VerificationURI).
		Msg("Device code received")

	// Store device code for polling
	a.deviceCode = deviceResp.DeviceCode

	// Open browser using native command (more reliable in .app bundle on macOS)
	a.logger.Info().Str("url", deviceResp.VerificationURI).Msg("Opening browser")
	if err := openBrowser(deviceResp.VerificationURI); err != nil {
		a.logger.Warn().Err(err).Msg("Native browser open failed, trying Wails runtime")
		// Fallback to Wails runtime
		if a.ctx != nil {
			runtime.BrowserOpenURL(a.ctx, deviceResp.VerificationURI)
		}
	}

	return &DeviceCodeResponse{
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		ExpiresIn:       deviceResp.ExpiresIn,
		Interval:        deviceResp.Interval,
	}, nil
}

// PollDeviceLogin polls for the completion of device login.
// This should be called repeatedly by the frontend until success or error.
func (a *App) PollDeviceLogin(deviceCode string) (*AuthResult, error) {
	authMgr := newCopilotAuthManager()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a.logger.Debug().Str("device_code", deviceCode).Msg("Polling for device login completion")

	tokenResp, err := authMgr.PollOnce(ctx, deviceCode, a.logger)
	if err != nil {
		// Check if still pending or slow_down
		if isPendingError(err) {
			result := &AuthResult{Success: false, Error: "pending"}
			// If we have a response with interval, pass it to frontend
			if tokenResp != nil && tokenResp.Interval > 0 {
				result.Interval = tokenResp.Interval
				a.logger.Debug().Int("interval", tokenResp.Interval).Msg("Authorization pending, suggesting new interval")
			} else {
				a.logger.Debug().Msg("Authorization still pending")
			}
			return result, nil
		}
		a.logger.Error().Err(err).Msg("Device login polling failed")
		return &AuthResult{Success: false, Error: err.Error()}, nil
	}

	a.logger.Info().Msg("Device login successful, saving token")

	// Save token to config
	cfg, err := a.loadConfig()
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to load config")
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	cfg.Copilot.Token = tokenResp.AccessToken

	if err := a.saveConfig(cfg); err != nil {
		a.logger.Error().Err(err).Msg("Failed to save config")
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	a.logger.Info().Msg("Token saved, restarting service")

	// Restart service to pick up new config
	if err := a.RestartService(); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to restart service after auth")
	}

	return &AuthResult{Success: true}, nil
}

// Logout removes the authentication token.
func (a *App) Logout() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}

	cfg.Copilot.Token = ""

	return a.saveConfig(cfg)
}

// =============================================
// Config helper methods
// =============================================

func (a *App) loadConfig() (*MoteConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".mote", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &MoteConfig{}, nil
		}
		return nil, err
	}

	var cfg MoteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (a *App) saveConfig(cfg *MoteConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(homeDir, ".mote", "config.yaml")

	// Read existing config to preserve other fields
	existingData, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var fullConfig map[string]interface{}
	if len(existingData) > 0 {
		if err := yaml.Unmarshal(existingData, &fullConfig); err != nil {
			fullConfig = make(map[string]interface{})
		}
	} else {
		fullConfig = make(map[string]interface{})
	}

	// Update copilot section
	fullConfig["copilot"] = map[string]interface{}{
		"token": cfg.Copilot.Token,
	}

	data, err := yaml.Marshal(fullConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// =============================================
// Simplified Copilot Auth Manager (embedded)
// =============================================

const (
	gitHubDeviceCodeURL  = "https://github.com/login/device/code"
	gitHubAccessTokenURL = "https://github.com/login/oauth/access_token"
	copilotClientID      = "Iv1.b507a08c87ecfe98"
	copilotScope         = "copilot"
)

type copilotAuthManager struct {
	httpClient *http.Client
}

type copilotDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type copilotAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
	Interval    int    `json:"interval,omitempty"` // New interval when slow_down
}

func newCopilotAuthManager() *copilotAuthManager {
	return &copilotAuthManager{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (am *copilotAuthManager) RequestDeviceCode(ctx context.Context) (*copilotDeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", copilotClientID)
	data.Set("scope", copilotScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gitHubDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: %s", string(body))
	}

	var dcResp copilotDeviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, err
	}

	return &dcResp, nil
}

func (am *copilotAuthManager) PollOnce(ctx context.Context, deviceCode string, logger zerolog.Logger) (*copilotAccessTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", copilotClientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gitHubAccessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Log raw response for debugging
	logger.Debug().
		Int("status_code", resp.StatusCode).
		Str("body", string(body)).
		Msg("GitHub OAuth response")

	var tokenResp copilotAccessTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}

	// GitHub returns error field when authorization is pending or failed
	// Return the response with the error so caller can access interval
	if tokenResp.Error != "" {
		return &tokenResp, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Success case: access token should be present
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response (body: %s)", string(body))
	}

	return &tokenResp, nil
}

func isPendingError(err error) bool {
	return strings.Contains(err.Error(), "authorization_pending") ||
		strings.Contains(err.Error(), "slow_down")
}

// openBrowser opens a URL in the default browser across different platforms.
func openBrowser(urlStr string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "windows":
		// Use rundll32 which handles URLs more reliably than cmd /c start
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", urlStr)
	}
	return cmd.Run()
}
