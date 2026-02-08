package runner

import "errors"

// Runner errors.
var (
	// ErrMaxIterations indicates the maximum iteration count was reached.
	ErrMaxIterations = errors.New("maximum iterations reached")

	// ErrContextCanceled indicates the context was canceled.
	ErrContextCanceled = errors.New("context canceled")

	// ErrNoProvider indicates no provider is configured.
	ErrNoProvider = errors.New("no provider configured")

	// ErrNoMessages indicates the message list is empty.
	ErrNoMessages = errors.New("no messages to send")

	// ErrSessionNotFound indicates the session was not found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrHookInterrupted indicates a hook interrupted the execution.
	ErrHookInterrupted = errors.New("execution interrupted by hook")
)
