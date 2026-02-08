// Package scheduler provides session management and run queue functionality.
package scheduler

import (
	"errors"
	"time"

	"mote/internal/storage"
)

// CachedSession represents a session with its messages cached in memory.
type CachedSession struct {
	// Session is the underlying session entity.
	Session *storage.Session

	// Messages contains the cached messages for this session.
	Messages []*storage.Message

	// Dirty indicates if the session has unsaved changes.
	Dirty bool

	// LastAccess records when this session was last accessed.
	LastAccess time.Time
}

// Sentinel errors for the scheduler package.
var (
	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionClosed is returned when operating on a closed session.
	ErrSessionClosed = errors.New("session closed")

	// ErrQueueFull is returned when the run queue is at capacity.
	ErrQueueFull = errors.New("run queue full")

	// ErrRunNotFound is returned when a run is not found.
	ErrRunNotFound = errors.New("run not found")

	// ErrRunCancelled is returned when a run has been cancelled.
	ErrRunCancelled = errors.New("run cancelled")
)

// RunState represents the state of a run.
type RunState string

const (
	// RunStatePending indicates the run is waiting to be processed.
	RunStatePending RunState = "pending"

	// RunStateRunning indicates the run is currently being processed.
	RunStateRunning RunState = "running"

	// RunStateCompleted indicates the run finished successfully.
	RunStateCompleted RunState = "completed"

	// RunStateFailed indicates the run failed with an error.
	RunStateFailed RunState = "failed"

	// RunStateCancelled indicates the run was cancelled.
	RunStateCancelled RunState = "cancelled"
)

// Run represents a single agent run request.
type Run struct {
	// ID is the unique identifier for this run.
	ID string

	// SessionID is the session this run belongs to.
	SessionID string

	// State is the current state of the run.
	State RunState

	// Input is the user input that triggered this run.
	Input string

	// Output is the final response from the agent.
	Output string

	// Error contains the error message if the run failed.
	Error string

	// CreatedAt is when the run was created.
	CreatedAt time.Time

	// StartedAt is when the run started processing.
	StartedAt *time.Time

	// CompletedAt is when the run finished.
	CompletedAt *time.Time
}

// IsPending returns true if the run is waiting to be processed.
func (r *Run) IsPending() bool {
	return r.State == RunStatePending
}

// IsRunning returns true if the run is currently being processed.
func (r *Run) IsRunning() bool {
	return r.State == RunStateRunning
}

// IsCompleted returns true if the run finished (success, failure, or cancelled).
func (r *Run) IsCompleted() bool {
	return r.State == RunStateCompleted || r.State == RunStateFailed || r.State == RunStateCancelled
}
