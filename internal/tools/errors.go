package tools

import (
	"errors"
	"fmt"
)

// Sentinel errors for the tools package.
var (
	// ErrToolNotFound is returned when a requested tool is not registered.
	ErrToolNotFound = errors.New("tool not found")

	// ErrToolAlreadyExists is returned when attempting to register a tool
	// with a name that is already in use.
	ErrToolAlreadyExists = errors.New("tool already exists")

	// ErrInvalidArgs is returned when tool arguments are invalid or malformed.
	ErrInvalidArgs = errors.New("invalid tool arguments")

	// ErrToolTimeout is returned when a tool execution exceeds its time limit.
	ErrToolTimeout = errors.New("tool execution timeout")
)

// ToolNotFoundError provides detailed information about a missing tool.
type ToolNotFoundError struct {
	Name string
}

// Error implements the error interface.
func (e *ToolNotFoundError) Error() string {
	return fmt.Sprintf("tool not found: %s", e.Name)
}

// Is allows errors.Is to match against ErrToolNotFound.
func (e *ToolNotFoundError) Is(target error) bool {
	return target == ErrToolNotFound
}

// Unwrap returns the underlying sentinel error.
func (e *ToolNotFoundError) Unwrap() error {
	return ErrToolNotFound
}

// ToolAlreadyExistsError provides detailed information about a duplicate tool.
type ToolAlreadyExistsError struct {
	Name string
}

// Error implements the error interface.
func (e *ToolAlreadyExistsError) Error() string {
	return fmt.Sprintf("tool already exists: %s", e.Name)
}

// Is allows errors.Is to match against ErrToolAlreadyExists.
func (e *ToolAlreadyExistsError) Is(target error) bool {
	return target == ErrToolAlreadyExists
}

// Unwrap returns the underlying sentinel error.
func (e *ToolAlreadyExistsError) Unwrap() error {
	return ErrToolAlreadyExists
}

// InvalidArgsError provides detailed information about invalid arguments.
type InvalidArgsError struct {
	Tool    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *InvalidArgsError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("invalid arguments for tool %s: %s: %v", e.Tool, e.Message, e.Cause)
	}
	return fmt.Sprintf("invalid arguments for tool %s: %s", e.Tool, e.Message)
}

// Is allows errors.Is to match against ErrInvalidArgs.
func (e *InvalidArgsError) Is(target error) bool {
	return target == ErrInvalidArgs
}

// Unwrap returns the underlying cause or sentinel error.
func (e *InvalidArgsError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return ErrInvalidArgs
}

// ToolTimeoutError provides detailed information about a timeout.
type ToolTimeoutError struct {
	Tool     string
	Duration string
}

// Error implements the error interface.
func (e *ToolTimeoutError) Error() string {
	return fmt.Sprintf("tool %s execution timed out after %s", e.Tool, e.Duration)
}

// Is allows errors.Is to match against ErrToolTimeout.
func (e *ToolTimeoutError) Is(target error) bool {
	return target == ErrToolTimeout
}

// Unwrap returns the underlying sentinel error.
func (e *ToolTimeoutError) Unwrap() error {
	return ErrToolTimeout
}

// NewToolNotFoundError creates a ToolNotFoundError for the given tool name.
func NewToolNotFoundError(name string) error {
	return &ToolNotFoundError{Name: name}
}

// NewToolAlreadyExistsError creates a ToolAlreadyExistsError for the given tool name.
func NewToolAlreadyExistsError(name string) error {
	return &ToolAlreadyExistsError{Name: name}
}

// NewInvalidArgsError creates an InvalidArgsError with the given details.
func NewInvalidArgsError(tool, message string, cause error) error {
	return &InvalidArgsError{
		Tool:    tool,
		Message: message,
		Cause:   cause,
	}
}

// NewToolTimeoutError creates a ToolTimeoutError with the given details.
func NewToolTimeoutError(tool, duration string) error {
	return &ToolTimeoutError{
		Tool:     tool,
		Duration: duration,
	}
}
