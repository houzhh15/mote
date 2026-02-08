// Package skills provides the skill system for Mote Agent Runtime.
// Skills are extensible capability packages that can inject tools, prompts, and hooks.
package skills

import (
	"encoding/json"
	"time"

	"mote/internal/tools"
)

// Skill represents a skill package loaded from a manifest.json file.
type Skill struct {
	// Core identity
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`

	// Capability definitions
	Tools        []*ToolDef   `json:"tools,omitempty"`
	Prompts      []*PromptDef `json:"prompts,omitempty"`
	Hooks        []*HookDef   `json:"hooks,omitempty"`
	Config       ConfigMap    `json:"config,omitempty"`
	ConfigSchema *JSONSchema  `json:"configSchema,omitempty"`
	Dependencies []string     `json:"dependencies,omitempty"`

	// Metadata (not serialized to JSON)
	FilePath string    `json:"-"`
	LoadedAt time.Time `json:"-"`
}

// ToolDef defines a tool provided by a skill.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Handler     string      `json:"handler"` // Format: "filename.js#functionName"
	Parameters  *JSONSchema `json:"parameters,omitempty"`
	Timeout     Duration    `json:"timeout,omitempty"` // Default: 30s
	Runtime     string      `json:"runtime,omitempty"` // "js" or "shell", defaults to "js"
}

// PromptDef defines a prompt template provided by a skill.
type PromptDef struct {
	Name    string   `json:"name"`
	File    string   `json:"file,omitempty"`    // Relative path to prompt file
	Content string   `json:"content,omitempty"` // Inline content
	Tags    []string `json:"tags,omitempty"`
}

// HookDef defines a hook handler provided by a skill.
type HookDef struct {
	Type        string `json:"type"`                  // Hook type (e.g., "before_message")
	Handler     string `json:"handler"`               // Format: "filename.js#functionName"
	Priority    int    `json:"priority,omitempty"`    // Higher = earlier execution
	Description string `json:"description,omitempty"` // Human-readable description
}

// JSONSchema represents a JSON Schema for validation.
type JSONSchema struct {
	Type       string                 `json:"type,omitempty"`
	Properties map[string]*JSONSchema `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Items      *JSONSchema            `json:"items,omitempty"`
	Enum       []any                  `json:"enum,omitempty"`
	Default    any                    `json:"default,omitempty"`
}

// ConfigMap is an alias for configuration values.
type ConfigMap = map[string]any

// Duration is a wrapper for time.Duration that supports JSON marshaling.
type Duration struct {
	time.Duration
}

// MarshalJSON implements json.Marshaler for Duration.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler for Duration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	}
	return nil
}

// SkillState represents the current state of a skill.
type SkillState string

const (
	// SkillStateRegistered indicates the skill is loaded but not active.
	SkillStateRegistered SkillState = "registered"
	// SkillStateActive indicates the skill is currently active.
	SkillStateActive SkillState = "active"
	// SkillStateError indicates the skill encountered an error.
	SkillStateError SkillState = "error"
)

// SkillStatus represents the current status of a skill.
type SkillStatus struct {
	Skill       *Skill     `json:"skill"`
	State       SkillState `json:"state"`
	Error       error      `json:"error,omitempty"`
	Config      ConfigMap  `json:"config,omitempty"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
}

// SkillInstance represents an activated skill instance.
type SkillInstance struct {
	Skill       *Skill         `json:"skill"`
	Config      ConfigMap      `json:"config,omitempty"`
	Tools       []tools.Tool   `json:"-"` // Registered tools (not serialized)
	Prompts     []*SkillPrompt `json:"-"` // Resolved prompts (not serialized)
	HookIDs     []string       `json:"hook_ids,omitempty"`
	ActivatedAt time.Time      `json:"activated_at"`
}

// SkillPrompt represents a resolved prompt from a skill.
type SkillPrompt struct {
	SkillID string   `json:"skill_id"`
	Name    string   `json:"name"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
}

// SkillSource identifies the format of a skill definition.
type SkillSource string

const (
	// SourceManifest indicates a skill loaded from manifest.json.
	SourceManifest SkillSource = "manifest"
	// SourceSkillMD indicates a skill loaded from SKILL.md.
	SourceSkillMD SkillSource = "skillmd"
)

// SkillEntry is a unified representation of an available skill (from manifest.json or SKILL.md).
// Used for system prompt injection and skill discovery.
type SkillEntry struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Location    string           `json:"location"` // Path to SKILL.md or manifest.json
	Source      SkillSource      `json:"source"`
	Skill       *Skill           `json:"skill,omitempty"`    // Only for manifest.json
	Metadata    *SkillMDMetadata `json:"metadata,omitempty"` // Only for SKILL.md
}

// SkillMDMetadata represents the metadata section from a SKILL.md file.
type SkillMDMetadata struct {
	Homepage string             `json:"homepage,omitempty"`
	Emoji    string             `json:"emoji,omitempty"`
	OS       []string           `json:"os,omitempty"`
	Requires *SkillRequirements `json:"requires,omitempty"`
	Install  []SkillInstallSpec `json:"install,omitempty"`
}

// SkillRequirements defines the requirements for a skill to be available.
type SkillRequirements struct {
	Bins    []string `json:"bins,omitempty"`    // Required binaries (all must exist)
	AnyBins []string `json:"anyBins,omitempty"` // At least one must exist
	Env     []string `json:"env,omitempty"`     // Required environment variables
	Config  []string `json:"config,omitempty"`  // Required config keys
	Paths   []string `json:"paths,omitempty"`   // Required paths (all must exist, supports env expansion)
}

// SkillInstallSpec describes how to install a skill dependency.
type SkillInstallSpec struct {
	Kind    string   `json:"kind"`              // brew, node, go, uv, download
	ID      string   `json:"id,omitempty"`      // Package identifier
	Label   string   `json:"label,omitempty"`   // Human-readable label
	Bins    []string `json:"bins,omitempty"`    // Binaries provided
	Formula string   `json:"formula,omitempty"` // Homebrew formula
	Package string   `json:"package,omitempty"` // Package name
}
