package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandGroups(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "expand single group",
			input:    []string{"group:fs"},
			expected: []string{"read_file", "write_file", "list_dir", "delete_file"},
		},
		{
			name:     "expand multiple groups",
			input:    []string{"group:fs", "group:runtime"},
			expected: []string{"read_file", "write_file", "list_dir", "delete_file", "shell", "exec"},
		},
		{
			name:     "mix groups and tools",
			input:    []string{"group:runtime", "custom_tool"},
			expected: []string{"shell", "exec", "custom_tool"},
		},
		{
			name:     "unknown group returns as-is",
			input:    []string{"group:unknown"},
			expected: []string{"group:unknown"},
		},
		{
			name:     "deduplicate tools",
			input:    []string{"group:fs", "read_file"},
			expected: []string{"read_file", "write_file", "list_dir", "delete_file"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "wildcard preserved",
			input:    []string{"group:mcp"},
			expected: []string{"mcp_*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandGroups(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ReadFile", "readfile"},
		{"  shell  ", "shell"},
		{"WRITE_FILE", "write_file"},
		{"mcp_tool", "mcp_tool"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGroupReference(t *testing.T) {
	tests := []struct {
		pattern  string
		expected bool
	}{
		{"group:fs", true},
		{"group:runtime", true},
		{"group:", true},
		{"shell", false},
		{"groupfs", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			result := IsGroupReference(tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestListGroups(t *testing.T) {
	groups := ListGroups()
	require.NotEmpty(t, groups)

	// Check that all expected groups are present
	expected := []string{"group:fs", "group:runtime", "group:memory", "group:mcp", "group:web"}
	for _, exp := range expected {
		assert.Contains(t, groups, exp)
	}
}

func TestToolGroups_Completeness(t *testing.T) {
	// Verify each group has at least one tool
	for name, tools := range ToolGroups {
		t.Run(name, func(t *testing.T) {
			assert.NotEmpty(t, tools, "group %s should have at least one tool", name)
		})
	}
}
