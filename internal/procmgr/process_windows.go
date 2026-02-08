//go:build windows
// +build windows

package procmgr

import (
	"os/exec"
	"syscall"
)

// configurePlatformProcess configures platform-specific process settings
func configurePlatformProcess(cmd *exec.Cmd, cfg *ProcessConfig) {
	// On Windows, hide the window if requested
	if cfg.Hidden {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}
}
