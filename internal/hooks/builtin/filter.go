package builtin

import (
	"context"
	"regexp"
	"strings"

	"mote/internal/hooks"

	"github.com/rs/zerolog/log"
)

// FilterAction defines what action to take when a pattern matches.
type FilterAction string

const (
	// FilterActionMask replaces matched content with asterisks.
	FilterActionMask FilterAction = "mask"
	// FilterActionBlock stops message processing.
	FilterActionBlock FilterAction = "block"
	// FilterActionAllow allows the message to continue (whitelist mode).
	FilterActionAllow FilterAction = "allow"
	// FilterActionWarn logs a warning but allows the message to continue.
	FilterActionWarn FilterAction = "warn"
)

// FilterRule defines a single filter rule.
type FilterRule struct {
	// Name is a descriptive name for the rule.
	Name string
	// Pattern is the regex pattern to match.
	Pattern string
	// Action is what to do when the pattern matches.
	Action FilterAction
	// Replacement is used when Action is FilterActionMask (optional, default: "***").
	Replacement string
	// compiled is the compiled regex pattern.
	compiled *regexp.Regexp
}

// FilterConfig configures the filter hook.
type FilterConfig struct {
	// Rules is the list of filter rules.
	Rules []FilterRule
	// DefaultAction is the action to take when no rules match (default: allow).
	DefaultAction FilterAction
	// BlockMessage is the message to return when blocking (optional).
	BlockMessage string
}

// FilterHook provides message filtering functionality.
type FilterHook struct {
	rules         []FilterRule
	defaultAction FilterAction
	blockMessage  string
}

// NewFilterHook creates a new filter hook with the given configuration.
func NewFilterHook(cfg FilterConfig) (*FilterHook, error) {
	rules := make([]FilterRule, 0, len(cfg.Rules))

	for _, rule := range cfg.Rules {
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, &ErrInvalidPattern{Pattern: rule.Pattern, Err: err}
		}
		rule.compiled = compiled
		if rule.Replacement == "" {
			rule.Replacement = "***"
		}
		rules = append(rules, rule)
	}

	defaultAction := cfg.DefaultAction
	if defaultAction == "" {
		defaultAction = FilterActionAllow
	}

	return &FilterHook{
		rules:         rules,
		defaultAction: defaultAction,
		blockMessage:  cfg.BlockMessage,
	}, nil
}

// ErrInvalidPattern is returned when a regex pattern is invalid.
type ErrInvalidPattern struct {
	Pattern string
	Err     error
}

func (e *ErrInvalidPattern) Error() string {
	return "invalid filter pattern '" + e.Pattern + "': " + e.Err.Error()
}

func (e *ErrInvalidPattern) Unwrap() error {
	return e.Err
}

// Handler returns a hook handler that filters messages.
func (h *FilterHook) Handler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    90, // High priority, after logging
		Source:      "_builtin",
		Description: "Filters messages based on configured rules",
		Enabled:     true,
		Handler:     h.handle,
	}
}

func (h *FilterHook) handle(_ context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	// Only filter messages
	if hookCtx.Message == nil || hookCtx.Message.Content == "" {
		return hooks.ContinueResult(), nil
	}

	content := hookCtx.Message.Content
	modified := false
	blocked := false

	for _, rule := range h.rules {
		if rule.compiled == nil {
			continue
		}

		if !rule.compiled.MatchString(content) {
			continue
		}

		log.Debug().
			Str("rule_name", rule.Name).
			Str("action", string(rule.Action)).
			Msg("filter rule matched")

		switch rule.Action {
		case FilterActionMask:
			content = rule.compiled.ReplaceAllString(content, rule.Replacement)
			modified = true

		case FilterActionBlock:
			blocked = true

		case FilterActionWarn:
			log.Warn().
				Str("rule_name", rule.Name).
				Str("pattern", rule.Pattern).
				Msg("filter warn: suspicious content detected")

		case FilterActionAllow:
			// Explicitly allow, skip remaining rules
			return hooks.ContinueResult(), nil
		}

		if blocked {
			break
		}
	}

	if blocked {
		log.Info().Msg("message blocked by filter")
		result := hooks.StopResult()
		if h.blockMessage != "" {
			result.Data = map[string]any{"block_message": h.blockMessage}
		}
		return result, nil
	}

	if modified {
		return hooks.ModifiedResult(map[string]any{
			"content": content,
		}), nil
	}

	return hooks.ContinueResult(), nil
}

