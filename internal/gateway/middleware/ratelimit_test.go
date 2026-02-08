package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             5,
		Enabled:           true,
		CleanupInterval:   time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	ip := "192.168.1.1"

	// First burst requests should be allowed
	for i := 0; i < 5; i++ {
		allowed, remaining, _ := rl.Allow(ip)
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
		expectedRemaining := 4 - i
		if remaining != expectedRemaining {
			t.Errorf("Request %d: expected remaining %d, got %d", i+1, expectedRemaining, remaining)
		}
	}

	// 6th request should be denied (burst exhausted)
	allowed, remaining, _ := rl.Allow(ip)
	if allowed {
		t.Error("6th request should be denied")
	}
	if remaining != 0 {
		t.Errorf("Expected remaining 0, got %d", remaining)
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             5,
		Enabled:           false,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	ip := "192.168.1.1"

	// All requests should be allowed when disabled
	for i := 0; i < 100; i++ {
		allowed, _, _ := rl.Allow(ip)
		if !allowed {
			t.Errorf("Request %d should be allowed when disabled", i+1)
		}
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 600, // 10 per second for faster testing
		Burst:             2,
		Enabled:           true,
		CleanupInterval:   time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	ip := "192.168.1.1"

	// Exhaust burst
	rl.Allow(ip)
	rl.Allow(ip)

	// Wait for token refill (100ms should add 1 token at 10/s rate)
	time.Sleep(150 * time.Millisecond)

	// Should be allowed now
	allowed, _, _ := rl.Allow(ip)
	if !allowed {
		t.Error("Request should be allowed after token refill")
	}
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             2,
		Enabled:           true,
		CleanupInterval:   time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Each client has their own bucket
	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Exhaust ip1's burst
	rl.Allow(ip1)
	rl.Allow(ip1)
	allowed, _, _ := rl.Allow(ip1)
	if allowed {
		t.Error("ip1 should be rate limited")
	}

	// ip2 should still have full burst
	allowed, remaining, _ := rl.Allow(ip2)
	if !allowed {
		t.Error("ip2 should be allowed")
	}
	if remaining != 1 {
		t.Errorf("ip2 should have 1 remaining, got %d", remaining)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             2,
		Enabled:           true,
		CleanupInterval:   time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := rl.RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rr.Code)
		}

		// Check rate limit headers
		if rr.Header().Get("X-RateLimit-Limit") != "60" {
			t.Errorf("Expected X-RateLimit-Limit 60, got %s", rr.Header().Get("X-RateLimit-Limit"))
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rr.Code)
	}

	if rr.Header().Get("Retry-After") == "" {
		t.Error("Expected Retry-After header")
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             2,
		Enabled:           false,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := rl.RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// All requests should succeed when disabled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

// NOTE: TestGetClientIP is defined in logging_test.go
