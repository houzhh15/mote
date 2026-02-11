//go:build !windows

package copilot

import "os/exec"

// hideProcessWindow is a no-op on non-Windows platforms.
// Unix systems don't open visible windows for child processes.
func hideProcessWindow(cmd *exec.Cmd) {
	// No-op on macOS/Linux
}
