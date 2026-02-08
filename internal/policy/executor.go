package policy

import (
	"context"
	"fmt"
	"log/slog"
)

// PolicyExecutor implements the PolicyChecker interface.
type PolicyExecutor struct {
	policy  *ToolPolicy
	matcher PatternMatcher
	logger  *slog.Logger
}

// NewPolicyExecutor creates a new PolicyExecutor with the given policy.
func NewPolicyExecutor(policy *ToolPolicy) *PolicyExecutor {
	return &PolicyExecutor{
		policy:  policy,
		matcher: NewDefaultMatcher(),
		logger:  slog.Default(),
	}
}

// SetMatcher sets a custom pattern matcher.
func (e *PolicyExecutor) SetMatcher(m PatternMatcher) {
	e.matcher = m
}

// SetLogger sets a custom logger.
func (e *PolicyExecutor) SetLogger(l *slog.Logger) {
	e.logger = l
}

// Check evaluates whether a tool call is allowed.
// Returns PolicyResult with allow/deny decision and reasons.
//
// Check order:
// 1. Blocklist check (takes precedence)
// 2. Allowlist check (if not default allow)
// 3. Dangerous operations check
// 4. Parameter rules check
func (e *PolicyExecutor) Check(ctx context.Context, call *ToolCall) (*PolicyResult, error) {
	if call == nil {
		return nil, fmt.Errorf("policy: nil tool call")
	}

	result := &PolicyResult{
		Allowed:  true,
		Warnings: []string{},
	}

	e.logger.Debug("policy check started",
		"tool", call.Name,
		"session_id", call.SessionID,
	)

	// 1. Blocklist check - takes precedence over everything
	if e.checkBlocklist(call, result) {
		e.logger.Info("policy check result",
			"tool", call.Name,
			"allowed", false,
			"reason", "blocklist",
		)
		return result, nil
	}

	// 2. Allowlist check - if not default allow
	if e.checkAllowlist(call, result) {
		e.logger.Info("policy check result",
			"tool", call.Name,
			"allowed", false,
			"reason", "not in allowlist",
		)
		return result, nil
	}

	// 3. Dangerous operations check
	if e.checkDangerousOps(call, result) {
		e.logger.Info("policy check result",
			"tool", call.Name,
			"allowed", result.Allowed,
			"require_approval", result.RequireApproval,
			"reason", "dangerous operation",
		)
		// If blocked, return immediately; otherwise continue with warnings/approval
		if !result.Allowed {
			return result, nil
		}
	}

	// 4. Parameter rules check
	if e.checkParamRules(call, result) {
		e.logger.Info("policy check result",
			"tool", call.Name,
			"allowed", false,
			"reason", "parameter validation failed",
		)
		return result, nil
	}

	// 5. Global require approval check
	if e.policy.RequireApproval && !result.RequireApproval {
		result.RequireApproval = true
		result.ApprovalReason = "global approval required"
	}

	e.logger.Info("policy check result",
		"tool", call.Name,
		"allowed", true,
		"require_approval", result.RequireApproval,
	)

	return result, nil
}

// checkBlocklist checks if the tool is in the blocklist.
// Returns true if the tool is blocked (result.Allowed set to false).
func (e *PolicyExecutor) checkBlocklist(call *ToolCall, result *PolicyResult) bool {
	if len(e.policy.Blocklist) == 0 {
		return false
	}

	expanded := ExpandGroups(e.policy.Blocklist)
	if e.matcher.MatchTool(call.Name, expanded) {
		result.Allowed = false
		result.Reason = fmt.Sprintf("tool '%s' is in blocklist", call.Name)
		result.MatchedRules = append(result.MatchedRules, "blocklist")
		return true
	}

	return false
}

// checkAllowlist checks if the tool is in the allowlist.
// Returns true if the tool is denied (result.Allowed set to false).
func (e *PolicyExecutor) checkAllowlist(call *ToolCall, result *PolicyResult) bool {
	// If default allow is true, skip allowlist check
	if e.policy.DefaultAllow {
		return false
	}

	// If no allowlist defined, deny all
	if len(e.policy.Allowlist) == 0 {
		result.Allowed = false
		result.Reason = "no tools allowed (empty allowlist with default_allow=false)"
		result.MatchedRules = append(result.MatchedRules, "empty_allowlist")
		return true
	}

	expanded := ExpandGroups(e.policy.Allowlist)
	if !e.matcher.MatchTool(call.Name, expanded) {
		result.Allowed = false
		result.Reason = fmt.Sprintf("tool '%s' is not in allowlist", call.Name)
		result.MatchedRules = append(result.MatchedRules, "allowlist")
		return true
	}

	return false
}

