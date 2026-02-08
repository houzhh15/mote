// Package prompt provides prompt slot system for injection points.
package prompt

import (
	"sort"
	"sync"
)

// PromptSlot represents a named injection point in the system prompt.
type PromptSlot string

// Slot constants for prompt injection points.
const (
	// SlotIdentity is for agent identity and persona injections.
	SlotIdentity PromptSlot = "identity"
	// SlotCapabilities is for tool and capability descriptions.
	SlotCapabilities PromptSlot = "capabilities"
	// SlotContext is for contextual information (memory, workspace).
	SlotContext PromptSlot = "context"
	// SlotConstraints is for rules and constraints.
	SlotConstraints PromptSlot = "constraints"
	// SlotTail is for final instructions appended at the end.
	SlotTail PromptSlot = "tail"
)

// AllPromptSlots returns all defined prompt slots.
func AllPromptSlots() []PromptSlot {
	return []PromptSlot{
		SlotIdentity,
		SlotCapabilities,
		SlotContext,
		SlotConstraints,
		SlotTail,
	}
}

// IsValidSlot checks if the given slot is a valid prompt slot.
func IsValidSlot(slot PromptSlot) bool {
	for _, s := range AllPromptSlots() {
		if s == slot {
			return true
		}
	}
	return false
}

// PromptInjection represents content to be injected into a prompt slot.
type PromptInjection struct {
	Content  string `json:"content"`  // The content to inject
	Priority int    `json:"priority"` // Higher = earlier in slot (default 0)
	Source   string `json:"source"`   // Source identifier (skill ID, builtin, etc.)
}

// PromptInjector manages prompt injections across slots.
type PromptInjector struct {
	mu         sync.RWMutex
	injections map[PromptSlot][]*PromptInjection
}

// NewPromptInjector creates a new prompt injector.
func NewPromptInjector() *PromptInjector {
	return &PromptInjector{
		injections: make(map[PromptSlot][]*PromptInjection),
	}
}

// Inject adds an injection to a slot.
func (p *PromptInjector) Inject(slot PromptSlot, injection *PromptInjection) {
	if injection == nil || injection.Content == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.injections[slot] == nil {
		p.injections[slot] = make([]*PromptInjection, 0)
	}
	p.injections[slot] = append(p.injections[slot], injection)
}

// Remove removes all injections from a source in all slots.
func (p *PromptInjector) Remove(source string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for slot, injections := range p.injections {
		filtered := make([]*PromptInjection, 0)
		for _, inj := range injections {
			if inj.Source != source {
				filtered = append(filtered, inj)
			}
		}
		p.injections[slot] = filtered
	}
}

// RemoveFromSlot removes all injections from a source in a specific slot.
func (p *PromptInjector) RemoveFromSlot(slot PromptSlot, source string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	injections, exists := p.injections[slot]
	if !exists {
		return
	}

	filtered := make([]*PromptInjection, 0)
	for _, inj := range injections {
		if inj.Source != source {
			filtered = append(filtered, inj)
		}
	}
	p.injections[slot] = filtered
}

// GetBySlot returns all injections for a slot, sorted by priority (descending).
func (p *PromptInjector) GetBySlot(slot PromptSlot) []*PromptInjection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	injections, exists := p.injections[slot]
	if !exists || len(injections) == 0 {
		return nil
	}

	// Make a copy and sort by priority (higher first)
	result := make([]*PromptInjection, len(injections))
	copy(result, injections)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// GetContentBySlot returns the concatenated content for a slot.
// Content is joined with newlines, sorted by priority.
func (p *PromptInjector) GetContentBySlot(slot PromptSlot) string {
	injections := p.GetBySlot(slot)
	if len(injections) == 0 {
		return ""
	}

	var content string
	for i, inj := range injections {
		if i > 0 {
			content += "\n"
		}
		content += inj.Content
	}
	return content
}

// Clear removes all injections from all slots.
func (p *PromptInjector) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.injections = make(map[PromptSlot][]*PromptInjection)
}

// ClearSlot removes all injections from a specific slot.
func (p *PromptInjector) ClearSlot(slot PromptSlot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.injections, slot)
}

// Len returns the total number of injections across all slots.
func (p *PromptInjector) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, injections := range p.injections {
		count += len(injections)
	}
	return count
}

// SlotCount returns the number of slots with injections.
func (p *PromptInjector) SlotCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, injections := range p.injections {
		if len(injections) > 0 {
			count++
		}
	}
	return count
}

// Has returns true if there are injections for the slot.
func (p *PromptInjector) Has(slot PromptSlot) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	injections, exists := p.injections[slot]
	return exists && len(injections) > 0
}
