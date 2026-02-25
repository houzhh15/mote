package runner

import (
	"testing"
)

func TestFormatBlockMessage_DefaultTemplate(t *testing.T) {
	got := formatBlockMessage("", "write_file", "not in allowlist")
	expected := "Tool call blocked by policy: not in allowlist"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestFormatBlockMessage_CustomTemplate(t *testing.T) {
	template := "⛔ {tool} was blocked: {reason}"
	got := formatBlockMessage(template, "exec_command", "dangerous operation")
	expected := "⛔ exec_command was blocked: dangerous operation"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestFormatBlockMessage_TemplateNoPlaceholders(t *testing.T) {
	template := "This tool is not allowed."
	got := formatBlockMessage(template, "rm_rf", "blocked")
	if got != template {
		t.Errorf("expected %q, got %q", template, got)
	}
}

func TestFormatBlockMessage_PartialPlaceholders(t *testing.T) {
	template := "Blocked: {tool}"
	got := formatBlockMessage(template, "delete_file", "policy violation")
	expected := "Blocked: delete_file"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestIncrementBlockCount(t *testing.T) {
	r := &Runner{}

	// First increment
	count := r.incrementBlockCount("session1", "write_file")
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Second increment same tool
	count = r.incrementBlockCount("session1", "write_file")
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Different tool in same session
	count = r.incrementBlockCount("session1", "exec")
	if count != 1 {
		t.Errorf("expected count 1 for different tool, got %d", count)
	}

	// Different session
	count = r.incrementBlockCount("session2", "write_file")
	if count != 1 {
		t.Errorf("expected count 1 for different session, got %d", count)
	}
}

func TestClearBlockCounts(t *testing.T) {
	r := &Runner{}

	r.incrementBlockCount("session1", "write_file")
	r.incrementBlockCount("session1", "write_file")
	r.incrementBlockCount("session2", "exec")

	r.clearBlockCounts("session1")

	// session1 should be gone - next increment starts at 1
	count := r.incrementBlockCount("session1", "write_file")
	if count != 1 {
		t.Errorf("expected count reset to 1 after clear, got %d", count)
	}

	// session2 should be unaffected
	count = r.incrementBlockCount("session2", "exec")
	if count != 2 {
		t.Errorf("expected session2 unaffected, count 2, got %d", count)
	}
}

func TestSetBlockMessageTemplate(t *testing.T) {
	r := &Runner{}
	r.SetBlockMessageTemplate("Custom: {tool} - {reason}")

	r.mu.RLock()
	tmpl := r.blockMessageTemplate
	r.mu.RUnlock()

	if tmpl != "Custom: {tool} - {reason}" {
		t.Errorf("expected template to be set, got %q", tmpl)
	}
}

func TestSetCircuitBreakerThreshold(t *testing.T) {
	r := &Runner{}
	r.SetCircuitBreakerThreshold(5)

	r.mu.RLock()
	threshold := r.circuitBreakerThreshold
	r.mu.RUnlock()

	if threshold != 5 {
		t.Errorf("expected threshold 5, got %d", threshold)
	}
}

func TestSetWorkspaceResolver(t *testing.T) {
	r := &Runner{}
	r.SetWorkspaceResolver(func(sessionID string) string {
		return "/workspace/" + sessionID
	})

	r.mu.RLock()
	resolver := r.workspaceResolver
	r.mu.RUnlock()

	if resolver == nil {
		t.Fatal("expected resolver to be set")
	}
	if got := resolver("test"); got != "/workspace/test" {
		t.Errorf("expected '/workspace/test', got %q", got)
	}
}
