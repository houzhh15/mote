package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SkillLifecycle tests the complete skill lifecycle:
// scan -> load -> activate -> use tools -> deactivate
func TestIntegration_SkillLifecycle(t *testing.T) {
	// Get the testdata directory path
	testdataDir := filepath.Join("testdata", "example-skill")

	// Verify testdata exists
	_, err := os.Stat(testdataDir)
	require.NoError(t, err, "testdata directory should exist")

	// Create manager
	manager := NewManager(ManagerConfig{
		SkillsDir: "testdata",
	})
	require.NotNil(t, manager)

	// Load the skill from manifest
	manifestPath := filepath.Join(testdataDir, "manifest.json")
	skill, err := ParseManifest(manifestPath)
	require.NoError(t, err, "should parse manifest")
	require.NotNil(t, skill)

	// Verify skill properties
	assert.Equal(t, "example-skill", skill.ID)
	assert.Equal(t, "Example Skill", skill.Name)
	assert.Equal(t, "1.0.0", skill.Version)
	assert.Len(t, skill.Tools, 2)
	assert.Len(t, skill.Prompts, 1)
	assert.Len(t, skill.Hooks, 1)

	// Load the skill via manager
	loadedSkill, err := manager.LoadSkill(testdataDir)
	require.NoError(t, err, "should load skill")
	assert.Equal(t, skill.ID, loadedSkill.ID)

	// Verify skill is registered but not active
	assert.False(t, manager.IsActive(skill.ID), "skill should not be active initially")

	// List skills
	skills := manager.ListSkills()
	require.Len(t, skills, 1)
	assert.Equal(t, SkillStateRegistered, skills[0].State)

	// Activate the skill (without JS runtime, tools won't be registered)
	err = manager.Activate(skill.ID, nil)
	require.NoError(t, err, "should activate skill")

	// Verify skill is now active
	assert.True(t, manager.IsActive(skill.ID), "skill should be active")

	// List skills again - state should be active
	skills = manager.ListSkills()
	require.Len(t, skills, 1)
	assert.Equal(t, SkillStateActive, skills[0].State)

	// Check that prompts are available
	prompts := manager.GetActivePrompts()
	assert.Len(t, prompts, 1, "should have 1 active prompt")
	assert.Equal(t, "greeting", prompts[0].Name)

	// Deactivate the skill
	err = manager.Deactivate(skill.ID)
	require.NoError(t, err, "should deactivate skill")

	// Verify skill is inactive
	assert.False(t, manager.IsActive(skill.ID), "skill should not be active after deactivation")

	// List skills - state should be registered again
	skills = manager.ListSkills()
	require.Len(t, skills, 1)
	assert.Equal(t, SkillStateRegistered, skills[0].State)
}

