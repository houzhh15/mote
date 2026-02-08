// Package jsvmerr provides error types for the jsvm package.
// This package exists to avoid import cycles between jsvm and jsvm/hostapi.
package jsvmerr

import (
	"errors"
	"fmt"
)

// Sentinel errors for JS VM operations.
var (
	// ErrTimeout indicates script execution exceeded the timeout limit.
	ErrTimeout = errors.New("jsvm: execution timeout")

	// ErrMemoryLimit indicates script execution exceeded memory limit.
	ErrMemoryLimit = errors.New("jsvm: memory limit exceeded")

	// ErrVMPoolExhausted indicates no VM instances available in the pool.
	ErrVMPoolExhausted = errors.New("jsvm: vm pool exhausted")

	// ErrModuleNotFound indicates the requested module could not be found.
	ErrModuleNotFound = errors.New("jsvm: module not found")
)

// PathNotAllowedError indicates a file path is not in the allowed whitelist.
type PathNotAllowedError struct {
	Path string
}

func (e *PathNotAllowedError) Error() string {
	return fmt.Sprintf("jsvm: path not allowed: %s", e.Path)
}

// Is implements errors.Is for PathNotAllowedError.
func (e *PathNotAllowedError) Is(target error) bool {
	_, ok := target.(*PathNotAllowedError)
	return ok
}

// ErrPathNotAllowed is a sentinel for errors.Is matching.
var ErrPathNotAllowed = &PathNotAllowedError{}

// ScriptSyntaxError indicates a JavaScript syntax error.
type ScriptSyntaxError struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (e *ScriptSyntaxError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("jsvm: syntax error in %s at line %d, column %d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("jsvm: syntax error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// Is implements errors.Is for ScriptSyntaxError.
func (e *ScriptSyntaxError) Is(target error) bool {
	_, ok := target.(*ScriptSyntaxError)
	return ok
}

// ErrScriptSyntax is a sentinel for errors.Is matching.
var ErrScriptSyntax = &ScriptSyntaxError{}

// ExecutionError wraps runtime errors during script execution.
type ExecutionError struct {
	Script string
	Cause  error
}

func (e *ExecutionError) Error() string {
	if e.Script != "" {
		return fmt.Sprintf("jsvm: execution error in %s: %v", e.Script, e.Cause)
	}
	return fmt.Sprintf("jsvm: execution error: %v", e.Cause)
}

func (e *ExecutionError) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is for ExecutionError.
func (e *ExecutionError) Is(target error) bool {
	_, ok := target.(*ExecutionError)
	return ok
}

// ErrExecution is a sentinel for errors.Is matching.
var ErrExecution = &ExecutionError{}
