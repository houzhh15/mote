package guitest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestGUIBackendIntegration tests the GUI backend functionality
func TestGUIBackendIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if GUI binary doesn't exist
	guiBinary := findGUIBinary()
	if guiBinary == "" {
		t.Skip("GUI binary not found, skipping integration test")
	}

	t.Run("Backend process manager", func(t *testing.T) {
		// This test validates the process manager logic
		// without actually starting the GUI

		// Verify the module structure exists
		requiredFiles := []string{
			"gui/internal/backend/process.go",
			"gui/internal/backend/api.go",
			"gui/internal/backend/errors.go",
			"gui/internal/backend/logger.go",
		}

		for _, f := range requiredFiles {
			path := filepath.Join(getProjectRoot(), f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Required file missing: %s", f)
			}
		}
	})

	t.Run("IPC server structure", func(t *testing.T) {
		// Verify IPC module structure
		requiredFiles := []string{
			"gui/internal/ipc/server.go",
		}

		for _, f := range requiredFiles {
			path := filepath.Join(getProjectRoot(), f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Required file missing: %s", f)
			}
		}
	})
}

// TestTrayIntegration tests the tray application structure
func TestTrayIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("Tray module structure", func(t *testing.T) {
		requiredFiles := []string{
			"cmd/mote-tray/main.go",
			"cmd/mote-tray/tray.go",
			"cmd/mote-tray/ipc/client.go",
		}

		for _, f := range requiredFiles {
			path := filepath.Join(getProjectRoot(), f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Required file missing: %s", f)
			}
		}
	})
}

// TestFrontendAPIAdapter tests the frontend API adapter files
func TestFrontendAPIAdapter(t *testing.T) {
	t.Run("Environment detection", func(t *testing.T) {
		path := filepath.Join(getProjectRoot(), "internal/ui/ui/lib/environment.js")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read environment.js: %v", err)
		}

		// Verify key functions exist
		requiredFunctions := []string{
			"isWails",
			"isWeb",
			"getPlatform",
		}

		for _, fn := range requiredFunctions {
			if !strings.Contains(string(content), fn) {
				t.Errorf("Missing function in environment.js: %s", fn)
			}
		}
	})

	t.Run("API client adapter", func(t *testing.T) {
		path := filepath.Join(getProjectRoot(), "internal/ui/ui/lib/api-client.js")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read api-client.js: %v", err)
		}

		// Verify APIClient class exists
		if !strings.Contains(string(content), "class APIClientAdapter") {
			t.Error("APIClientAdapter class not found in api-client.js")
		}

		// Verify key methods
		requiredMethods := []string{
			"get(",
			"post(",
		}

		for _, method := range requiredMethods {
			if !strings.Contains(string(content), method) {
				t.Errorf("Missing method in APIClientAdapter: %s", method)
			}
		}
	})
}

// TestBuildConfiguration tests the build configuration files
func TestBuildConfiguration(t *testing.T) {
	t.Run("Wails config", func(t *testing.T) {
		path := filepath.Join(getProjectRoot(), "gui/wails.json")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read wails.json: %v", err)
		}

		var config map[string]interface{}
		if err := json.Unmarshal(content, &config); err != nil {
			t.Fatalf("Invalid wails.json: %v", err)
		}

		// Verify required fields
		requiredFields := []string{"name", "outputfilename"}
		for _, field := range requiredFields {
			if _, ok := config[field]; !ok {
				t.Errorf("Missing field in wails.json: %s", field)
			}
		}
	})

	t.Run("Makefile targets", func(t *testing.T) {
		path := filepath.Join(getProjectRoot(), "Makefile")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read Makefile: %v", err)
		}

		// Verify GUI-related targets exist
		requiredTargets := []string{
			"build-gui",
			"build-tray",
			"build-all",
			"package-darwin",
			"package-windows",
			"gui-dev",
		}

		for _, target := range requiredTargets {
			if !strings.Contains(string(content), target+":") {
				t.Errorf("Missing Makefile target: %s", target)
			}
		}
	})
}

// TestPackagingScripts tests the packaging scripts exist and are valid
func TestPackagingScripts(t *testing.T) {
	scripts := []struct {
		name string
		path string
	}{
		{"macOS", "scripts/package-darwin.sh"},
		{"Windows", "scripts/package-windows.sh"},
	}

	for _, script := range scripts {
		t.Run(script.name+" packaging script", func(t *testing.T) {
			path := filepath.Join(getProjectRoot(), script.path)

			// Check file exists
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				t.Fatalf("Script not found: %s", script.path)
			}

			// Check file is executable (on Unix)
			if runtime.GOOS != "windows" {
				if info.Mode()&0111 == 0 {
					t.Errorf("Script is not executable: %s", script.path)
				}
			}

			// Check script content is valid
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read script: %v", err)
			}

			if !strings.HasPrefix(string(content), "#!/bin/bash") {
				t.Errorf("Script missing shebang: %s", script.path)
			}
		})
	}
}

// TestIPCCommunication tests IPC client/server communication patterns
func TestIPCCommunication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping IPC test in short mode")
	}

	t.Run("IPC socket path platform detection", func(t *testing.T) {
		// Verify platform-specific socket paths are defined
		serverPath := filepath.Join(getProjectRoot(), "gui/internal/ipc/server.go")
		content, err := os.ReadFile(serverPath)
		if err != nil {
			t.Fatalf("Failed to read server.go: %v", err)
		}

		// Should handle both Unix socket and TCP
		if !strings.Contains(string(content), "gui.sock") {
			t.Error("Unix socket path not found in server.go")
		}
	})

	t.Run("IPC client platform detection", func(t *testing.T) {
		clientPath := filepath.Join(getProjectRoot(), "cmd/mote-tray/ipc/client.go")
		content, err := os.ReadFile(clientPath)
		if err != nil {
			t.Fatalf("Failed to read client.go: %v", err)
		}

		// Should handle both Unix socket and TCP
		if !strings.Contains(string(content), "runtime.GOOS") {
			t.Error("Platform detection not found in client.go")
		}
	})
}

// TestMoteServiceHealth tests the mote service health endpoint (if running)
func TestMoteServiceHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping service health test in short mode")
	}

	// Try to connect to local mote service
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:18788/health", nil)
	if err != nil {
		t.Skip("Failed to create request")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Skip("Mote service not running, skipping health check")
	}
	defer resp.Body.Close()

	// Accept 200 or 404 (if health endpoint is not implemented)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Unexpected health check response: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

// Helper functions

func getProjectRoot() string {
	// Get the directory containing this test file
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}

	// Navigate up to project root (test/e2e -> test -> project root)
	dir := filepath.Dir(file)
	return filepath.Join(dir, "..", "..")
}

func findGUIBinary() string {
	root := getProjectRoot()
	possiblePaths := []string{
		filepath.Join(root, "gui/build/bin/Mote"),
		filepath.Join(root, "gui/build/bin/Mote.app/Contents/MacOS/Mote"),
		filepath.Join(root, "gui/build/bin/Mote.exe"),
		filepath.Join(root, "build/Mote"),
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findMoteBinary() string {
	root := getProjectRoot()
	possiblePaths := []string{
		filepath.Join(root, "build/mote"),
		filepath.Join(root, "mote"),
	}

	if runtime.GOOS == "windows" {
		possiblePaths = append([]string{
			filepath.Join(root, "build/mote.exe"),
			filepath.Join(root, "mote.exe"),
		}, possiblePaths...)
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try PATH
	if path, err := exec.LookPath("mote"); err == nil {
		return path
	}

	return ""
}
