//go:build !windows

package backend

import (
	"os"
	"syscall"
)

// sysProcAttr returns the platform-specific SysProcAttr for Unix systems.
// Setpgid creates a new process group, allowing proper cleanup of child processes.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// sendTermSignal sends SIGTERM to gracefully terminate the process.
func sendTermSignal(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// isProcessAlive checks if the process is still running by sending signal 0.
func isProcessAlive(p *os.Process) bool {
	return p.Signal(syscall.Signal(0)) == nil
}
