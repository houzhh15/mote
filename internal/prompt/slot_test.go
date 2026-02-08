package prompt

import (
	"strings"
	"sync"
	"testing"
)

func TestAllPromptSlots(t *testing.T) {
	slots := AllPromptSlots()
	if len(slots) != 5 {
		t.Errorf("expected 5 slots, got %d", len(slots))
	}

	expected := []PromptSlot{SlotIdentity, SlotCapabilities, SlotContext, SlotConstraints, SlotTail}
	for _, exp := range expected {
		found := false
		for _, s := range slots {
			if s == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing slot: %s", exp)
		}
	}
}

func TestIsValidSlot(t *testing.T) {
	validSlots := []PromptSlot{SlotIdentity, SlotCapabilities, SlotContext, SlotConstraints, SlotTail}
	for _, s := range validSlots {
		if !IsValidSlot(s) {
			t.Errorf("IsValidSlot(%s) = false, expected true", s)
		}
	}

	invalidSlots := []PromptSlot{"invalid", "unknown", ""}
	for _, s := range invalidSlots {
		if IsValidSlot(s) {
			t.Errorf("IsValidSlot(%s) = true, expected false", s)
		}
	}
}

func TestPromptInjector_Inject(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{
		Content:  "I am a helpful assistant",
		Priority: 10,
		Source:   "skill-1",
	})

	if !injector.Has(SlotIdentity) {
		t.Error("expected injection in SlotIdentity")
	}

	if injector.Len() != 1 {
		t.Errorf("expected 1 injection, got %d", injector.Len())
	}
}

func TestPromptInjector_Inject_NilOrEmpty(t *testing.T) {
	injector := NewPromptInjector()

	// Nil injection
	injector.Inject(SlotIdentity, nil)
	if injector.Len() != 0 {
		t.Error("expected no injections after nil inject")
	}

	// Empty content
	injector.Inject(SlotIdentity, &PromptInjection{Content: ""})
	if injector.Len() != 0 {
		t.Error("expected no injections after empty content inject")
	}
}

func TestPromptInjector_Remove(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "a", Source: "skill-1"})
	injector.Inject(SlotIdentity, &PromptInjection{Content: "b", Source: "skill-2"})
	injector.Inject(SlotCapabilities, &PromptInjection{Content: "c", Source: "skill-1"})

	// Remove all from skill-1
	injector.Remove("skill-1")

	if injector.Len() != 1 {
		t.Errorf("expected 1 injection after remove, got %d", injector.Len())
	}

	// Check skill-2 injection remains
	injections := injector.GetBySlot(SlotIdentity)
	if len(injections) != 1 || injections[0].Source != "skill-2" {
		t.Error("expected skill-2 injection to remain")
	}
}

func TestPromptInjector_RemoveFromSlot(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "a", Source: "skill-1"})
	injector.Inject(SlotCapabilities, &PromptInjection{Content: "b", Source: "skill-1"})

	// Remove from specific slot only
	injector.RemoveFromSlot(SlotIdentity, "skill-1")

	if injector.Has(SlotIdentity) {
		t.Error("expected no injections in SlotIdentity")
	}
	if !injector.Has(SlotCapabilities) {
		t.Error("expected injection to remain in SlotCapabilities")
	}
}

func TestPromptInjector_GetBySlot_Priority(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "low", Priority: 1, Source: "a"})
	injector.Inject(SlotIdentity, &PromptInjection{Content: "high", Priority: 10, Source: "b"})
	injector.Inject(SlotIdentity, &PromptInjection{Content: "medium", Priority: 5, Source: "c"})

	injections := injector.GetBySlot(SlotIdentity)

	if len(injections) != 3 {
		t.Fatalf("expected 3 injections, got %d", len(injections))
	}

	// Should be sorted by priority descending
	if injections[0].Content != "high" {
		t.Errorf("expected first to be 'high', got '%s'", injections[0].Content)
	}
	if injections[1].Content != "medium" {
		t.Errorf("expected second to be 'medium', got '%s'", injections[1].Content)
	}
	if injections[2].Content != "low" {
		t.Errorf("expected third to be 'low', got '%s'", injections[2].Content)
	}
}

