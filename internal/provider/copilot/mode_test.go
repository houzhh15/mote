package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsValidMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"ask", true},
		{"edit", true},
		{"agent", true},
		{"plan", true},
		{"Ask", false},   // Case sensitive
		{"AGENT", false}, // Case sensitive
		{"invalid", false},
		{"", false},
		{"code", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := IsValidMode(tt.mode)
			if got != tt.want {
				t.Errorf("IsValidMode(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		input   string
		want    Mode
		wantErr bool
	}{
		{"ask", ModeAsk, false},
		{"edit", ModeEdit, false},
		{"agent", ModeAgent, false},
		{"plan", ModePlan, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMode(%q) should have returned error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseMode(%q) returned error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewModeManager(t *testing.T) {
	mm := NewModeManager(WithPersistence(false))

	if mm == nil {
		t.Fatal("NewModeManager returned nil")
	}

	if mm.GetMode() != DefaultMode {
		t.Errorf("GetMode() = %v, want %v", mm.GetMode(), DefaultMode)
	}
}

func TestNewModeManager_WithOptions(t *testing.T) {
	customPath := "/tmp/test-mode.json"

	mm := NewModeManager(
		WithConfigPath(customPath),
		WithPersistence(false),
		WithInitialMode(ModeAsk),
	)

	if mm.configPath != customPath {
		t.Errorf("configPath = %s, want %s", mm.configPath, customPath)
	}

	if mm.persist {
		t.Error("persist should be false")
	}

	if mm.GetMode() != ModeAsk {
		t.Errorf("GetMode() = %v, want %v", mm.GetMode(), ModeAsk)
	}
}

func TestModeManager_GetSetMode(t *testing.T) {
	mm := NewModeManager(WithPersistence(false))

	// Test setting each valid mode
	for _, mode := range ValidModes {
		t.Run(string(mode), func(t *testing.T) {
			err := mm.SetMode(mode)
			if err != nil {
				t.Errorf("SetMode(%q) returned error: %v", mode, err)
			}

			got := mm.GetMode()
			if got != mode {
				t.Errorf("GetMode() = %v, want %v", got, mode)
			}
		})
	}
}

func TestModeManager_SetMode_Invalid(t *testing.T) {
	mm := NewModeManager(WithPersistence(false))

	err := mm.SetMode(Mode("invalid"))
	if err == nil {
		t.Error("SetMode with invalid mode should return error")
	}

	// Mode should remain unchanged
	if mm.GetMode() != DefaultMode {
		t.Errorf("Mode changed to %v after invalid SetMode", mm.GetMode())
	}
}

func TestModeManager_GetModeInfo(t *testing.T) {
	mm := NewModeManager(
		WithPersistence(false),
		WithInitialMode(ModeAgent),
	)

	info := mm.GetModeInfo()

	if info.Mode != ModeAgent {
		t.Errorf("ModeInfo.Mode = %v, want %v", info.Mode, ModeAgent)
	}

	if info.DisplayName != "Agent" {
		t.Errorf("ModeInfo.DisplayName = %s, want Agent", info.DisplayName)
	}

	if !info.IsDefault {
		t.Error("Agent mode should be marked as default")
	}
}

func TestModeManager_ListModes(t *testing.T) {
	mm := NewModeManager(WithPersistence(false))

	modes := mm.ListModes()

	if len(modes) != len(ValidModes) {
		t.Errorf("ListModes() returned %d modes, want %d", len(modes), len(ValidModes))
	}

	// Check that each valid mode is present
	modeSet := make(map[Mode]bool)
	for _, info := range modes {
		modeSet[info.Mode] = true
	}

	for _, m := range ValidModes {
		if !modeSet[m] {
			t.Errorf("ListModes() missing mode: %v", m)
		}
	}
}

func TestModeManager_Reset(t *testing.T) {
	mm := NewModeManager(
		WithPersistence(false),
		WithInitialMode(ModeAsk),
	)

	if mm.GetMode() != ModeAsk {
		t.Errorf("Initial mode = %v, want %v", mm.GetMode(), ModeAsk)
	}

	err := mm.Reset()
	if err != nil {
		t.Errorf("Reset() returned error: %v", err)
	}

	if mm.GetMode() != DefaultMode {
		t.Errorf("After Reset(), mode = %v, want %v", mm.GetMode(), DefaultMode)
	}
}

func TestModeManager_String(t *testing.T) {
	mm := NewModeManager(
		WithPersistence(false),
		WithInitialMode(ModeEdit),
	)

	if mm.String() != "edit" {
		t.Errorf("String() = %s, want edit", mm.String())
	}
}

func TestModeManager_Persistence(t *testing.T) {
	// Create temp directory for config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mode.json")

	// Create manager and set mode
	mm1 := NewModeManager(
		WithConfigPath(configPath),
		WithPersistence(true),
	)

	err := mm1.SetMode(ModeEdit)
	if err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	// Create new manager and verify it loads the saved mode
	mm2 := NewModeManager(
		WithConfigPath(configPath),
		WithPersistence(true),
	)

	if mm2.GetMode() != ModeEdit {
		t.Errorf("Loaded mode = %v, want %v", mm2.GetMode(), ModeEdit)
	}
}

func TestMode_JSON(t *testing.T) {
	tests := []struct {
		mode Mode
		json string
	}{
		{ModeAsk, `"ask"`},
		{ModeEdit, `"edit"`},
		{ModeAgent, `"agent"`},
		{ModePlan, `"plan"`},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			// Test Marshal
			data, err := json.Marshal(tt.mode)
			if err != nil {
				t.Errorf("Marshal error: %v", err)
			}
			if string(data) != tt.json {
				t.Errorf("Marshal = %s, want %s", string(data), tt.json)
			}

			// Test Unmarshal
			var mode Mode
			err = json.Unmarshal([]byte(tt.json), &mode)
			if err != nil {
				t.Errorf("Unmarshal error: %v", err)
			}
			if mode != tt.mode {
				t.Errorf("Unmarshal = %v, want %v", mode, tt.mode)
			}
		})
	}
}

func TestMode_UnmarshalJSON_Invalid(t *testing.T) {
	var mode Mode
	err := json.Unmarshal([]byte(`"invalid"`), &mode)
	if err == nil {
		t.Error("Unmarshal with invalid mode should return error")
	}
}

func TestModeRegistry(t *testing.T) {
	// Verify all valid modes are in registry
	for _, m := range ValidModes {
		info, ok := ModeRegistry[m]
		if !ok {
			t.Errorf("Mode %v not in registry", m)
			continue
		}

		if info.Mode != m {
			t.Errorf("Registry mode mismatch: info.Mode = %v, key = %v", info.Mode, m)
		}

		if info.DisplayName == "" {
			t.Errorf("Mode %v has empty DisplayName", m)
		}

		if info.Description == "" {
			t.Errorf("Mode %v has empty Description", m)
		}
	}

	// Verify exactly one default mode
	defaultCount := 0
	for _, info := range ModeRegistry {
		if info.IsDefault {
			defaultCount++
			if info.Mode != DefaultMode {
				t.Errorf("IsDefault=true for %v, but DefaultMode=%v", info.Mode, DefaultMode)
			}
		}
	}

	if defaultCount != 1 {
		t.Errorf("Found %d default modes, want 1", defaultCount)
	}
}

func TestModeManager_LoadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent", "mode.json")

	mm := NewModeManager(
		WithConfigPath(configPath),
		WithPersistence(true),
	)

	// Should use default mode when config doesn't exist
	if mm.GetMode() != DefaultMode {
		t.Errorf("GetMode() = %v, want %v (default)", mm.GetMode(), DefaultMode)
	}
}

func TestModeManager_LoadCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mode.json")

	// Write corrupt JSON
	err := os.WriteFile(configPath, []byte("not valid json"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mm := NewModeManager(
		WithConfigPath(configPath),
		WithPersistence(true),
	)

	// Should use default mode when config is corrupt
	if mm.GetMode() != DefaultMode {
		t.Errorf("GetMode() = %v, want %v (default)", mm.GetMode(), DefaultMode)
	}
}
