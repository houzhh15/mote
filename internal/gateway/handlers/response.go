// Package handlers provides HTTP handler utilities for the gateway.
package handlers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents an error response body.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error code and message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SendJSON writes a JSON response with the given status code.
func SendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// SendError writes an error response with the given status code, error code, and message.
func SendError(w http.ResponseWriter, status int, code, message string) {
	SendJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// Common error codes.
const (
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeRateLimited        = "RATE_LIMITED"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeGatewayTimeout     = "GATEWAY_TIMEOUT"
)
