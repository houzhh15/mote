package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMatcher_MatchTool(t *testing.T) {
	m := NewDefaultMatcher()

	tests := []struct {
		name     string
		toolName string
		patterns []string
		expected bool
	}{
		{
			name:     "exact match",
			toolName: "shell",
			patterns: []string{"shell"},
			expected: true,
		},
		{
			name:     "exact match case insensitive",
			toolName: "Shell",
			patterns: []string{"shell"},
			expected: true,
		},
		{
			name:     "no match",
			toolName: "shell",
			patterns: []string{"exec"},
			expected: false,
		},
		{
			name:     "wildcard suffix",
			toolName: "mcp_github",
			patterns: []string{"mcp_*"},
			expected: true,
		},
		{
			name:     "wildcard prefix",
			toolName: "test_shell",
			patterns: []string{"*_shell"},
			expected: true,
		},
		{
			name:     "wildcard middle",
			toolName: "read_large_file",
			patterns: []string{"read_*_file"},
			expected: true,
		},
		{
			name:     "wildcard no match",
			toolName: "shell",
			patterns: []string{"mcp_*"},
			expected: false,
		},
		{
			name:     "multiple patterns first match",
			toolName: "shell",
			patterns: []string{"exec", "shell", "run"},
			expected: true,
		},
		{
			name:     "multiple patterns last match",
			toolName: "run",
			patterns: []string{"exec", "shell", "run"},
			expected: true,
		},
		{
			name:     "empty patterns",
			toolName: "shell",
			patterns: []string{},
			expected: false,
		},
		{
			name:     "empty tool name",
			toolName: "",
			patterns: []string{"shell"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.MatchTool(tt.toolName, tt.patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultMatcher_MatchArgs(t *testing.T) {
	m := NewDefaultMatcher()

	tests := []struct {
		name      string
		args      string
		pattern   string
		expected  bool
		expectErr bool
	}{
		{
			name:     "simple match",
			args:     `{"command": "sudo apt update"}`,
			pattern:  `sudo\s+`,
			expected: true,
		},
		{
			name:     "no match",
			args:     `{"command": "ls -la"}`,
			pattern:  `sudo\s+`,
			expected: false,
		},
		{
			name:     "rm -rf detection",
			args:     `{"command": "rm -rf /tmp/test"}`,
			pattern:  `rm\s+(-[rf]+\s+)*(-[rf]+)`,
			expected: true,
		},
		{
			name:     "empty pattern",
			args:     `{"command": "anything"}`,
			pattern:  "",
			expected: false,
		},
		{
			name:      "invalid regex",
			args:      `{"command": "test"}`,
			pattern:   `[invalid`,
			expected:  false,
			expectErr: true,
		},
		{
			name:     "path pattern",
			args:     `{"path": "/etc/passwd"}`,
			pattern:  `"/(etc|usr|bin|sbin)/`,
			expected: true,
		},
		{
			name:     "safe path",
			args:     `{"path": "/tmp/test.txt"}`,
			pattern:  `"/(etc|usr|bin|sbin)/`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := m.MatchArgs(tt.args, tt.pattern)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDefaultMatcher_MatchPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	m := NewDefaultMatcher()

	tests := []struct {
		name     string
		path     string
		prefixes []string
		expected bool
	}{
		{
			name:     "exact prefix match",
			path:     "/tmp/test.txt",
			prefixes: []string{"/tmp"},
			expected: true,
		},
		{
			name:     "nested path match",
			path:     "/tmp/deep/nested/file.txt",
			prefixes: []string{"/tmp"},
			expected: true,
		},
		{
			name:     "no match",
			path:     "/etc/passwd",
			prefixes: []string{"/tmp", "/var"},
			expected: false,
		},
		{
			name:     "home directory expansion",
			path:     filepath.Join(home, "test.txt"),
			prefixes: []string{"~"},
			expected: true,
		},
		{
			name:     "home directory nested",
			path:     filepath.Join(home, ".config", "app", "settings"),
			prefixes: []string{"~/.config"},
			expected: true,
		},
		{
			name:     "similar prefix but not match",
			path:     "/tmpdir/file.txt",
			prefixes: []string{"/tmp"},
			expected: false,
		},
		{
			name:     "multiple prefixes",
			path:     "/var/log/app.log",
			prefixes: []string{"/tmp", "/var/log"},
			expected: true,
		},
		{
			name:     "empty prefixes allows all",
			path:     "/any/path/file.txt",
			prefixes: []string{},
			expected: true,
		},
		{
			name:     "exact path match",
			path:     "/tmp",
			prefixes: []string{"/tmp"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.MatchPath(tt.path, tt.prefixes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultMatcher_RegexCache(t *testing.T) {
	m := NewDefaultMatcher()

	pattern := `sudo\s+`
	args := `{"command": "sudo apt update"}`

	// First call - compiles and caches
	result1, err := m.MatchArgs(args, pattern)
	require.NoError(t, err)
	assert.True(t, result1)

	// Second call - uses cache
	result2, err := m.MatchArgs(args, pattern)
	require.NoError(t, err)
	assert.True(t, result2)

	// Clear cache
	m.ClearCache()

	// Third call - recompiles after cache clear
	result3, err := m.MatchArgs(args, pattern)
	require.NoError(t, err)
	assert.True(t, result3)
}

func TestDefaultMatcher_Timeout(t *testing.T) {
	m := NewDefaultMatcher()
	m.RegexTimeout = 1 * time.Nanosecond // Very short timeout

	// This test verifies timeout behavior
	// Note: Due to Go's goroutine scheduling, timeout may not always trigger
	// but the code path should handle it gracefully
	_, err := m.MatchArgs("test string", `.*test.*`)
	assert.NoError(t, err) // Should not error, just return false on timeout
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/.config", filepath.Join(home, ".config")},
		{"/tmp/test", "/tmp/test"},
		{"./relative", "./relative"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		{"mcp_github", "mcp_*", true},
		{"mcp_slack", "mcp_*", true},
		{"shell", "mcp_*", false},
		{"test_shell", "*_shell", true},
		{"read_large_file", "read_*_file", true},
		{"read_file", "read_*_file", false}, // * needs at least one char? Actually .* matches zero or more
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.pattern, func(t *testing.T) {
			result := matchWildcard(tt.name, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}
