// Package copilot provides GitHub Copilot integration for Mote.
package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"mote/pkg/logger"
)

// Mode represents the Copilot agent mode.
type Mode string

const (
	// ModeAsk is the ask mode for simple Q&A interactions.
	ModeAsk Mode = "ask"

	// ModeEdit is the edit mode for code editing tasks.
	ModeEdit Mode = "edit"

	// ModeAgent is the agent mode for autonomous multi-step tasks.
	// This is the default mode.
	ModeAgent Mode = "agent"

	// ModePlan is the plan mode for generating execution plans.
	ModePlan Mode = "plan"
)

// DefaultMode is the default agent mode.
const DefaultMode = ModeAgent

// ValidModes contains all valid mode values.
var ValidModes = []Mode{ModeAsk, ModeEdit, ModeAgent, ModePlan}

// ModeInfo contains metadata about a mode.
type ModeInfo struct {
	Mode        Mode   `json:"mode"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// ModeRegistry contains information about all available modes.
var ModeRegistry = map[Mode]ModeInfo{
	ModeAsk: {
		Mode:        ModeAsk,
		DisplayName: "Ask",
		Description: "Simple Q&A mode for answering questions without making changes",
		IsDefault:   false,
	},
	ModeEdit: {
		Mode:        ModeEdit,
		DisplayName: "Edit",
		Description: "Code editing mode for focused file modifications",
		IsDefault:   false,
	},
	ModeAgent: {
		Mode:        ModeAgent,
		DisplayName: "Agent",
		Description: "Autonomous agent mode for complex multi-step tasks",
		IsDefault:   true,
	},
	ModePlan: {
		Mode:        ModePlan,
		DisplayName: "Plan",
		Description: "Planning mode for generating step-by-step execution plans",
		IsDefault:   false,
	},
}

// IsValidMode checks if a mode string is valid.
func IsValidMode(mode string) bool {
	for _, m := range ValidModes {
		if string(m) == mode {
			return true
		}
	}
	return false
}

// ParseMode parses a string into a Mode, returning an error if invalid.
func ParseMode(s string) (Mode, error) {
	if !IsValidMode(s) {
		return "", fmt.Errorf("invalid mode %q: must be one of ask, edit, agent, plan", s)
	}
	return Mode(s), nil
}

// ModeConfig represents the persisted mode configuration.
type ModeConfig struct {
	CurrentMode Mode `json:"current_mode"`
}

// ModeManager manages the current agent mode.
type ModeManager struct {
	mu         sync.RWMutex
	mode       Mode
	configPath string
	persist    bool
}

// ModeManagerOption is a functional option for ModeManager.
type ModeManagerOption func(*ModeManager)

// WithConfigPath sets the configuration file path.
func WithConfigPath(path string) ModeManagerOption {
	return func(mm *ModeManager) {
		mm.configPath = path
	}
}

// WithPersistence enables or disables mode persistence.
func WithPersistence(persist bool) ModeManagerOption {
	return func(mm *ModeManager) {
		mm.persist = persist
	}
}

// WithInitialMode sets the initial mode.
func WithInitialMode(mode Mode) ModeManagerOption {
	return func(mm *ModeManager) {
		mm.mode = mode
	}
}

// NewModeManager creates a new ModeManager with the given options.
func NewModeManager(opts ...ModeManagerOption) *ModeManager {
	// Default config path
	homeDir, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(homeDir, ".config", "mote", "mode.json")

	mm := &ModeManager{
		mode:       DefaultMode,
		configPath: defaultConfigPath,
		persist:    true,
	}

	for _, opt := range opts {
		opt(mm)
	}

	// Try to load existing config
	if mm.persist {
		if err := mm.load(); err != nil {
			logger.Debug().Err(err).Msg("Could not load mode config, using default")
		}
	}

	return mm
}

// GetMode returns the current mode.
func (mm *ModeManager) GetMode() Mode {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.mode
}

// SetMode sets the current mode.
func (mm *ModeManager) SetMode(mode Mode) error {
	if !IsValidMode(string(mode)) {
		return fmt.Errorf("invalid mode %q: must be one of ask, edit, agent, plan", mode)
	}

	mm.mu.Lock()
	mm.mode = mode
	mm.mu.Unlock()

	if mm.persist {
		if err := mm.save(); err != nil {
			logger.Warn().Err(err).Msg("Could not persist mode config")
		}
	}

	logger.Info().Str("mode", string(mode)).Msg("Mode changed")
	return nil
}

// GetModeInfo returns information about the current mode.
func (mm *ModeManager) GetModeInfo() ModeInfo {
	mm.mu.RLock()
	mode := mm.mode
	mm.mu.RUnlock()

	if info, ok := ModeRegistry[mode]; ok {
		return info
	}

	// Fallback to default
	return ModeRegistry[DefaultMode]
}

// ListModes returns all available modes with their information.
func (mm *ModeManager) ListModes() []ModeInfo {
	modes := make([]ModeInfo, 0, len(ValidModes))
	for _, m := range ValidModes {
		if info, ok := ModeRegistry[m]; ok {
			modes = append(modes, info)
		}
	}
	return modes
}

// save persists the current mode to the config file.
func (mm *ModeManager) save() error {
	mm.mu.RLock()
	config := ModeConfig{CurrentMode: mm.mode}
	mm.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(mm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(mm.configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// load reads the mode from the config file.
func (mm *ModeManager) load() error {
	data, err := os.ReadFile(mm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No config file, use defaults
		}
		return fmt.Errorf("read config: %w", err)
	}

	var config ModeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	if IsValidMode(string(config.CurrentMode)) {
		mm.mu.Lock()
		mm.mode = config.CurrentMode
		mm.mu.Unlock()
	}

	return nil
}

// Reset resets the mode to the default.
func (mm *ModeManager) Reset() error {
	return mm.SetMode(DefaultMode)
}

// String returns the string representation of the current mode.
func (mm *ModeManager) String() string {
	return string(mm.GetMode())
}

// MarshalJSON implements json.Marshaler.
func (m Mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Mode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	mode, err := ParseMode(s)
	if err != nil {
		return err
	}

	*m = mode
	return nil
}
