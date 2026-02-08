package policy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the full policy configuration.
type Config struct {
	ToolPolicy ToolPolicy     `yaml:"tool_policy" json:"tool_policy"`
	Approval   ApprovalConfig `yaml:"approval" json:"approval"`
}

// ApprovalConfig defines approval-related settings.
type ApprovalConfig struct {
	// Enabled determines whether approval is enabled.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Timeout is the approval timeout in seconds.
	Timeout int `yaml:"timeout" json:"timeout"`

	// MaxPending is the maximum number of pending approval requests.
	MaxPending int `yaml:"max_pending" json:"max_pending"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		ToolPolicy: DefaultPolicy(),
		Approval: ApprovalConfig{
			Enabled:    true,
			Timeout:    300,
			MaxPending: 100,
		},
	}
}

// DefaultPolicy returns the default security policy.
func DefaultPolicy() ToolPolicy {
	return ToolPolicy{
		DefaultAllow:    true,
		RequireApproval: false,
		Blocklist:       []string{},
		Allowlist:       []string{},
		DangerousOps: []DangerousOpRule{
			{
				Tool:     "shell",
				Pattern:  `rm\s+(-[rf]+\s+)*(-[rf]+)`,
				Severity: "critical",
				Action:   "block",
				Message:  "rm -rf is prohibited",
			},
			{
				Tool:     "shell",
				Pattern:  `sudo\s+`,
				Severity: "high",
				Action:   "approve",
				Message:  "sudo requires approval",
			},
			{
				Tool:     "write_file",
				Pattern:  `"/(etc|usr|bin|sbin)/`,
				Severity: "high",
				Action:   "block",
				Message:  "writing to system directories is prohibited",
			},
		},
		ParamRules: map[string]ParamRule{
			"read_file": {
				PathPrefix: []string{"~", "/tmp"},
			},
			"write_file": {
				PathPrefix: []string{"~/.mote", "/tmp"},
			},
		},
	}
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: failed to read config file: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses configuration from YAML data.
func ParseConfig(data []byte) (*Config, error) {
	config := DefaultConfig()

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("policy: failed to parse config: %w", err)
	}

	if err := ValidateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// ValidateConfig validates the configuration.
func ValidateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("policy: config is nil")
	}

	// Validate dangerous ops rules
	for i, rule := range config.ToolPolicy.DangerousOps {
		if rule.Tool == "" && rule.Pattern == "" {
			return fmt.Errorf("policy: dangerous_ops[%d]: must specify tool or pattern", i)
		}

		if rule.Action != "" && rule.Action != "block" && rule.Action != "approve" && rule.Action != "warn" {
			return fmt.Errorf("policy: dangerous_ops[%d]: invalid action '%s' (must be block, approve, or warn)", i, rule.Action)
		}

		if rule.Severity != "" {
			validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
			if !validSeverities[rule.Severity] {
				return fmt.Errorf("policy: dangerous_ops[%d]: invalid severity '%s'", i, rule.Severity)
			}
		}

		// Validate regex pattern
		if rule.Pattern != "" {
			_, err := rule.CompiledPattern()
			if err != nil {
				return fmt.Errorf("policy: dangerous_ops[%d]: invalid pattern '%s': %w", i, rule.Pattern, err)
			}
		}
	}

	// Validate approval config
	if config.Approval.Timeout < 0 {
		return fmt.Errorf("policy: approval.timeout must be non-negative")
	}

	if config.Approval.MaxPending < 0 {
		return fmt.Errorf("policy: approval.max_pending must be non-negative")
	}

	return nil
}

// SaveConfig saves configuration to a YAML file.
func SaveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("policy: failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("policy: failed to write config file: %w", err)
	}

	return nil
}

// MergePolicy merges two policies, with override taking precedence.
func MergePolicy(base, override *ToolPolicy) *ToolPolicy {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}

	merged := *base

	// Override simple fields
	merged.DefaultAllow = override.DefaultAllow
	merged.RequireApproval = override.RequireApproval

	// Merge lists
	if len(override.Allowlist) > 0 {
		merged.Allowlist = override.Allowlist
	}
	if len(override.Blocklist) > 0 {
		merged.Blocklist = override.Blocklist
	}
	if len(override.DangerousOps) > 0 {
		merged.DangerousOps = override.DangerousOps
	}
	if len(override.ParamRules) > 0 {
		merged.ParamRules = override.ParamRules
	}

	return &merged
}
