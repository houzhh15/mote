package builtin

import (
	"context"
	"testing"

	"mote/internal/hooks"
)

func TestFilterHook_Handler(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "test", Pattern: "secret", Action: FilterActionMask},
		},
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	handler := hook.Handler("test-filter")
	if handler == nil {
		t.Fatal("expected handler to be created")
	}

	if handler.ID != "test-filter" {
		t.Errorf("expected ID 'test-filter', got '%s'", handler.ID)
	}

	if handler.Priority != 90 {
		t.Errorf("expected priority 90, got %d", handler.Priority)
	}
}

func TestFilterHook_Mask(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "secret", Pattern: "secret-\\w+", Action: FilterActionMask, Replacement: "[REDACTED]"},
		},
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Message = &hooks.MessageContext{
		Content: "my password is secret-abc123",
		Role:    "user",
	}

	handler := hook.Handler("test-filter")
	result, rErr := handler.Handler(context.Background(), hookCtx)
	if rErr != nil {
		t.Fatalf("unexpected error: %v", rErr)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if !result.Continue {
		t.Error("expected Continue to be true for mask action")
	}

	modified, ok := result.Data["content"].(string)
	if !ok {
		t.Fatal("expected modified content")
	}

	if modified != "my password is [REDACTED]" {
		t.Errorf("expected masked content, got '%s'", modified)
	}
}

func TestFilterHook_Block(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "blocked", Pattern: "blocked-word", Action: FilterActionBlock},
		},
		BlockMessage: "Message blocked",
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Message = &hooks.MessageContext{
		Content: "this contains blocked-word",
		Role:    "user",
	}

	handler := hook.Handler("test-filter")
	result, rErr := handler.Handler(context.Background(), hookCtx)
	if rErr != nil {
		t.Fatalf("unexpected error: %v", rErr)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if result.Continue {
		t.Error("expected Continue to be false for block action")
	}

	blockMsg := result.Data["block_message"]
	if blockMsg != "Message blocked" {
		t.Errorf("expected block message, got '%v'", blockMsg)
	}
}

func TestFilterHook_Allow(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "whitelist", Pattern: "allowed-\\w+", Action: FilterActionAllow},
			{Name: "block-all", Pattern: ".*", Action: FilterActionBlock},
		},
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Message = &hooks.MessageContext{
		Content: "this has allowed-word",
		Role:    "user",
	}

	handler := hook.Handler("test-filter")
	result, rErr := handler.Handler(context.Background(), hookCtx)
	if rErr != nil {
		t.Fatalf("unexpected error: %v", rErr)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if !result.Continue {
		t.Error("expected Continue to be true for allow action")
	}
}

func TestFilterHook_NoMessage(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "test", Pattern: "secret", Action: FilterActionBlock},
		},
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	// No message set

	handler := hook.Handler("test-filter")
	result, rErr := handler.Handler(context.Background(), hookCtx)
	if rErr != nil {
		t.Fatalf("unexpected error: %v", rErr)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if !result.Continue {
		t.Error("expected Continue to be true when no message")
	}
}

func TestFilterHook_InvalidPattern(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "invalid", Pattern: "[invalid", Action: FilterActionMask},
		},
	}
	_, err := NewFilterHook(cfg)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}

	if !isErrInvalidPattern(err) {
		t.Errorf("expected ErrInvalidPattern, got %T", err)
	}
}

func isErrInvalidPattern(err error) bool {
	_, ok := err.(*ErrInvalidPattern)
	return ok
}

func TestNewSensitiveDataFilter(t *testing.T) {
	filter, err := NewSensitiveDataFilter()
	if err != nil {
		t.Fatalf("failed to create sensitive data filter: %v", err)
	}

	if filter == nil {
		t.Fatal("expected filter to be created")
	}

	if len(filter.rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(filter.rules))
	}
}

func TestApplyFilterToContent(t *testing.T) {
	cfg := FilterConfig{
		Rules: []FilterRule{
			{Name: "ssn", Pattern: `\d{3}-\d{2}-\d{4}`, Action: FilterActionMask, Replacement: "[SSN]"},
		},
	}
	hook, err := NewFilterHook(cfg)
	if err != nil {
		t.Fatalf("failed to create filter hook: %v", err)
	}

	content := "My SSN is 123-45-6789"
	result, modified, blocked := hook.ApplyFilterToContent(content)

	if blocked {
		t.Error("expected not blocked")
	}

	if !modified {
		t.Error("expected modified")
	}

	if result != "My SSN is [SSN]" {
		t.Errorf("expected masked content, got '%s'", result)
	}
}

func TestMaskString(t *testing.T) {
	tests := []struct {
		input        string
		visibleStart int
		visibleEnd   int
		expected     string
	}{
		{"1234567890", 4, 4, "1234**7890"},
		{"short", 2, 2, "sh*rt"},
		{"ab", 1, 1, "**"},
		{"", 1, 1, ""},
	}

	for _, tt := range tests {
		result := MaskString(tt.input, tt.visibleStart, tt.visibleEnd)
		if result != tt.expected {
			t.Errorf("MaskString(%q, %d, %d) = %q, expected %q",
				tt.input, tt.visibleStart, tt.visibleEnd, result, tt.expected)
		}
	}
}

func TestRegisterFilterHook(t *testing.T) {
	manager := hooks.NewManager()

	err := RegisterFilterHook(manager, FilterConfig{
		Rules: []FilterRule{
			{Name: "test", Pattern: "test", Action: FilterActionMask},
		},
	})
	if err != nil {
		t.Fatalf("failed to register filter hook: %v", err)
	}

	if !manager.HasHandlers(hooks.HookBeforeMessage) {
		t.Error("expected handler registered for before_message")
	}
}
