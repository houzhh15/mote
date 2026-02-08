//go:build !windows
// +build !windows

package procmgr

import (
	"os/exec"
	"syscall"
)

// configurePlatformProcess configures platform-specific process settings
func configurePlatformProcess(cmd *exec.Cmd, cfg *ProcessConfig) {
	// On Unix, create a new process group so child processes can be killed together
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
