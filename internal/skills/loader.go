package skills

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// Loader defines the interface for loading skills.
type Loader interface {
	// Load loads a single skill from a directory.
	Load(skillDir string) (*Skill, error)

	// ScanDir scans a directory for skills and loads them all.
	ScanDir(rootDir string) ([]*Skill, error)

	// Reload reloads a skill from its original directory.
	Reload(skill *Skill) (*Skill, error)
}

// defaultLoader implements the Loader interface.
type defaultLoader struct{}

// NewLoader creates a new skill loader.
func NewLoader() Loader {
	return &defaultLoader{}
}

// Load loads a skill from the given directory.
// It expects a manifest.json file in the directory.
func (l *defaultLoader) Load(skillDir string) (*Skill, error) {
	manifestPath := filepath.Join(skillDir, "manifest.json")

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return nil, ErrManifestInvalid
	}

	// Parse manifest
	skill, err := ParseManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	// Set metadata
	skill.FilePath = manifestPath
	skill.LoadedAt = time.Now()

	return skill, nil
}

// ScanDir scans the given directory for skill subdirectories.
// Each subdirectory containing a manifest.json is loaded as a skill.
// Also scans for SKILL.md files directly in the root directory.
// Errors in individual skills are logged but don't stop the scan.
func (l *defaultLoader) ScanDir(rootDir string) ([]*Skill, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist, return empty list
			return nil, nil
		}
		return nil, err
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(rootDir, entry.Name())
		manifestPath := filepath.Join(skillDir, "manifest.json")

		// Skip directories without manifest.json
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		skill, err := l.Load(skillDir)
		if err != nil {
			log.Warn().
				Str("skill_dir", skillDir).
				Err(err).
				Msg("failed to load skill, skipping")
			continue
		}

		skills = append(skills, skill)
		log.Debug().
			Str("skill_id", skill.ID).
			Str("skill_name", skill.Name).
			Str("version", skill.Version).
			Msg("loaded skill")
	}

	return skills, nil
}

// ScanDirForSkillMD scans a directory for SKILL.md files.
// It looks for SKILL.md in subdirectories and returns SkillEntry items.
func ScanDirForSkillMD(rootDir string) ([]*SkillEntry, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []*SkillEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(rootDir, entry.Name())
		skillMDPath := filepath.Join(skillDir, "SKILL.md")

		// Check for SKILL.md
		if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
			continue
		}

		// Skip if manifest.json exists (manifest takes precedence)
		manifestPath := filepath.Join(skillDir, "manifest.json")
		if _, err := os.Stat(manifestPath); err == nil {
			continue
		}

		skillEntry, err := ParseSkillMD(skillMDPath)
		if err != nil {
			log.Warn().
				Str("skill_dir", skillDir).
				Err(err).
				Msg("failed to parse SKILL.md, skipping")
			continue
		}

		results = append(results, skillEntry)
		log.Debug().
			Str("skill_name", skillEntry.Name).
			Str("location", skillEntry.Location).
			Msg("loaded SKILL.md")
	}

	return results, nil
}

// Reload reloads a skill from its original directory.
func (l *defaultLoader) Reload(skill *Skill) (*Skill, error) {
	if skill.FilePath == "" {
		return nil, ErrManifestInvalid
	}

	skillDir := filepath.Dir(skill.FilePath)
	return l.Load(skillDir)
}

// GetSkillDir returns the directory containing the skill.
func GetSkillDir(skill *Skill) string {
	if skill.FilePath == "" {
		return ""
	}
	return filepath.Dir(skill.FilePath)
}
