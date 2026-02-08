package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.True(t, config.ToolPolicy.DefaultAllow)
	assert.False(t, config.ToolPolicy.RequireApproval)
	assert.True(t, config.Approval.Enabled)
	assert.Equal(t, 300, config.Approval.Timeout)
	assert.Equal(t, 100, config.Approval.MaxPending)
}

func TestDefaultPolicy(t *testing.T) {
	policy := DefaultPolicy()

	assert.True(t, policy.DefaultAllow)
	assert.False(t, policy.RequireApproval)
	assert.Empty(t, policy.Blocklist)
	assert.NotEmpty(t, policy.DangerousOps)
	assert.NotEmpty(t, policy.ParamRules)

	// Check default dangerous ops
	var hasRmRf, hasSudo bool
	for _, rule := range policy.DangerousOps {
		if rule.Message == "rm -rf is prohibited" {
			hasRmRf = true
			assert.Equal(t, "block", rule.Action)
		}
		if rule.Message == "sudo requires approval" {
			hasSudo = true
			assert.Equal(t, "approve", rule.Action)
		}
	}
	assert.True(t, hasRmRf, "should have rm -rf rule")
	assert.True(t, hasSudo, "should have sudo rule")
}

func TestParseConfig(t *testing.T) {
	yaml := `
tool_policy:
  default_allow: false
  require_approval: true
  allowlist:
    - read_file
    - group:memory
  blocklist:
    - dangerous_tool
  dangerous_ops:
    - tool: shell
      pattern: "test_pattern"
      severity: high
      action: block
      message: "test rule"
approval:
  enabled: true
  timeout: 600
  max_pending: 50
`
	config, err := ParseConfig([]byte(yaml))
	require.NoError(t, err)

	assert.False(t, config.ToolPolicy.DefaultAllow)
	assert.True(t, config.ToolPolicy.RequireApproval)
	assert.Equal(t, []string{"read_file", "group:memory"}, config.ToolPolicy.Allowlist)
	assert.Equal(t, []string{"dangerous_tool"}, config.ToolPolicy.Blocklist)
	assert.Len(t, config.ToolPolicy.DangerousOps, 1)
	assert.Equal(t, 600, config.Approval.Timeout)
	assert.Equal(t, 50, config.Approval.MaxPending)
}

func TestParseConfig_Invalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "invalid yaml",
			yaml: "not: valid: yaml:",
		},
		{
			name: "invalid action",
			yaml: `
tool_policy:
  dangerous_ops:
    - tool: shell
      action: invalid_action
`,
		},
		{
			name: "invalid severity",
			yaml: `
tool_policy:
  dangerous_ops:
    - tool: shell
      severity: invalid_severity
`,
		},
		{
			name: "invalid regex pattern",
			yaml: `
tool_policy:
  dangerous_ops:
    - tool: shell
      pattern: "[invalid"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			assert.Error(t, err)
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name:      "nil config",
			config:    nil,
			expectErr: true,
		},
		{
			name: "valid config",
			config: &Config{
				ToolPolicy: ToolPolicy{DefaultAllow: true},
				Approval:   ApprovalConfig{Timeout: 300, MaxPending: 100},
			},
			expectErr: false,
		},
		{
			name: "negative timeout",
			config: &Config{
				ToolPolicy: ToolPolicy{DefaultAllow: true},
				Approval:   ApprovalConfig{Timeout: -1},
			},
			expectErr: true,
		},
		{
			name: "negative max pending",
			config: &Config{
				ToolPolicy: ToolPolicy{DefaultAllow: true},
				Approval:   ApprovalConfig{MaxPending: -1},
			},
			expectErr: true,
		},
		{
			name: "dangerous op without tool or pattern",
			config: &Config{
				ToolPolicy: ToolPolicy{
					DangerousOps: []DangerousOpRule{
						{Action: "block"},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "policy.yaml")

	yaml := `
tool_policy:
  default_allow: true
approval:
  timeout: 120
`
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.True(t, config.ToolPolicy.DefaultAllow)
	assert.Equal(t, 120, config.Approval.Timeout)
}

func TestLoadConfig_NotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "policy.yaml")

	config := &Config{
		ToolPolicy: ToolPolicy{
			DefaultAllow: true,
			Blocklist:    []string{"dangerous_tool"},
		},
		Approval: ApprovalConfig{
			Enabled:    true,
			Timeout:    300,
			MaxPending: 100,
		},
	}

	err := SaveConfig(config, configPath)
	require.NoError(t, err)

	// Load it back
	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, config.ToolPolicy.DefaultAllow, loaded.ToolPolicy.DefaultAllow)
	assert.Equal(t, config.ToolPolicy.Blocklist, loaded.ToolPolicy.Blocklist)
}

func TestMergePolicy(t *testing.T) {
	base := &ToolPolicy{
		DefaultAllow: true,
		Allowlist:    []string{"base_tool"},
		DangerousOps: []DangerousOpRule{
			{Tool: "shell", Action: "warn"},
		},
	}

	override := &ToolPolicy{
		DefaultAllow:    false,
		RequireApproval: true,
		Blocklist:       []string{"blocked_tool"},
	}

	merged := MergePolicy(base, override)

	assert.False(t, merged.DefaultAllow)
	assert.True(t, merged.RequireApproval)
	assert.Equal(t, []string{"base_tool"}, merged.Allowlist) // Not overridden
	assert.Equal(t, []string{"blocked_tool"}, merged.Blocklist)
	assert.Len(t, merged.DangerousOps, 1) // Not overridden
}

func TestMergePolicy_NilInputs(t *testing.T) {
	policy := &ToolPolicy{DefaultAllow: true}

	// base nil
	result := MergePolicy(nil, policy)
	assert.Equal(t, policy, result)

	// override nil
	result = MergePolicy(policy, nil)
	assert.Equal(t, policy, result)
}
