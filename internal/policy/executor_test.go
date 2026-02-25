package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyExecutor_Check_Blocklist(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		Blocklist:    []string{"dangerous_tool", "group:runtime"},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name     string
		toolName string
		allowed  bool
	}{
		{"blocked tool", "dangerous_tool", false},
		{"blocked group member", "shell", false},
		{"allowed tool", "read_file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: tt.toolName}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}

func TestPolicyExecutor_Check_Allowlist(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: false,
		Allowlist:    []string{"read_file", "group:memory"},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name     string
		toolName string
		allowed  bool
	}{
		{"allowed tool", "read_file", true},
		{"allowed group member", "mote_memory_search", true},
		{"not in allowlist", "shell", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: tt.toolName}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}

func TestPolicyExecutor_Check_BlocklistPrecedence(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: false,
		Allowlist:    []string{"shell"},
		Blocklist:    []string{"shell"},
	}
	executor := NewPolicyExecutor(policy)

	call := &ToolCall{Name: "shell"}
	result, err := executor.Check(context.Background(), call)
	require.NoError(t, err)

	// Blocklist should take precedence
	assert.False(t, result.Allowed)
	assert.Contains(t, result.MatchedRules, "blocklist")
}

func TestPolicyExecutor_Check_DangerousOps_Block(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		DangerousOps: []DangerousOpRule{
			{
				Tool:     "shell",
				Pattern:  `rm\s+(-[rf]+\s+)*(-[rf]+)`,
				Severity: "critical",
				Action:   "block",
				Message:  "rm -rf is prohibited",
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name    string
		args    string
		allowed bool
	}{
		{"rm -rf blocked", `{"command": "rm -rf /tmp"}`, false},
		{"rm -r blocked", `{"command": "rm -r /tmp"}`, false},
		{"normal rm allowed", `{"command": "rm file.txt"}`, true},
		{"ls allowed", `{"command": "ls -la"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: "shell", Arguments: tt.args}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}

func TestPolicyExecutor_Check_DangerousOps_Approve(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		DangerousOps: []DangerousOpRule{
			{
				Tool:     "shell",
				Pattern:  `sudo\s+`,
				Severity: "high",
				Action:   "approve",
				Message:  "sudo requires approval",
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name            string
		args            string
		requireApproval bool
	}{
		{"sudo requires approval", `{"command": "sudo apt update"}`, true},
		{"normal command no approval", `{"command": "ls -la"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: "shell", Arguments: tt.args}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Equal(t, tt.requireApproval, result.RequireApproval)
		})
	}
}

func TestPolicyExecutor_Check_DangerousOps_Warn(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		DangerousOps: []DangerousOpRule{
			{
				Tool:     "shell",
				Pattern:  `curl\s+`,
				Severity: "medium",
				Action:   "warn",
				Message:  "network access detected",
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	call := &ToolCall{Name: "shell", Arguments: `{"command": "curl https://example.com"}`}
	result, err := executor.Check(context.Background(), call)
	require.NoError(t, err)

	assert.True(t, result.Allowed)
	assert.False(t, result.RequireApproval)
	assert.Contains(t, result.Warnings, "network access detected")
}

func TestPolicyExecutor_Check_ParamRules_MaxLength(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		ParamRules: map[string]ParamRule{
			"shell": {MaxLength: 100},
		},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name    string
		args    string
		allowed bool
	}{
		{"within limit", `{"command": "ls"}`, true},
		{"exceeds limit", string(make([]byte, 200)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: "shell", Arguments: tt.args}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}

func TestPolicyExecutor_Check_ParamRules_Forbidden(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		ParamRules: map[string]ParamRule{
			"shell": {
				Forbidden: []string{`/etc/passwd`, `/etc/shadow`},
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name    string
		args    string
		allowed bool
	}{
		{"forbidden path", `{"path": "/etc/passwd"}`, false},
		{"safe path", `{"path": "/tmp/test.txt"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{Name: "shell", Arguments: tt.args}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed)
		})
	}
}

func TestPolicyExecutor_Check_GlobalRequireApproval(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow:    true,
		RequireApproval: true,
	}
	executor := NewPolicyExecutor(policy)

	call := &ToolCall{Name: "read_file", Arguments: `{"path": "/tmp/test.txt"}`}
	result, err := executor.Check(context.Background(), call)
	require.NoError(t, err)

	assert.True(t, result.Allowed)
	assert.True(t, result.RequireApproval)
	assert.Equal(t, "global approval required", result.ApprovalReason)
}

func TestPolicyExecutor_Check_NilCall(t *testing.T) {
	policy := &ToolPolicy{DefaultAllow: true}
	executor := NewPolicyExecutor(policy)

	result, err := executor.Check(context.Background(), nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPolicyExecutor_Check_EmptyAllowlist(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: false,
		Allowlist:    []string{},
	}
	executor := NewPolicyExecutor(policy)

	call := &ToolCall{Name: "any_tool"}
	result, err := executor.Check(context.Background(), call)
	require.NoError(t, err)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "empty allowlist")
}

func TestPolicyExecutor_Check_MultipleRules(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		DangerousOps: []DangerousOpRule{
			{
				Tool:     "shell",
				Pattern:  `curl\s+`,
				Severity: "low",
				Action:   "warn",
				Message:  "network access",
			},
			{
				Tool:     "shell",
				Pattern:  `sudo\s+`,
				Severity: "high",
				Action:   "approve",
				Message:  "sudo requires approval",
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	// Both curl and sudo in command
	call := &ToolCall{Name: "shell", Arguments: `{"command": "sudo curl https://example.com"}`}
	result, err := executor.Check(context.Background(), call)
	require.NoError(t, err)

	assert.True(t, result.Allowed)
	assert.True(t, result.RequireApproval)
	assert.Contains(t, result.Warnings, "network access")
	assert.Len(t, result.MatchedRules, 2)
}

func TestPolicyExecutor_SetPolicy(t *testing.T) {
	policy1 := &ToolPolicy{DefaultAllow: true}
	policy2 := &ToolPolicy{DefaultAllow: false, Allowlist: []string{"read_file"}}

	executor := NewPolicyExecutor(policy1)

	// With policy1, shell is allowed
	call := &ToolCall{Name: "shell"}
	result1, err := executor.Check(context.Background(), call)
	require.NoError(t, err)
	assert.True(t, result1.Allowed)

	// Change policy
	executor.SetPolicy(policy2)

	// With policy2, shell is not allowed
	result2, err := executor.Check(context.Background(), call)
	require.NoError(t, err)
	assert.False(t, result2.Allowed)
}

func TestPolicyExecutor_Check_ParamRules_PathPrefix(t *testing.T) {
	home, _ := os.UserHomeDir()

	policy := &ToolPolicy{
		DefaultAllow: true,
		ParamRules: map[string]ParamRule{
			"read_file": {
				PathPrefix: []string{"~", "/tmp"},
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	tests := []struct {
		name    string
		args    string
		allowed bool
	}{
		{"path within home", `{"path": "` + home + `/project/main.go"}`, true},
		{"path within /tmp", `{"path": "/tmp/test.txt"}`, true},
		{"path outside allowed", `{"path": "/etc/passwd"}`, false},
		{"path outside allowed /usr", `{"path": "/usr/bin/ls"}`, false},
		{"file_path key", `{"file_path": "` + home + `/docs/readme.md"}`, true},
		{"file_path key outside", `{"file_path": "/etc/shadow"}`, false},
		{"no path key in args", `{"content": "hello"}`, true},
		{"invalid JSON args", `not-json`, true},
		{"empty path", `{"path": ""}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &ToolCall{
				Name:      "read_file",
				Arguments: tt.args,
			}
			result, err := executor.Check(context.Background(), call)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, result.Allowed, "args: %s", tt.args)
			if !tt.allowed {
				assert.Contains(t, result.Reason, "not within allowed prefixes")
				assert.Contains(t, result.MatchedRules, "param_rule:path_prefix")
			}
		})
	}
}

func TestPolicyExecutor_Check_ParamRules_PathPrefix_Workspace(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		ParamRules: map[string]ParamRule{
			"write_file": {
				PathPrefix: []string{"$WORKSPACE", "/tmp"},
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	t.Run("path within workspace", func(t *testing.T) {
		call := &ToolCall{
			Name:          "write_file",
			Arguments:     `{"path": "/Users/john/project/src/app.go"}`,
			WorkspacePath: "/Users/john/project",
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})

	t.Run("path outside workspace", func(t *testing.T) {
		call := &ToolCall{
			Name:          "write_file",
			Arguments:     `{"path": "/etc/config"}`,
			WorkspacePath: "/Users/john/project",
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
	})

	t.Run("workspace not set skips $WORKSPACE prefix", func(t *testing.T) {
		call := &ToolCall{
			Name:          "write_file",
			Arguments:     `{"path": "/tmp/test.txt"}`,
			WorkspacePath: "",
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.True(t, result.Allowed) // /tmp still matches
	})

	t.Run("workspace not set blocks non-tmp path", func(t *testing.T) {
		call := &ToolCall{
			Name:          "write_file",
			Arguments:     `{"path": "/Users/john/project/app.go"}`,
			WorkspacePath: "",
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.False(t, result.Allowed) // $WORKSPACE skipped, /tmp doesn't match
	})
}

func TestPolicyExecutor_Check_ParamRules_PathPrefix_DotDot(t *testing.T) {
	policy := &ToolPolicy{
		DefaultAllow: true,
		ParamRules: map[string]ParamRule{
			"read_file": {
				PathPrefix: []string{"/home/user/project"},
			},
		},
	}
	executor := NewPolicyExecutor(policy)

	t.Run("path with dotdot escape", func(t *testing.T) {
		call := &ToolCall{
			Name:      "read_file",
			Arguments: `{"path": "/home/user/project/../../../etc/passwd"}`,
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.False(t, result.Allowed) // filepath.Clean resolves to /etc/passwd
	})

	t.Run("path with dotdot within prefix", func(t *testing.T) {
		call := &ToolCall{
			Name:      "read_file",
			Arguments: `{"path": "/home/user/project/sub/../main.go"}`,
		}
		result, err := executor.Check(context.Background(), call)
		require.NoError(t, err)
		assert.True(t, result.Allowed) // resolves to /home/user/project/main.go
	})
}

func TestExtractPaths(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected []string
	}{
		{"path key", `{"path": "/tmp/test.txt"}`, []string{"/tmp/test.txt"}},
		{"file_path key", `{"file_path": "/tmp/test.txt"}`, []string{"/tmp/test.txt"}},
		{"filename key", `{"filename": "/tmp/test.txt"}`, []string{"/tmp/test.txt"}},
		{"directory key", `{"directory": "/tmp/subdir"}`, []string{"/tmp/subdir"}},
		{"multiple keys", `{"path": "/a", "file": "/b"}`, []string{"/a", "/b"}},
		{"no path key", `{"content": "hello"}`, nil},
		{"empty string path", `{"path": ""}`, nil},
		{"invalid json", `not-json`, nil},
		{"non-string path", `{"path": 123}`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPaths(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandPrefixes(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name          string
		prefixes      []string
		workspacePath string
		expected      []string
	}{
		{"tilde expansion", []string{"~"}, "", []string{home}},
		{"tilde with subpath", []string{"~/project"}, "", []string{filepath.Join(home, "project")}},
		{"workspace expansion", []string{"$WORKSPACE"}, "/Users/john/proj", []string{"/Users/john/proj"}},
		{"workspace not set", []string{"$WORKSPACE"}, "", []string{}},
		{"mixed", []string{"~", "$WORKSPACE", "/tmp"}, "/w", []string{home, "/w", "/tmp"}},
		{"no expansion needed", []string{"/tmp", "/var"}, "", []string{"/tmp", "/var"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPrefixes(tt.prefixes, tt.workspacePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}
