package skills

import (
	"testing"
)

func TestPromptSection_Build(t *testing.T) {
	// Create a manager with some skills
	manager := NewManager(ManagerConfig{SkillsDir: "/tmp/skills"})

	// Add a test skill to the registry
	testSkill := &Skill{
		ID:          "test-skill",
		Name:        "Test Skill",
		Description: "A skill for testing",
		FilePath:    "/tmp/skills/test-skill/manifest.json",
	}
	manager.registry["test-skill"] = &SkillStatus{
		Skill: testSkill,
		State: SkillStateRegistered,
	}

	section := NewPromptSection(manager)
	result := section.Build()

	if result == "" {
		t.Error("PromptSection.Build() returned empty string")
	}

	// Check for expected content
	if !containsSubstring(result, "available_skills") {
		t.Error("Build() should contain available_skills XML")
	}
	if !containsSubstring(result, "Test Skill") {
		t.Error("Build() should contain skill name")
	}
	if !containsSubstring(result, "A skill for testing") {
		t.Error("Build() should contain skill description")
	}
	if !containsSubstring(result, "read_file") {
		t.Error("Build() should mention read_file tool")
	}
}

func TestPromptSection_BuildNoSkills(t *testing.T) {
	manager := NewManager(ManagerConfig{SkillsDir: "/tmp/skills"})
	section := NewPromptSection(manager)
	result := section.Build()

	if result != "" {
		t.Errorf("PromptSection.Build() with no skills should return empty string, got: %s", result)
	}
}

func TestPromptSection_BuildNilManager(t *testing.T) {
	section := &PromptSection{Manager: nil}
	result := section.Build()

	if result != "" {
		t.Errorf("PromptSection.Build() with nil manager should return empty string, got: %s", result)
	}
}

func TestPromptSection_BuildActivePrompts(t *testing.T) {
	manager := NewManager(ManagerConfig{SkillsDir: "/tmp/skills"})

	// Add an active skill with prompts
	testSkill := &Skill{
		ID:          "test-skill",
		Name:        "Test Skill",
		Description: "A skill for testing",
		FilePath:    "/tmp/skills/test-skill/manifest.json",
	}
	manager.registry["test-skill"] = &SkillStatus{
		Skill: testSkill,
		State: SkillStateActive,
	}
	manager.active["test-skill"] = &SkillInstance{
		Skill: testSkill,
		Prompts: []*SkillPrompt{
			{
				SkillID: "test-skill",
				Name:    "test-prompt",
				Content: "This is a test prompt content.",
			},
		},
	}

	section := NewPromptSection(manager)
	result := section.BuildActivePrompts()

	if result == "" {
		t.Error("BuildActivePrompts() returned empty string with active prompts")
	}
	if !containsSubstring(result, "test-prompt") {
		t.Error("BuildActivePrompts() should contain prompt name")
	}
	if !containsSubstring(result, "This is a test prompt content") {
		t.Error("BuildActivePrompts() should contain prompt content")
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
