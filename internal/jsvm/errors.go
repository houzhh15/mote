// Package jsvm provides a JavaScript execution engine based on goja.
package jsvm

import "mote/internal/jsvmerr"

// Re-export errors from jsvmerr package to avoid breaking existing code.
var (
	ErrTimeout         = jsvmerr.ErrTimeout
	ErrMemoryLimit     = jsvmerr.ErrMemoryLimit
	ErrVMPoolExhausted = jsvmerr.ErrVMPoolExhausted
	ErrModuleNotFound  = jsvmerr.ErrModuleNotFound
	ErrPathNotAllowed  = jsvmerr.ErrPathNotAllowed
	ErrScriptSyntax    = jsvmerr.ErrScriptSyntax
	ErrExecution       = jsvmerr.ErrExecution
)

// Type aliases for error types.
type PathNotAllowedError = jsvmerr.PathNotAllowedError
type ScriptSyntaxError = jsvmerr.ScriptSyntaxError
type ExecutionError = jsvmerr.ExecutionError
