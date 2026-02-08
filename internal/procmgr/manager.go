// Package procmgr provides subprocess management for Mote's multi-process architecture
package procmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"mote/internal/ipc"
	"mote/pkg/logger"
)

// ProcessConfig defines configuration for a subprocess
type ProcessConfig struct {
	// Name is a unique identifier for this process
	Name string

	// Path is the path to the executable
	Path string

	// Args are command line arguments
	Args []string

	// Env are additional environment variables
	Env []string

	// Role is the IPC role for this process
	Role ipc.ProcessRole

	// MaxRestarts is the maximum number of restart attempts (0 = unlimited)
	MaxRestarts int

	// RestartDelay is the delay between restart attempts
	RestartDelay time.Duration

	// StartTimeout is how long to wait for the process to register
	StartTimeout time.Duration

	// Hidden controls whether the process should be hidden (no window on Windows)
	Hidden bool
}

// Process represents a running subprocess
type Process struct {
	config     *ProcessConfig
	cmd        *exec.Cmd
	pid        int
	running    bool
	restarts   int
	lastStart  time.Time
	mu         sync.Mutex
	exitCh     chan error
	registered chan struct{}
}

// Manager manages subprocesses
type Manager struct {
	ipc         *ipc.Server
	processes   map[string]*Process
	processesMu sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewManager creates a new process manager
func NewManager(ipcServer *ipc.Server) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		ipc:       ipcServer,
		processes: make(map[string]*Process),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Register IPC handlers for child process events
	if ipcServer != nil {
		ipcServer.RegisterHandler(ipc.MsgRegister, ipc.HandlerFunc(m.handleRegister))
	}

	return m
}

// Start starts a subprocess with the given configuration
func (m *Manager) Start(cfg *ProcessConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("process name is required")
	}

	m.processesMu.Lock()
	if _, exists := m.processes[cfg.Name]; exists {
		m.processesMu.Unlock()
		return fmt.Errorf("process %s already exists", cfg.Name)
	}
	m.processesMu.Unlock()

	proc := &Process{
		config:     cfg,
		exitCh:     make(chan error, 1),
		registered: make(chan struct{}),
	}

	if err := m.startProcess(proc); err != nil {
		return err
	}

	m.processesMu.Lock()
	m.processes[cfg.Name] = proc
	m.processesMu.Unlock()

	// Start supervisor goroutine
	go m.supervise(proc)

	return nil
}

// Stop stops a subprocess
func (m *Manager) Stop(name string) error {
	m.processesMu.RLock()
	proc, ok := m.processes[name]
	m.processesMu.RUnlock()

	if !ok {
		return fmt.Errorf("process %s not found", name)
	}

	return m.stopProcess(proc)
}

// StopAll stops all subprocesses
func (m *Manager) StopAll() error {
	m.cancel()

	m.processesMu.RLock()
	processes := make([]*Process, 0, len(m.processes))
	for _, proc := range m.processes {
		processes = append(processes, proc)
	}
	m.processesMu.RUnlock()

	// Send exit signal via IPC first
	if m.ipc != nil {
		exitMsg := ipc.NewMessage(ipc.MsgExit, ipc.RoleMain)
		m.ipc.Broadcast(exitMsg)

		// Give processes time to exit gracefully
		time.Sleep(500 * time.Millisecond)
	}

	var lastErr error
	for _, proc := range processes {
		if err := m.stopProcess(proc); err != nil {
			lastErr = err
			logger.Warnf("failed to stop process %s: %v", proc.config.Name, err)
		}
	}

	return lastErr
}

