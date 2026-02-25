package policy

import (
	"fmt"
	"os"
	"regexp"

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
			// M08B: High-risk command patterns
			{
				Tool:     "shell",
				Pattern:  `curl\s+.*\|\s*(ba)?sh`,
				Severity: "critical",
				Action:   "block",
				Message:  "pipe to shell execution is prohibited",
			},
			{
				Tool:     "shell",
				Pattern:  `wget\s+.*\|\s*(ba)?sh`,
				Severity: "critical",
				Action:   "block",
				Message:  "pipe to shell execution is prohibited",
			},
			{
				Tool:     "shell",
				Pattern:  `python[23]?\s+-c\s+`,
				Severity: "medium",
				Action:   "approve",
				Message:  "inline Python code requires approval",
			},
			{
				Tool:     "shell",
				Pattern:  `node\s+-e\s+`,
				Severity: "medium",
				Action:   "approve",
				Message:  "inline Node.js code requires approval",
			},
			{
				Tool:     "shell",
				Pattern:  `base64\s+(-d|--decode)`,
				Severity: "medium",
				Action:   "approve",
				Message:  "base64 decode requires approval",
			},
			{
				Tool:     "shell",
				Pattern:  `chmod\s+(777|[+]s)`,
				Severity: "high",
				Action:   "approve",
				Message:  "dangerous permission change requires approval",
			},
		},
		ParamRules: map[string]ParamRule{
			"read_file": {
				PathPrefix: []string{"$WORKSPACE", "~", "/tmp"},
			},
			"write_file": {
				PathPrefix: []string{"$WORKSPACE", "~/.mote", "/tmp"},
			},
		},
		ScrubRules:              []ScrubRule{},
		BlockMessageTemplate:    "",
		CircuitBreakerThreshold: 3,
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

	// Validate scrub rules
	for i, rule := range config.ToolPolicy.ScrubRules {
		if rule.Pattern == "" {
			return fmt.Errorf("policy: scrub_rules[%d]: pattern must not be empty", i)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("policy: scrub_rules[%d]: invalid pattern '%s': %w", i, rule.Pattern, err)
		}
	}

	// Validate circuit breaker threshold
	if config.ToolPolicy.CircuitBreakerThreshold < 0 {
		return fmt.Errorf("policy: circuit_breaker_threshold must be non-negative")
	}

	return nil
}

// ValidatePolicy validates a ToolPolicy independently.
func ValidatePolicy(p *ToolPolicy) error {
	if p == nil {
		return fmt.Errorf("policy: tool policy is nil")
	}
	cfg := &Config{ToolPolicy: *p}
	return ValidateConfig(cfg)
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
	if len(override.ScrubRules) > 0 {
		merged.ScrubRules = override.ScrubRules
	}
	if override.BlockMessageTemplate != "" {
		merged.BlockMessageTemplate = override.BlockMessageTemplate
	}
	if override.CircuitBreakerThreshold > 0 {
		merged.CircuitBreakerThreshold = override.CircuitBreakerThreshold
	}

	return &merged
}
