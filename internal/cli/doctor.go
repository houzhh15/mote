package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"mote/internal/config"
	"mote/internal/provider/copilot"
)

// NewDoctorCmd creates the doctor command.
func NewDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose system health",
		Long: `Run diagnostic checks on your Mote installation.

This command checks:
- Configuration file validity
- Authentication token status
- Network connectivity
- Server status
- MCP server connections
- Database accessibility`,
		RunE: runDoctor,
	}

	return cmd
}

type checkResult struct {
	name    string
	status  string // ok, warning, error
	message string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("Mote Doctor")
	fmt.Println("===========")
	fmt.Println()

	var results []checkResult

	// 1. Check Go version and system info
	results = append(results, checkSystemInfo())

	// 2. Check config file
	results = append(results, checkConfigFile())

	// 3. Check token
	results = append(results, checkToken())

	// 4. Check Copilot API access
	results = append(results, checkCopilotAPI())

	// 5. Check data directory
	results = append(results, checkDataDirectory())

	// 6. Check server connectivity
	results = append(results, checkServerConnectivity())

	// Print results
	fmt.Println()
	hasErrors := false
	hasWarnings := false

	for _, r := range results {
		icon := "✓"
		if r.status == "warning" {
			icon = "⚠️"
			hasWarnings = true
		} else if r.status == "error" {
			icon = "✗"
			hasErrors = true
		}

		fmt.Printf("%s %s: %s\n", icon, r.name, r.message)
	}

	// Summary
	fmt.Println()
	if hasErrors {
		fmt.Println("❌ Some checks failed. Please address the issues above.")
		return nil
	} else if hasWarnings {
		fmt.Println("⚠️  Some warnings detected. Your setup should work but may have issues.")
	} else {
		fmt.Println("✅ All checks passed! Mote is ready to use.")
	}

	return nil
}

func checkSystemInfo() checkResult {
	return checkResult{
		name:   "System",
		status: "ok",
		message: fmt.Sprintf("Go %s on %s/%s",
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		),
	}
}

func checkConfigFile() checkResult {
	configPath, err := config.DefaultConfigPath()
	if err != nil {
		return checkResult{
			name:    "Config File",
			status:  "error",
			message: fmt.Sprintf("Cannot determine config path: %v", err),
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return checkResult{
			name:    "Config File",
			status:  "warning",
			message: fmt.Sprintf("Not found: %s (using defaults)", configPath),
		}
	}

	// Try to load the config
	cfg, err := config.Load(configPath)
	if err != nil {
		return checkResult{
			name:    "Config File",
			status:  "error",
			message: fmt.Sprintf("Invalid config: %v", err),
		}
	}

	// Check for common misconfigurations
	if cfg.Gateway.Port == 0 {
		return checkResult{
			name:    "Config File",
			status:  "warning",
			message: fmt.Sprintf("Found: %s (gateway.port not set, using default)", configPath),
		}
	}

	return checkResult{
		name:    "Config File",
		status:  "ok",
		message: fmt.Sprintf("Found: %s", configPath),
	}
}

func checkToken() checkResult {
	configPath, _ := config.DefaultConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return checkResult{
			name:    "Authentication",
			status:  "error",
			message: "Cannot load config to check token",
		}
	}

	if cfg.Copilot.Token == "" {
		return checkResult{
			name:    "Authentication",
			status:  "error",
			message: "No token configured. Run: mote auth login",
		}
	}

	// Basic token format validation
	token := cfg.Copilot.Token
	if len(token) < 10 {
		return checkResult{
			name:    "Authentication",
			status:  "error",
			message: "Token appears invalid (too short)",
		}
	}

	// Mask the token for display
	maskedToken := token[:4] + "..." + token[len(token)-4:]

	return checkResult{
		name:    "Authentication",
		status:  "ok",
		message: fmt.Sprintf("Token configured (%s)", maskedToken),
	}
}

