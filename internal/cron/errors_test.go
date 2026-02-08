package cron

import (
	"errors"
	"testing"
)

func TestErrJobNotFound(t *testing.T) {
	err := ErrJobNotFound
	if err.Error() != "cron: job not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestErrJobExists(t *testing.T) {
	err := ErrJobExists
	if err.Error() != "cron: job already exists" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestErrSchedulerNotRunning(t *testing.T) {
	err := ErrSchedulerNotRunning
	if err.Error() != "cron: scheduler not running" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestInvalidScheduleError(t *testing.T) {
	err := &InvalidScheduleError{Schedule: "invalid-cron", Message: "parse error"}
	expected := "cron: invalid schedule 'invalid-cron': parse error"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	// Test errors.Is matching
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Error("InvalidScheduleError should match ErrInvalidSchedule sentinel")
	}
}

func TestExecutionFailedError(t *testing.T) {
	cause := errors.New("network timeout")

	tests := []struct {
		name     string
		err      *ExecutionFailedError
		expected string
	}{
		{
			name:     "without retries",
			err:      &ExecutionFailedError{JobName: "daily-report", RetryCount: 0, Cause: cause},
			expected: "cron: job 'daily-report' execution failed: network timeout",
		},
		{
			name:     "with retries",
			err:      &ExecutionFailedError{JobName: "sync-data", RetryCount: 3, Cause: cause},
			expected: "cron: job 'sync-data' execution failed after 3 retries: network timeout",
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
	if !errors.Is(&ExecutionFailedError{}, ErrExecutionFailed) {
		t.Error("ExecutionFailedError should match ErrExecutionFailed sentinel")
	}

	// Test Unwrap
	execErr := &ExecutionFailedError{Cause: cause}
	if errors.Unwrap(execErr) != cause {
		t.Error("Unwrap should return the cause error")
	}
}