func TestPromptInjector_GetContentBySlot(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "first", Priority: 10, Source: "a"})
	injector.Inject(SlotIdentity, &PromptInjection{Content: "second", Priority: 5, Source: "b"})

	content := injector.GetContentBySlot(SlotIdentity)

	if !strings.Contains(content, "first") || !strings.Contains(content, "second") {
		t.Errorf("expected content to contain both injections, got '%s'", content)
	}

	// first should come before second due to higher priority
	firstIdx := strings.Index(content, "first")
	secondIdx := strings.Index(content, "second")
	if firstIdx > secondIdx {
		t.Error("expected 'first' to appear before 'second' based on priority")
	}
}

func TestPromptInjector_GetContentBySlot_Empty(t *testing.T) {
	injector := NewPromptInjector()

	content := injector.GetContentBySlot(SlotIdentity)
	if content != "" {
		t.Errorf("expected empty content for empty slot, got '%s'", content)
	}
}

func TestPromptInjector_Clear(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "a", Source: "1"})
	injector.Inject(SlotCapabilities, &PromptInjection{Content: "b", Source: "2"})
	injector.Inject(SlotConstraints, &PromptInjection{Content: "c", Source: "3"})

	injector.Clear()

	if injector.Len() != 0 {
		t.Errorf("expected 0 injections after clear, got %d", injector.Len())
	}
	if injector.SlotCount() != 0 {
		t.Errorf("expected 0 slots after clear, got %d", injector.SlotCount())
	}
}

func TestPromptInjector_ClearSlot(t *testing.T) {
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{Content: "a", Source: "1"})
	injector.Inject(SlotCapabilities, &PromptInjection{Content: "b", Source: "2"})

	injector.ClearSlot(SlotIdentity)

	if injector.Has(SlotIdentity) {
		t.Error("expected SlotIdentity to be cleared")
	}
	if !injector.Has(SlotCapabilities) {
		t.Error("expected SlotCapabilities to remain")
	}
}

func TestPromptInjector_SlotCount(t *testing.T) {
	injector := NewPromptInjector()

	if injector.SlotCount() != 0 {
		t.Errorf("expected 0 slots, got %d", injector.SlotCount())
	}

	injector.Inject(SlotIdentity, &PromptInjection{Content: "a", Source: "1"})
	injector.Inject(SlotCapabilities, &PromptInjection{Content: "b", Source: "2"})
	injector.Inject(SlotIdentity, &PromptInjection{Content: "c", Source: "3"})

	if injector.SlotCount() != 2 {
		t.Errorf("expected 2 slots, got %d", injector.SlotCount())
	}
}

func TestPromptInjector_Concurrent(t *testing.T) {
	injector := NewPromptInjector()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				slot := AllPromptSlots()[id%5]
				injector.Inject(slot, &PromptInjection{
					Content:  "content",
					Priority: j,
					Source:   "source",
				})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				injector.GetBySlot(SlotIdentity)
				injector.GetContentBySlot(SlotCapabilities)
				injector.Len()
				injector.Has(SlotContext)
			}
		}()
	}

	wg.Wait()

	// Verify consistency
	if injector.Len() == 0 {
		t.Error("expected some injections after concurrent writes")
	}
}

func TestSystemPromptBuilder_WithInjector(t *testing.T) {
	builder := NewSystemPromptBuilder(DefaultPromptConfig(), nil)
	injector := NewPromptInjector()

	builder.WithInjector(injector)

	if builder.injector != injector {
		t.Error("expected injector to be set")
	}
}

func TestSystemPromptBuilder_GetInjector(t *testing.T) {
	builder := NewSystemPromptBuilder(DefaultPromptConfig(), nil)

	// Should create injector if not set
	injector := builder.GetInjector()
	if injector == nil {
		t.Error("expected injector to be created")
	}

	// Should return same injector
	injector2 := builder.GetInjector()
	if injector != injector2 {
		t.Error("expected same injector instance")
	}
}

func TestSystemPromptBuilder_RenderWithInjections(t *testing.T) {
	builder := NewSystemPromptBuilder(DefaultPromptConfig(), nil)
	injector := NewPromptInjector()

	injector.Inject(SlotIdentity, &PromptInjection{
		Content:  "### Skill Persona\nI can help with coding.",
		Priority: 10,
		Source:   "coding-skill",
	})
	injector.Inject(SlotTail, &PromptInjection{
		Content:  "Remember to be helpful!",
		Priority: 1,
		Source:   "reminder-skill",
	})

	builder.WithInjector(injector)

	prompt := builder.BuildStatic()

	if !strings.Contains(prompt, "Skill Persona") {
		t.Error("expected identity injection in prompt")
	}
	if !strings.Contains(prompt, "Remember to be helpful!") {
		t.Error("expected tail injection in prompt")
	}
}
