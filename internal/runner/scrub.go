package runner

import (
	"fmt"
	"regexp"
	"strings"

	"mote/internal/policy"
)

// CredentialPattern defines a named regex pattern for credential detection.
type CredentialPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// CompiledScrubRule is a pre-compiled custom scrub rule ready for execution.
type CompiledScrubRule struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement string
}

// defaultPatterns holds compiled credential patterns.
var defaultPatterns = []CredentialPattern{
	{
		Name:    "EnvSecret",
		Pattern: regexp.MustCompile(`(?i)(API_KEY|SECRET|TOKEN|PASSWORD|CREDENTIAL|AUTH|PRIVATE[._]KEY)\s*[=:]\s*['"]?(\S{8,})`),
	},
	{
		Name:    "BearerToken",
		Pattern: regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9\-._~+/]{20,}=*)`),
	},
	{
		Name:    "OpenAIKey",
		Pattern: regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	},
	{
		Name:    "GitHubPAT",
		Pattern: regexp.MustCompile(`gh[ps]_[A-Za-z0-9]{36}`),
	},
	{
		Name:    "AWSAccessKey",
		Pattern: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:    "GenericHex",
		Pattern: regexp.MustCompile(`(?i)(secret|key|token)['":\s]+[0-9a-f]{32,}`),
	},
}

// ScrubCredentials replaces detected credentials with redacted placeholders.
// Applies built-in patterns first, then any custom rules.
func ScrubCredentials(input string, customRules ...CompiledScrubRule) string {
	result := input
	// 1. Apply built-in patterns
	for _, cp := range defaultPatterns {
		name := cp.Name
		result = cp.Pattern.ReplaceAllStringFunc(result, func(match string) string {
			return redactValue(match, name)
		})
	}
	// 2. Apply custom rules
	for _, cr := range customRules {
		if cr.Replacement != "" {
			result = cr.Pattern.ReplaceAllString(result, cr.Replacement)
		} else {
			result = cr.Pattern.ReplaceAllStringFunc(result, func(match string) string {
				return partialRedact(match)
			})
		}
	}
	return result
}

// CompileScrubRules compiles policy ScrubRules into executable CompiledScrubRules.
// Returns an error if any enabled rule has an invalid regex pattern.
func CompileScrubRules(rules []policy.ScrubRule) ([]CompiledScrubRule, error) {
	var compiled []CompiledScrubRule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid scrub rule '%s': %w", r.Name, err)
		}
		compiled = append(compiled, CompiledScrubRule{
			Name:        r.Name,
			Pattern:     re,
			Replacement: r.Replacement,
		})
	}
	return compiled, nil
}

// redactValue replaces the credential value portion of a match.
// For key=value patterns, preserves the key; for bare tokens, preserves prefix.
func redactValue(match string, patternName string) string {
	switch patternName {
	case "EnvSecret":
		// Preserve "KEY_NAME=" prefix, redact value
		idx := strings.IndexAny(match, "=:")
		if idx >= 0 {
			sep := match[:idx+1]
			val := strings.TrimSpace(match[idx+1:])
			val = strings.Trim(val, `'"`)
			return sep + " " + partialRedact(val)
		}
	case "BearerToken":
		parts := strings.SplitN(match, " ", 2)
		if len(parts) == 2 {
			return parts[0] + " " + partialRedact(parts[1])
		}
	case "GenericHex":
		idx := strings.IndexAny(match, `'"=: `)
		if idx >= 0 {
			prefix := match[:idx+1]
			val := strings.TrimLeft(match[idx+1:], `'"=: `)
			return prefix + partialRedact(val)
		}
	}
	return partialRedact(match)
}

// partialRedact keeps the first 4 chars and replaces the rest.
func partialRedact(s string) string {
	if len(s) <= 4 {
		return "[REDACTED]"
	}
	return s[:4] + "...[REDACTED]"
}
