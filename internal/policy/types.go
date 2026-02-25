// Package policy provides tool execution security controls and approval mechanisms.
package policy

import (
	"context"
	"regexp"
)

// ToolCall represents a tool invocation to be checked against policies.
type ToolCall struct {
	// Name is the name of the tool being called.
	Name string `json:"name"`

	// Arguments contains the serialized tool arguments (JSON string).
	Arguments string `json:"arguments"`

	// SessionID is the session this call belongs to.
	SessionID string `json:"session_id"`

	// AgentID is the agent making the call.
	AgentID string `json:"agent_id"`

	// RequestID is an optional pre-generated ID for the approval request.
	// If set, the approval manager will use this instead of generating a new UUID.
	RequestID string `json:"request_id,omitempty"`

	// WorkspacePath is the workspace directory for the current session.
	// Used to expand $WORKSPACE in PathPrefix rules.
	WorkspacePath string `json:"workspace_path,omitempty"`
}

// ToolPolicy defines the security policy for tool execution.
type ToolPolicy struct {
	// DefaultAllow determines whether tools are allowed by default.
	// If false, only allowlisted tools can be executed.
	DefaultAllow bool `yaml:"default_allow" json:"default_allow"`

	// RequireApproval requires all tool calls to be approved.
	RequireApproval bool `yaml:"require_approval" json:"require_approval"`

	// Allowlist contains tools or tool groups that are explicitly allowed.
	// Supports group:xxx syntax and wildcards.
	Allowlist []string `yaml:"allowlist" json:"allowlist"`

	// Blocklist contains tools or tool groups that are explicitly denied.
	// Takes precedence over allowlist.
	Blocklist []string `yaml:"blocklist" json:"blocklist"`

	// DangerousOps defines rules for detecting dangerous operations.
	DangerousOps []DangerousOpRule `yaml:"dangerous_ops" json:"dangerous_ops"`

	// ParamRules defines validation rules for tool parameters.
	ParamRules map[string]ParamRule `yaml:"param_rules" json:"param_rules"`

	// ScrubRules defines custom credential scrubbing rules.
	// Applied after built-in patterns.
	ScrubRules []ScrubRule `yaml:"scrub_rules,omitempty" json:"scrub_rules,omitempty"`

	// BlockMessageTemplate is a customizable template for blocked tool messages.
	// Supports {tool} and {reason} placeholders.
	// Empty string uses default format.
	BlockMessageTemplate string `yaml:"block_message_template,omitempty" json:"block_message_template,omitempty"`

	// CircuitBreakerThreshold is the number of consecutive blocks before
	// injecting a circuit breaker message. 0 disables circuit breaker.
	CircuitBreakerThreshold int `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold,omitempty"`
}

// DangerousOpRule defines a rule for detecting dangerous operations.
type DangerousOpRule struct {
	// Tool is the name of the tool this rule applies to.
	Tool string `yaml:"tool" json:"tool"`

	// Pattern is the regex pattern to match against tool arguments.
	Pattern string `yaml:"pattern" json:"pattern"`

	// Severity indicates the risk level: low, medium, high, critical.
	Severity string `yaml:"severity" json:"severity"`

	// Action determines how to handle a match: block, approve, warn.
	Action string `yaml:"action" json:"action"`

	// Message is the human-readable explanation shown to users.
	Message string `yaml:"message" json:"message"`

	// Enabled controls whether this rule is active. Default true.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// compiledPattern is the cached compiled regex (internal use).
	compiledPattern *regexp.Regexp `yaml:"-" json:"-"`
}

// IsEnabled returns true if the rule is enabled.
// A nil Enabled pointer defaults to true (backward compatible).
func (r *DangerousOpRule) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// CompiledPattern returns the compiled regex, compiling it if needed.
func (r *DangerousOpRule) CompiledPattern() (*regexp.Regexp, error) {
	if r.compiledPattern != nil {
		return r.compiledPattern, nil
	}
	if r.Pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return nil, err
	}
	r.compiledPattern = re
	return re, nil
}

// ParamRule defines validation rules for tool parameters.
type ParamRule struct {
	// MaxLength limits the maximum length of string parameters.
	MaxLength int `yaml:"max_length" json:"max_length"`

	// Pattern is a regex that parameters must match.
	Pattern string `yaml:"pattern" json:"pattern"`

	// Forbidden is a list of values that are not allowed.
	Forbidden []string `yaml:"forbidden" json:"forbidden"`

	// PathPrefix limits file paths to specific prefixes.
	// Supports ~ for home directory expansion and $WORKSPACE for session workspace.
	PathPrefix []string `yaml:"path_prefix" json:"path_prefix"`

	// Enabled controls whether this rule is active. Default true.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// IsEnabled returns true if the rule is enabled.
// A nil Enabled pointer defaults to true (backward compatible).
func (r *ParamRule) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// ScrubRule defines a custom credential scrubbing rule.
type ScrubRule struct {
	// Name is a human-readable name for this rule.
	Name string `yaml:"name" json:"name"`

	// Pattern is the regex pattern to match credentials.
	Pattern string `yaml:"pattern" json:"pattern"`

	// Replacement is the text to replace matched credentials with.
	// If empty, uses the default partial redaction (first 4 chars + ...[REDACTED]).
	Replacement string `yaml:"replacement,omitempty" json:"replacement,omitempty"`

	// Enabled controls whether this rule is active.
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// PolicyResult represents the result of a policy check.
type PolicyResult struct {
	// Allowed indicates whether the tool call is permitted.
	Allowed bool `json:"allowed"`

	// RequireApproval indicates that human approval is needed.
	RequireApproval bool `json:"require_approval"`

	// ApprovalReason explains why approval is required.
	ApprovalReason string `json:"approval_reason,omitempty"`

	// Warnings contains non-blocking warning messages.
	Warnings []string `json:"warnings,omitempty"`

	// Reason explains why the call was denied (if not allowed).
	Reason string `json:"reason,omitempty"`

	// MatchedRules lists the rules that were triggered.
	MatchedRules []string `json:"matched_rules,omitempty"`
}

// PolicyChecker defines the interface for policy checking.
type PolicyChecker interface {
	// Check evaluates whether a tool call is allowed.
	// Returns PolicyResult with allow/deny decision and reasons.
	Check(ctx context.Context, call *ToolCall) (*PolicyResult, error)
}
