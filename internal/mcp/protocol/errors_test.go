package protocol

import (
	"errors"
	"testing"
)

func TestErrorCodeConstants(t *testing.T) {
	// JSON-RPC 2.0 standard error codes
	tests := []struct {
		name string
		code int
		want int
	}{
		{"ParseError", ErrCodeParseError, -32700},
		{"InvalidRequest", ErrCodeInvalidRequest, -32600},
		{"MethodNotFound", ErrCodeMethodNotFound, -32601},
		{"InvalidParams", ErrCodeInvalidParams, -32602},
		{"InternalError", ErrCodeInternalError, -32603},
		// MCP extension error codes
		{"ToolNotFound", ErrCodeToolNotFound, -32001},
		{"ToolExecutionFailed", ErrCodeToolExecutionFailed, -32002},
		{"NotInitialized", ErrCodeNotInitialized, -32003},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.want)
			}
		})
	}
}

func TestNewParseError(t *testing.T) {
	err := NewParseError("invalid json")
	if err.Code != ErrCodeParseError {
		t.Errorf("expected code %d, got %d", ErrCodeParseError, err.Code)
	}
	if err.Message != "Parse error" {
		t.Errorf("expected message 'Parse error', got %s", err.Message)
	}
	if err.Data != "invalid json" {
		t.Errorf("expected data 'invalid json', got %v", err.Data)
	}
}

func TestNewInvalidRequestError(t *testing.T) {
	err := NewInvalidRequestError("missing method")
	if err.Code != ErrCodeInvalidRequest {
		t.Errorf("expected code %d, got %d", ErrCodeInvalidRequest, err.Code)
	}
	if err.Message != "Invalid request: missing method" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestNewMethodNotFoundError(t *testing.T) {
	err := NewMethodNotFoundError("unknown/method")
	if err.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, err.Code)
	}
	if err.Message != "Method not found: unknown/method" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestNewInvalidParamsError(t *testing.T) {
	err := NewInvalidParamsError("missing required field")
	if err.Code != ErrCodeInvalidParams {
		t.Errorf("expected code %d, got %d", ErrCodeInvalidParams, err.Code)
	}
	if err.Message != "Invalid params: missing required field" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestNewInternalError(t *testing.T) {
	err := NewInternalError("database connection failed")
	if err.Code != ErrCodeInternalError {
		t.Errorf("expected code %d, got %d", ErrCodeInternalError, err.Code)
	}
	if err.Message != "Internal error: database connection failed" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestNewToolNotFoundError(t *testing.T) {
	err := NewToolNotFoundError("nonexistent_tool")
	if err.Code != ErrCodeToolNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeToolNotFound, err.Code)
	}
	if err.Message != "Tool not found: nonexistent_tool" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestNewToolExecutionError(t *testing.T) {
	cause := errors.New("permission denied")
	err := NewToolExecutionError("write_file", cause)
	if err.Code != ErrCodeToolExecutionFailed {
		t.Errorf("expected code %d, got %d", ErrCodeToolExecutionFailed, err.Code)
	}
	if err.Message != "Tool execution failed: write_file" {
		t.Errorf("unexpected message: %s", err.Message)
	}
	if err.Data != "permission denied" {
		t.Errorf("expected data 'permission denied', got %v", err.Data)
	}
}

func TestNewNotInitializedError(t *testing.T) {
	err := NewNotInitializedError()
	if err.Code != ErrCodeNotInitialized {
		t.Errorf("expected code %d, got %d", ErrCodeNotInitialized, err.Code)
	}
	if err.Message != "Server not initialized" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}
