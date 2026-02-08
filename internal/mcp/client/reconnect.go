// Package client provides auto-reconnect functionality for MCP clients.
package client

import (
	"math"
	"time"
)

// ReconnectPolicy defines the policy for automatic reconnection.
type ReconnectPolicy struct {
	// MaxRetries is the maximum number of reconnection attempts.
	MaxRetries int
	// InitialDelay is the initial delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier is the factor by which the delay increases.
	Multiplier float64
}

// DefaultReconnectPolicy returns a default reconnection policy.
func DefaultReconnectPolicy() *ReconnectPolicy {
	return &ReconnectPolicy{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     16 * time.Second,
		Multiplier:   2.0,
	}
}

// NextDelay calculates the delay before the next reconnection attempt.
// retryCount is 0-indexed (first retry has retryCount=0).
func (p *ReconnectPolicy) NextDelay(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}

	// Calculate delay using exponential backoff: InitialDelay * Multiplier^retryCount
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(retryCount))

	// Cap at MaxDelay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	return time.Duration(delay)
}

// ShouldRetry returns true if another retry attempt should be made.
// retryCount is 0-indexed (first retry has retryCount=0).
func (p *ReconnectPolicy) ShouldRetry(retryCount int) bool {
	return retryCount < p.MaxRetries
}

// ReconnectConfig holds configuration for auto-reconnect behavior.
type ReconnectConfig struct {
	// Enabled controls whether auto-reconnect is enabled.
	Enabled bool
	// Policy defines the reconnection policy.
	Policy *ReconnectPolicy
	// OnReconnecting is called when a reconnection attempt starts.
	OnReconnecting func(attempt int)
	// OnReconnected is called when reconnection succeeds.
	OnReconnected func()
	// OnReconnectFailed is called when all reconnection attempts fail.
	OnReconnectFailed func(lastErr error)
}

// DefaultReconnectConfig returns a default reconnect configuration.
func DefaultReconnectConfig() *ReconnectConfig {
	return &ReconnectConfig{
		Enabled: true,
		Policy:  DefaultReconnectPolicy(),
	}
}

// Reconnector manages the reconnection logic for a client.
type Reconnector struct {
	policy     *ReconnectPolicy
	retryCount int
	enabled    bool

	// Callbacks
	onReconnecting    func(attempt int)
	onReconnected     func()
	onReconnectFailed func(lastErr error)
}

// NewReconnector creates a new Reconnector with the given configuration.
func NewReconnector(config *ReconnectConfig) *Reconnector {
	if config == nil {
		config = DefaultReconnectConfig()
	}
	if config.Policy == nil {
		config.Policy = DefaultReconnectPolicy()
	}

	return &Reconnector{
		policy:            config.Policy,
		enabled:           config.Enabled,
		onReconnecting:    config.OnReconnecting,
		onReconnected:     config.OnReconnected,
		onReconnectFailed: config.OnReconnectFailed,
	}
}

// IsEnabled returns whether auto-reconnect is enabled.
func (r *Reconnector) IsEnabled() bool {
	return r.enabled
}

// SetEnabled enables or disables auto-reconnect.
func (r *Reconnector) SetEnabled(enabled bool) {
	r.enabled = enabled
}

// Reset resets the retry count to zero.
func (r *Reconnector) Reset() {
	r.retryCount = 0
}

// ShouldRetry returns true if another reconnection attempt should be made.
func (r *Reconnector) ShouldRetry() bool {
	if !r.enabled {
		return false
	}
	return r.policy.ShouldRetry(r.retryCount)
}

// NextDelay returns the delay before the next reconnection attempt
// and increments the retry count.
func (r *Reconnector) NextDelay() time.Duration {
	delay := r.policy.NextDelay(r.retryCount)
	r.retryCount++
	return delay
}

// RetryCount returns the current retry count.
func (r *Reconnector) RetryCount() int {
	return r.retryCount
}

// NotifyReconnecting notifies that a reconnection attempt is starting.
func (r *Reconnector) NotifyReconnecting() {
	if r.onReconnecting != nil {
		r.onReconnecting(r.retryCount)
	}
}

// NotifyReconnected notifies that reconnection succeeded.
func (r *Reconnector) NotifyReconnected() {
	r.Reset()
	if r.onReconnected != nil {
		r.onReconnected()
	}
}

// NotifyReconnectFailed notifies that all reconnection attempts failed.
func (r *Reconnector) NotifyReconnectFailed(lastErr error) {
	if r.onReconnectFailed != nil {
		r.onReconnectFailed(lastErr)
	}
}
