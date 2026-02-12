package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mote/internal/hooks"
	"mote/internal/jsvm"
	"mote/internal/tools"

	"github.com/rs/zerolog/log"
)

// Manager manages skill lifecycle: loading, activation, and deactivation.
type Manager struct {
	// registry stores all loaded skills (by ID)
	registry map[string]*SkillStatus
	// active stores activated skill instances (by ID)
	active map[string]*SkillInstance
	// skillMDEntries stores SKILL.md format skill entries
	skillMDEntries []*SkillEntry

	// External dependencies
	loader       Loader
	toolRegistry *tools.Registry
	hookManager  *hooks.Manager
	jsRuntime    *jsvm.Runtime
	skillsDir    string

	// Multi-path discovery
	discoveryPaths []string
	gating         *Gating

	// Configuration persistence
	configStore ConfigStore

	// Prompt collection
	promptCollector *PromptCollector

	mu sync.RWMutex
}

// ManagerConfig holds configuration for the skill manager.
type ManagerConfig struct {
	SkillsDir string
}

// NewManager creates a new skill manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		registry:        make(map[string]*SkillStatus),
		active:          make(map[string]*SkillInstance),
		skillMDEntries:  make([]*SkillEntry, 0),
		loader:          NewLoader(),
		skillsDir:       cfg.SkillsDir,
		discoveryPaths:  make([]string, 0),
		gating:          NewGating(),
		configStore:     NewFileConfigStore(""),
		promptCollector: NewPromptCollector(),
	}
}

// SetConfigStore sets a custom config store for skill configurations.
func (m *Manager) SetConfigStore(store ConfigStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configStore = store
}

// GetSkillConfig retrieves the configuration for a skill.
func (m *Manager) GetSkillConfig(skillID string) (ConfigMap, error) {
	m.mu.RLock()
	store := m.configStore
	m.mu.RUnlock()

	if store == nil {
		return nil, nil
	}
	return store.Get(skillID)
}

// SetSkillConfig stores the configuration for a skill.
func (m *Manager) SetSkillConfig(skillID string, cfg ConfigMap) error {
	m.mu.RLock()
	store := m.configStore
	m.mu.RUnlock()

	if store == nil {
		return fmt.Errorf("config store not initialized")
	}
	return store.Set(skillID, cfg)
}

// SetDiscoveryPaths sets the paths for multi-path skill discovery.
func (m *Manager) SetDiscoveryPaths(paths []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discoveryPaths = paths
}

// ScanAllPaths scans all configured discovery paths for skills.
// It iterates through discoveryPaths and calls ScanDirectory for each.
// It clears previously scanned entries first to avoid accumulation on refresh,
// while preserving active skill instances.
func (m *Manager) ScanAllPaths() error {
	m.mu.Lock()
	paths := make([]string, len(m.discoveryPaths))
	copy(paths, m.discoveryPaths)
	// Preserve active instances but clear scanned registry & skillMDEntries
	// to avoid duplicates on re-scan.
	activeIDs := make(map[string]bool, len(m.active))
	for id := range m.active {
		activeIDs[id] = true
	}
	newRegistry := make(map[string]*SkillStatus, len(m.registry))
	for id, status := range m.registry {
		if activeIDs[id] {
			newRegistry[id] = status
		}
	}
	m.registry = newRegistry
	m.skillMDEntries = make([]*SkillEntry, 0)
	m.mu.Unlock()

	// Also include the main skills dir
	if m.skillsDir != "" {
		found := false
		for _, p := range paths {
			if p == m.skillsDir {
				found = true
				break
			}
		}
		if !found {
			paths = append([]string{m.skillsDir}, paths...)
		}
	}

	for _, path := range paths {
		if err := m.ScanDirectory(path); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("failed to scan discovery path")
			// Continue scanning other paths
		}
	}
	return nil
}

// SetToolRegistry sets the tool registry for registering skill tools.
func (m *Manager) SetToolRegistry(registry *tools.Registry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolRegistry = registry
}

// SetHookManager sets the hook manager for registering skill hooks.
func (m *Manager) SetHookManager(manager *hooks.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hookManager = manager
}

