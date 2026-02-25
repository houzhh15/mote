package runner

import (
	"testing"

	"mote/internal/policy"
)

func TestScrubCredentials_EnvSecret(t *testing.T) {
	input := `OPENAI_API_KEY=sk-abc123def456ghijklmno`
	got := ScrubCredentials(input)
	if got == input {
		t.Errorf("expected redaction, got unchanged: %s", got)
	}
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
	// Key name should be preserved
	if !contains(got, "OPENAI_API_KEY=") {
		t.Errorf("expected key name preserved, got: %s", got)
	}
}

func TestScrubCredentials_BearerToken(t *testing.T) {
	input := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature`
	got := ScrubCredentials(input)
	if got == input {
		t.Errorf("expected redaction, got unchanged: %s", got)
	}
	if !contains(got, "Bearer") {
		t.Errorf("expected Bearer prefix preserved, got: %s", got)
	}
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
}

func TestScrubCredentials_OpenAIKey(t *testing.T) {
	input := `Using key sk-abcdefghijklmnopqrstuvwxyz for API access`
	got := ScrubCredentials(input)
	if contains(got, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("expected OpenAI key to be redacted, got: %s", got)
	}
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
}

func TestScrubCredentials_GitHubPAT(t *testing.T) {
	input := `token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij`
	got := ScrubCredentials(input)
	if contains(got, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij") {
		t.Errorf("expected GitHub PAT to be redacted, got: %s", got)
	}
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
}

func TestScrubCredentials_AWSAccessKey(t *testing.T) {
	input := `aws_access_key_id = AKIAIOSFODNN7EXAMPLE`
	got := ScrubCredentials(input)
	if contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("expected AWS key to be redacted, got: %s", got)
	}
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
}

func TestScrubCredentials_NoMatch(t *testing.T) {
	input := `Hello, world! status=200 OK`
	got := ScrubCredentials(input)
	if got != input {
		t.Errorf("expected no change for safe content, got: %s", got)
	}
}

func TestScrubCredentials_Mixed(t *testing.T) {
	input := `config:
  OPENAI_API_KEY=sk-proj-abc123def456ghijklmno
  Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0
  github_token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij
  status: 200 OK`

	got := ScrubCredentials(input)

	// All credentials should be scrubbed
	if contains(got, "sk-proj-abc123def456ghijklmno") {
		t.Errorf("OpenAI key not scrubbed: %s", got)
	}
	if contains(got, "eyJhbGciOiJIUzI1NiJ9") {
		t.Errorf("Bearer token not scrubbed: %s", got)
	}
	if contains(got, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij") {
		t.Errorf("GitHub PAT not scrubbed: %s", got)
	}
	// Safe content should remain
	if !contains(got, "status: 200 OK") {
		t.Errorf("safe content was damaged: %s", got)
	}
}

func TestPartialRedact(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ab", "[REDACTED]"},
		{"abcd", "[REDACTED]"},
		{"abcde", "abcd...[REDACTED]"},
		{"sk-abcdefghij", "sk-a...[REDACTED]"},
	}
	for _, tt := range tests {
		got := partialRedact(tt.input)
		if got != tt.expected {
			t.Errorf("partialRedact(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// helpers
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsRedacted(s string) bool {
	return contains(s, "[REDACTED]")
}

// --- Custom ScrubRule Tests ---

func TestCompileScrubRules_Valid(t *testing.T) {
	rules := []policy.ScrubRule{
		{Name: "SSN", Pattern: `\b\d{3}-\d{2}-\d{4}\b`, Replacement: "***-**-****", Enabled: true},
		{Name: "Email", Pattern: `[\w.]+@[\w.]+`, Replacement: "[EMAIL]", Enabled: true},
	}
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(compiled) != 2 {
		t.Fatalf("expected 2 compiled rules, got %d", len(compiled))
	}
	if compiled[0].Name != "SSN" {
		t.Errorf("expected rule name 'SSN', got '%s'", compiled[0].Name)
	}
}

func TestCompileScrubRules_DisabledSkipped(t *testing.T) {
	rules := []policy.ScrubRule{
		{Name: "Active", Pattern: `secret`, Replacement: "[SECRET]", Enabled: true},
		{Name: "Disabled", Pattern: `password`, Replacement: "[PASS]", Enabled: false},
		{Name: "EmptyPattern", Pattern: "", Replacement: "[EMPTY]", Enabled: true},
	}
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule (disabled and empty skipped), got %d", len(compiled))
	}
	if compiled[0].Name != "Active" {
		t.Errorf("expected rule 'Active', got '%s'", compiled[0].Name)
	}
}

func TestCompileScrubRules_InvalidRegex(t *testing.T) {
	rules := []policy.ScrubRule{
		{Name: "Bad", Pattern: `[invalid`, Enabled: true},
	}
	_, err := CompileScrubRules(rules)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !contains(err.Error(), "Bad") {
		t.Errorf("error should mention rule name 'Bad', got: %s", err.Error())
	}
}

func TestScrubCredentials_CustomRuleWithReplacement(t *testing.T) {
	rules := []policy.ScrubRule{
		{Name: "SSN", Pattern: `\b\d{3}-\d{2}-\d{4}\b`, Replacement: "***-**-****", Enabled: true},
	}
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	input := "SSN is 123-45-6789 and status is OK"
	got := ScrubCredentials(input, compiled...)
	if contains(got, "123-45-6789") {
		t.Errorf("SSN not scrubbed: %s", got)
	}
	if !contains(got, "***-**-****") {
		t.Errorf("expected replacement '***-**-****', got: %s", got)
	}
	if !contains(got, "status is OK") {
		t.Errorf("safe content damaged: %s", got)
	}
}

func TestScrubCredentials_CustomRuleNoReplacement(t *testing.T) {
	rules := []policy.ScrubRule{
		{Name: "CreditCard", Pattern: `\b\d{4}-\d{4}-\d{4}-\d{4}\b`, Replacement: "", Enabled: true},
	}
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	input := "Card: 1234-5678-9012-3456"
	got := ScrubCredentials(input, compiled...)
	if contains(got, "1234-5678-9012-3456") {
		t.Errorf("credit card not scrubbed: %s", got)
	}
	// With no replacement, partialRedact should be used
	if !containsRedacted(got) {
		t.Errorf("expected [REDACTED] marker, got: %s", got)
	}
}

func TestScrubCredentials_CustomRulesAfterBuiltin(t *testing.T) {
	// Both built-in and custom rules should apply
	rules := []policy.ScrubRule{
		{Name: "PhoneNumber", Pattern: `\b\d{3}-\d{3}-\d{4}\b`, Replacement: "[PHONE]", Enabled: true},
	}
	compiled, err := CompileScrubRules(rules)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	input := "key: sk-abcdefghijklmnopqrstuvwxyz and phone: 555-123-4567"
	got := ScrubCredentials(input, compiled...)

	// Built-in should scrub the OpenAI key
	if contains(got, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("OpenAI key not scrubbed by builtin: %s", got)
	}
	// Custom rule should scrub the phone number
	if contains(got, "555-123-4567") {
		t.Errorf("phone not scrubbed by custom rule: %s", got)
	}
	if !contains(got, "[PHONE]") {
		t.Errorf("expected custom replacement '[PHONE]', got: %s", got)
	}
}

func TestScrubCredentials_NoCustomRules(t *testing.T) {
	// Variadic with no custom rules - same as before
	input := `OPENAI_API_KEY=sk-abc123def456ghijklmno`
	got := ScrubCredentials(input)
	if got == input {
		t.Errorf("expected built-in redaction, got unchanged: %s", got)
	}
}
