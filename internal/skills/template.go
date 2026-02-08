package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TargetType represents the target location for skills.
type TargetType string

const (
	// TargetUser indicates user-level skills directory (~/.mote/skills).
	TargetUser TargetType = "user"
	// TargetWorkspace indicates workspace-level skills directory (.mote/skills).
	TargetWorkspace TargetType = "workspace"
)

// DefaultManifest returns a default manifest.json content for a new skill.
func DefaultManifest(name, description string) map[string]any {
	return map[string]any{
		"id":          strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		"name":        name,
		"version":     "1.0.0",
		"description": description,
		"author":      "",
		"tools":       []any{},
		"prompts":     []any{},
		"hooks":       []any{},
	}
}

// DefaultSkillMD returns a default SKILL.md content.
func DefaultSkillMD(name, description string) string {
	return fmt.Sprintf(`# %s

%s

## Overview

This skill provides custom capabilities for the Mote agent.

## Configuration

No configuration required.

## Tools

This skill does not provide any tools yet.

## Hooks

This skill does not provide any hooks yet.
`, name, description)
}

// CreateTemplate creates a new skill template at the specified target location.
// It creates the directory structure:
//
//	<skill-name>/
//	├── SKILL.md         # Skill documentation
//	├── manifest.json    # Metadata
//	├── gating.js        # Gating script (placeholder)
//	├── tools/           # Tools directory
//	└── hooks/           # Hooks directory
//
// Returns the path to the created skill directory.
func CreateTemplate(name string, target TargetType) (string, error) {
	if name == "" {
		return "", fmt.Errorf("skill name cannot be empty")
	}

	// Sanitize the name for use as a directory
	dirName := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Determine base path based on target
	basePath, err := GetSkillsDir(target)
	if err != nil {
		return "", fmt.Errorf("failed to get skills directory: %w", err)
	}

	skillDir := filepath.Join(basePath, dirName)

	// Check if directory already exists
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		return "", fmt.Errorf("skill directory already exists: %s", skillDir)
	}

	// Create skill directory
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{"tools", "hooks"}
	for _, subdir := range subdirs {
		path := filepath.Join(skillDir, subdir)
		if err := os.MkdirAll(path, 0755); err != nil {
			// Clean up on failure
			os.RemoveAll(skillDir)
			return "", fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Create SKILL.md
	description := fmt.Sprintf("A custom skill named %s.", name)
	skillMDContent := DefaultSkillMD(name, description)
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMDPath, []byte(skillMDContent), 0644); err != nil {
		os.RemoveAll(skillDir)
		return "", fmt.Errorf("failed to create SKILL.md: %w", err)
	}

	// Create manifest.json
	manifest := DefaultManifest(name, description)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		os.RemoveAll(skillDir)
		return "", fmt.Errorf("failed to marshal manifest: %w", err)
	}
	manifestPath := filepath.Join(skillDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
		os.RemoveAll(skillDir)
		return "", fmt.Errorf("failed to create manifest.json: %w", err)
	}

	// Create gating.js placeholder
	gatingContent := `// Gating script for skill activation
// Return true to allow, false to deny
// module.exports = async (context) => {
//   return true;
// };
`
	gatingPath := filepath.Join(skillDir, "gating.js")
	if err := os.WriteFile(gatingPath, []byte(gatingContent), 0644); err != nil {
		os.RemoveAll(skillDir)
		return "", fmt.Errorf("failed to create gating.js: %w", err)
	}

	// Create .gitkeep files in empty directories
	for _, subdir := range subdirs {
		gitkeepPath := filepath.Join(skillDir, subdir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			// Non-fatal, just log
			continue
		}
	}

	return skillDir, nil
}

// GetSkillsDir returns the skills directory path for the given target.
func GetSkillsDir(target TargetType) (string, error) {
	switch target {
	case TargetUser:
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		skillsDir := filepath.Join(homeDir, ".mote", "skills")
		// Ensure directory exists
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create skills directory: %w", err)
		}
		return skillsDir, nil

	case TargetWorkspace:
		// Use current working directory for workspace-level skills
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		skillsDir := filepath.Join(cwd, ".mote", "skills")
		// Ensure directory exists
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create skills directory: %w", err)
		}
		return skillsDir, nil

	default:
		return "", fmt.Errorf("invalid target: %s (must be 'user' or 'workspace')", target)
	}
}

// OpenSkillsDir opens the skills directory in the system file manager.
// Uses platform-specific commands: open (macOS), xdg-open (Linux), explorer (Windows).
// Note: This function only returns the directory path. The actual opening
// should be done by the caller using os/exec with platform-specific commands.
func OpenSkillsDir(target TargetType) (string, error) {
	return GetSkillsDir(target)
}
