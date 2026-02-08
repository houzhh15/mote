package hooks

import "errors"

// Hook system errors.
var (
	// ErrHandlerNotFound is returned when a handler cannot be found by ID.
	ErrHandlerNotFound = errors.New("handler not found")

	// ErrHandlerExists is returned when trying to register a handler with an existing ID.
	ErrHandlerExists = errors.New("handler already registered")

	// ErrHookTypeInvalid is returned when an invalid hook type is provided.
	ErrHookTypeInvalid = errors.New("invalid hook type")

	// ErrChainInterrupted is returned when the hook chain is interrupted.
	ErrChainInterrupted = errors.New("hook chain interrupted")

	// ErrHandlerPanic is returned when a handler panics during execution.
	ErrHandlerPanic = errors.New("handler panic")

	// ErrHandlerTimeout is returned when a handler execution times out.
	ErrHandlerTimeout = errors.New("handler timeout")

	// ErrHandlerDisabled is returned when trying to execute a disabled handler.
	ErrHandlerDisabled = errors.New("handler disabled")
)