// RegisterFilterHook registers the filter hook for message processing.
func RegisterFilterHook(manager *hooks.Manager, cfg FilterConfig) error {
	hook, err := NewFilterHook(cfg)
	if err != nil {
		return err
	}

	// Register for before_message to filter incoming messages
	handler := hook.Handler("builtin:filter:before_message")
	if err := manager.Register(hooks.HookBeforeMessage, handler); err != nil {
		return err
	}

	return nil
}

// CommonFilterPatterns provides commonly used filter patterns.
var CommonFilterPatterns = struct {
	// CreditCard matches credit card numbers (simplified).
	CreditCard string
	// SSN matches US Social Security Numbers.
	SSN string
	// Email matches email addresses.
	Email string
	// Phone matches phone numbers (simplified).
	Phone string
	// IPAddress matches IP addresses.
	IPAddress string
	// APIKey matches common API key patterns.
	APIKey string
}{
	CreditCard: `\b(?:\d[ -]*?){13,16}\b`,
	SSN:        `\b\d{3}-\d{2}-\d{4}\b`,
	Email:      `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
	Phone:      `\b(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`,
	IPAddress:  `\b(?:\d{1,3}\.){3}\d{1,3}\b`,
	APIKey:     `\b(?:sk|api|key|token|secret)[-_]?[A-Za-z0-9]{20,}\b`,
}

// NewSensitiveDataFilter creates a filter configured for common sensitive data patterns.
func NewSensitiveDataFilter() (*FilterHook, error) {
	return NewFilterHook(FilterConfig{
		Rules: []FilterRule{
			{Name: "credit_card", Pattern: CommonFilterPatterns.CreditCard, Action: FilterActionMask},
			{Name: "ssn", Pattern: CommonFilterPatterns.SSN, Action: FilterActionMask},
			{Name: "api_key", Pattern: CommonFilterPatterns.APIKey, Action: FilterActionMask},
		},
		DefaultAction: FilterActionAllow,
	})
}

// ApplyFilterToContent applies filter rules to content directly.
func (h *FilterHook) ApplyFilterToContent(content string) (string, bool, bool) {
	modified := false
	blocked := false

	for _, rule := range h.rules {
		if rule.compiled == nil {
			continue
		}

		if !rule.compiled.MatchString(content) {
			continue
		}

		switch rule.Action {
		case FilterActionMask:
			newContent := rule.compiled.ReplaceAllString(content, rule.Replacement)
			if newContent != content {
				content = newContent
				modified = true
			}
		case FilterActionBlock:
			blocked = true
		case FilterActionAllow:
			return content, false, false
		}

		if blocked {
			break
		}
	}

	return content, modified, blocked
}

// MaskString masks a string while preserving some visible characters.
func MaskString(s string, visibleStart, visibleEnd int) string {
	if len(s) <= visibleStart+visibleEnd {
		return strings.Repeat("*", len(s))
	}

	masked := make([]byte, len(s))
	for i := range masked {
		if i < visibleStart || i >= len(s)-visibleEnd {
			masked[i] = s[i]
		} else {
			masked[i] = '*'
		}
	}
	return string(masked)
}

// PromptInjectionPatterns contains patterns detecting common prompt injection attempts.
var PromptInjectionPatterns = map[string]string{
	"IgnorePrevious":  `(?i)ignore\s+(all\s+)?(previous|prior|above)\s+instructions?`,
	"SystemOverride":  `(?i)system\s*:\s*you\s+are`,
	"RoleSwitch":      `(?i)\](system|assistant)\]?:`,
	"NewInstructions": `(?i)new\s+instructions?\s*:`,
	"JailbreakDAN":    `(?i)(DAN|do\s+anything\s+now)`,
}

// NewPromptInjectionDetector creates a FilterHook that detects injection patterns.
// Default action is "warn" — logs but does not block, to avoid false positives.
func NewPromptInjectionDetector() (*FilterHook, error) {
	rules := make([]FilterRule, 0, len(PromptInjectionPatterns))
	for name, pattern := range PromptInjectionPatterns {
		rules = append(rules, FilterRule{
			Name:    "injection_" + name,
			Pattern: pattern,
			Action:  FilterActionWarn,
		})
	}
	return NewFilterHook(FilterConfig{
		Rules:         rules,
		DefaultAction: FilterActionAllow,
	})
}

// RegisterPromptInjectionDetector registers the injection detection hook.
func RegisterPromptInjectionDetector(manager *hooks.Manager) error {
	hook, err := NewPromptInjectionDetector()
	if err != nil {
		return err
	}

	handler := hook.Handler("builtin:injection-detect")
	handler.Priority = 100 // High priority, before rate limit (200)
	handler.Description = "Detects common prompt injection patterns"

	return manager.Register(hooks.HookBeforeMessage, handler)
}
