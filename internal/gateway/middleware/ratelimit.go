package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiterConfig configures the rate limiter.
type RateLimiterConfig struct {
	// RequestsPerMinute is the maximum number of requests allowed per minute.
	RequestsPerMinute int
	// Burst is the maximum burst size.
	Burst int
	// Enabled enables or disables rate limiting.
	Enabled bool
	// CleanupInterval is how often to clean up old entries.
	CleanupInterval time.Duration
}

// DefaultRateLimiterConfig returns the default rate limiter configuration.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerMinute: 60,
		Burst:             10,
		Enabled:           true,
		CleanupInterval:   5 * time.Minute,
	}
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

// RateLimiter provides per-client rate limiting.
type RateLimiter struct {
	config  RateLimiterConfig
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
	stopCh  chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		config:  config,
		buckets: make(map[string]*tokenBucket),
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	if config.Enabled && config.CleanupInterval > 0 {
		go rl.cleanup()
	}

	return rl
}

// Stop stops the rate limiter's cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// cleanup periodically removes old token buckets.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, bucket := range rl.buckets {
				bucket.mu.Lock()
				// Remove buckets that haven't been used in a while
				if now.Sub(bucket.lastRefill) > rl.config.CleanupInterval*2 {
					delete(rl.buckets, ip)
				}
				bucket.mu.Unlock()
			}
			rl.mu.Unlock()
		}
	}
}

// getBucket retrieves or creates a token bucket for the given IP.
func (rl *RateLimiter) getBucket(ip string) *tokenBucket {
	rl.mu.RLock()
	bucket, ok := rl.buckets[ip]
	rl.mu.RUnlock()

	if ok {
		return bucket
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, ok = rl.buckets[ip]; ok {
		return bucket
	}

	bucket = &tokenBucket{
		tokens:     float64(rl.config.Burst),
		lastRefill: time.Now(),
	}
	rl.buckets[ip] = bucket
	return bucket
}

// Allow checks if a request from the given IP is allowed.
// Returns (allowed, remaining tokens, reset time).
func (rl *RateLimiter) Allow(ip string) (bool, int, time.Time) {
	if !rl.config.Enabled {
		return true, rl.config.RequestsPerMinute, time.Now().Add(time.Minute)
	}

	bucket := rl.getBucket(ip)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	tokensToAdd := elapsed * (float64(rl.config.RequestsPerMinute) / 60.0)
	bucket.tokens += tokensToAdd
	bucket.lastRefill = now

	// Cap at burst limit
	if bucket.tokens > float64(rl.config.Burst) {
		bucket.tokens = float64(rl.config.Burst)
	}

	// Calculate reset time (when bucket would be full again)
	tokensNeeded := float64(rl.config.Burst) - bucket.tokens
	secondsToFull := tokensNeeded / (float64(rl.config.RequestsPerMinute) / 60.0)
	resetTime := now.Add(time.Duration(secondsToFull) * time.Second)

	// Check if we can take a token
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, int(bucket.tokens), resetTime
	}

	return false, 0, resetTime
}

// RateLimit returns a middleware that rate limits requests.
func (rl *RateLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		ip := getClientIP(r)
		allowed, remaining, resetTime := rl.Allow(ip)

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.config.RequestsPerMinute))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", strconv.FormatInt(int64(time.Until(resetTime).Seconds())+1, 10))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// NOTE: getClientIP is defined in logging.go
