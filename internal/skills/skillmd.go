// Package skills provides the skill system for Mote Agent Runtime.
package skills

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterRegex matches YAML frontmatter in SKILL.md files.
var frontmatterRegex = regexp.MustCompile(`(?s)^---\s*\n(.+?)\n---\s*\n`)

// SkillMDFrontmatter represents the YAML frontmatter of a SKILL.md file.
type SkillMDFrontmatter struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Homepage    string      `yaml:"homepage,omitempty"`
	Metadata    interface{} `yaml:"metadata,omitempty"` // Can be JSON string or inline map
}

// MetadataWrapper wraps the OpenClaw-specific metadata.
type MetadataWrapper struct {
	OpenClaw *SkillMDMetadata `json:"openclaw"`
}

// ParseSkillMD parses a SKILL.md file and returns a SkillEntry.
func ParseSkillMD(path string) (*SkillEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSkillMDContent(string(content), path)
}

// ParseSkillMDContent parses SKILL.md content and returns a SkillEntry.
func ParseSkillMDContent(content, path string) (*SkillEntry, error) {
	matches := frontmatterRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil, ErrManifestInvalid
	}

	var fm SkillMDFrontmatter
	if err := yaml.Unmarshal([]byte(matches[1]), &fm); err != nil {
		return nil, err
	}

	if fm.Name == "" {
		return nil, ErrManifestInvalid
	}

	entry := &SkillEntry{
		Name:        fm.Name,
		Description: fm.Description,
		Location:    path,
		Source:      SourceSkillMD,
	}

	// Parse extended metadata if present
	if fm.Metadata != nil {
		meta, err := parseMetadata(fm.Metadata)
		if err == nil && meta != nil {
			entry.Metadata = meta
			// Prepend emoji to name if present
			if meta.Emoji != "" {
				entry.Name = meta.Emoji + " " + entry.Name
			}
		}
	}

	// Set homepage in metadata
	if fm.Homepage != "" {
		if entry.Metadata == nil {
			entry.Metadata = &SkillMDMetadata{}
		}
		entry.Metadata.Homepage = fm.Homepage
	}

	return entry, nil
}

// parseMetadata parses metadata from SKILL.md frontmatter.
// It handles both JSON string format and inline map/object format.
func parseMetadata(raw interface{}) (*SkillMDMetadata, error) {
	if raw == nil {
		return nil, nil
	}

	// Handle string format (JSON string)
	if str, ok := raw.(string); ok {
		return parseMetadataJSON(str)
	}

	// Handle map format (inline YAML object)
	// Convert to JSON and then parse
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return parseMetadataJSON(string(jsonBytes))
}

// parseMetadataJSON parses the metadata JSON string from SKILL.md frontmatter.
// It expects a structure like {"openclaw": {...}}.
func parseMetadataJSON(raw string) (*SkillMDMetadata, error) {
	// Trim whitespace
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// Handle multi-line JSON5-like strings (common in SKILL.md)
	// For now, we only support standard JSON
	var wrapper MetadataWrapper
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		// Try parsing as direct SkillMDMetadata
		var meta SkillMDMetadata
		if directErr := json.Unmarshal([]byte(raw), &meta); directErr != nil {
			return nil, err
		}
		return &meta, nil
	}
	return wrapper.OpenClaw, nil
}

// GetSkillMDBody extracts the body content (after frontmatter) from SKILL.md content.
func GetSkillMDBody(content string) string {
	matches := frontmatterRegex.FindStringSubmatch(content)
	if len(matches) == 0 {
		return content
	}
	// Return content after the frontmatter
	idx := frontmatterRegex.FindStringIndex(content)
	if idx == nil {
		return content
	}
	return strings.TrimSpace(content[idx[1]:])
}
