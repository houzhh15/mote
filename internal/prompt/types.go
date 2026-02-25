package prompt

import "context"

// MemorySearcher is an interface for searching memories.
type MemorySearcher interface {
	Search(ctx context.Context, query string, topK int) ([]MemoryResult, error)
}

// MemoryResult represents a memory search result.
type MemoryResult struct {
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// PromptConfig holds configuration for the system prompt builder.
type PromptConfig struct {
	AgentName           string   `json:"agent_name"`
	Timezone            string   `json:"timezone"`
	WorkspaceDir        string   `json:"workspace_dir"`
	ExtraPrompt         string   `json:"extra_prompt"`
	Constraints         []string `json:"constraints"`
	DisableSafetyPrompt bool     `json:"disable_safety_prompt"`
}

// DefaultPromptConfig returns a PromptConfig with default values.
func DefaultPromptConfig() PromptConfig {
	return PromptConfig{
		AgentName: "Mote",
		Timezone:  "UTC",
	}
}

// PromptData holds all data for template rendering.
type PromptData struct {
	AgentName       string
	Tools           []ToolInfo
	Agents          []AgentInfo
	Memories        []MemoryResult
	Timezone        string
	CurrentTime     string
	WorkspaceDir    string
	Constraints     []string
	ExtraPrompt     string
	MaxOutputTokens int // Maximum output tokens the model can generate (0 = unknown)
}

// ToolInfo holds information about a tool for prompt rendering.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// AgentInfo holds information about a sub-agent for prompt rendering.
type AgentInfo struct {
	Name        string
	Description string
	Model       string
	Tools       []string
}
