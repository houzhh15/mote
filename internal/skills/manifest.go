package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// skillIDPattern validates skill IDs: lowercase letters, numbers, and hyphens only.
	skillIDPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	// semverPattern validates semantic version strings.
	semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
	// handlerPattern validates tool handler format: filename.js#functionName
	handlerPattern = regexp.MustCompile(`^[\w.-]+\.js#\w+$`)
)

// ParseManifest parses a manifest.json file and returns a Skill.
func ParseManifest(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	return ParseManifestBytes(data)
}

// ParseManifestBytes parses manifest.json content from bytes and returns a Skill.
func ParseManifestBytes(data []byte) (*Skill, error) {
	var skill Skill
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("%w: JSON parse error: %v", ErrManifestInvalid, err)
	}

	if err := validateSkill(&skill); err != nil {
		return nil, err
	}

	// Set default timeout for tools
	for _, tool := range skill.Tools {
		if tool.Timeout.Duration == 0 {
			tool.Timeout.Duration = DefaultToolTimeout
		}
	}

	return &skill, nil
}

// DefaultToolTimeout is the default timeout for tool execution.
const DefaultToolTimeout = 30e9 // 30 seconds in nanoseconds

// validateSkill validates required fields and formats.
func validateSkill(skill *Skill) error {
	// Validate required fields
	if skill.ID == "" {
		return fmt.Errorf("%w: missing required field 'id'", ErrManifestInvalid)
	}
	if skill.Name == "" {
		return fmt.Errorf("%w: missing required field 'name'", ErrManifestInvalid)
	}
	if skill.Version == "" {
		return fmt.Errorf("%w: missing required field 'version'", ErrManifestInvalid)
	}

	// Validate ID format
	if !skillIDPattern.MatchString(skill.ID) {
		return fmt.Errorf("%w: id must match pattern ^[a-z0-9-]+$, got '%s'", ErrSkillIDInvalid, skill.ID)
	}

	// Validate version format
	if !semverPattern.MatchString(skill.Version) {
		return fmt.Errorf("%w: version must be semver format, got '%s'", ErrVersionInvalid, skill.Version)
	}

	// Validate tools
	for i, tool := range skill.Tools {
		if err := validateToolDef(tool, i); err != nil {
			return err
		}
	}

	// Validate prompts
	for i, prompt := range skill.Prompts {
		if err := validatePromptDef(prompt, i); err != nil {
			return err
		}
	}

	// Validate hooks
	for i, hook := range skill.Hooks {
		if err := validateHookDef(hook, i); err != nil {
			return err
		}
	}

	return nil
}

// validateToolDef validates a tool definition.
func validateToolDef(tool *ToolDef, index int) error {
	if tool.Name == "" {
		return fmt.Errorf("%w: tools[%d] missing required field 'name'", ErrManifestInvalid, index)
	}
	if tool.Handler == "" {
		return fmt.Errorf("%w: tools[%d] missing required field 'handler'", ErrManifestInvalid, index)
	}
	if !handlerPattern.MatchString(tool.Handler) {
		return fmt.Errorf("%w: tools[%d] handler must match format 'filename.js#functionName', got '%s'",
			ErrToolHandlerInvalid, index, tool.Handler)
	}
	return nil
}

// validatePromptDef validates a prompt definition.
func validatePromptDef(prompt *PromptDef, index int) error {
	if prompt.Name == "" {
		return fmt.Errorf("%w: prompts[%d] missing required field 'name'", ErrManifestInvalid, index)
	}
	if prompt.File == "" && prompt.Content == "" {
		return fmt.Errorf("%w: prompts[%d] must have either 'file' or 'content'", ErrPromptInvalid, index)
	}
	return nil
}

// validateHookDef validates a hook definition.
func validateHookDef(hook *HookDef, index int) error {
	if hook.Type == "" {
		return fmt.Errorf("%w: hooks[%d] missing required field 'type'", ErrManifestInvalid, index)
	}
	if hook.Handler == "" {
		return fmt.Errorf("%w: hooks[%d] missing required field 'handler'", ErrManifestInvalid, index)
	}
	if !handlerPattern.MatchString(hook.Handler) {
		return fmt.Errorf("%w: hooks[%d] handler must match format 'filename.js#functionName', got '%s'",
			ErrToolHandlerInvalid, index, hook.Handler)
	}
	return nil
}

// ParseHandler parses a handler string into filename and function name.
// Handler format: "filename.js#functionName"
func ParseHandler(handler string) (filename string, funcName string, err error) {
	parts := strings.SplitN(handler, "#", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: expected 'filename.js#functionName', got '%s'",
			ErrToolHandlerInvalid, handler)
	}
	return parts[0], parts[1], nil
}

// ResolveHandlerPath resolves the full path to a handler JS file.
func ResolveHandlerPath(skillDir, handler string) (string, error) {
	filename, _, err := ParseHandler(handler)
	if err != nil {
		return "", err
	}
	return filepath.Join(skillDir, filename), nil
}
