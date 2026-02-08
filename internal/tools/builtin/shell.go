// Package builtin provides built-in tools for the Mote agent runtime.
package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"mote/internal/tools"
)

// ShellArgs defines the parameters for the shell tool.
type ShellArgs struct {
	Command string `json:"command" jsonschema:"description=The shell command to execute,required"`
	Timeout int    `json:"timeout" jsonschema:"description=Timeout in seconds (default: 30)"`
	WorkDir string `json:"work_dir" jsonschema:"description=Working directory for the command"`
}

// ShellTool executes shell commands.
type ShellTool struct {
	tools.BaseTool
	// MaxOutputSize is the maximum size of command output in bytes.
	MaxOutputSize int
}

// NewShellTool creates a new shell tool.
func NewShellTool() *ShellTool {
	return &ShellTool{
		BaseTool: tools.BaseTool{
			ToolName:        "shell",
			ToolDescription: "Execute a shell command and return its output. Use this to run system commands, scripts, or interact with the operating system.",
			ToolParameters:  tools.BuildSchema(ShellArgs{}),
		},
		MaxOutputSize: 1024 * 1024, // 1MB default
	}
}

// Execute runs the shell command.
func (t *ShellTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "command is required", nil)
	}

	timeout := 30
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		timeout = int(v)
	}

	workDir, _ := args["work_dir"].(string)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Determine shell based on OS
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", command)
	}

	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Build result
	var result strings.Builder
	if stdout.Len() > 0 {
		output := stdout.String()
		if len(output) > t.MaxOutputSize {
			output = output[:t.MaxOutputSize] + "\n... (output truncated)"
		}
		result.WriteString(output)
	}

	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		errOutput := stderr.String()
		if len(errOutput) > t.MaxOutputSize {
			errOutput = errOutput[:t.MaxOutputSize] + "\n... (output truncated)"
		}
		result.WriteString(errOutput)
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return tools.ToolResult{}, tools.NewToolTimeoutError(t.Name(), fmt.Sprintf("%ds", timeout))
		}

		// Include error info but still return output
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(fmt.Sprintf("Exit error: %v", err))
		return tools.NewErrorResult(result.String()), nil
	}

	if result.Len() == 0 {
		return tools.NewSuccessResult("(no output)"), nil
	}

	return tools.NewSuccessResult(result.String()), nil
}
