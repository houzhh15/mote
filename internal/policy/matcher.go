package policy

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PatternMatcher provides pattern matching capabilities.
type PatternMatcher interface {
	// MatchTool checks if a tool name matches any pattern in the list.
	MatchTool(toolName string, patterns []string) bool

	// MatchArgs checks if tool arguments match a regex pattern.
	MatchArgs(args string, pattern string) (bool, error)

	// MatchPath checks if a path is within allowed prefixes.
	MatchPath(path string, prefixes []string) bool
}

// DefaultMatcher is the default implementation of PatternMatcher.
type DefaultMatcher struct {
	// regexCache caches compiled regex patterns.
	regexCache sync.Map

	// RegexTimeout is the timeout for regex matching (default: 100ms).
	RegexTimeout time.Duration
}

// NewDefaultMatcher creates a new DefaultMatcher with default settings.
func NewDefaultMatcher() *DefaultMatcher {
	return &DefaultMatcher{
		RegexTimeout: 100 * time.Millisecond,
	}
}

// MatchTool checks if a tool name matches any pattern in the list.
// Supports:
// - Exact match: "shell" matches "shell"
// - Wildcard: "mcp_*" matches "mcp_github", "mcp_slack", etc.
// - Group references are expected to be expanded before calling this method.
func (m *DefaultMatcher) MatchTool(toolName string, patterns []string) bool {
	normalizedName := NormalizeName(toolName)

	for _, pattern := range patterns {
		normalizedPattern := NormalizeName(pattern)

		// Exact match
		if normalizedName == normalizedPattern {
			return true
		}

		// Wildcard match
		if strings.Contains(normalizedPattern, "*") {
			if matchWildcard(normalizedName, normalizedPattern) {
				return true
			}
		}
	}

	return false
}

// matchWildcard performs simple wildcard matching.
// Supports * as a wildcard that matches any characters.
func matchWildcard(name, pattern string) bool {
	// Simple implementation: convert wildcard to regex
	// Escape special regex characters except *
	escaped := regexp.QuoteMeta(pattern)
	// Replace escaped \* with .*
	regexPattern := strings.ReplaceAll(escaped, `\*`, `.*`)
	// Anchor the pattern
	regexPattern = "^" + regexPattern + "$"

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return false
	}

	return re.MatchString(name)
}

// MatchArgs checks if tool arguments match a regex pattern.
// Returns (matched, error). Error is returned for invalid patterns.
func (m *DefaultMatcher) MatchArgs(args string, pattern string) (bool, error) {
	if pattern == "" {
		return false, nil
	}

	re, err := m.getOrCompileRegex(pattern)
	if err != nil {
		return false, err
	}

	// Use timeout-protected matching
	return m.matchWithTimeout(re, args), nil
}

// MatchPath checks if a path is within any of the allowed prefixes.
// Supports ~ for home directory expansion.
func (m *DefaultMatcher) MatchPath(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true // No restrictions
	}

	expandedPath := expandPath(path)
	cleanPath := filepath.Clean(expandedPath)

	for _, prefix := range prefixes {
		expandedPrefix := expandPath(prefix)
		cleanPrefix := filepath.Clean(expandedPrefix)

		// Check if path starts with prefix
		if strings.HasPrefix(cleanPath, cleanPrefix) {
			// Ensure it's a proper prefix (not just substring)
			// e.g., /tmp should match /tmp/foo but not /tmpdir
			if len(cleanPath) == len(cleanPrefix) {
				return true
			}
			if cleanPath[len(cleanPrefix)] == filepath.Separator {
				return true
			}
		}
	}

	return false
}

// getOrCompileRegex gets a cached regex or compiles and caches it.
func (m *DefaultMatcher) getOrCompileRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := m.regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, ErrInvalidPattern
	}

	m.regexCache.Store(pattern, re)
	return re, nil
}

// matchWithTimeout performs regex matching with a timeout.
// Returns false if timeout occurs (treat as no match for safety).
func (m *DefaultMatcher) matchWithTimeout(re *regexp.Regexp, s string) bool {
	timeout := m.RegexTimeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- re.MatchString(s)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		// Timeout - treat as no match for safety
		return false
	}
}

// expandPath expands ~ to the home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// ClearCache clears the regex cache.
func (m *DefaultMatcher) ClearCache() {
	m.regexCache = sync.Map{}
}