// SetJSRuntime sets the JavaScript runtime for executing skill tools.
func (m *Manager) SetJSRuntime(runtime *jsvm.Runtime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jsRuntime = runtime
}

// ScanDirectory scans a directory for skills and registers them.
// It loads both manifest.json and SKILL.md format skills.
func (m *Manager) ScanDirectory(dir string) error {
	// Load manifest.json skills
	skills, err := m.loader.ScanDir(dir)
	if err != nil {
		return fmt.Errorf("failed to scan skills directory: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, skill := range skills {
		// Preserve state if skill is already active
		state := SkillStateRegistered
		var activatedAt *time.Time
		var config map[string]any
		if existing, exists := m.registry[skill.ID]; exists {
			if _, active := m.active[skill.ID]; active {
				state = existing.State
				activatedAt = existing.ActivatedAt
				config = existing.Config
			}
		}
		m.registry[skill.ID] = &SkillStatus{
			Skill:       skill,
			State:       state,
			ActivatedAt: activatedAt,
			Config:      config,
		}
		log.Info().
			Str("skill_id", skill.ID).
			Str("skill_name", skill.Name).
			Str("version", skill.Version).
			Msg("registered skill")
	}

	// Also scan for SKILL.md format skills
	skillMDEntries, err := ScanDirForSkillMD(dir)
	if err != nil {
		log.Warn().Err(err).Msg("failed to scan for SKILL.md files")
	} else {
		m.skillMDEntries = append(m.skillMDEntries, skillMDEntries...)
		for _, entry := range skillMDEntries {
			// Convert SkillEntry to Skill so it can be activated
			skill := ConvertSkillEntryToSkill(entry)
			// Preserve state if skill is already active
			state := SkillStateRegistered
			var activatedAt *time.Time
			var config map[string]any
			if existing, exists := m.registry[skill.ID]; exists {
				if _, active := m.active[skill.ID]; active {
					state = existing.State
					activatedAt = existing.ActivatedAt
					config = existing.Config
				}
			}
			m.registry[skill.ID] = &SkillStatus{
				Skill:       skill,
				State:       state,
				ActivatedAt: activatedAt,
				Config:      config,
			}
			log.Info().
				Str("skill_id", skill.ID).
				Str("skill_name", entry.Name).
				Str("location", entry.Location).
				Msg("registered SKILL.md skill")
		}
	}

	return nil
}

// LoadSkill loads a single skill from a directory.
func (m *Manager) LoadSkill(skillDir string) (*Skill, error) {
	skill, err := m.loader.Load(skillDir)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.registry[skill.ID] = &SkillStatus{
		Skill: skill,
		State: SkillStateRegistered,
	}

	return skill, nil
}

// GetSkill returns a skill by ID.
func (m *Manager) GetSkill(id string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.registry[id]
	if !exists {
		return nil, false
	}
	return status.Skill, true
}

// ListSkills returns all registered skills with their status.
func (m *Manager) ListSkills() []*SkillStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SkillStatus, 0, len(m.registry))
	for _, status := range m.registry {
		result = append(result, status)
	}
	return result
}

