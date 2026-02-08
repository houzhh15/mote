package protocol

import "fmt"

// JSON-RPC 2.0 standard error codes.
const (
	ErrCodeParseError     = -32700 // Invalid JSON was received
	ErrCodeInvalidRequest = -32600 // The JSON sent is not a valid Request object
	ErrCodeMethodNotFound = -32601 // The method does not exist / is not available
	ErrCodeInvalidParams  = -32602 // Invalid method parameter(s)
	ErrCodeInternalError  = -32603 // Internal JSON-RPC error
)

// MCP extension error codes.
const (
	ErrCodeToolNotFound        = -32001 // Tool not found
	ErrCodeToolExecutionFailed = -32002 // Tool execution failed
	ErrCodeNotInitialized      = -32003 // Server not initialized
)

// NewParseError creates a parse error response.
func NewParseError(data any) *RPCError {
	return &RPCError{
		Code:    ErrCodeParseError,
		Message: "Parse error",
		Data:    data,
	}
}

// NewInvalidRequestError creates an invalid request error response.
func NewInvalidRequestError(msg string) *RPCError {
	return &RPCError{
		Code:    ErrCodeInvalidRequest,
		Message: fmt.Sprintf("Invalid request: %s", msg),
	}
}

// NewMethodNotFoundError creates a method not found error response.
func NewMethodNotFoundError(method string) *RPCError {
	return &RPCError{
		Code:    ErrCodeMethodNotFound,
		Message: fmt.Sprintf("Method not found: %s", method),
	}
}

// NewInvalidParamsError creates an invalid params error response.
func NewInvalidParamsError(msg string) *RPCError {
	return &RPCError{
		Code:    ErrCodeInvalidParams,
		Message: fmt.Sprintf("Invalid params: %s", msg),
	}
}

// NewInternalError creates an internal error response.
func NewInternalError(msg string) *RPCError {
	return &RPCError{
		Code:    ErrCodeInternalError,
		Message: fmt.Sprintf("Internal error: %s", msg),
	}
}

// NewToolNotFoundError creates a tool not found error response.
func NewToolNotFoundError(name string) *RPCError {
	return &RPCError{
		Code:    ErrCodeToolNotFound,
		Message: fmt.Sprintf("Tool not found: %s", name),
	}
}

// NewToolExecutionError creates a tool execution failed error response.
func NewToolExecutionError(name string, err error) *RPCError {
	return &RPCError{
		Code:    ErrCodeToolExecutionFailed,
		Message: fmt.Sprintf("Tool execution failed: %s", name),
		Data:    err.Error(),
	}
}

// NewNotInitializedError creates a not initialized error response.
func NewNotInitializedError() *RPCError {
	return &RPCError{
		Code:    ErrCodeNotInitialized,
		Message: "Server not initialized",
	}
}
