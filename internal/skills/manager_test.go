package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillTool_Name(t *testing.T) {
	tool := &SkillTool{
		skillID: "test-skill",
		def: &ToolDef{
			Name:        "test_tool",
			Description: "A test tool",
		},
	}

	if tool.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", tool.Name())
	}
}

func TestSkillTool_Description(t *testing.T) {
	tool := &SkillTool{
		def: &ToolDef{
			Description: "A test tool",
		},
	}

	if tool.Description() != "A test tool" {
		t.Errorf("expected description 'A test tool', got '%s'", tool.Description())
	}
}

func TestSkillTool_Parameters(t *testing.T) {
	tool := &SkillTool{
		def: &ToolDef{
			Parameters: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"arg1": {Type: "string"},
				},
				Required: []string{"arg1"},
			},
		},
	}

	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", params["type"])
	}
}

func TestSkillTool_Parameters_Nil(t *testing.T) {
	tool := &SkillTool{
		def: &ToolDef{
			Parameters: nil,
		},
	}

	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("expected type 'object' for nil parameters, got '%v'", params["type"])
	}
}

func TestSkillTool_SkillID(t *testing.T) {
	tool := &SkillTool{
		skillID: "test-skill",
	}

	if tool.SkillID() != "test-skill" {
		t.Errorf("expected skill ID 'test-skill', got '%s'", tool.SkillID())
	}
}

func TestSkillTool_HandlerPath(t *testing.T) {
	tool := &SkillTool{
		skillDir: "/path/to/skill",
		def: &ToolDef{
			Handler: "tools.js#myFunc",
		},
	}

	path := tool.HandlerPath()
	expected := "/path/to/skill/tools.js"
	if path != expected {
		t.Errorf("expected path '%s', got '%s'", expected, path)
	}
}

func TestBuildToolScript(t *testing.T) {
	script := `function myFunc(args) { return args.name; }`
	args := map[string]any{"name": "test"}

	result := buildToolScript(script, "myFunc", args)

	if result == "" {
		t.Error("expected non-empty script")
	}
	// Check that it contains the function call
	if !contains(result, "myFunc(args)") {
		t.Error("expected script to contain function call")
	}
}

func TestEncodeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		contains []string
	}{
		{
			name:     "string value",
			args:     map[string]any{"name": "test"},
			contains: []string{`"name":`, `"test"`},
		},
		{
			name:     "int value",
			args:     map[string]any{"count": 42},
			contains: []string{`"count":`, "42"},
		},
		{
			name:     "bool value",
			args:     map[string]any{"flag": true},
			contains: []string{`"flag":`, "true"},
		},
		{
			name:     "nil value",
			args:     map[string]any{"empty": nil},
			contains: []string{`"empty":`, "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeArgs(tt.args)
			for _, c := range tt.contains {
				if !contains(result, c) {
					t.Errorf("expected '%s' to contain '%s'", result, c)
				}
			}
		})
	}
}

func TestEscapeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello`, `hello`},
		{`hello"world`, `hello\"world`},
		{`hello\world`, `hello\\world`},
		{"hello\nworld", `hello\nworld`},
		{"hello\tworld", `hello\tworld`},
	}

	for _, tt := range tests {
		result := escapeString(tt.input)
		if result != tt.expected {
			t.Errorf("escapeString(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Integration test with actual JS file (requires JSVM)
func TestSkillTool_Execute_Integration(t *testing.T) {
	// Skip if JSVM is not available
	t.Skip("requires JSVM runtime setup")
}

func TestManager_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	manifest := map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(skillDir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Scan directory
	if err := m.ScanDirectory(tmpDir); err != nil {
		t.Fatalf("failed to scan directory: %v", err)
	}

	// Check skill was loaded
	skill, exists := m.GetSkill("test-skill")
	if !exists {
		t.Fatal("skill not found")
	}
	if skill.ID != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.ID)
	}

	// List skills
	skills := m.ListSkills()
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestManager_ActivateDeactivate(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	manifest := map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(skillDir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Load skill
	_, err := m.LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("failed to load skill: %v", err)
	}

	// Activate
	err = m.Activate("test-skill", nil)
	if err != nil {
		t.Fatalf("failed to activate skill: %v", err)
	}

	if !m.IsActive("test-skill") {
		t.Error("skill should be active")
	}

	// Try to activate again (should fail)
	err = m.Activate("test-skill", nil)
	if err == nil {
		t.Error("expected error when activating already active skill")
	}

	// Deactivate
	err = m.Deactivate("test-skill")
	if err != nil {
		t.Fatalf("failed to deactivate skill: %v", err)
	}

	if m.IsActive("test-skill") {
		t.Error("skill should not be active")
	}

	// Try to deactivate again (should fail)
	err = m.Deactivate("test-skill")
	if err == nil {
		t.Error("expected error when deactivating inactive skill")
	}
}

func TestManager_Dependencies(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create base skill
	baseDir := filepath.Join(tmpDir, "base-skill")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("failed to create base dir: %v", err)
	}
	baseManifest, _ := json.Marshal(map[string]any{
		"id":      "base-skill",
		"name":    "Base Skill",
		"version": "1.0.0",
	})
	os.WriteFile(filepath.Join(baseDir, "manifest.json"), baseManifest, 0644)

	// Create dependent skill
	depDir := filepath.Join(tmpDir, "dep-skill")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}
	depManifest, _ := json.Marshal(map[string]any{
		"id":           "dep-skill",
		"name":         "Dependent Skill",
		"version":      "1.0.0",
		"dependencies": []string{"base-skill"},
	})
	os.WriteFile(filepath.Join(depDir, "manifest.json"), depManifest, 0644)

	// Scan
	m.ScanDirectory(tmpDir)

	// Try to activate dependent skill without base (should fail)
	err := m.Activate("dep-skill", nil)
	if err == nil {
		t.Error("expected error when dependency not active")
	}

	// Activate base skill
	err = m.Activate("base-skill", nil)
	if err != nil {
		t.Fatalf("failed to activate base skill: %v", err)
	}

	// Now activate dependent skill
	err = m.Activate("dep-skill", nil)
	if err != nil {
		t.Fatalf("failed to activate dependent skill: %v", err)
	}

	// Try to deactivate base while dep is active (should fail)
	err = m.Deactivate("base-skill")
	if err == nil {
		t.Error("expected error when deactivating skill with dependents")
	}

	// Deactivate in correct order
	m.Deactivate("dep-skill")
	err = m.Deactivate("base-skill")
	if err != nil {
		t.Fatalf("failed to deactivate base skill: %v", err)
	}
}

func TestManager_ActivateNonexistent(t *testing.T) {
	m := NewManager(ManagerConfig{})
	defer m.Close()

	err := m.Activate("nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestManager_DeactivateNonexistent(t *testing.T) {
	m := NewManager(ManagerConfig{})
	defer m.Close()

	err := m.Deactivate("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestManager_GetInstance(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	manifest, _ := json.Marshal(map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	})
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), manifest, 0644)

	m.LoadSkill(skillDir)

	// Should not exist before activation
	_, exists := m.GetInstance("test-skill")
	if exists {
		t.Error("instance should not exist before activation")
	}

	// Activate
	m.Activate("test-skill", map[string]any{"key": "value"})

	// Should exist after activation
	instance, exists := m.GetInstance("test-skill")
	if !exists {
		t.Fatal("instance should exist after activation")
	}
	if instance.Config["key"] != "value" {
		t.Error("config was not set correctly")
	}
}

func TestManager_GetActivePromptsAndTools(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create a test skill with prompts
	skillDir := filepath.Join(tmpDir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	manifest, _ := json.Marshal(map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
		"prompts": []map[string]any{
			{"name": "test_prompt", "content": "Test prompt content"},
		},
	})
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), manifest, 0644)

	m.LoadSkill(skillDir)
	m.Activate("test-skill", nil)

	// Check prompts
	prompts := m.GetActivePrompts()
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(prompts))
	}
	if prompts[0].Content != "Test prompt content" {
		t.Errorf("expected content 'Test prompt content', got '%s'", prompts[0].Content)
	}

	// Check tools (empty since no JSVM)
	tools := m.GetActiveTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools without JSVM, got %d", len(tools))
	}
}