// checkDangerousOps checks if the tool call matches any dangerous operation rules.
// Returns true if any rule matched (may set RequireApproval or block).
func (e *PolicyExecutor) checkDangerousOps(call *ToolCall, result *PolicyResult) bool {
	if len(e.policy.DangerousOps) == 0 {
		return false
	}

	matched := false
	for i := range e.policy.DangerousOps {
		rule := &e.policy.DangerousOps[i]

		// Check if rule applies to this tool
		if rule.Tool != "" && !e.matcher.MatchTool(call.Name, []string{rule.Tool}) {
			continue
		}

		// Check if arguments match the pattern
		if rule.Pattern != "" {
			matches, err := e.matcher.MatchArgs(call.Arguments, rule.Pattern)
			if err != nil {
				e.logger.Warn("invalid dangerous op pattern",
					"rule", rule.Message,
					"pattern", rule.Pattern,
					"error", err,
				)
				continue
			}
			if !matches {
				continue
			}
		}

		// Rule matched - apply action
		matched = true
		ruleName := rule.Message
		if ruleName == "" {
			ruleName = fmt.Sprintf("dangerous_op:%s", rule.Tool)
		}
		result.MatchedRules = append(result.MatchedRules, ruleName)

		e.logger.Warn("dangerous operation matched",
			"tool", call.Name,
			"rule", ruleName,
			"severity", rule.Severity,
			"action", rule.Action,
		)

		switch rule.Action {
		case "block":
			result.Allowed = false
			result.Reason = rule.Message
			return true // Stop processing on block
		case "approve":
			result.RequireApproval = true
			if result.ApprovalReason == "" {
				result.ApprovalReason = rule.Message
			}
		case "warn":
			result.Warnings = append(result.Warnings, rule.Message)
		default:
			// Unknown action, treat as warn
			result.Warnings = append(result.Warnings, rule.Message)
		}
	}

	return matched
}

// checkParamRules checks if the tool call violates any parameter rules.
// Returns true if validation failed (result.Allowed set to false).
func (e *PolicyExecutor) checkParamRules(call *ToolCall, result *PolicyResult) bool {
	if len(e.policy.ParamRules) == 0 {
		return false
	}

	rule, ok := e.policy.ParamRules[call.Name]
	if !ok {
		return false
	}

	// Check max length
	if rule.MaxLength > 0 && len(call.Arguments) > rule.MaxLength {
		result.Allowed = false
		result.Reason = fmt.Sprintf("arguments exceed max length (%d > %d)", len(call.Arguments), rule.MaxLength)
		result.MatchedRules = append(result.MatchedRules, "param_rule:max_length")
		return true
	}

	// Check pattern
	if rule.Pattern != "" {
		matches, err := e.matcher.MatchArgs(call.Arguments, rule.Pattern)
		if err != nil {
			e.logger.Warn("invalid param rule pattern",
				"tool", call.Name,
				"pattern", rule.Pattern,
				"error", err,
			)
		} else if !matches {
			result.Allowed = false
			result.Reason = "arguments do not match required pattern"
			result.MatchedRules = append(result.MatchedRules, "param_rule:pattern")
			return true
		}
	}

	// Check forbidden values
	for _, forbidden := range rule.Forbidden {
		matches, err := e.matcher.MatchArgs(call.Arguments, forbidden)
		if err != nil {
			continue
		}
		if matches {
			result.Allowed = false
			result.Reason = "arguments contain forbidden value"
			result.MatchedRules = append(result.MatchedRules, "param_rule:forbidden")
			return true
		}
	}

	return false
}

// PolicyStatus holds summary information about the current policy.
type PolicyStatus struct {
	DefaultAllow        bool
	RequireApproval     bool
	BlocklistCount      int
	AllowlistCount      int
	DangerousRulesCount int
	ParamRulesCount     int
}

// Status returns a summary of the current policy configuration.
func (e *PolicyExecutor) Status() PolicyStatus {
	if e.policy == nil {
		return PolicyStatus{
			DefaultAllow: true,
		}
	}

	return PolicyStatus{
		DefaultAllow:        e.policy.DefaultAllow,
		RequireApproval:     e.policy.RequireApproval,
		BlocklistCount:      len(e.policy.Blocklist),
		AllowlistCount:      len(e.policy.Allowlist),
		DangerousRulesCount: len(e.policy.DangerousOps),
		ParamRulesCount:     len(e.policy.ParamRules),
	}
}

// GetPolicy returns the current policy.
func (e *PolicyExecutor) GetPolicy() *ToolPolicy {
	return e.policy
}

// SetPolicy updates the policy.
func (e *PolicyExecutor) SetPolicy(policy *ToolPolicy) {
	e.policy = policy
}
