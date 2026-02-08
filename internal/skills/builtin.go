// Package skills provides the skill system for Mote Agent Runtime.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"mote/internal/cli/defaults"
)

// InstallBuiltinSkills copies builtin skills to the user's skills directory.
// It only copies if the skill doesn't already exist.
func InstallBuiltinSkills(targetDir string) error {
	builtinFS := defaults.GetDefaultsFS()
	return fs.WalkDir(builtinFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root "skills" directory
		if path == "skills" {
			return nil
		}

		// Calculate relative path from "skills/"
		relPath, err := filepath.Rel("skills", path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			// Create directory if it doesn't exist
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}
			return nil
		}

		// Skip if file already exists (don't overwrite user modifications)
		if _, err := os.Stat(targetPath); err == nil {
			log.Debug().Str("path", targetPath).Msg("builtin skill file already exists, skipping")
			return nil
		}

		// Copy file
		content, err := builtinFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read builtin file %s: %w", path, err)
		}

		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		log.Info().Str("path", targetPath).Msg("installed builtin skill file")
		return nil
	})
}

// EnsureBuiltinSkills ensures builtin skills are installed in the user's skills directory.
// Call this during Mote initialization.
func EnsureBuiltinSkills(skillsDir string) error {
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}
	return InstallBuiltinSkills(skillsDir)
}

// ListBuiltinSkills returns the list of builtin skill IDs.
func ListBuiltinSkills() []string {
	return []string{
		"mote-mcp-config",
		"mote-self",
		"mote-memory",
		"mote-cron",
	}
}