func checkCopilotAPI() checkResult {
	configPath, _ := config.DefaultConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil || cfg.Copilot.Token == "" {
		return checkResult{
			name:    "Copilot API",
			status:  "warning",
			message: "Skipped (no token configured)",
		}
	}

	// Try to get a Copilot token
	tokenMgr := copilot.NewTokenManager(cfg.Copilot.Token)
	_, err = tokenMgr.GetToken()
	if err != nil {
		return checkResult{
			name:    "Copilot API",
			status:  "error",
			message: fmt.Sprintf("Failed to get Copilot token: %v", err),
		}
	}

	return checkResult{
		name:    "Copilot API",
		status:  "ok",
		message: "Successfully obtained Copilot API token",
	}
}

func checkDataDirectory() checkResult {
	dataPath, err := config.DefaultDataPath()
	if err != nil {
		return checkResult{
			name:    "Data Directory",
			status:  "error",
			message: fmt.Sprintf("Cannot determine data path: %v", err),
		}
	}

	dir := filepath.Dir(dataPath)

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return checkResult{
			name:    "Data Directory",
			status:  "warning",
			message: fmt.Sprintf("Will be created: %s", dir),
		}
	}

	// Check if we can write to it
	testFile := filepath.Join(dir, ".mote-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return checkResult{
			name:    "Data Directory",
			status:  "error",
			message: fmt.Sprintf("Cannot write to: %s", dir),
		}
	}
	os.Remove(testFile)

	// Check for database file
	dbPath := dataPath
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return checkResult{
			name:    "Data Directory",
			status:  "ok",
			message: fmt.Sprintf("Ready: %s (database will be created on first run)", dir),
		}
	}

	// Get database size
	info, err := os.Stat(dbPath)
	if err == nil {
		sizeMB := float64(info.Size()) / 1024 / 1024
		return checkResult{
			name:    "Data Directory",
			status:  "ok",
			message: fmt.Sprintf("Found: %s (database: %.2f MB)", dir, sizeMB),
		}
	}

	return checkResult{
		name:    "Data Directory",
		status:  "ok",
		message: fmt.Sprintf("Found: %s", dir),
	}
}

func checkServerConnectivity() checkResult {
	// Try to connect to the local server
	client := &http.Client{Timeout: 5 * time.Second}

	// Try default port
	ports := []int{18788, 8080}
	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/api/v1/health", port)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var health map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
				status, _ := health["status"].(string)
				return checkResult{
					name:    "Server",
					status:  "ok",
					message: fmt.Sprintf("Running on port %d (status: %s)", port, status),
				}
			}
			return checkResult{
				name:    "Server",
				status:  "ok",
				message: fmt.Sprintf("Running on port %d", port),
			}
		}
	}

	return checkResult{
		name:    "Server",
		status:  "warning",
		message: "Not running. Start with: mote serve",
	}
}

// checkMCPServers checks MCP server connectivity (only if server is running)
//
//nolint:unused // Future use
func checkMCPServers(serverURL string) checkResult {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/mcp/servers", serverURL))
	if err != nil {
		return checkResult{
			name:    "MCP Servers",
			status:  "warning",
			message: "Cannot check (server not running)",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return checkResult{
			name:    "MCP Servers",
			status:  "error",
			message: fmt.Sprintf("API error: %s", string(body)),
		}
	}

	var response struct {
		Servers []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"servers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return checkResult{
			name:    "MCP Servers",
			status:  "warning",
			message: "Cannot parse response",
		}
	}

	if len(response.Servers) == 0 {
		return checkResult{
			name:    "MCP Servers",
			status:  "ok",
			message: "No servers configured",
		}
	}

	connected := 0
	for _, s := range response.Servers {
		if s.Status == "connected" || s.Status == "Connected" {
			connected++
		}
	}

	return checkResult{
		name:   "MCP Servers",
		status: "ok",
		message: fmt.Sprintf("%d/%d connected",
			connected,
			len(response.Servers),
		),
	}
}
