package memory

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// WriteStrategy represents the memory write strategy.
type WriteStrategy string

// Write strategy constants.
const (
	WriteManual WriteStrategy = "manual" // Manual write only
	WriteAuto   WriteStrategy = "auto"   // Auto write enabled
	WriteGated  WriteStrategy = "gated"  // Gated write (requires rule evaluation)
)

// RuleAction represents the action to take when a rule matches.
type RuleAction string

// Rule action constants.
const (
	ActionAllow     RuleAction = "allow"     // Allow the write
	ActionDeny      RuleAction = "deny"      // Deny the write
	ActionTransform RuleAction = "transform" // Transform before write
)

// ConditionType represents the type of rule condition.
type ConditionType string

// Condition type constants.
const (
	ConditionKeyword  ConditionType = "keyword"  // Keyword matching
	ConditionLength   ConditionType = "length"   // Content length check
	ConditionSemantic ConditionType = "semantic" // Semantic similarity
	ConditionCustom   ConditionType = "custom"   // Custom script evaluation
)

// RuleCondition defines a condition for gating rules.
type RuleCondition struct {
	Type    ConditionType `json:"type"`              // Condition type
	Pattern string        `json:"pattern,omitempty"` // Regex pattern for keyword
	Min     int           `json:"min,omitempty"`     // Minimum value for length
	Max     int           `json:"max,omitempty"`     // Maximum value for length
	Script  string        `json:"script,omitempty"`  // Custom script for custom type
}

// GatingRule defines a rule for gating memory writes.
type GatingRule struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Condition   RuleCondition `json:"condition"`
	Action      RuleAction    `json:"action"`
	Priority    int           `json:"priority"` // Higher priority evaluated first
	Enabled     bool          `json:"enabled"`
}

// RuleEvaluationResult represents the result of rule evaluation.
type RuleEvaluationResult struct {
	Allowed     bool        `json:"allowed"`
	MatchedRule *GatingRule `json:"matched_rule,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

// RuleEngine evaluates gating rules for memory writes.
type RuleEngine struct {
	mu    sync.RWMutex
	rules map[string]*GatingRule
}

// NewRuleEngine creates a new RuleEngine.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules: make(map[string]*GatingRule),
	}
}

// AddRule adds a gating rule to the engine.
func (e *RuleEngine) AddRule(rule *GatingRule) error {
	if rule.ID == "" {
		return ErrInvalidRule
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules[rule.ID] = rule
	return nil
}

// RemoveRule removes a gating rule from the engine.
func (e *RuleEngine) RemoveRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.rules[id]; !exists {
		return ErrRuleNotFound
	}

	delete(e.rules, id)
	return nil
}

// GetRule returns a gating rule by ID.
func (e *RuleEngine) GetRule(id string) (*GatingRule, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rule, exists := e.rules[id]
	return rule, exists
}

// ListRules returns all gating rules sorted by priority (descending).
func (e *RuleEngine) ListRules() []*GatingRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make([]*GatingRule, 0, len(e.rules))
	for _, rule := range e.rules {
		rules = append(rules, rule)
	}

	// Sort by priority (higher first)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	return rules
}

// Evaluate evaluates all rules against content and returns the result.
// Rules are evaluated in priority order (highest first).
// Returns allowed=true if no deny rules match or an allow rule matches first.
func (e *RuleEngine) Evaluate(ctx context.Context, content string) RuleEvaluationResult {
	rules := e.ListRules()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		matched := e.evaluateCondition(ctx, rule.Condition, content)
		if matched {
			switch rule.Action {
			case ActionDeny:
				return RuleEvaluationResult{
					Allowed:     false,
					MatchedRule: rule,
					Reason:      "denied by rule: " + rule.Name,
				}
			case ActionAllow:
				return RuleEvaluationResult{
					Allowed:     true,
					MatchedRule: rule,
					Reason:      "allowed by rule: " + rule.Name,
				}
			case ActionTransform:
				// Transform is treated as allow for now
				return RuleEvaluationResult{
					Allowed:     true,
					MatchedRule: rule,
					Reason:      "transform by rule: " + rule.Name,
				}
			}
		}
	}

	// No matching rule, default to allow
	return RuleEvaluationResult{
		Allowed: true,
		Reason:  "no matching rules",
	}
}

// evaluateCondition evaluates a single condition against content.
func (e *RuleEngine) evaluateCondition(ctx context.Context, cond RuleCondition, content string) bool {
	switch cond.Type {
	case ConditionKeyword:
		return e.evaluateKeyword(cond.Pattern, content)
	case ConditionLength:
		return e.evaluateLength(cond.Min, cond.Max, content)
	case ConditionSemantic:
		// Semantic requires embedder, skip for now
		return false
	case ConditionCustom:
		// Custom requires script execution, skip for now
		return false
	default:
		return false
	}
}

// evaluateKeyword checks if content matches the pattern.
func (e *RuleEngine) evaluateKeyword(pattern, content string) bool {
	if pattern == "" {
		return false
	}

	// Try regex first
	re, err := regexp.Compile(pattern)
	if err == nil {
		return re.MatchString(content)
	}

	// Fall back to simple substring match
	return strings.Contains(strings.ToLower(content), strings.ToLower(pattern))
}

// evaluateLength checks if content length is within bounds.
func (e *RuleEngine) evaluateLength(min, max int, content string) bool {
	length := len(content)

	if min > 0 && length < min {
		return false
	}
	if max > 0 && length > max {
		return true // Content exceeds max length
	}

	return min > 0 && length >= min
}