// Activate activates a skill, registering its tools and hooks.
func (m *Manager) Activate(id string, config map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if skill exists
	status, exists := m.registry[id]
	if !exists {
		return ErrSkillNotFound
	}

	// Check if already active
	if _, active := m.active[id]; active {
		return ErrSkillAlreadyActive
	}

	skill := status.Skill

	// Check dependencies
	for _, depID := range skill.Dependencies {
		if _, active := m.active[depID]; !active {
			return fmt.Errorf("%w: %s requires %s", ErrDependencyMissing, id, depID)
		}
	}

	// Create instance
	instance := &SkillInstance{
		Skill:       skill,
		Config:      config,
		Tools:       make([]tools.Tool, 0),
		Prompts:     make([]*SkillPrompt, 0),
		HookIDs:     make([]string, 0),
		ActivatedAt: time.Now(),
	}

	skillDir := GetSkillDir(skill)

	// Register tools
	if m.toolRegistry != nil && m.jsRuntime != nil {
		for _, toolDef := range skill.Tools {
			skillTool := NewSkillTool(skill.ID, skillDir, toolDef, m.jsRuntime)
			if err := m.toolRegistry.Register(skillTool); err != nil {
				// Rollback registered tools
				m.rollbackTools(instance)
				return fmt.Errorf("failed to register tool %s: %w", toolDef.Name, err)
			}
			instance.Tools = append(instance.Tools, skillTool)
		}
	}

	// Resolve prompts
	for _, promptDef := range skill.Prompts {
		prompt, err := m.resolvePrompt(skill.ID, skillDir, promptDef)
		if err != nil {
			log.Warn().Err(err).Str("prompt", promptDef.Name).Msg("failed to resolve prompt")
			continue
		}
		instance.Prompts = append(instance.Prompts, prompt)
	}

	// Register hooks
	if m.hookManager != nil {
		for _, hookDef := range skill.Hooks {
			handlerID := fmt.Sprintf("%s:%s:%s", skill.ID, hookDef.Type, hookDef.Handler)
			// Note: Hook handler execution via JSVM would be implemented similarly to tools
			// For now, we just record the hook ID
			instance.HookIDs = append(instance.HookIDs, handlerID)
		}
	}

	// Update state
	m.active[id] = instance
	status.State = SkillStateActive
	now := time.Now()
	status.ActivatedAt = &now
	status.Config = config

	// Collect prompts via promptCollector
	if m.promptCollector != nil {
		m.promptCollector.Collect(instance)
	}

	log.Info().
		Str("skill_id", skill.ID).
		Int("tools", len(instance.Tools)).
		Int("prompts", len(instance.Prompts)).
		Int("hooks", len(instance.HookIDs)).
		Msg("activated skill")

	return nil
}

// Deactivate deactivates a skill, unregistering its tools and hooks.
func (m *Manager) Deactivate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if skill exists
	status, exists := m.registry[id]
	if !exists {
		return ErrSkillNotFound
	}

	// Check if active
	instance, active := m.active[id]
	if !active {
		return ErrSkillNotActive
	}

	// Check if other skills depend on this one
	for depID, depInstance := range m.active {
		if depID == id {
			continue
		}
		for _, dep := range depInstance.Skill.Dependencies {
			if dep == id {
				return fmt.Errorf("cannot deactivate: %s depends on %s", depID, id)
			}
		}
	}

	// Unregister tools
	if m.toolRegistry != nil {
		for _, tool := range instance.Tools {
			_ = m.toolRegistry.Unregister(tool.Name())
		}
	}

	// Remove prompts from collector
	if m.promptCollector != nil {
		m.promptCollector.Remove(id)
	}

	// Update state
	delete(m.active, id)
	status.State = SkillStateRegistered
	status.ActivatedAt = nil
	status.Config = nil

	log.Info().Str("skill_id", id).Msg("deactivated skill")

	return nil
}

// IsActive returns whether a skill is currently active.
func (m *Manager) IsActive(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, active := m.active[id]
	return active
}

// GetActiveTools returns all tools from active skills.
func (m *Manager) GetActiveTools() []tools.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []tools.Tool
	for _, instance := range m.active {
		result = append(result, instance.Tools...)
	}
	return result
}

// GetActivePrompts returns all prompts from active skills.
func (m *Manager) GetActivePrompts() []*SkillPrompt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*SkillPrompt
	for _, instance := range m.active {
		result = append(result, instance.Prompts...)
	}
	return result
}

// GetContributedPrompts returns all prompts collected via the PromptCollector.
// This is the preferred way to get skill-contributed prompts.
func (m *Manager) GetContributedPrompts() []*SkillPrompt {
	m.mu.RLock()
	collector := m.promptCollector
	m.mu.RUnlock()

	if collector == nil {
		return nil
	}
	return collector.GetAll()
}

// GetContributedPromptsByTag returns prompts with the specified tag.
func (m *Manager) GetContributedPromptsByTag(tag string) []*SkillPrompt {
	m.mu.RLock()
	collector := m.promptCollector
	m.mu.RUnlock()

	if collector == nil {
		return nil
	}
	return collector.GetByTag(tag)
}

// GetInstance returns the active instance for a skill.
func (m *Manager) GetInstance(id string) (*SkillInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, exists := m.active[id]
	return instance, exists
}

