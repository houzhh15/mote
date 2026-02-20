// Package provider defines the LLM provider interface and types.
package provider

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode defines Provider error codes
type ErrorCode string

const (
	// Authentication errors
	ErrCodeAuthFailed   ErrorCode = "AUTH_FAILED"   // Invalid or expired credentials
	ErrCodeTokenExpired ErrorCode = "TOKEN_EXPIRED" // Token has expired (auto-refreshable)

	// Rate limiting and quota
	ErrCodeRateLimited   ErrorCode = "RATE_LIMITED"   // Too many requests
	ErrCodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED" // Usage quota exceeded

	// Service availability
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE" // Service temporarily unavailable
	ErrCodeModelNotFound      ErrorCode = "MODEL_NOT_FOUND"     // Requested model not found

	// Network and request
	ErrCodeNetworkError          ErrorCode = "NETWORK_ERROR"           // Network connectivity issues
	ErrCodeInvalidRequest        ErrorCode = "INVALID_REQUEST"         // Malformed request
	ErrCodeTimeout               ErrorCode = "TIMEOUT"                 // Request timeout
	ErrCodeContextWindowExceeded ErrorCode = "CONTEXT_WINDOW_EXCEEDED" // Input exceeds model context window

	// Unknown
	ErrCodeUnknown ErrorCode = "UNKNOWN" // Unclassified error
)

// ProviderError is a structured error for Provider operations
type ProviderError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Provider   string    `json:"provider"`
	Retryable  bool      `json:"retryable"`
	RetryAfter int       `json:"retry_after,omitempty"` // seconds until retry is allowed
}

// Error implements the error interface
func (e *ProviderError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Provider, e.Code, e.Message)
}

// IsUserRecoverable returns true if the error requires user action to recover
func (e *ProviderError) IsUserRecoverable() bool {
	switch e.Code {
	case ErrCodeAuthFailed, ErrCodeQuotaExceeded:
		return true
	case ErrCodeRateLimited:
		return true // user needs to wait
	default:
		return false
	}
}

// ShouldAutoRetry returns true if the error should be automatically retried
// Errors related to billing/rate limiting should NOT be auto-retried
func (e *ProviderError) ShouldAutoRetry() bool {
	switch e.Code {
	case ErrCodeAuthFailed, ErrCodeRateLimited, ErrCodeQuotaExceeded:
		return false // billing/rate related - do not auto retry
	case ErrCodeServiceUnavailable, ErrCodeNetworkError, ErrCodeTimeout:
		return e.Retryable
	default:
		return false
	}
}

// NewProviderError creates a new ProviderError
func NewProviderError(code ErrorCode, message, provider string, retryable bool) *ProviderError {
	return &ProviderError{
		Code:      code,
		Message:   message,
		Provider:  provider,
		Retryable: retryable,
	}
}

// NewProviderErrorWithRetryAfter creates a ProviderError with retry delay
func NewProviderErrorWithRetryAfter(code ErrorCode, message, provider string, retryAfter int) *ProviderError {
	return &ProviderError{
		Code:       code,
		Message:    message,
		Provider:   provider,
		Retryable:  true,
		RetryAfter: retryAfter,
	}
}

// IsContextWindowExceeded checks if the error indicates that the input
// exceeded the model's context window limit.  It first checks for a typed
// ProviderError with ErrCodeContextWindowExceeded, then falls back to
// keyword matching on the error message for untyped errors.
func IsContextWindowExceeded(err error) bool {
	if err == nil {
		return false
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Code == ErrCodeContextWindowExceeded
	}
	// Fallback: keyword matching for providers that don't return typed errors
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context window") ||
		strings.Contains(msg, "context length exceeded") ||
		strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "token limit exceeded") ||
		strings.Contains(msg, "too many tokens")
}

// IsRetryable checks if the error is a transient provider error that
// should be automatically retried (e.g., empty response, temporary
// service unavailability).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Retryable
	}
	return false
}
