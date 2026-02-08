package client

import (
	"testing"
	"time"
)

func TestDefaultReconnectPolicy(t *testing.T) {
	policy := DefaultReconnectPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("MaxRetries: got %d, want 5", policy.MaxRetries)
	}
	if policy.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay: got %v, want 1s", policy.InitialDelay)
	}
	if policy.MaxDelay != 16*time.Second {
		t.Errorf("MaxDelay: got %v, want 16s", policy.MaxDelay)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("Multiplier: got %v, want 2.0", policy.Multiplier)
	}
}

func TestReconnectPolicy_NextDelay(t *testing.T) {
	policy := &ReconnectPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     16 * time.Second,
		Multiplier:   2.0,
	}

	tests := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 1 * time.Second},  // 1 * 2^0 = 1
		{1, 2 * time.Second},  // 1 * 2^1 = 2
		{2, 4 * time.Second},  // 1 * 2^2 = 4
		{3, 8 * time.Second},  // 1 * 2^3 = 8
		{4, 16 * time.Second}, // 1 * 2^4 = 16
		{5, 16 * time.Second}, // 1 * 2^5 = 32, capped at 16
		{-1, 1 * time.Second}, // Negative should be treated as 0
	}

	for _, tt := range tests {
		got := policy.NextDelay(tt.retryCount)
		if got != tt.want {
			t.Errorf("NextDelay(%d) = %v, want %v", tt.retryCount, got, tt.want)
		}
	}
}

func TestReconnectPolicy_ShouldRetry(t *testing.T) {
	policy := &ReconnectPolicy{
		MaxRetries: 3,
	}

	tests := []struct {
		retryCount int
		want       bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, tt := range tests {
		got := policy.ShouldRetry(tt.retryCount)
		if got != tt.want {
			t.Errorf("ShouldRetry(%d) = %v, want %v", tt.retryCount, got, tt.want)
		}
	}
}

func TestDefaultReconnectConfig(t *testing.T) {
	config := DefaultReconnectConfig()

	if !config.Enabled {
		t.Error("Enabled should be true by default")
	}
	if config.Policy == nil {
		t.Error("Policy should not be nil")
	}
}

func TestNewReconnector(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		r := NewReconnector(nil)
		if r == nil {
			t.Fatal("Reconnector should not be nil")
		}
		if !r.enabled {
			t.Error("Should be enabled by default")
		}
		if r.policy == nil {
			t.Error("Policy should not be nil")
		}
	})

	t.Run("with config", func(t *testing.T) {
		config := &ReconnectConfig{
			Enabled: false,
			Policy: &ReconnectPolicy{
				MaxRetries: 10,
			},
		}
		r := NewReconnector(config)
		if r.enabled {
			t.Error("Should not be enabled")
		}
		if r.policy.MaxRetries != 10 {
			t.Errorf("MaxRetries: got %d, want 10", r.policy.MaxRetries)
		}
	})

	t.Run("config with nil policy", func(t *testing.T) {
		config := &ReconnectConfig{
			Enabled: true,
			Policy:  nil,
		}
		r := NewReconnector(config)
		if r.policy == nil {
			t.Error("Policy should be set to default")
		}
	})
}

func TestReconnector_IsEnabled(t *testing.T) {
	r := NewReconnector(&ReconnectConfig{Enabled: true})
	if !r.IsEnabled() {
		t.Error("Should be enabled")
	}

	r.SetEnabled(false)
	if r.IsEnabled() {
		t.Error("Should be disabled")
	}
}

func TestReconnector_Reset(t *testing.T) {
	r := NewReconnector(nil)

	// Increment retry count
	r.NextDelay()
	r.NextDelay()
	if r.RetryCount() != 2 {
		t.Errorf("RetryCount: got %d, want 2", r.RetryCount())
	}

	r.Reset()
	if r.RetryCount() != 0 {
		t.Errorf("RetryCount after reset: got %d, want 0", r.RetryCount())
	}
}