// Restart restarts a subprocess
func (m *Manager) Restart(name string) error {
	m.processesMu.RLock()
	proc, ok := m.processes[name]
	m.processesMu.RUnlock()

	if !ok {
		return fmt.Errorf("process %s not found", name)
	}

	// Stop the process
	if err := m.stopProcess(proc); err != nil {
		logger.Warnf("failed to stop process for restart: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Start again
	return m.startProcess(proc)
}

// IsRunning checks if a process is running
func (m *Manager) IsRunning(name string) bool {
	m.processesMu.RLock()
	proc, ok := m.processes[name]
	m.processesMu.RUnlock()

	if !ok {
		return false
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()
	return proc.running
}

// GetProcess returns process info
func (m *Manager) GetProcess(name string) (*Process, bool) {
	m.processesMu.RLock()
	defer m.processesMu.RUnlock()
	proc, ok := m.processes[name]
	return proc, ok
}

// startProcess starts a single process
func (m *Manager) startProcess(proc *Process) error {
	proc.mu.Lock()
	defer proc.mu.Unlock()

	if proc.running {
		return nil
	}

	cfg := proc.config

	// Prepare command
	cmd := exec.CommandContext(m.ctx, cfg.Path, cfg.Args...)

	// Set environment
	cmd.Env = append(os.Environ(), cfg.Env...)
	if m.ipc != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("MOTE_IPC_PATH=%s", m.ipc.SocketPath()))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("MOTE_ROLE=%s", cfg.Role))

	// Configure based on platform
	configurePlatformProcess(cmd, cfg)

	// Set stdout/stderr for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process %s: %w", cfg.Name, err)
	}

	proc.cmd = cmd
	proc.pid = cmd.Process.Pid
	proc.running = true
	proc.lastStart = time.Now()

	logger.Infof("started process %s (pid: %d)", cfg.Name, proc.pid)

	// Monitor process exit
	go func() {
		err := cmd.Wait()
		proc.exitCh <- err
	}()

	// Wait for registration if IPC server is available
	if m.ipc != nil && cfg.StartTimeout > 0 {
		select {
		case <-proc.registered:
			logger.Infof("process %s registered via IPC", cfg.Name)
		case <-time.After(cfg.StartTimeout):
			logger.Warnf("process %s did not register within timeout", cfg.Name)
		case err := <-proc.exitCh:
			proc.running = false
			return fmt.Errorf("process %s exited before registration: %w", cfg.Name, err)
		}
	}

	return nil
}

// stopProcess stops a single process
func (m *Manager) stopProcess(proc *Process) error {
	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.running || proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}

	// Try graceful termination first
	if runtime.GOOS == "windows" {
		proc.cmd.Process.Kill()
	} else {
		proc.cmd.Process.Signal(os.Interrupt)

		// Wait for graceful exit
		select {
		case <-proc.exitCh:
			proc.running = false
			return nil
		case <-time.After(5 * time.Second):
			// Force kill
			proc.cmd.Process.Kill()
		}
	}

	proc.running = false
	logger.Infof("stopped process %s", proc.config.Name)

	return nil
}

// supervise monitors a process and restarts it if needed
func (m *Manager) supervise(proc *Process) {
	for {
		select {
		case <-m.ctx.Done():
			return

		case err := <-proc.exitCh:
			proc.mu.Lock()
			proc.running = false
			proc.mu.Unlock()

			if err != nil {
				logger.Warnf("process %s exited with error: %v", proc.config.Name, err)
			} else {
				logger.Infof("process %s exited normally", proc.config.Name)
			}

			// Check if we should restart
			cfg := proc.config
			if cfg.MaxRestarts > 0 && proc.restarts >= cfg.MaxRestarts {
				logger.Errorf("process %s exceeded max restarts (%d)", cfg.Name, cfg.MaxRestarts)
				return
			}

			// Don't restart if context is cancelled
			select {
			case <-m.ctx.Done():
				return
			default:
			}

			// Wait before restart
			time.Sleep(cfg.RestartDelay)

			proc.restarts++
			logger.Infof("restarting process %s (attempt %d)", cfg.Name, proc.restarts)

			if err := m.startProcess(proc); err != nil {
				logger.Errorf("failed to restart process %s: %v", cfg.Name, err)
			}
		}
	}
}

// handleRegister handles process registration messages
func (m *Manager) handleRegister(msg *ipc.Message) error {
	var payload ipc.RegisterPayload
	if err := msg.ParsePayload(&payload); err != nil {
		return err
	}

	// Find the process by role and notify registration
	m.processesMu.RLock()
	for _, proc := range m.processes {
		if proc.config.Role == payload.Role {
			select {
			case proc.registered <- struct{}{}:
			default:
			}
			break
		}
	}
	m.processesMu.RUnlock()

	return nil
}

// GetHelperPath returns the path to a helper executable based on the current platform
func GetHelperPath(name string) string {
	execPath, err := os.Executable()
	if err != nil {
		return name
	}

	execDir := filepath.Dir(execPath)

	switch runtime.GOOS {
	case "darwin":
		// macOS: Look in Contents/Helpers/<Name>.app/Contents/MacOS/<name>
		// or fallback to same directory
		bundlePath := filepath.Join(execDir, "..", "Helpers", name+".app", "Contents", "MacOS", name)
		if _, err := os.Stat(bundlePath); err == nil {
			return bundlePath
		}
		// Fallback: same directory
		return filepath.Join(execDir, name)

	case "windows":
		return filepath.Join(execDir, name+".exe")

	default:
		return filepath.Join(execDir, name)
	}
}
