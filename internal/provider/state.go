// Package provider defines the LLM provider interface and types.
package provider

import (
	"context"
	"time"
)

// ProviderStatus represents the connection status of a provider
type ProviderStatus string

const (
	StatusConnected     ProviderStatus = "connected"
	StatusDisconnected  ProviderStatus = "disconnected"
	StatusAuthFailed    ProviderStatus = "auth_failed"
	StatusRateLimited   ProviderStatus = "rate_limited"
	StatusQuotaExceeded ProviderStatus = "quota_exceeded"
	StatusUnavailable   ProviderStatus = "unavailable"
)

// ProviderState contains complete status information for a provider
type ProviderState struct {
	Name        string         `json:"name"`
	Status      ProviderStatus `json:"status"`
	LastCheck   time.Time      `json:"last_check"`
	LastError   string         `json:"last_error,omitempty"`
	RetryAfter  int            `json:"retry_after,omitempty"` // seconds
	Models      []string       `json:"models"`
	TokenExpiry *time.Time     `json:"token_expiry,omitempty"` // Copilot specific
}

// IsHealthy returns true if the provider is in a healthy state
func (s *ProviderState) IsHealthy() bool {
	return s.Status == StatusConnected
}

// HealthCheckable defines the interface for providers that support health checking
type HealthCheckable interface {
	// Ping checks if the provider is available
	Ping(ctx context.Context) error

	// GetState returns the current provider state
	GetState() ProviderState
}

// ProviderStatusResponse is the API response format for provider status
type ProviderStatusResponse struct {
	Providers []ProviderState `json:"providers"`
}
