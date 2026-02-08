package builtin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mote/internal/hooks"

	"github.com/rs/zerolog/log"
)

// RateLimitConfig configures the rate limit hook.
type RateLimitConfig struct {
	// MaxRequests is the maximum number of requests allowed in the window.
	MaxRequests int
	// Window is the time window for rate limiting.
	Window time.Duration
	// KeyFunc extracts the rate limit key from the context (e.g., session ID, user ID).
	// If nil, uses a global rate limit.
	KeyFunc func(hookCtx *hooks.Context) string
	// OnLimit is called when the rate limit is exceeded (optional).
	OnLimit func(key string, count int)
	// Message is the error message when rate limited.
	Message string
}

// RateLimitHook provides rate limiting functionality.
type RateLimitHook struct {
	maxRequests int
	window      time.Duration
	keyFunc     func(hookCtx *hooks.Context) string
	onLimit     func(key string, count int)
	message     string

	// Sliding window counters per key
	counters map[string]*slidingWindow
	mu       sync.RWMutex
}

// slidingWindow implements a simple sliding window counter.
type slidingWindow struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// NewRateLimitHook creates a new rate limit hook with the given configuration.
func NewRateLimitHook(cfg RateLimitConfig) *RateLimitHook {
	if cfg.MaxRequests <= 0 {
		cfg.MaxRequests = 60
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.Message == "" {
		cfg.Message = "rate limit exceeded"
	}

	return &RateLimitHook{
		maxRequests: cfg.MaxRequests,
		window:      cfg.Window,
		keyFunc:     cfg.KeyFunc,
		onLimit:     cfg.OnLimit,
		message:     cfg.Message,
		counters:    make(map[string]*slidingWindow),
	}
}

// Handler returns a hook handler that enforces rate limits.
func (h *RateLimitHook) Handler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    80, // High priority, after filter
		Source:      "_builtin",
		Description: "Enforces rate limits",
		Enabled:     true,
		Handler:     h.handle,
	}
}

func (h *RateLimitHook) handle(_ context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	// Extract the rate limit key
	key := "_global"
	if h.keyFunc != nil {
		key = h.keyFunc(hookCtx)
	}

	// Get or create sliding window for this key
	h.mu.Lock()
	window, exists := h.counters[key]
	if !exists {
		window = &slidingWindow{
			timestamps: make([]time.Time, 0),
		}
		h.counters[key] = window
	}
	h.mu.Unlock()

	// Check rate limit
	count := window.add(hookCtx.Timestamp, h.window)
	if count > h.maxRequests {
		log.Warn().
			Str("key", key).
			Int("count", count).
			Int("max", h.maxRequests).
			Dur("window", h.window).
			Msg("rate limit exceeded")

		if h.onLimit != nil {
			h.onLimit(key, count)
		}

		result := hooks.ErrorResult(errors.New(h.message))
		result.Data = map[string]any{
			"rate_limit_key":   key,
			"rate_limit_count": count,
			"rate_limit_max":   h.maxRequests,
		}
		return result, nil
	}

	return hooks.ContinueResult(), nil
}

// add adds a timestamp and returns the count within the window.
func (sw *slidingWindow) add(now time.Time, window time.Duration) int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Add current timestamp
	sw.timestamps = append(sw.timestamps, now)

	// Remove timestamps outside the window
	cutoff := now.Add(-window)
	validStart := 0
	for i, ts := range sw.timestamps {
		if ts.After(cutoff) {
			validStart = i
			break
		}
	}
	sw.timestamps = sw.timestamps[validStart:]

	return len(sw.timestamps)
}

// count returns the current count within the window.
func (sw *slidingWindow) count(now time.Time, window time.Duration) int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	cutoff := now.Add(-window)
	count := 0
	for _, ts := range sw.timestamps {
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}

// Reset resets the rate limit counter for a key.
func (h *RateLimitHook) Reset(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.counters, key)
}

// ResetAll resets all rate limit counters.
func (h *RateLimitHook) ResetAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counters = make(map[string]*slidingWindow)
}

// GetCount returns the current count for a key.
func (h *RateLimitHook) GetCount(key string) int {
	h.mu.RLock()
	window, exists := h.counters[key]
	h.mu.RUnlock()

	if !exists {
		return 0
	}

	return window.count(time.Now(), h.window)
}

// RegisterRateLimitHook registers the rate limit hook for message processing.
func RegisterRateLimitHook(manager *hooks.Manager, cfg RateLimitConfig) error {
	hook := NewRateLimitHook(cfg)
	handler := hook.Handler("builtin:ratelimit:before_message")
	return manager.Register(hooks.HookBeforeMessage, handler)
}

// SessionRateLimitKeyFunc returns a key function that uses the session ID.
func SessionRateLimitKeyFunc() func(hookCtx *hooks.Context) string {
	return func(hookCtx *hooks.Context) string {
		if hookCtx.Session != nil && hookCtx.Session.ID != "" {
			return fmt.Sprintf("session:%s", hookCtx.Session.ID)
		}
		return "_global"
	}
}

// UserRateLimitKeyFunc returns a key function that uses user ID from session metadata.
func UserRateLimitKeyFunc(metadataKey string) func(hookCtx *hooks.Context) string {
	return func(hookCtx *hooks.Context) string {
		if hookCtx.Session != nil && hookCtx.Session.Metadata != nil {
			if userID, ok := hookCtx.Session.Metadata[metadataKey].(string); ok && userID != "" {
				return fmt.Sprintf("user:%s", userID)
			}
		}
		return "_global"
	}
}

// RateLimitStats provides statistics about rate limiting.
type RateLimitStats struct {
	ActiveKeys   int
	TotalHits    int64
	TotalBlocked int64
}

// Stats returns current rate limit statistics.
func (h *RateLimitHook) Stats() RateLimitStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return RateLimitStats{
		ActiveKeys: len(h.counters),
	}
}

// Cleanup removes expired entries from the counters.
func (h *RateLimitHook) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-h.window)

	for key, window := range h.counters {
		window.mu.Lock()
		active := false
		for _, ts := range window.timestamps {
			if ts.After(cutoff) {
				active = true
				break
			}
		}
		window.mu.Unlock()

		if !active {
			delete(h.counters, key)
		}
	}
}
