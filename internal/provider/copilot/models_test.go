package copilot

import (
	"testing"
)

func TestGetModelInfo(t *testing.T) {
	tests := []struct {
		name     string
		modelID  string
		wantNil  bool
		wantID   string
		wantFree bool
	}{
		{
			name:     "exact match gpt-4.1",
			modelID:  "gpt-4.1",
			wantNil:  false,
			wantID:   "gpt-4.1",
			wantFree: true,
		},
		{
			name:     "gpt-4o",
			modelID:  "gpt-4o",
			wantNil:  false,
			wantID:   "gpt-4o",
			wantFree: true,
		},
		{
			name:     "gpt-5-mini",
			modelID:  "gpt-5-mini",
			wantNil:  false,
			wantID:   "gpt-5-mini",
			wantFree: true,
		},
		{
			name:     "grok-code-fast-1",
			modelID:  "grok-code-fast-1",
			wantNil:  false,
			wantID:   "grok-code-fast-1",
			wantFree: true,
		},
		{
			name:    "ACP model not in API registry",
			modelID: "claude-sonnet-4.5",
			wantNil: true,
		},
		{
			name:    "unknown model",
			modelID: "unknown-model",
			wantNil: true,
		},
		{
			name:    "empty string",
			modelID: "",
			wantNil: true,
		},
		{
			name:     "with whitespace",
			modelID:  "  gpt-4.1  ",
			wantNil:  false,
			wantID:   "gpt-4.1",
			wantFree: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := GetModelInfo(tt.modelID)
			if tt.wantNil {
				if info != nil {
					t.Errorf("GetModelInfo(%q) = %+v, want nil", tt.modelID, info)
				}
				return
			}
			if info == nil {
				t.Fatalf("GetModelInfo(%q) = nil, want non-nil", tt.modelID)
			}
			if info.ID != tt.wantID { //nolint:staticcheck // SA5011: Check above ensures non-nil
				t.Errorf("GetModelInfo(%q).ID = %s, want %s", tt.modelID, info.ID, tt.wantID)
			}
			if info.IsFree != tt.wantFree {
				t.Errorf("GetModelInfo(%q).IsFree = %v, want %v", tt.modelID, info.IsFree, tt.wantFree)
			}
		})
	}
}

func TestListModels(t *testing.T) {
	models := ListModels()

	if len(models) == 0 {
		t.Fatal("ListModels() returned empty list")
	}

	// Check that list is sorted
	for i := 1; i < len(models); i++ {
		if models[i] < models[i-1] {
			t.Errorf("ListModels() not sorted: %s < %s", models[i], models[i-1])
		}
	}

	// Check that expected API models are present
	expectedModels := []string{"gpt-4.1", "gpt-4o", "gpt-5-mini", "grok-code-fast-1"}
	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListModels() missing expected model: %s", expected)
		}
	}
}

func TestListFreeModels(t *testing.T) {
	freeModels := ListFreeModels()

	if len(freeModels) == 0 {
		t.Fatal("ListFreeModels() returned empty list")
	}

	// Verify all returned models are actually free
	for _, modelID := range freeModels {
		info := GetModelInfo(modelID)
		if info == nil {
			t.Errorf("ListFreeModels() returned unknown model: %s", modelID)
			continue
		}
		if !info.IsFree {
			t.Errorf("ListFreeModels() returned non-free model: %s (Multiplier=%d)", modelID, info.Multiplier)
		}
	}

	// Check that list is sorted
	for i := 1; i < len(freeModels); i++ {
		if freeModels[i] < freeModels[i-1] {
			t.Errorf("ListFreeModels() not sorted: %s < %s", freeModels[i], freeModels[i-1])
		}
	}
}

func TestListPremiumModels(t *testing.T) {
	premiumModels := ListPremiumModels()

	// All API models are free, so premium list should be empty
	if len(premiumModels) != 0 {
		t.Errorf("ListPremiumModels() returned %d models, want 0 (all API models are free)", len(premiumModels))
	}

	// Verify all returned models are actually premium
	for _, modelID := range premiumModels {
		info := GetModelInfo(modelID)
		if info == nil {
			t.Errorf("ListPremiumModels() returned unknown model: %s", modelID)
			continue
		}
		if info.IsFree {
			t.Errorf("ListPremiumModels() returned free model: %s", modelID)
		}
	}

	// Check that list is sorted
	for i := 1; i < len(premiumModels); i++ {
		if premiumModels[i] < premiumModels[i-1] {
			t.Errorf("ListPremiumModels() not sorted: %s < %s", premiumModels[i], premiumModels[i-1])
		}
	}
}

