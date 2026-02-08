package tools

import (
	"context"
	"testing"
	"time"
)

// mockJSExecutor is a mock implementation of JSExecutor for testing.
type mockJSExecutor struct {
	result    *JSExecuteResult
	err       error
	callCount int
}

func (m *mockJSExecutor) Execute(ctx context.Context, script, scriptName, executionID string) (*JSExecuteResult, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestScriptTool_Name(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "test-tool",
		Runtime: RuntimeJS,
	})

	if tool.Name() != "test-tool" {
		t.Errorf("expected name 'test-tool', got '%s'", tool.Name())
	}
}

func TestScriptTool_Description(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:        "test-tool",
		Description: "A test tool",
		Runtime:     RuntimeJS,
	})

	if tool.Description() != "A test tool" {
		t.Errorf("expected description 'A test tool', got '%s'", tool.Description())
	}
}

func TestScriptTool_Parameters(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	tool := NewScriptTool(ScriptToolConfig{
		Name:       "test-tool",
		Parameters: params,
		Runtime:    RuntimeJS,
	})

	p := tool.Parameters()
	if p["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", p["type"])
	}
}

func TestScriptTool_Parameters_Default(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "test-tool",
		Runtime: RuntimeJS,
	})

	p := tool.Parameters()
	if p["type"] != "object" {
		t.Errorf("expected default type 'object', got '%v'", p["type"])
	}
}

func TestScriptTool_ExecuteJS(t *testing.T) {
	executor := &mockJSExecutor{
		result: &JSExecuteResult{Value: "hello world", Logs: nil},
	}

	tool := NewScriptTool(ScriptToolConfig{
		Name:       "js-tool",
		Runtime:    RuntimeJS,
		Script:     "return 'hello world';",
		Timeout:    5 * time.Second,
		JSExecutor: executor,
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	if result.Content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result.Content)
	}

	if executor.callCount != 1 {
		t.Errorf("expected executor to be called once, called %d times", executor.callCount)
	}
}

func TestScriptTool_ExecuteJS_NoExecutor(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "js-tool",
		Runtime: RuntimeJS,
		Script:  "return 'hello';",
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when no executor configured")
	}
}

func TestScriptTool_ExecuteShell(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "shell-tool",
		Runtime: RuntimeShell,
		Script:  "echo hello shell",
		Timeout: 5 * time.Second,
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	expected := "hello shell\n"
	if result.Content != expected {
		t.Errorf("expected '%s', got '%s'", expected, result.Content)
	}
}

func TestScriptTool_ExecuteShell_WithArgs(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "shell-tool",
		Runtime: RuntimeShell,
		Script:  "echo $MSG",
		Timeout: 5 * time.Second,
	})

	args := map[string]any{"MSG": "hello arg"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	expected := "hello arg\n"
	if result.Content != expected {
		t.Errorf("expected '%s', got '%s'", expected, result.Content)
	}
}

func TestScriptTool_ExecuteShell_Timeout(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "shell-tool",
		Runtime: RuntimeShell,
		Script:  "sleep 5",
		Timeout: 100 * time.Millisecond,
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error due to timeout")
	}

	if result.Content != "script execution timed out" {
		t.Errorf("expected timeout message, got '%s'", result.Content)
	}
}

func TestScriptTool_ExecuteShell_Error(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "shell-tool",
		Runtime: RuntimeShell,
		Script:  "exit 1",
		Timeout: 5 * time.Second,
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for exit 1")
	}
}

func TestScriptTool_UnsupportedRuntime(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "unknown-tool",
		Runtime: ScriptRuntime("python"),
		Script:  "print('hello')",
	})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for unsupported runtime")
	}
}

func TestScriptTool_DefaultTimeout(t *testing.T) {
	tool := NewScriptTool(ScriptToolConfig{
		Name:    "test-tool",
		Runtime: RuntimeJS,
	})

	if tool.timeout != 30*time.Second {
		t.Errorf("expected default timeout of 30s, got %v", tool.timeout)
	}
}

func TestArgsToJS(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		contains []string
	}{
		{
			name:     "nil args",
			args:     nil,
			contains: []string{"{}"},
		},
		{
			name:     "empty args",
			args:     map[string]any{},
			contains: []string{"{}"},
		},
		{
			name:     "string arg",
			args:     map[string]any{"name": "test"},
			contains: []string{"name", "test"},
		},
		{
			name:     "bool arg",
			args:     map[string]any{"flag": true},
			contains: []string{"flag", "true"},
		},
		{
			name:     "nil value",
			args:     map[string]any{"val": nil},
			contains: []string{"val", "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := argsToJS(tt.args)
			for _, s := range tt.contains {
				if len(result) == 0 && s != "{}" {
					continue
				}
				// Simple check - contains substring
				found := false
				for i := 0; i <= len(result)-len(s); i++ {
					if result[i:i+len(s)] == s {
						found = true
						break
					}
				}
				if !found && len(s) > 0 {
					t.Errorf("expected result to contain '%s', got '%s'", s, result)
				}
			}
		})
	}
}
