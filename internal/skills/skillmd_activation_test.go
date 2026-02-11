package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSkillMDActivation tests that SKILL.md format skills can be activated.
func TestSkillMDActivation(t *testing.T) {
	// Create a temp directory with a SKILL.md file
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillMD := `---
name: test-skill
description: A test skill in SKILL.md format
homepage: https://example.com
---

# Test Skill

This is a test skill.`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	// Create manager and scan
	mgr := NewManager(ManagerConfig{SkillsDir: tmpDir})
	if err := mgr.ScanDirectory(tmpDir); err != nil {
		t.Fatalf("Failed to scan directory: %v", err)
	}

	// Check if skill was registered
	skill, found := mgr.GetSkill("test-skill")
	if !found {
		t.Fatal("Skill not found in registry")
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", skill.Name)
	}

	if skill.ID != "test-skill" {
		t.Errorf("Expected skill ID 'test-skill', got '%s'", skill.ID)
	}

	if skill.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", skill.Version)
	}

	// Try to activate
	if err := mgr.Activate("test-skill", nil); err != nil {
		t.Fatalf("Failed to activate skill: %v", err)
	}

	// Check if active
	if !mgr.IsActive("test-skill") {
		t.Error("Skill should be active after activation")
	}

	// Check status
	statuses := mgr.ListSkills()
	found = false
	for _, status := range statuses {
		if status.Skill.ID == "test-skill" {
			found = true
			if status.State != SkillStateActive {
				t.Errorf("Expected state %v, got %v", SkillStateActive, status.State)
			}
		}
	}
	if !found {
		t.Error("Skill not found in status list")
	}

	// Deactivate
	if err := mgr.Deactivate("test-skill"); err != nil {
		t.Fatalf("Failed to deactivate skill: %v", err)
	}

	// Check if inactive
	if mgr.IsActive("test-skill") {
		t.Error("Skill should not be active after deactivation")
	}
}