func TestListModelsByFamily(t *testing.T) {
	tests := []struct {
		family       ModelFamily
		wantNotEmpty bool
	}{
		{FamilyOpenAI, true},
		{FamilyAnthropic, false}, // Anthropic models are in ACP registry
		{FamilyGoogle, false},    // Google models are in ACP registry
		{FamilyXAI, true},
		{ModelFamily("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.family), func(t *testing.T) {
			models := ListModelsByFamily(tt.family)

			if tt.wantNotEmpty && len(models) == 0 {
				t.Errorf("ListModelsByFamily(%q) returned empty list", tt.family)
			}
			if !tt.wantNotEmpty && len(models) != 0 {
				t.Errorf("ListModelsByFamily(%q) = %v, want empty", tt.family, models)
			}

			// Verify all returned models belong to the family
			for _, modelID := range models {
				info := GetModelInfo(modelID)
				if info == nil {
					t.Errorf("ListModelsByFamily(%q) returned unknown model: %s", tt.family, modelID)
					continue
				}
				if info.Family != tt.family {
					t.Errorf("ListModelsByFamily(%q) returned model with different family: %s (family=%s)", tt.family, modelID, info.Family)
				}
			}
		})
	}
}

func TestGetModelMultiplier(t *testing.T) {
	tests := []struct {
		modelID string
		want    int
	}{
		{"gpt-4.1", 0},            // free
		{"gpt-4o", 0},             // free
		{"gpt-5-mini", 0},         // free
		{"grok-code-fast-1", 0},   // free
		{"unknown", -1},           // not found
		{"claude-sonnet-4.5", -1}, // ACP model, not in API registry
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			got := GetModelMultiplier(tt.modelID)
			if got != tt.want {
				t.Errorf("GetModelMultiplier(%q) = %d, want %d", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestIsModelSupported(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{"gpt-4.1", true},
		{"gpt-4o", true},
		{"grok-code-fast-1", true},
		{"claude-sonnet-4.5", false}, // ACP model, not in API registry
		{"unknown-model", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			got := IsModelSupported(tt.modelID)
			if got != tt.want {
				t.Errorf("IsModelSupported(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestDefaultModels(t *testing.T) {
	// Verify default models exist
	if !IsModelSupported(DefaultModel) {
		t.Errorf("DefaultModel %q is not supported", DefaultModel)
	}
	if !IsModelSupported(DefaultAgentModel) {
		t.Errorf("DefaultAgentModel %q is not supported", DefaultAgentModel)
	}

	// Verify default model is free
	info := GetModelInfo(DefaultModel)
	if info != nil && !info.IsFree {
		t.Errorf("DefaultModel %q should be free", DefaultModel)
	}
}

func TestModelInfoFields(t *testing.T) {
	// Verify all models have required fields populated
	for id, info := range SupportedModels {
		t.Run(id, func(t *testing.T) {
			if info.ID == "" {
				t.Error("ID is empty")
			}
			if info.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if info.Family == "" {
				t.Error("Family is empty")
			}
			if info.Version == "" {
				t.Error("Version is empty")
			}
			if info.ContextWindow <= 0 {
				t.Error("ContextWindow should be positive")
			}
			if info.MaxOutput <= 0 {
				t.Error("MaxOutput should be positive")
			}
			if info.Multiplier < 0 {
				t.Error("Multiplier should be non-negative")
			}
			// Verify consistency: IsFree should match Multiplier == 0
			if info.IsFree && info.Multiplier != 0 {
				t.Errorf("IsFree=true but Multiplier=%d (should be 0)", info.Multiplier)
			}
			if !info.IsFree && info.Multiplier == 0 {
				t.Error("IsFree=false but Multiplier=0 (should be >0)")
			}
		})
	}
}