func TestReconnector_ShouldRetry(t *testing.T) {
	r := NewReconnector(&ReconnectConfig{
		Enabled: true,
		Policy:  &ReconnectPolicy{MaxRetries: 2},
	})

	if !r.ShouldRetry() {
		t.Error("Should retry at count 0")
	}

	r.NextDelay() // count = 1
	if !r.ShouldRetry() {
		t.Error("Should retry at count 1")
	}

	r.NextDelay() // count = 2
	if r.ShouldRetry() {
		t.Error("Should not retry at count 2 (max is 2)")
	}
}

func TestReconnector_ShouldRetry_Disabled(t *testing.T) {
	r := NewReconnector(&ReconnectConfig{
		Enabled: false,
		Policy:  &ReconnectPolicy{MaxRetries: 10},
	})

	if r.ShouldRetry() {
		t.Error("Should not retry when disabled")
	}
}

func TestReconnector_NextDelay(t *testing.T) {
	r := NewReconnector(&ReconnectConfig{
		Enabled: true,
		Policy: &ReconnectPolicy{
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
		},
	})

	delays := []time.Duration{
		100 * time.Millisecond,  // 2^0
		200 * time.Millisecond,  // 2^1
		400 * time.Millisecond,  // 2^2
		800 * time.Millisecond,  // 2^3
		1000 * time.Millisecond, // capped at MaxDelay
	}

	for i, expected := range delays {
		got := r.NextDelay()
		if got != expected {
			t.Errorf("NextDelay() call %d: got %v, want %v", i+1, got, expected)
		}
	}

	if r.RetryCount() != 5 {
		t.Errorf("RetryCount: got %d, want 5", r.RetryCount())
	}
}

func TestReconnector_Callbacks(t *testing.T) {
	var reconnectingCalls []int
	var reconnectedCalls int
	var failedCalls int
	var lastError error

	r := NewReconnector(&ReconnectConfig{
		Enabled: true,
		Policy:  DefaultReconnectPolicy(),
		OnReconnecting: func(attempt int) {
			reconnectingCalls = append(reconnectingCalls, attempt)
		},
		OnReconnected: func() {
			reconnectedCalls++
		},
		OnReconnectFailed: func(err error) {
			failedCalls++
			lastError = err
		},
	})

	// Test OnReconnecting
	r.NotifyReconnecting()
	if len(reconnectingCalls) != 1 || reconnectingCalls[0] != 0 {
		t.Errorf("OnReconnecting: got %v, want [0]", reconnectingCalls)
	}

	r.NextDelay() // count = 1
	r.NotifyReconnecting()
	if len(reconnectingCalls) != 2 || reconnectingCalls[1] != 1 {
		t.Errorf("OnReconnecting: got %v, want [0, 1]", reconnectingCalls)
	}

	// Test OnReconnected
	r.NotifyReconnected()
	if reconnectedCalls != 1 {
		t.Errorf("OnReconnected calls: got %d, want 1", reconnectedCalls)
	}
	if r.RetryCount() != 0 {
		t.Error("RetryCount should be reset after NotifyReconnected")
	}

	// Test OnReconnectFailed
	testErr := errFoo{}
	r.NotifyReconnectFailed(testErr)
	if failedCalls != 1 {
		t.Errorf("OnReconnectFailed calls: got %d, want 1", failedCalls)
	}
	if lastError != testErr {
		t.Errorf("OnReconnectFailed error: got %v, want %v", lastError, testErr)
	}
}

type errFoo struct{}

func (errFoo) Error() string { return "test error" }

func TestReconnector_NilCallbacks(t *testing.T) {
	r := NewReconnector(&ReconnectConfig{
		Enabled: true,
		Policy:  DefaultReconnectPolicy(),
		// All callbacks are nil
	})

	// These should not panic
	r.NotifyReconnecting()
	r.NotifyReconnected()
	r.NotifyReconnectFailed(errFoo{})
}
