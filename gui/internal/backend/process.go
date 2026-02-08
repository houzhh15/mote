// Package backend provides Go bindings for the Wails frontend.
package backend

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ProcessManager manages the mote service process lifecycle.
type ProcessManager struct {
	motePath      string
	port          int
	cmd           *exec.Cmd
	running       bool
	mu            sync.Mutex
	startedAt     time.Time
	onStateChange func(bool)
}

// NewProcessManager creates a new process manager.
func NewProcessManager(motePath string, port int) *ProcessManager {
	return &ProcessManager{
		motePath: motePath,
		port:     port,
	}
}

// SetStateChangeCallback sets the callback for state changes.
func (pm *ProcessManager) SetStateChangeCallback(cb func(bool)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onStateChange = cb
}

// ensureInitialized checks if mote is initialized and runs init if needed.
func (pm *ProcessManager) ensureInitialized() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Use filepath.Join for cross-platform compatibility (Windows/Unix)
	configPath := filepath.Join(homeDir, ".mote", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config doesn't exist, run mote init
		cmd := exec.Command(pm.motePath, "init")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize mote: %w", err)
		}
	}
	return nil
}

// Start starts the mote service.
func (pm *ProcessManager) Start() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running && pm.cmd != nil && pm.cmd.Process != nil {
		return nil
	}

	if pm.isPortInUse() {
		pm.running = true
		return nil
	}

	// Ensure mote is initialized before starting service
	if err := pm.ensureInitialized(); err != nil {
		return err
	}

	pm.cmd = exec.Command(pm.motePath, "serve")
	pm.cmd.Stdout = os.Stdout
	pm.cmd.Stderr = os.Stderr
	pm.cmd.SysProcAttr = sysProcAttr()

	if err := pm.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mote service: %w", err)
	}

	pm.running = true
	pm.startedAt = time.Now()
	go pm.monitorProcess()

	if pm.onStateChange != nil {
		pm.onStateChange(true)
	}

	return nil
}

// Stop stops the mote service.
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running || pm.cmd == nil || pm.cmd.Process == nil {
		pm.running = false
		return nil
	}

	if err := sendTermSignal(pm.cmd.Process); err != nil {
		if killErr := pm.cmd.Process.Kill(); killErr != nil {
			return fmt.Errorf("failed to stop mote service: %w", killErr)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- pm.cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = pm.cmd.Process.Kill()
	}

	pm.running = false
	pm.cmd = nil

	if pm.onStateChange != nil {
		pm.onStateChange(false)
	}

	return nil
}

// IsRunning checks if the service is running.
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cmd != nil && pm.cmd.Process != nil {
		if !isProcessAlive(pm.cmd.Process) {
			pm.running = false
			pm.cmd = nil
		}
	}

	if !pm.running {
		pm.running = pm.isPortInUse()
	}

	return pm.running
}

// WaitForReady waits for the service to be ready.
func (pm *ProcessManager) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://localhost:%d/api/v1/health", pm.port)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return ErrServiceTimeout
}

// GetStatus returns the service status.
func (pm *ProcessManager) GetStatus() *ServiceStatus {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	status := &ServiceStatus{
		Running: pm.running,
		Port:    pm.port,
	}

	if pm.running && pm.cmd != nil && pm.cmd.Process != nil {
		status.PID = pm.cmd.Process.Pid
		status.StartedAt = pm.startedAt
	}

	if pm.running {
		status.Version = pm.getServiceVersion()
	}

	return status
}

func (pm *ProcessManager) isPortInUse() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", pm.port), 1*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (pm *ProcessManager) monitorProcess() {
	if pm.cmd == nil {
		return
	}
	_ = pm.cmd.Wait()

	pm.mu.Lock()
	pm.running = false
	pm.cmd = nil
	callback := pm.onStateChange
	pm.mu.Unlock()

	if callback != nil {
		callback(false)
	}
}

func (pm *ProcessManager) getServiceVersion() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/version", pm.port))
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	return "1.0.0"
}
