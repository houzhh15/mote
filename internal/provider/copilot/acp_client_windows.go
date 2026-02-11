//go:build windows

package copilot

import (
	"os/exec"
	"syscall"
)

// hideProcessWindow sets Windows-specific process attributes to prevent
// the CLI from opening a visible console window. This is critical for
// background services — without it, launching copilot (especially via
// cmd.exe /C copilot.cmd) would pop up a black console window.
func hideProcessWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
