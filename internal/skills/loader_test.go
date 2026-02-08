package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func createTestSkill(t *testing.T, dir string, manifest map[string]any) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
}

func TestLoader_Load_ValidSkill(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	manifest := map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	}
	createTestSkill(t, skillDir, manifest)

	loader := NewLoader()
	skill, err := loader.Load(skillDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.ID != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.ID)
	}
	if skill.FilePath != filepath.Join(skillDir, "manifest.json") {
		t.Errorf("expected FilePath '%s', got '%s'", filepath.Join(skillDir, "manifest.json"), skill.FilePath)
	}
	if skill.LoadedAt.IsZero() {
		t.Error("expected LoadedAt to be set")
	}
}

func TestLoader_Load_NoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Load(skillDir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestLoader_Load_InvalidManifest(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	// Write invalid JSON
	manifestPath := filepath.Join(skillDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{invalid}"), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Load(skillDir)
	if err == nil {
		t.Fatal("expected error for invalid manifest")
	}
}

func TestLoader_ScanDir_MultipleSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple skill directories
	for i, skillID := range []string{"skill-one", "skill-two", "skill-three"} {
		skillDir := filepath.Join(tmpDir, skillID)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill dir: %v", err)
		}
		manifest := map[string]any{
			"id":      skillID,
			"name":    skillID,
			"version": "1.0." + string(rune('0'+i)),
		}
		createTestSkill(t, skillDir, manifest)
	}

	loader := NewLoader()
	skills, err := loader.ScanDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 3 {
		t.Errorf("expected 3 skills, got %d", len(skills))
	}
}

func TestLoader_ScanDir_SkipsNonDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill directory
	skillDir := filepath.Join(tmpDir, "valid-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	manifest := map[string]any{
		"id":      "valid-skill",
		"name":    "Valid Skill",
		"version": "1.0.0",
	}
	createTestSkill(t, skillDir, manifest)

	// Create a file (should be skipped)
	filePath := filepath.Join(tmpDir, "not-a-skill.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	loader := NewLoader()
	skills, err := loader.ScanDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestLoader_ScanDir_SkipsDirsWithoutManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill directory with manifest
	skillDir := filepath.Join(tmpDir, "valid-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	manifest := map[string]any{
		"id":      "valid-skill",
		"name":    "Valid Skill",
		"version": "1.0.0",
	}
	createTestSkill(t, skillDir, manifest)

	// Create a directory without manifest (should be skipped)
	emptyDir := filepath.Join(tmpDir, "empty-dir")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create empty dir: %v", err)
	}

	loader := NewLoader()
	skills, err := loader.ScanDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestLoader_ScanDir_SkipsInvalidSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid skill
	validDir := filepath.Join(tmpDir, "valid-skill")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	validManifest := map[string]any{
		"id":      "valid-skill",
		"name":    "Valid Skill",
		"version": "1.0.0",
	}
	createTestSkill(t, validDir, validManifest)

	// Create an invalid skill (missing required fields)
	invalidDir := filepath.Join(tmpDir, "invalid-skill")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	invalidManifest := map[string]any{
		"name": "Invalid Skill", // missing id and version
	}
	createTestSkill(t, invalidDir, invalidManifest)

	loader := NewLoader()
	skills, err := loader.ScanDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only load the valid skill
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].ID != "valid-skill" {
		t.Errorf("expected ID 'valid-skill', got '%s'", skills[0].ID)
	}
}

func TestLoader_ScanDir_NonexistentDir(t *testing.T) {
	loader := NewLoader()
	skills, err := loader.ScanDir("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil skills for nonexistent dir, got: %v", skills)
	}
}

func TestLoader_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	// Create initial manifest
	manifest := map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	}
	createTestSkill(t, skillDir, manifest)

	loader := NewLoader()
	skill, err := loader.Load(skillDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update manifest
	manifest["version"] = "1.1.0"
	createTestSkill(t, skillDir, manifest)

	// Reload
	reloaded, err := loader.Reload(skill)
	if err != nil {
		t.Fatalf("unexpected error on reload: %v", err)
	}

	if reloaded.Version != "1.1.0" {
		t.Errorf("expected version '1.1.0', got '%s'", reloaded.Version)
	}
}

func TestLoader_Reload_NoFilePath(t *testing.T) {
	loader := NewLoader()
	skill := &Skill{
		ID:       "test-skill",
		Name:     "Test Skill",
		Version:  "1.0.0",
		FilePath: "", // No file path
	}

	_, err := loader.Reload(skill)
	if err == nil {
		t.Fatal("expected error for skill without file path")
	}
}

func TestGetSkillDir(t *testing.T) {
	skill := &Skill{
		FilePath: "/path/to/skill/manifest.json",
	}

	dir := GetSkillDir(skill)
	if dir != "/path/to/skill" {
		t.Errorf("expected '/path/to/skill', got '%s'", dir)
	}
}

func TestGetSkillDir_EmptyPath(t *testing.T) {
	skill := &Skill{
		FilePath: "",
	}

	dir := GetSkillDir(skill)
	if dir != "" {
		t.Errorf("expected empty string, got '%s'", dir)
	}
}
