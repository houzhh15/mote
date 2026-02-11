package copilot

import (
	"sort"
	"strings"
)

// ACPModelInfo contains metadata about a supported Copilot ACP model.
// ACP models use Copilot CLI for authentication and per-prompt billing.
type ACPModelInfo struct {
	// ID is the model identifier used in ACP requests.
	ID string `json:"id"`
	// DisplayName is the human-readable name for UI display.
	DisplayName string `json:"display_name"`
	// Family is the model vendor (openai, anthropic, google).
	Family ModelFamily `json:"family"`
	// Multiplier is the premium request multiplier.
	Multiplier float64 `json:"multiplier"`
	// ContextWindow is the maximum context length in tokens.
	ContextWindow int `json:"context_window"`
	// MaxOutput is the maximum output tokens.
	MaxOutput int `json:"max_output"`
	// SupportsVision indicates if the model supports image inputs.
	SupportsVision bool `json:"supports_vision"`
	// SupportsTools indicates if the model supports function calling.
	SupportsTools bool `json:"supports_tools"`
	// Description is a brief description of the model.
	Description string `json:"description,omitempty"`
}

// ACPSupportedModels is the registry of all supported Copilot ACP models.
// These models require premium requests and use the ACP protocol via Copilot CLI.
//
// IMPORTANT: Requires Copilot CLI v0.0.406+ (Node.js v22+) for full model support.
// Older CLI versions (Node v20) only support: claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5, gpt-5
//
// Multiplier values based on GitHub Copilot billing:
//   - 0.33 = low premium (Claude Haiku 4.5)
//   - 1    = standard premium (Claude Sonnet 4/4.5, Gemini 3 Pro, GPT-5.2, GPT-5.1 Codex Max)
//   - 3    = high premium (Claude Opus 4.5, Claude Opus 4.6 Fast)
//   - 10   = ultra premium (Claude Opus 4.6)
//
// Reference: https://docs.github.com/zh/copilot/concepts/billing/copilot-requests
var ACPSupportedModels = map[string]ACPModelInfo{
	// === Anthropic Models ===
	"claude-sonnet-4.5": {
		ID:             "claude-sonnet-4.5",
		DisplayName:    "Claude Sonnet 4.5",
		Family:         FamilyAnthropic,
		Multiplier:     1,
		ContextWindow:  200000,
		MaxOutput:      16384,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Anthropic's balanced model, excellent for coding tasks",
	},
	"claude-sonnet-4": {
		ID:             "claude-sonnet-4",
		DisplayName:    "Claude Sonnet 4",
		Family:         FamilyAnthropic,
		Multiplier:     1,
		ContextWindow:  200000,
		MaxOutput:      16384,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Reliable coding assistant with strong tool use",
	},
	"claude-haiku-4.5": {
		ID:             "claude-haiku-4.5",
		DisplayName:    "Claude Haiku 4.5",
		Family:         FamilyAnthropic,
		Multiplier:     0.33,
		ContextWindow:  200000,
		MaxOutput:      8192,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Anthropic's fast and cost-effective model",
	},
	"claude-opus-4.6": {
		ID:             "claude-opus-4.6",
		DisplayName:    "Claude Opus 4.6",
		Family:         FamilyAnthropic,
		Multiplier:     10,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Anthropic's most capable model, best for complex reasoning",
	},
	"claude-opus-4.6-fast": {
		ID:             "claude-opus-4.6-fast",
		DisplayName:    "Claude Opus 4.6 Fast",
		Family:         FamilyAnthropic,
		Multiplier:     3,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Faster variant of Opus 4.6 with reduced latency",
	},
	"claude-opus-4.5": {
		ID:             "claude-opus-4.5",
		DisplayName:    "Claude Opus 4.5",
		Family:         FamilyAnthropic,
		Multiplier:     3,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Anthropic's powerful reasoning model",
	},

	// === Google Models ===
	"gemini-3-pro-preview": {
		ID:             "gemini-3-pro-preview",
		DisplayName:    "Gemini 3 Pro Preview",
		Family:         FamilyGoogle,
		Multiplier:     1,
		ContextWindow:  1000000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Google's latest pro model with 1M context window",
	},

	// === OpenAI Models (Premium) ===
	"gpt-5.2-codex": {
		ID:             "gpt-5.2-codex",
		DisplayName:    "GPT-5.2 Codex",
		Family:         FamilyOpenAI,
		Multiplier:     1,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "OpenAI's code-specialized GPT-5.2 model",
	},
	"gpt-5.2": {
		ID:             "gpt-5.2",
		DisplayName:    "GPT-5.2",
		Family:         FamilyOpenAI,
		Multiplier:     1,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "OpenAI's latest general-purpose model",
	},
	"gpt-5.1-codex-max": {
		ID:             "gpt-5.1-codex-max",
		DisplayName:    "GPT-5.1 Codex Max",
		Family:         FamilyOpenAI,
		Multiplier:     1,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "OpenAI's high-capacity code generation model",
	},
}

// ACPDefaultModel is the default model for ACP mode.
const ACPDefaultModel = "claude-sonnet-4.5"

// ACPListModels returns a sorted list of all supported ACP model IDs.
func ACPListModels() []string {
	models := make([]string, 0, len(ACPSupportedModels))
	for id := range ACPSupportedModels {
		models = append(models, id)
	}
	sort.Strings(models)
	return models
}

// ACPGetModelInfo returns the ACPModelInfo for a given model ID.
// Returns nil if the model is not found in the ACP registry.
func ACPGetModelInfo(modelID string) *ACPModelInfo {
	normalizedID := strings.ToLower(strings.TrimSpace(modelID))
	if info, ok := ACPSupportedModels[normalizedID]; ok {
		return &info
	}
	if info, ok := ACPSupportedModels[modelID]; ok {
		return &info
	}
	return nil
}

// IsACPModel checks if a model ID belongs to the ACP model registry.
// This is used to determine whether a model should use ACP protocol.
func IsACPModel(modelID string) bool {
	return ACPGetModelInfo(modelID) != nil
}

// IsAPIModel checks if a model ID belongs to the API model registry.
// This is used to determine whether a model should use the Copilot REST API.
func IsAPIModel(modelID string) bool {
	return GetModelInfo(modelID) != nil
}

// ACPGetModelMultiplier returns the premium request multiplier for an ACP model.
// Returns -1 if the model is not found.
func ACPGetModelMultiplier(modelID string) float64 {
	info := ACPGetModelInfo(modelID)
	if info == nil {
		return -1
	}
	return info.Multiplier
}

// ACPListModelsByFamily returns ACP model IDs filtered by family.
func ACPListModelsByFamily(family ModelFamily) []string {
	models := make([]string, 0)
	for id, info := range ACPSupportedModels {
		if info.Family == family {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	return models
}
