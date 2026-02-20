package cron

import (
	"errors"
	"math"
	"time"
)

// RetryableError is an error that can be retried.
type RetryableError interface {
	error
	Retryable() bool
}

// RetryPolicy defines the retry behavior for failed jobs.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of retry attempts (0 = no retries).
	MaxAttempts int
	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier is the exponential backoff multiplier.
	Multiplier float64
}

// DefaultRetryPolicy returns the default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Minute,
		Multiplier:   2.0,
	}
}

// NewRetryPolicy creates a retry policy from configuration values.
func NewRetryPolicy(maxAttempts int, initialDelay, maxDelay time.Duration) RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  maxAttempts,
		InitialDelay: initialDelay,
		MaxDelay:     maxDelay,
		Multiplier:   2.0,
	}
}

// ShouldRetry determines if a failed job should be retried.
func (p *RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if p.MaxAttempts <= 0 {
		return false
	}
	if attempt >= p.MaxAttempts {
		return false
	}

	// Check if error is explicitly non-retryable
	var retryable RetryableError
	if errors.As(err, &retryable) {
		return retryable.Retryable()
	}

	// Check provider errors via ShouldAutoRetry (e.g., ProviderError).
	// This uses an interface to avoid importing the provider package.
	// 400 Bad Request, 401 Auth, 403 Forbidden etc. return false,
	// preventing wasteful retries on client-side errors.
	type autoRetryChecker interface {
		error
		ShouldAutoRetry() bool
	}
	var checker autoRetryChecker
	if errors.As(err, &checker) {
		return checker.ShouldAutoRetry()
	}

	// By default, retry on error
	return err != nil
}

// NextDelay calculates the delay before the next retry attempt.
func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return p.InitialDelay
	}

	// Calculate exponential backoff
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt))

	// Cap at max delay
	if delay > float64(p.MaxDelay) {
		return p.MaxDelay
	}

	return time.Duration(delay)
}

// nonRetryableError wraps an error to mark it as non-retryable.
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string {
	return e.err.Error()
}

func (e *nonRetryableError) Unwrap() error {
	return e.err
}

func (e *nonRetryableError) Retryable() bool {
	return false
}

// NonRetryable wraps an error to mark it as non-retryable.
func NonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableError{err: err}
}

// retryableError wraps an error to mark it as retryable.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	return e.err
}

func (e *retryableError) Retryable() bool {
	return true
}

// Retryable wraps an error to mark it as retryable.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}
