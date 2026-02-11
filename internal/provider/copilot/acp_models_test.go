package copilot

import (
	"testing"
)

func TestACPSupportedModels_Count(t *testing.T) {
	if len(ACPSupportedModels) != 10 {
		t.Errorf("Expected 10 ACP models, got %d", len(ACPSupportedModels))
	}
}

func TestACPListModels(t *testing.T) {
	models := ACPListModels()
	if len(models) != 10 {
		t.Errorf("Expected 10 ACP models, got %d", len(models))
	}

	// Verify sorted order
	for i := 1; i < len(models); i++ {
		if models[i] < models[i-1] {
			t.Errorf("Models not sorted: %s > %s", models[i-1], models[i])
		}
	}

	// Verify all expected models are present
	expected := []string{
		"claude-sonnet-4.5", "claude-sonnet-4", "claude-haiku-4.5",
		"claude-opus-4.6", "claude-opus-4.6-fast", "claude-opus-4.5",
		"gemini-3-pro-preview", "gpt-5.2-codex", "gpt-5.2", "gpt-5.1-codex-max",
	}
	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}
	for _, e := range expected {
		if !modelSet[e] {
			t.Errorf("Expected model %q not found in ACPListModels()", e)
		}
	}
}

func TestACPGetModelInfo(t *testing.T) {
	tests := []struct {
		modelID     string
		expectNil   bool
		expectMulti float64
	}{
		{"claude-sonnet-4.5", false, 1},
		{"claude-sonnet-4", false, 1},
		{"claude-haiku-4.5", false, 0.33},
		{"claude-opus-4.6", false, 10},
		{"claude-opus-4.6-fast", false, 3},
		{"claude-opus-4.5", false, 3},
		{"gemini-3-pro-preview", false, 1},
		{"gpt-5.2-codex", false, 1},
		{"gpt-5.2", false, 1},
		{"gpt-5.1-codex-max", false, 1},
		{"nonexistent-model", true, 0},
		// API models should NOT be found in ACP registry
		{"gpt-4.1", true, 0},
		{"gpt-4o", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			info := ACPGetModelInfo(tt.modelID)
			if tt.expectNil {
				if info != nil {
					t.Errorf("Expected nil for %q, got %+v", tt.modelID, info)
				}
			} else {
				if info == nil {
					t.Fatalf("Expected non-nil for %q", tt.modelID)
				}
				if info.Multiplier != tt.expectMulti {
					t.Errorf("Expected multiplier %v for %q, got %v", tt.expectMulti, tt.modelID, info.Multiplier)
				}
				if info.ID != tt.modelID {
					t.Errorf("Expected ID %q, got %q", tt.modelID, info.ID)
				}
			}
		})
	}
}

func TestIsACPModel(t *testing.T) {
	// ACP models should return true
	acpModels := []string{
		"claude-sonnet-4.5", "claude-sonnet-4", "claude-haiku-4.5",
		"claude-opus-4.6", "claude-opus-4.6-fast", "claude-opus-4.5",
		"gemini-3-pro-preview", "gpt-5.2-codex", "gpt-5.2", "gpt-5.1-codex-max",
	}
	for _, m := range acpModels {
		if !IsACPModel(m) {
			t.Errorf("Expected IsACPModel(%q) = true", m)
		}
	}

	// API models should return false
	apiModels := []string{"gpt-4.1", "gpt-4o", "gpt-5-mini", "grok-code-fast-1"}
	for _, m := range apiModels {
		if IsACPModel(m) {
			t.Errorf("Expected IsACPModel(%q) = false", m)
		}
	}
}

func TestIsAPIModel(t *testing.T) {
	// API models should return true
	apiModels := []string{"gpt-4.1", "gpt-4o", "gpt-5-mini", "grok-code-fast-1"}
	for _, m := range apiModels {
		if !IsAPIModel(m) {
			t.Errorf("Expected IsAPIModel(%q) = true", m)
		}
	}

	// ACP models should return false
	acpModels := []string{"claude-sonnet-4.5", "claude-opus-4.6", "gemini-3-pro-preview"}
	for _, m := range acpModels {
		if IsAPIModel(m) {
			t.Errorf("Expected IsAPIModel(%q) = false", m)
		}
	}
}

func TestACPAndAPIModels_NoOverlap(t *testing.T) {
	// Ensure no model exists in both registries
	for id := range ACPSupportedModels {
		if _, exists := SupportedModels[id]; exists {
			t.Errorf("Model %q exists in both ACP and API registries", id)
		}
	}
	for id := range SupportedModels {
		if _, exists := ACPSupportedModels[id]; exists {
			t.Errorf("Model %q exists in both API and ACP registries", id)
		}
	}
}

func TestACPModels_AllHaveToolSupport(t *testing.T) {
	for id, info := range ACPSupportedModels {
		if !info.SupportsTools {
			t.Errorf("ACP model %q should support tools", id)
		}
	}
}

func TestACPModels_Families(t *testing.T) {
	anthropic := ACPListModelsByFamily(FamilyAnthropic)
	if len(anthropic) != 6 {
		t.Errorf("Expected 6 Anthropic ACP models, got %d: %v", len(anthropic), anthropic)
	}

	google := ACPListModelsByFamily(FamilyGoogle)
	if len(google) != 1 {
		t.Errorf("Expected 1 Google ACP model, got %d: %v", len(google), google)
	}

	openai := ACPListModelsByFamily(FamilyOpenAI)
	if len(openai) != 3 {
		t.Errorf("Expected 3 OpenAI ACP models, got %d: %v", len(openai), openai)
	}
}

func TestACPDefaultModel(t *testing.T) {
	if ACPDefaultModel == "" {
		t.Error("ACPDefaultModel should not be empty")
	}
	if !IsACPModel(ACPDefaultModel) {
		t.Errorf("ACPDefaultModel %q should be a valid ACP model", ACPDefaultModel)
	}
}

func TestACPGetModelMultiplier(t *testing.T) {
	tests := []struct {
		modelID  string
		expected float64
	}{
		{"claude-opus-4.6", 10},
		{"claude-opus-4.5", 3},
		{"claude-haiku-4.5", 0.33},
		{"claude-sonnet-4.5", 1},
		{"nonexistent", -1},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			result := ACPGetModelMultiplier(tt.modelID)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
