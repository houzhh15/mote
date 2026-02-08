// Package backend provides Go bindings for the Wails frontend.
package backend

import (
	"errors"
	"fmt"
	"time"
)

// ServiceStatus represents the mote service status.
type ServiceStatus struct {
	Running   bool      `json:"running"`
	Port      int       `json:"port"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

// APIError represents an API error response.
type APIError struct {
	StatusCode int    `json:"status_code"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    any    `json:"details"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("[%d] %s: %s", e.StatusCode, e.Code, e.Message)
}

// Error definitions.
var (
	ErrServiceNotRunning  = errors.New("mote service is not running")
	ErrServiceStartFailed = errors.New("failed to start mote service")
	ErrServiceTimeout     = errors.New("service startup timeout")
	ErrPortInUse          = errors.New("port is already in use")
	ErrAPIRequestFailed   = errors.New("API request failed")
	ErrAPIResponseInvalid = errors.New("API response invalid")
)

// WrapAPIError wraps an HTTP error into an APIError.
func WrapAPIError(statusCode int, code, message string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}
