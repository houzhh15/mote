// Package cron provides a cron-based task scheduling system.
package cron

import (
	"errors"
	"fmt"
)

// Sentinel errors for cron operations.
var (
	// ErrJobNotFound indicates the requested job does not exist.
	ErrJobNotFound = errors.New("cron: job not found")

	// ErrJobExists indicates a job with the same name already exists.
	ErrJobExists = errors.New("cron: job already exists")

	// ErrSchedulerNotRunning indicates the scheduler is not running.
	ErrSchedulerNotRunning = errors.New("cron: scheduler not running")

	// ErrHistoryNotFound indicates the requested history entry does not exist.
	ErrHistoryNotFound = errors.New("cron: history entry not found")
)

// InvalidScheduleError indicates an invalid cron schedule expression.
type InvalidScheduleError struct {
	Schedule string
	Message  string
}

func (e *InvalidScheduleError) Error() string {
	return fmt.Sprintf("cron: invalid schedule '%s': %s", e.Schedule, e.Message)
}

// Is implements errors.Is for InvalidScheduleError.
func (e *InvalidScheduleError) Is(target error) bool {
	_, ok := target.(*InvalidScheduleError)
	return ok
}

// ErrInvalidSchedule is a sentinel for errors.Is matching.
var ErrInvalidSchedule = &InvalidScheduleError{}

// ExecutionFailedError indicates a job execution failed.
type ExecutionFailedError struct {
	JobName    string
	RetryCount int
	Cause      error
}

func (e *ExecutionFailedError) Error() string {
	if e.RetryCount > 0 {
		return fmt.Sprintf("cron: job '%s' execution failed after %d retries: %v", e.JobName, e.RetryCount, e.Cause)
	}
	return fmt.Sprintf("cron: job '%s' execution failed: %v", e.JobName, e.Cause)
}

func (e *ExecutionFailedError) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is for ExecutionFailedError.
func (e *ExecutionFailedError) Is(target error) bool {
	_, ok := target.(*ExecutionFailedError)
	return ok
}

// ErrExecutionFailed is a sentinel for errors.Is matching.
var ErrExecutionFailed = &ExecutionFailedError{}
