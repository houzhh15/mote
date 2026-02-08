// Package backend provides Go bindings for the Wails frontend.
package backend

import (
	"encoding/json"
	"fmt"
)

// Error codes for GUI operations.
const (
	ErrCodeServiceNotRunning   = "ERR_SERVICE_NOT_RUNNING"
	ErrCodeServiceStartFailed  = "ERR_SERVICE_START_FAILED"
	ErrCodeServiceTimeout      = "ERR_SERVICE_TIMEOUT"
	ErrCodeAPIRequestFailed    = "ERR_API_REQUEST_FAILED"
	ErrCodeAPIResponseInvalid  = "ERR_API_RESPONSE_INVALID"
	ErrCodeIPCConnectionFailed = "ERR_IPC_CONNECTION_FAILED"
	ErrCodePortInUse           = "ERR_PORT_IN_USE"
)

// NewAPIError creates a new APIError with the given parameters.
func NewAPIError(statusCode int, code, message string, details any) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Details:    details,
	}
}

// ParseAPIError parses an API error from response body.
func ParseAPIError(statusCode int, body []byte) *APIError {
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		return &APIError{
			StatusCode: statusCode,
			Code:       errResp.Code,
			Message:    errResp.Message,
			Details:    errResp.Details,
		}
	}

	return &APIError{
		StatusCode: statusCode,
		Code:       fmt.Sprintf("HTTP_%d", statusCode),
		Message:    string(body),
	}
}

// IsNotFound checks if the error is a 404 Not Found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsUnauthorized checks if the error is a 401 Unauthorized error.
func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == 401
}

// IsBadRequest checks if the error is a 400 Bad Request error.
func (e *APIError) IsBadRequest() bool {
	return e.StatusCode == 400
}

// IsServerError checks if the error is a 5xx Server Error.
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}