// Close releases all resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Deactivate all skills
	for id := range m.active {
		delete(m.active, id)
		if status, exists := m.registry[id]; exists {
			status.State = SkillStateRegistered
			status.ActivatedAt = nil
		}
	}

	return nil
}

// rollbackTools unregisters tools that were registered during a failed activation.
func (m *Manager) rollbackTools(instance *SkillInstance) {
	if m.toolRegistry == nil {
		return
	}
	for _, tool := range instance.Tools {
		_ = m.toolRegistry.Unregister(tool.Name())
	}
}

// resolvePrompt resolves a prompt definition to a SkillPrompt.
func (m *Manager) resolvePrompt(skillID, skillDir string, def *PromptDef) (*SkillPrompt, error) {
	prompt := &SkillPrompt{
		SkillID: skillID,
		Name:    def.Name,
		Tags:    def.Tags,
	}

	// If content is inline, use it directly
	if def.Content != "" {
		prompt.Content = def.Content
		return prompt, nil
	}

	// Otherwise, read from file
	if def.File != "" {
		filePath := filepath.Join(skillDir, def.File)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read prompt file: %w", err)
		}
		prompt.Content = string(content)
		return prompt, nil
	}

	return nil, ErrPromptInvalid
}

// ReloadSkill reloads a skill from disk.
func (m *Manager) ReloadSkill(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, exists := m.registry[id]
	if !exists {
		return ErrSkillNotFound
	}

	// Reload from disk
	newSkill, err := m.loader.Reload(status.Skill)
	if err != nil {
		return fmt.Errorf("failed to reload skill: %w", err)
	}

	// Update registry
	status.Skill = newSkill

	// If skill was active, it needs to be reactivated manually
	if _, active := m.active[id]; active {
		log.Warn().
			Str("skill_id", id).
			Msg("skill reloaded but still active with old version, deactivate and reactivate to apply changes")
	}

	return nil
}

// GetAvailableEntries returns all available skill entries for system prompt injection.
// It includes skills from both manifest.json and SKILL.md formats, filtered by gating.
// Note: SKILL.md skills are already converted and registered in the registry by ScanDirectory,
// so we only collect from registry to avoid double-listing.
func (m *Manager) GetAvailableEntries() []*SkillEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gating := &Gating{}
	var entries []*SkillEntry

	// Collect all skills from registry (includes both manifest.json and SKILL.md converted skills)
	for _, status := range m.registry {
		source := SourceManifest
		if strings.HasSuffix(status.Skill.FilePath, "SKILL.md") {
			source = SourceSkillMD
		}
		entry := &SkillEntry{
			Name:        status.Skill.Name,
			Description: status.Skill.Description,
			Location:    status.Skill.FilePath,
			Source:      source,
			Skill:       status.Skill,
		}
		if gating.ShouldInclude(entry) {
			entries = append(entries, entry)
		}
	}

	return entries
}

// FormatSkillsXML formats available skills as XML for system prompt injection.
func (m *Manager) FormatSkillsXML() string {
	entries := m.GetAvailableEntries()
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, entry := range entries {
		b.WriteString("  <skill>\n")
		b.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(entry.Name)))
		b.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(entry.Description)))
		b.WriteString(fmt.Sprintf("    <location>%s</location>\n", escapeXML(entry.Location)))
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// FormatSkillsXMLFiltered formats available skills as XML, filtered by selected skill IDs.
func (m *Manager) FormatSkillsXMLFiltered(selectedSkills []string) string {
	entries := m.GetAvailableEntries()
	if len(entries) == 0 {
		return ""
	}

	// Build lookup set
	selected := make(map[string]bool, len(selectedSkills))
	for _, id := range selectedSkills {
		selected[id] = true
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")
	count := 0
	for _, entry := range entries {
		if !selected[entry.Name] {
			continue
		}
		b.WriteString("  <skill>\n")
		b.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(entry.Name)))
		b.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(entry.Description)))
		b.WriteString(fmt.Sprintf("    <location>%s</location>\n", escapeXML(entry.Location)))
		b.WriteString("  </skill>\n")
		count++
	}
	b.WriteString("</available_skills>")

	if count == 0 {
		return ""
	}
	return b.String()
}

// escapeXML escapes special characters for XML content.
func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