// TestIntegration_ScanDirectory tests scanning a directory for skills
func TestIntegration_ScanDirectory(t *testing.T) {
	manager := NewManager(ManagerConfig{
		SkillsDir: "testdata",
	})

	// Scan the testdata directory
	err := manager.ScanDirectory("testdata")
	require.NoError(t, err, "should scan directory")

	// List all registered skills
	skills := manager.ListSkills()
	assert.GreaterOrEqual(t, len(skills), 1, "should find at least one skill")

	// Find our example skill
	var found bool
	for _, status := range skills {
		if status.Skill.ID == "example-skill" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find example-skill")
}

// TestIntegration_MultipleSkills tests managing multiple skills
func TestIntegration_MultipleSkills(t *testing.T) {
	// Create temp directories for skills
	tmpDir := t.TempDir()

	// Create skill1 manifest
	skill1Dir := filepath.Join(tmpDir, "skill-1")
	require.NoError(t, os.MkdirAll(skill1Dir, 0755))
	skill1Manifest := `{
		"id": "skill-1",
		"name": "Skill One",
		"version": "1.0.0",
		"tools": [{"name": "tool1", "description": "Tool 1", "handler": "main.js#tool1"}]
	}`
	require.NoError(t, _ = os.WriteFile(filepath.Join(skill1Dir, "manifest.json"), []byte(skill1Manifest), 0644))
	require.NoError(t, _ = os.WriteFile(filepath.Join(skill1Dir, "main.js"), []byte("function tool1() {}"), 0644))

	// Create skill2 manifest
	skill2Dir := filepath.Join(tmpDir, "skill-2")
	require.NoError(t, os.MkdirAll(skill2Dir, 0755))
	skill2Manifest := `{
		"id": "skill-2",
		"name": "Skill Two",
		"version": "1.0.0",
		"tools": [
			{"name": "tool2", "description": "Tool 2", "handler": "main.js#tool2"},
			{"name": "tool3", "description": "Tool 3", "handler": "main.js#tool3"}
		]
	}`
	require.NoError(t, _ = os.WriteFile(filepath.Join(skill2Dir, "manifest.json"), []byte(skill2Manifest), 0644))
	require.NoError(t, _ = os.WriteFile(filepath.Join(skill2Dir, "main.js"), []byte("function tool2() {} function tool3() {}"), 0644))

	manager := NewManager(ManagerConfig{
		SkillsDir: tmpDir,
	})

	// Load both skills
	_, err := manager.LoadSkill(skill1Dir)
	require.NoError(t, err)
	_, err = manager.LoadSkill(skill2Dir)
	require.NoError(t, err)

	// Verify both registered
	skills := manager.ListSkills()
	assert.Len(t, skills, 2)

	// Activate skill1 only
	require.NoError(t, manager.Activate("skill-1", nil))
	assert.True(t, manager.IsActive("skill-1"))
	assert.False(t, manager.IsActive("skill-2"))

	// Activate skill2 as well
	require.NoError(t, manager.Activate("skill-2", nil))
	assert.True(t, manager.IsActive("skill-1"))
	assert.True(t, manager.IsActive("skill-2"))

	// Deactivate skill1
	require.NoError(t, manager.Deactivate("skill-1"))
	assert.False(t, manager.IsActive("skill-1"))
	assert.True(t, manager.IsActive("skill-2"))
}

// TestIntegration_SkillError tests error handling during skill operations
func TestIntegration_SkillError(t *testing.T) {
	manager := NewManager(ManagerConfig{})

	// Try to activate non-existent skill
	err := manager.Activate("non-existent", nil)
	assert.ErrorIs(t, err, ErrSkillNotFound)

	// Try to deactivate non-existent skill
	err = manager.Deactivate("non-existent")
	assert.ErrorIs(t, err, ErrSkillNotFound)

	// Try to get non-existent skill
	_, found := manager.GetSkill("non-existent")
	assert.False(t, found)
}

// TestIntegration_SkillDuplicateActivation tests duplicate activation handling
func TestIntegration_SkillDuplicateActivation(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0"
	}`
	require.NoError(t, _ = os.WriteFile(filepath.Join(skillDir, "manifest.json"), []byte(manifest), 0644))

	manager := NewManager(ManagerConfig{})
	_, err := manager.LoadSkill(skillDir)
	require.NoError(t, err)

	// First activation should succeed
	require.NoError(t, manager.Activate("test-skill", nil))

	// Second activation should fail
	err = manager.Activate("test-skill", nil)
	assert.ErrorIs(t, err, ErrSkillAlreadyActive)
}

// TestIntegration_DeactivateInactiveSkill tests deactivating an inactive skill
func TestIntegration_DeactivateInactiveSkill(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0"
	}`
	require.NoError(t, _ = os.WriteFile(filepath.Join(skillDir, "manifest.json"), []byte(manifest), 0644))

	manager := NewManager(ManagerConfig{})
	_, err := manager.LoadSkill(skillDir)
	require.NoError(t, err)

	// Try to deactivate without activating first
	err = manager.Deactivate("test-skill")
	assert.ErrorIs(t, err, ErrSkillNotActive)
}

// TestIntegration_SkillPromptResolution tests that prompts are correctly resolved
func TestIntegration_SkillPromptResolution(t *testing.T) {
	manager := NewManager(ManagerConfig{
		SkillsDir: "testdata",
	})

	// Load and activate the example skill
	testdataDir := filepath.Join("testdata", "example-skill")
	_, err := manager.LoadSkill(testdataDir)
	require.NoError(t, err)

	require.NoError(t, manager.Activate("example-skill", nil))

	// Get active prompts
	prompts := manager.GetActivePrompts()
	require.Len(t, prompts, 1)

	// Verify prompt properties
	assert.Equal(t, "greeting", prompts[0].Name)
	assert.Contains(t, prompts[0].Content, "friendly assistant")
}

// TestIntegration_SkillGetByID tests retrieving a skill by ID
func TestIntegration_SkillGetByID(t *testing.T) {
	manager := NewManager(ManagerConfig{
		SkillsDir: "testdata",
	})

	// Load the skill
	testdataDir := filepath.Join("testdata", "example-skill")
	_, err := manager.LoadSkill(testdataDir)
	require.NoError(t, err)

	// Get by ID
	skill, found := manager.GetSkill("example-skill")
	require.True(t, found)
	assert.Equal(t, "example-skill", skill.ID)
	assert.Equal(t, "Example Skill", skill.Name)

	// Non-existent skill
	_, found = manager.GetSkill("non-existent")
	assert.False(t, found)
}
