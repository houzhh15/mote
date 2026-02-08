package cron

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", policy.MaxAttempts)
	}
	if policy.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay = %v, want 1s", policy.InitialDelay)
	}
	if policy.MaxDelay != 5*time.Minute {
		t.Errorf("MaxDelay = %v, want 5m", policy.MaxDelay)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", policy.Multiplier)
	}
}

func TestNewRetryPolicy(t *testing.T) {
	policy := NewRetryPolicy(5, 2*time.Second, 10*time.Minute)

	if policy.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", policy.MaxAttempts)
	}
	if policy.InitialDelay != 2*time.Second {
		t.Errorf("InitialDelay = %v, want 2s", policy.InitialDelay)
	}
	if policy.MaxDelay != 10*time.Minute {
		t.Errorf("MaxDelay = %v, want 10m", policy.MaxDelay)
	}
}

func TestRetryPolicyShouldRetry(t *testing.T) {
	policy := DefaultRetryPolicy()
	testErr := errors.New("test error")

	tests := []struct {
		name    string
		attempt int
		err     error
		want    bool
	}{
		{"first attempt with error", 0, testErr, true},
		{"second attempt with error", 1, testErr, true},
		{"third attempt with error", 2, testErr, true},
		{"fourth attempt (exceeded)", 3, testErr, false},
		{"nil error", 0, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.ShouldRetry(tt.attempt, tt.err)
			if got != tt.want {
				t.Errorf("ShouldRetry(%d, %v) = %v, want %v", tt.attempt, tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryPolicyShouldRetryDisabled(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 0}
	testErr := errors.New("test error")

	if policy.ShouldRetry(0, testErr) {
		t.Error("ShouldRetry should return false when MaxAttempts is 0")
	}
}

func TestRetryPolicyShouldRetryNonRetryable(t *testing.T) {
	policy := DefaultRetryPolicy()
	err := NonRetryable(errors.New("non-retryable error"))

	if policy.ShouldRetry(0, err) {
		t.Error("ShouldRetry should return false for non-retryable errors")
	}
}

func TestRetryPolicyShouldRetryRetryable(t *testing.T) {
	policy := DefaultRetryPolicy()
	err := Retryable(errors.New("retryable error"))

	if !policy.ShouldRetry(0, err) {
		t.Error("ShouldRetry should return true for retryable errors")
	}
}

func TestRetryPolicyNextDelay(t *testing.T) {
	policy := RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     1 * time.Minute,
		Multiplier:   2.0,
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},  // Initial delay
		{1, 2 * time.Second},  // 1s * 2^1
		{2, 4 * time.Second},  // 1s * 2^2
		{3, 8 * time.Second},  // 1s * 2^3
		{4, 16 * time.Second}, // 1s * 2^4
		{5, 32 * time.Second}, // 1s * 2^5
		{6, 1 * time.Minute},  // Capped at max
		{10, 1 * time.Minute}, // Still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := policy.NextDelay(tt.attempt)
			if got != tt.want {
				t.Errorf("NextDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestNonRetryable(t *testing.T) {
	err := errors.New("base error")
	wrapped := NonRetryable(err)

	if wrapped.Error() != "base error" {
		t.Errorf("Error() = %s, want 'base error'", wrapped.Error())
	}

	// Check Retryable interface
	var retryable RetryableError
	if !errors.As(wrapped, &retryable) {
		t.Fatal("expected error to implement RetryableError")
	}
	if retryable.Retryable() {
		t.Error("NonRetryable error should not be retryable")
	}

	// Check Unwrap
	if !errors.Is(wrapped, err) {
		t.Error("wrapped error should unwrap to original")
	}
}

func TestRetryableWrapper(t *testing.T) {
	err := errors.New("base error")
	wrapped := Retryable(err)

	if wrapped.Error() != "base error" {
		t.Errorf("Error() = %s, want 'base error'", wrapped.Error())
	}

	// Check Retryable interface
	var retryable RetryableError
	if !errors.As(wrapped, &retryable) {
		t.Fatal("expected error to implement RetryableError")
	}
	if !retryable.Retryable() {
		t.Error("Retryable error should be retryable")
	}
}

func TestNonRetryableNil(t *testing.T) {
	if NonRetryable(nil) != nil {
		t.Error("NonRetryable(nil) should return nil")
	}
}

func TestRetryableNil(t *testing.T) {
	if Retryable(nil) != nil {
		t.Error("Retryable(nil) should return nil")
	}
}
