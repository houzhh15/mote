// Package skills provides the skill system for Mote Agent Runtime.
package skills

import "fmt"

// PromptSection builds the skills section for system prompts.
type PromptSection struct {
	Manager      *Manager
	ReadToolName string // Name of the tool used to read files (default: "read_file")
}

// NewPromptSection creates a new PromptSection with the given manager.
func NewPromptSection(manager *Manager) *PromptSection {
	return &PromptSection{
		Manager:      manager,
		ReadToolName: "read_file",
	}
}

// Build constructs the skills prompt section for system prompt injection.
// Returns empty string if no skills are available.
func (s *PromptSection) Build() string {
	if s.Manager == nil {
		return ""
	}

	xml := s.Manager.FormatSkillsXML()
	if xml == "" {
		return ""
	}

	readTool := s.ReadToolName
	if readTool == "" {
		readTool = "read_file"
	}

	return fmt.Sprintf(`## Skills (mandatory)
Before replying: scan <available_skills> <description> entries.
- If exactly one skill clearly applies: read its SKILL.md or manifest.json at <location> with `+"`%s`"+`, then follow it.
- If multiple could apply: choose the most specific one, then read/follow it.
- If none clearly apply: do not read any skill file.
Constraints: never read more than one skill up front; only read after selecting.

%s
`, readTool, xml)
}

// BuildActivePrompts returns all prompts from activated skills as a combined string.
func (s *PromptSection) BuildActivePrompts() string {
	if s.Manager == nil {
		return ""
	}

	prompts := s.Manager.GetActivePrompts()
	if len(prompts) == 0 {
		return ""
	}

	var result string
	for _, p := range prompts {
		if p.Content != "" {
			result += fmt.Sprintf("\n### Skill: %s\n%s\n", p.Name, p.Content)
		}
	}
	return result
}
