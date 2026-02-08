package policy

import "strings"

// ToolGroups defines predefined groups of related tools.
var ToolGroups = map[string][]string{
	"group:fs": {
		"read_file",
		"write_file",
		"list_dir",
		"delete_file",
	},
	"group:runtime": {
		"shell",
		"exec",
	},
	"group:memory": {
		"memory_search",
		"memory_add",
	},
	"group:mcp": {
		"mcp_*", // Wildcard for all MCP tools
	},
	"group:web": {
		"http_request",
		"web_search",
		"web_fetch",
	},
}

// ExpandGroups expands group references in a list of tool patterns.
// For example, ["group:fs", "shell"] -> ["read_file", "write_file", "list_dir", "delete_file", "shell"]
func ExpandGroups(patterns []string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		expanded := expandSinglePattern(pattern)
		for _, tool := range expanded {
			if !seen[tool] {
				seen[tool] = true
				result = append(result, tool)
			}
		}
	}

	return result
}

// expandSinglePattern expands a single pattern, handling group references.
func expandSinglePattern(pattern string) []string {
	// Check if it's a group reference
	if strings.HasPrefix(pattern, "group:") {
		if tools, ok := ToolGroups[pattern]; ok {
			return tools
		}
		// Unknown group, return as-is (will not match anything)
		return []string{pattern}
	}

	// Not a group reference, return as-is
	return []string{pattern}
}

// NormalizeName normalizes a tool name for matching.
// Converts to lowercase and trims whitespace.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// IsGroupReference returns true if the pattern is a group reference.
func IsGroupReference(pattern string) bool {
	return strings.HasPrefix(pattern, "group:")
}

// ListGroups returns all available group names.
func ListGroups() []string {
	groups := make([]string, 0, len(ToolGroups))
	for name := range ToolGroups {
		groups = append(groups, name)
	}
	return groups
}
