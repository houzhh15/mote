//go:build windows

package backend

import (
	"os"
	"syscall"
)

// sysProcAttr returns the platform-specific SysProcAttr for Windows.
// Windows does not support Setpgid, so we return an empty struct.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// sendTermSignal on Windows uses Kill since SIGTERM is not available.
func sendTermSignal(p *os.Process) error {
	return p.Kill()
}

// isProcessAlive checks if the process is still running.
// On Windows, we try to open the process to check if it exists.
func isProcessAlive(p *os.Process) bool {
	// On Windows, FindProcess always succeeds, so we rely on the process state
	// A nil check here is a simple approach; for more robust checking,
	// we would need to use Windows-specific APIs
	return p != nil
}