func TestManager_ResolvePromptFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})
	defer m.Close()

	// Create a test skill with prompt file
	skillDir := filepath.Join(tmpDir, "test-skill")
	os.MkdirAll(skillDir, 0755)

	// Write prompt file
	promptContent := "This is a prompt from a file"
	os.WriteFile(filepath.Join(skillDir, "prompt.txt"), []byte(promptContent), 0644)

	manifest, _ := json.Marshal(map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
		"prompts": []map[string]any{
			{"name": "file_prompt", "file": "prompt.txt"},
		},
	})
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), manifest, 0644)

	m.LoadSkill(skillDir)
	m.Activate("test-skill", nil)

	prompts := m.GetActivePrompts()
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(prompts))
	}
	if prompts[0].Content != promptContent {
		t.Errorf("expected content '%s', got '%s'", promptContent, prompts[0].Content)
	}
}

func TestManager_SetDiscoveryPaths(t *testing.T) {
	m := NewManager(ManagerConfig{})

	paths := []string{"/path/one", "/path/two", "/path/three"}
	m.SetDiscoveryPaths(paths)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.discoveryPaths) != 3 {
		t.Errorf("SetDiscoveryPaths() got %d paths, want 3", len(m.discoveryPaths))
	}

	for i, p := range paths {
		if m.discoveryPaths[i] != p {
			t.Errorf("discoveryPaths[%d] = %s, want %s", i, m.discoveryPaths[i], p)
		}
	}
}

func TestManager_ScanAllPaths(t *testing.T) {
	// Create temp directories with skills
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	// Create a skill in tempDir1
	skillDir1 := filepath.Join(tempDir1, "skill1")
	if err := os.MkdirAll(skillDir1, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}
	manifest1 := `{
		"id": "test-skill-1",
		"name": "Test Skill 1",
		"version": "1.0.0"
	}`
	if err := os.WriteFile(filepath.Join(skillDir1, "manifest.json"), []byte(manifest1), 0644); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create a skill in tempDir2
	skillDir2 := filepath.Join(tempDir2, "skill2")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}
	manifest2 := `{
		"id": "test-skill-2",
		"name": "Test Skill 2",
		"version": "1.0.0"
	}`
	if err := os.WriteFile(filepath.Join(skillDir2, "manifest.json"), []byte(manifest2), 0644); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create manager and set discovery paths
	m := NewManager(ManagerConfig{})
	m.SetDiscoveryPaths([]string{tempDir1, tempDir2})

	// Scan all paths
	if err := m.ScanAllPaths(); err != nil {
		t.Errorf("ScanAllPaths() error = %v", err)
	}

	// Check both skills were loaded
	skills := m.ListSkills()
	if len(skills) != 2 {
		t.Errorf("ScanAllPaths() loaded %d skills, want 2", len(skills))
	}

	// Verify skill IDs
	foundSkill1, foundSkill2 := false, false
	for _, status := range skills {
		if status.Skill.ID == "test-skill-1" {
			foundSkill1 = true
		}
		if status.Skill.ID == "test-skill-2" {
			foundSkill2 = true
		}
	}

	if !foundSkill1 {
		t.Error("ScanAllPaths() did not load test-skill-1")
	}
	if !foundSkill2 {
		t.Error("ScanAllPaths() did not load test-skill-2")
	}
}

func TestManager_ScanAllPaths_InvalidPath(t *testing.T) {
	m := NewManager(ManagerConfig{})

	// Set paths with one invalid path
	m.SetDiscoveryPaths([]string{"/nonexistent/path/xyz"})

	// ScanAllPaths should not return error, just log warning
	if err := m.ScanAllPaths(); err != nil {
		t.Errorf("ScanAllPaths() should not return error for invalid paths, got %v", err)
	}
}

func TestManager_ScanAllPaths_EmptyPaths(t *testing.T) {
	m := NewManager(ManagerConfig{})

	// No paths set
	if err := m.ScanAllPaths(); err != nil {
		t.Errorf("ScanAllPaths() should not return error for empty paths, got %v", err)
	}

	skills := m.ListSkills()
	if len(skills) != 0 {
		t.Errorf("ScanAllPaths() loaded %d skills with empty paths, want 0", len(skills))
	}
}
