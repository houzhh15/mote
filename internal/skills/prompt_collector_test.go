package skills

import (
	"sync"
	"testing"
)

func TestPromptCollector_Collect(t *testing.T) {
	collector := NewPromptCollector()

	// Create a mock skill instance
	instance := &SkillInstance{
		Skill: &Skill{ID: "test-skill"},
		Prompts: []*SkillPrompt{
			{SkillID: "test-skill", Name: "prompt1", Content: "Hello", Tags: []string{"greeting"}},
			{SkillID: "test-skill", Name: "prompt2", Content: "World", Tags: []string{"test"}},
		},
	}

	collector.Collect(instance)

	if !collector.Has("test-skill") {
		t.Error("expected skill prompts to be collected")
	}

	prompts := collector.GetBySkill("test-skill")
	if len(prompts) != 2 {
		t.Errorf("expected 2 prompts, got %d", len(prompts))
	}
}

func TestPromptCollector_Collect_NilInstance(t *testing.T) {
	collector := NewPromptCollector()

	// Should not panic
	collector.Collect(nil)

	if collector.Len() != 0 {
		t.Error("expected no prompts after nil collect")
	}
}

func TestPromptCollector_Remove(t *testing.T) {
	collector := NewPromptCollector()

	instance := &SkillInstance{
		Skill: &Skill{ID: "test-skill"},
		Prompts: []*SkillPrompt{
			{SkillID: "test-skill", Name: "prompt1", Content: "Hello"},
		},
	}
	collector.Collect(instance)

	// Verify collected
	if !collector.Has("test-skill") {
		t.Fatal("expected skill to be collected")
	}

	// Remove
	collector.Remove("test-skill")

	// Verify removed
	if collector.Has("test-skill") {
		t.Error("expected skill to be removed")
	}
	if collector.Len() != 0 {
		t.Error("expected no prompts after remove")
	}
}

func TestPromptCollector_GetAll(t *testing.T) {
	collector := NewPromptCollector()

	// Add prompts from multiple skills
	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-a"},
		Prompts: []*SkillPrompt{{Name: "p1"}, {Name: "p2"}},
	})
	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-b"},
		Prompts: []*SkillPrompt{{Name: "p3"}},
	})

	all := collector.GetAll()
	if len(all) != 3 {
		t.Errorf("expected 3 prompts, got %d", len(all))
	}
}

func TestPromptCollector_GetByTag(t *testing.T) {
	collector := NewPromptCollector()

	collector.Collect(&SkillInstance{
		Skill: &Skill{ID: "skill-a"},
		Prompts: []*SkillPrompt{
			{Name: "p1", Tags: []string{"system", "greeting"}},
			{Name: "p2", Tags: []string{"system"}},
		},
	})
	collector.Collect(&SkillInstance{
		Skill: &Skill{ID: "skill-b"},
		Prompts: []*SkillPrompt{
			{Name: "p3", Tags: []string{"greeting"}},
		},
	})

	// Find by tag "greeting"
	greetings := collector.GetByTag("greeting")
	if len(greetings) != 2 {
		t.Errorf("expected 2 greeting prompts, got %d", len(greetings))
	}

	// Find by tag "system"
	system := collector.GetByTag("system")
	if len(system) != 2 {
		t.Errorf("expected 2 system prompts, got %d", len(system))
	}

	// Find by nonexistent tag
	none := collector.GetByTag("nonexistent")
	if len(none) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(none))
	}
}

func TestPromptCollector_Clear(t *testing.T) {
	collector := NewPromptCollector()

	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-a"},
		Prompts: []*SkillPrompt{{Name: "p1"}, {Name: "p2"}},
	})
	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-b"},
		Prompts: []*SkillPrompt{{Name: "p3"}},
	})

	if collector.Len() != 3 {
		t.Fatalf("expected 3 prompts, got %d", collector.Len())
	}

	collector.Clear()

	if collector.Len() != 0 {
		t.Errorf("expected 0 prompts after clear, got %d", collector.Len())
	}
	if collector.SkillCount() != 0 {
		t.Errorf("expected 0 skills after clear, got %d", collector.SkillCount())
	}
}

func TestPromptCollector_SkillCount(t *testing.T) {
	collector := NewPromptCollector()

	if collector.SkillCount() != 0 {
		t.Errorf("expected 0 skills, got %d", collector.SkillCount())
	}

	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-a"},
		Prompts: []*SkillPrompt{{Name: "p1"}},
	})
	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "skill-b"},
		Prompts: []*SkillPrompt{{Name: "p2"}},
	})

	if collector.SkillCount() != 2 {
		t.Errorf("expected 2 skills, got %d", collector.SkillCount())
	}
}

func TestPromptCollector_Concurrent(t *testing.T) {
	collector := NewPromptCollector()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			skillID := string(rune('a' + id))
			for j := 0; j < 20; j++ {
				collector.Collect(&SkillInstance{
					Skill:   &Skill{ID: skillID},
					Prompts: []*SkillPrompt{{Name: "prompt"}},
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
				collector.GetAll()
				collector.GetByTag("system")
				collector.Len()
			}
		}()
	}

	wg.Wait()

	// Verify consistency
	if collector.SkillCount() == 0 {
		t.Error("expected some skills after concurrent writes")
	}
}

func TestPromptCollector_GetBySkill_Copy(t *testing.T) {
	collector := NewPromptCollector()

	collector.Collect(&SkillInstance{
		Skill:   &Skill{ID: "test-skill"},
		Prompts: []*SkillPrompt{{Name: "p1"}, {Name: "p2"}},
	})

	// Get prompts
	prompts := collector.GetBySkill("test-skill")
	originalLen := len(prompts)

	// Modify the returned slice
	prompts = append(prompts, &SkillPrompt{Name: "p3"})

	// Verify internal state wasn't modified
	prompts2 := collector.GetBySkill("test-skill")
	if len(prompts2) != originalLen {
		t.Errorf("internal state was modified: expected %d, got %d", originalLen, len(prompts2))
	}
}
