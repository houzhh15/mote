// Package skills provides the skill system for Mote Agent Runtime.
package skills

import (
	"sync"
)

// PromptCollector collects prompts from activated skill instances.
// It provides a unified view of all skill-contributed prompts.
type PromptCollector struct {
	mu      sync.RWMutex
	prompts map[string][]*SkillPrompt // skillID -> prompts
}

// NewPromptCollector creates a new prompt collector.
func NewPromptCollector() *PromptCollector {
	return &PromptCollector{
		prompts: make(map[string][]*SkillPrompt),
	}
}

// Collect extracts and stores prompts from a skill instance.
// Call this when activating a skill to register its prompts.
func (c *PromptCollector) Collect(instance *SkillInstance) {
	if instance == nil || instance.Skill == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	skillID := instance.Skill.ID
	c.prompts[skillID] = make([]*SkillPrompt, len(instance.Prompts))
	copy(c.prompts[skillID], instance.Prompts)
}

// Remove removes all prompts for a skill.
// Call this when deactivating a skill to unregister its prompts.
func (c *PromptCollector) Remove(skillID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.prompts, skillID)
}

// GetAll returns all collected prompts from all skills.
// The returned slice is a flattened list of all prompts.
func (c *PromptCollector) GetAll() []*SkillPrompt {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*SkillPrompt
	for _, prompts := range c.prompts {
		result = append(result, prompts...)
	}
	return result
}

// GetBySkill returns prompts for a specific skill.
func (c *PromptCollector) GetBySkill(skillID string) []*SkillPrompt {
	c.mu.RLock()
	defer c.mu.RUnlock()

	prompts, exists := c.prompts[skillID]
	if !exists {
		return nil
	}
	// Return a copy to prevent external modification
	result := make([]*SkillPrompt, len(prompts))
	copy(result, prompts)
	return result
}

// GetByTag returns all prompts that have the specified tag.
func (c *PromptCollector) GetByTag(tag string) []*SkillPrompt {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*SkillPrompt
	for _, prompts := range c.prompts {
		for _, p := range prompts {
			for _, t := range p.Tags {
				if t == tag {
					result = append(result, p)
					break
				}
			}
		}
	}
	return result
}

// Clear removes all collected prompts.
func (c *PromptCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = make(map[string][]*SkillPrompt)
}

// Len returns the total number of collected prompts.
func (c *PromptCollector) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, prompts := range c.prompts {
		count += len(prompts)
	}
	return count
}

// SkillCount returns the number of skills with collected prompts.
func (c *PromptCollector) SkillCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.prompts)
}

// Has returns true if prompts have been collected for the skill.
func (c *PromptCollector) Has(skillID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.prompts[skillID]
	return exists
}
