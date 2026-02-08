package jsvm

import (
	"errors"
	"testing"
)

func TestErrTimeout(t *testing.T) {
	err := ErrTimeout
	if err.Error() != "jsvm: execution timeout" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestErrMemoryLimit(t *testing.T) {
	err := ErrMemoryLimit
	if err.Error() != "jsvm: memory limit exceeded" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestErrVMPoolExhausted(t *testing.T) {
	err := ErrVMPoolExhausted
	if err.Error() != "jsvm: vm pool exhausted" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestErrModuleNotFound(t *testing.T) {
	err := ErrModuleNotFound
	if err.Error() != "jsvm: module not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestPathNotAllowedError(t *testing.T) {
	err := &PathNotAllowedError{Path: "/etc/passwd"}
	if err.Error() != "jsvm: path not allowed: /etc/passwd" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Test errors.Is matching
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Error("PathNotAllowedError should match ErrPathNotAllowed sentinel")
	}
}

func TestScriptSyntaxError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ScriptSyntaxError
		expected string
	}{
		{
			name:     "with file",
			err:      &ScriptSyntaxError{File: "test.js", Line: 10, Column: 5, Message: "unexpected token"},
			expected: "jsvm: syntax error in test.js at line 10, column 5: unexpected token",
		},
		{
			name:     "without file",
			err:      &ScriptSyntaxError{Line: 3, Column: 1, Message: "missing semicolon"},
			expected: "jsvm: syntax error at line 3, column 1: missing semicolon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}

	// Test errors.Is matching
	if !errors.Is(&ScriptSyntaxError{}, ErrScriptSyntax) {
		t.Error("ScriptSyntaxError should match ErrScriptSyntax sentinel")
	}
}

func TestExecutionError(t *testing.T) {
	cause := errors.New("runtime error")

	tests := []struct {
		name     string
		err      *ExecutionError
		expected string
	}{
		{
			name:     "with script",
			err:      &ExecutionError{Script: "handler.js", Cause: cause},
			expected: "jsvm: execution error in handler.js: runtime error",
		},
		{
			name:     "without script",
			err:      &ExecutionError{Cause: cause},
			expected: "jsvm: execution error: runtime error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}

	// Test Unwrap
	execErr := &ExecutionError{Cause: cause}
	if errors.Unwrap(execErr) != cause {
		t.Error("Unwrap should return the cause error")
	}
}
