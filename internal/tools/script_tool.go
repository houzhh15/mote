// Package tools provides script tool implementation for JS and Shell runtimes.
package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ScriptRuntime specifies the runtime environment for a script.
type ScriptRuntime string

const (
	// RuntimeJS indicates JavaScript runtime (executed via jsvm).
	RuntimeJS ScriptRuntime = "js"
	// RuntimeShell indicates Shell runtime (executed via os/exec).
	RuntimeShell ScriptRuntime = "shell"
)

// JSExecutor is the interface for executing JavaScript code.
type JSExecutor interface {
	Execute(ctx context.Context, script, scriptName, executionID string) (*JSExecuteResult, error)
}

// JSExecuteResult holds the result of JS script execution.
type JSExecuteResult struct {
	Value interface{}
	Logs  []string
}

// ScriptTool is a tool that executes scripts in different runtimes.
type ScriptTool struct {
	name        string
	description string
	parameters  map[string]any
	runtime     ScriptRuntime
	script      string
	timeout     time.Duration
	jsExecutor  JSExecutor
}

// ScriptToolConfig holds configuration for creating a ScriptTool.
type ScriptToolConfig struct {
	Name        string
	Description string
	Parameters  map[string]any
	Runtime     ScriptRuntime
	Script      string
	Timeout     time.Duration
	JSExecutor  JSExecutor
}

// NewScriptTool creates a new script tool.
func NewScriptTool(cfg ScriptToolConfig) *ScriptTool {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &ScriptTool{
		name:        cfg.Name,
		description: cfg.Description,
		parameters:  cfg.Parameters,
		runtime:     cfg.Runtime,
		script:      cfg.Script,
		timeout:     timeout,
		jsExecutor:  cfg.JSExecutor,
	}
}

// Name returns the tool name.
func (t *ScriptTool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *ScriptTool) Description() string {
	return t.description
}

// Parameters returns the tool parameters schema.
func (t *ScriptTool) Parameters() map[string]any {
	if t.parameters == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return t.parameters
}

// Execute runs the script with the given arguments.
func (t *ScriptTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	// Apply timeout
	execCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	switch t.runtime {
	case RuntimeJS:
		return t.executeJS(execCtx, args)
	case RuntimeShell:
		return t.executeShell(execCtx, args)
	default:
		return NewErrorResult(fmt.Sprintf("unsupported runtime: %s", t.runtime)), nil
	}
}

// executeJS executes JavaScript script.
func (t *ScriptTool) executeJS(ctx context.Context, args map[string]any) (ToolResult, error) {
	if t.jsExecutor == nil {
		return NewErrorResult("JavaScript executor not configured"), nil
	}

	// Generate execution ID
	execID := fmt.Sprintf("%s-%d", t.name, time.Now().UnixNano())

	// Inject args into script wrapper
	wrapperScript := t.wrapJSScript(args)

	result, err := t.jsExecutor.Execute(ctx, wrapperScript, t.name, execID)
	if err != nil {
		// Check for context timeout
		if ctx.Err() == context.DeadlineExceeded {
			return NewErrorResult("script execution timed out"), nil
		}
		return NewErrorResult(fmt.Sprintf("JS execution error: %v", err)), nil
	}

	// Convert result to string
	content := fmt.Sprintf("%v", result.Value)
	if result.Value == nil {
		content = ""
	}

	return NewSuccessResult(content), nil
}

// wrapJSScript wraps the script with argument injection.
func (t *ScriptTool) wrapJSScript(args map[string]any) string {
	// Simple wrapper that makes args available as global 'args' object
	// The actual implementation may vary based on jsvm setup
	return fmt.Sprintf(`
(function() {
	const args = %s;
	%s
})();
`, argsToJS(args), t.script)
}

// argsToJS converts args map to JavaScript object literal.
func argsToJS(args map[string]any) string {
	if args == nil {
		return "{}"
	}
	// Simple JSON-like serialization
	// In production, use proper JSON encoding
	result := "{"
	first := true
	for k, v := range args {
		if !first {
			result += ", "
		}
		first = false
		result += fmt.Sprintf("%q: %v", k, formatJSValue(v))
	}
	result += "}"
	return result
}

// formatJSValue formats a value for JavaScript.
func formatJSValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case nil:
		return "null"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// executeShell executes Shell script.
func (t *ScriptTool) executeShell(ctx context.Context, args map[string]any) (ToolResult, error) {
	// Determine shell
	shell := "/bin/sh"
	if s, ok := args["shell"].(string); ok {
		shell = s
	}

	// Prepare command
	cmd := exec.CommandContext(ctx, shell, "-c", t.script)

	// Set environment variables from args
	for k, v := range args {
		if k == "shell" {
			continue
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		return NewErrorResult("script execution timed out"), nil
	}

	if err != nil {
		// Include stderr in error message
		errMsg := fmt.Sprintf("shell execution error: %v", err)
		if stderr.Len() > 0 {
			errMsg += "\nstderr: " + stderr.String()
		}
		return NewErrorResult(errMsg), nil
	}

	// Return stdout as result
	return NewSuccessResult(stdout.String()), nil
}
