// Package copilot provides GitHub Copilot integration for Mote.
package copilot

import (
	"sort"
	"strings"
)

// ModelFamily represents the vendor/family of a model.
type ModelFamily string

const (
	FamilyOpenAI    ModelFamily = "openai"
	FamilyAnthropic ModelFamily = "anthropic"
	FamilyGoogle    ModelFamily = "google"
	FamilyXAI       ModelFamily = "xai"
)

// ModelInfo contains metadata about a supported Copilot model.
type ModelInfo struct {
	// ID is the model identifier used in API requests.
	ID string `json:"id"`
	// DisplayName is the human-readable name for UI display.
	DisplayName string `json:"display_name"`
	// Family is the model vendor (openai, anthropic, google, xai).
	Family ModelFamily `json:"family"`
	// Version is the model version string.
	Version string `json:"version"`
	// Multiplier is the premium request multiplier (0 = free/included).
	Multiplier int `json:"multiplier"`
	// IsFree indicates if the model is included in base subscription.
	IsFree bool `json:"is_free"`
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

// SupportedModels is the registry of all supported Copilot models.
// Multiplier values based on GitHub Copilot billing (as of 2026-02):
// - 0 = free/included in paid plans (GPT-4.1, GPT-4o, GPT-5 mini, Raptor mini)
// - 0.25 = ultra-low premium (Grok Code Fast 1)
// - 0.33 = low premium (Claude Haiku 4.5, Gemini 3 Flash, GPT-5.1-Codex-Mini)
// - 1 = standard premium (Claude Sonnet 4/4.5, Gemini 2.5/3 Pro, GPT-5/5.1/5.2)
// - 3 = high premium (Claude Opus 4.5)
// - 10 = ultra premium (Claude Opus 4.1)
// Reference: https://docs.github.com/zh/copilot/concepts/billing/copilot-requests
var SupportedModels = map[string]ModelInfo{
	// === OpenAI Models (Free/Included in Paid Plans) ===
	"gpt-4.1": {
		ID:             "gpt-4.1",
		DisplayName:    "GPT-4.1",
		Family:         FamilyOpenAI,
		Version:        "4.1",
		Multiplier:     0,
		IsFree:         true,
		ContextWindow:  128000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Included model, optimized for code",
	},
	"gpt-4o": {
		ID:             "gpt-4o",
		DisplayName:    "GPT-4o",
		Family:         FamilyOpenAI,
		Version:        "4o",
		Multiplier:     0,
		IsFree:         true,
		ContextWindow:  128000,
		MaxOutput:      16384,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Included model with vision support",
	},
	"gpt-5-mini": {
		ID:             "gpt-5-mini",
		DisplayName:    "GPT-5 Mini",
		Family:         FamilyOpenAI,
		Version:        "5-mini",
		Multiplier:     0,
		IsFree:         true,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Included compact GPT-5 model",
	},
	"raptor-mini": {
		ID:             "raptor-mini",
		DisplayName:    "Raptor Mini",
		Family:         FamilyOpenAI,
		Version:        "raptor-mini",
		Multiplier:     0,
		IsFree:         true,
		ContextWindow:  128000,
		MaxOutput:      16384,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Included lightweight model",
	},

	// === OpenAI Models (Low Premium - 0.33x) ===
	"gpt-5.1-codex-mini": {
		ID:             "gpt-5.1-codex-mini",
		DisplayName:    "GPT-5.1 Codex Mini",
		Family:         FamilyOpenAI,
		Version:        "5.1-codex-mini",
		Multiplier:     0,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      32768,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Compact code-specialized model (0.33x)",
	},

	// === OpenAI Models (Standard Premium - 1x) ===
	"gpt-5": {
		ID:             "gpt-5",
		DisplayName:    "GPT-5",
		Family:         FamilyOpenAI,
		Version:        "5",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Most capable GPT model (1x)",
	},
	"gpt-5-codex": {
		ID:             "gpt-5-codex",
		DisplayName:    "GPT-5 Codex",
		Family:         FamilyOpenAI,
		Version:        "5-codex",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Code-specialized GPT-5 (1x)",
	},
	"gpt-5.1": {
		ID:             "gpt-5.1",
		DisplayName:    "GPT-5.1",
		Family:         FamilyOpenAI,
		Version:        "5.1",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Enhanced GPT-5 model (1x)",
	},
	"gpt-5.1-codex": {
		ID:             "gpt-5.1-codex",
		DisplayName:    "GPT-5.1 Codex",
		Family:         FamilyOpenAI,
		Version:        "5.1-codex",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Code-specialized GPT-5.1 (1x)",
	},
	"gpt-5.1-codex-max": {
		ID:             "gpt-5.1-codex-max",
		DisplayName:    "GPT-5.1 Codex Max",
		Family:         FamilyOpenAI,
		Version:        "5.1-codex-max",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Maximum capability code model (1x)",
	},
	"gpt-5.2": {
		ID:             "gpt-5.2",
		DisplayName:    "GPT-5.2",
		Family:         FamilyOpenAI,
		Version:        "5.2",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Latest GPT-5 iteration (1x)",
	},
	"gpt-5.2-codex": {
		ID:             "gpt-5.2-codex",
		DisplayName:    "GPT-5.2 Codex",
		Family:         FamilyOpenAI,
		Version:        "5.2-codex",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      65536,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "Latest code-specialized model (1x)",
	},

	// === Anthropic Models (Low Premium - 0.33x) ===
	"claude-haiku-4.5": {
		ID:             "claude-haiku-4.5",
		DisplayName:    "Claude Haiku 4.5",
		Family:         FamilyAnthropic,
		Version:        "haiku-4.5",
		Multiplier:     0,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      8192,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Fast, efficient Claude model (0.33x)",
	},

	// === Anthropic Models (Standard Premium - 1x) ===
	"claude-sonnet-4": {
		ID:             "claude-sonnet-4",
		DisplayName:    "Claude Sonnet 4",
		Family:         FamilyAnthropic,
		Version:        "sonnet-4",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      64000,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Balanced Claude model (1x)",
	},
	"claude-sonnet-4.5": {
		ID:             "claude-sonnet-4.5",
		DisplayName:    "Claude Sonnet 4.5",
		Family:         FamilyAnthropic,
		Version:        "sonnet-4.5",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      64000,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Enhanced balanced Claude model (1x)",
	},

	// === Anthropic Models (High Premium - 3x) ===
	"claude-opus-4.5": {
		ID:             "claude-opus-4.5",
		DisplayName:    "Claude Opus 4.5",
		Family:         FamilyAnthropic,
		Version:        "opus-4.5",
		Multiplier:     3,
		IsFree:         false,
		ContextWindow:  200000,
		MaxOutput:      64000,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Most capable Claude model (3x)",
	},

	// === Google Models (Low Premium - 0.33x) ===
	"gemini-3-flash": {
		ID:             "gemini-3-flash",
		DisplayName:    "Gemini 3 Flash",
		Family:         FamilyGoogle,
		Version:        "3-flash",
		Multiplier:     0,
		IsFree:         false,
		ContextWindow:  2000000,
		MaxOutput:      32768,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Fast Gemini 3 model with 2M context (0.33x)",
	},

	// === Google Models (Standard Premium - 1x) ===
	"gemini-2.5-pro": {
		ID:             "gemini-2.5-pro",
		DisplayName:    "Gemini 2.5 Pro",
		Family:         FamilyGoogle,
		Version:        "2.5-pro",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  2000000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Advanced Gemini with 2M context (1x)",
	},
	"gemini-3-pro": {
		ID:             "gemini-3-pro",
		DisplayName:    "Gemini 3 Pro",
		Family:         FamilyGoogle,
		Version:        "3-pro",
		Multiplier:     1,
		IsFree:         false,
		ContextWindow:  2000000,
		MaxOutput:      65536,
		SupportsVision: true,
		SupportsTools:  true,
		Description:    "Latest Gemini Pro with 2M context (1x)",
	},

	// === xAI Models (Ultra-Low Premium - 0.25x) ===
	"grok-code-fast-1": {
		ID:             "grok-code-fast-1",
		DisplayName:    "Grok Code Fast 1",
		Family:         FamilyXAI,
		Version:        "code-fast-1",
		Multiplier:     0,
		IsFree:         true,
		ContextWindow:  131072,
		MaxOutput:      32768,
		SupportsVision: false,
		SupportsTools:  true,
		Description:    "xAI's fast code-optimized model (0.25x)",
	},
}

// GetModelInfo returns the ModelInfo for a given model ID.
// Returns nil if the model is not found.
func GetModelInfo(modelID string) *ModelInfo {
	// Normalize model ID (lowercase, trim whitespace)
	normalizedID := strings.ToLower(strings.TrimSpace(modelID))

	if info, ok := SupportedModels[normalizedID]; ok {
		return &info
	}

	// Try exact match without normalization
	if info, ok := SupportedModels[modelID]; ok {
		return &info
	}

	return nil
}

// ListModels returns a sorted list of all supported model IDs.
func ListModels() []string {
	models := make([]string, 0, len(SupportedModels))
	for id := range SupportedModels {
		models = append(models, id)
	}
	sort.Strings(models)
	return models
}

// ListFreeModels returns model IDs that are free/included in base subscription.
func ListFreeModels() []string {
	models := make([]string, 0)
	for id, info := range SupportedModels {
		if info.IsFree {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	return models
}

// ListPremiumModels returns model IDs that require premium requests.
func ListPremiumModels() []string {
	models := make([]string, 0)
	for id, info := range SupportedModels {
		if !info.IsFree {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	return models
}

// ListModelsByFamily returns model IDs filtered by family.
func ListModelsByFamily(family ModelFamily) []string {
	models := make([]string, 0)
	for id, info := range SupportedModels {
		if info.Family == family {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	return models
}

// GetModelMultiplier returns the premium request multiplier for a model.
// Returns -1 if the model is not found.
func GetModelMultiplier(modelID string) int {
	info := GetModelInfo(modelID)
	if info == nil {
		return -1
	}
	return info.Multiplier
}

// IsModelSupported checks if a model ID is in the supported list.
func IsModelSupported(modelID string) bool {
	return GetModelInfo(modelID) != nil
}

// DefaultModel is the default model used when none is specified.
const DefaultModel = "grok-code-fast-1"

// DefaultAgentModel is the default model for agent mode.
const DefaultAgentModel = "grok-code-fast-1"
