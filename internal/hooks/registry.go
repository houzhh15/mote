package hooks

import (
	"fmt"
	"sort"
	"sync"
)

// Registry manages hook handler registrations.
type Registry struct {
	handlers map[HookType][]*Handler
	mu       sync.RWMutex
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[HookType][]*Handler),
	}
}

// Register registers a handler for the given hook type.
// Returns an error if a handler with the same ID already exists.
func (r *Registry) Register(hookType HookType, handler *Handler) error {
	if !IsValidHookType(hookType) {
		return fmt.Errorf("%w: %s", ErrHookTypeInvalid, hookType)
	}

	if handler == nil || handler.ID == "" {
		return fmt.Errorf("%w: handler ID is required", ErrHandlerNotFound)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if handler with same ID already exists
	for _, h := range r.handlers[hookType] {
		if h.ID == handler.ID {
			return fmt.Errorf("%w: %s", ErrHandlerExists, handler.ID)
		}
	}

	// Add handler
	r.handlers[hookType] = append(r.handlers[hookType], handler)

	// Sort by priority (descending - higher priority first)
	sort.Slice(r.handlers[hookType], func(i, j int) bool {
		return r.handlers[hookType][i].Priority > r.handlers[hookType][j].Priority
	})

	return nil
}

// Unregister removes a handler from the given hook type.
// Returns an error if the handler is not found.
func (r *Registry) Unregister(hookType HookType, handlerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	handlers, exists := r.handlers[hookType]
	if !exists {
		return fmt.Errorf("%w: %s", ErrHandlerNotFound, handlerID)
	}

	for i, h := range handlers {
		if h.ID == handlerID {
			// Remove handler by swapping with last and truncating
			r.handlers[hookType] = append(handlers[:i], handlers[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrHandlerNotFound, handlerID)
}

// GetHandlers returns a copy of all handlers for the given hook type.
// The handlers are sorted by priority (descending).
func (r *Registry) GetHandlers(hookType HookType) []*Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlers := r.handlers[hookType]
	if len(handlers) == 0 {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]*Handler, len(handlers))
	copy(result, handlers)
	return result
}

// HasHandlers returns true if there are any handlers registered for the given hook type.
func (r *Registry) HasHandlers(hookType HookType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[hookType]) > 0
}

// Clear removes all registered handlers.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = make(map[HookType][]*Handler)
}

// Count returns the total number of registered handlers across all hook types.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, handlers := range r.handlers {
		total += len(handlers)
	}
	return total
}

// ListTypes returns all hook types that have registered handlers.
func (r *Registry) ListTypes() []HookType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var types []HookType
	for hookType, handlers := range r.handlers {
		if len(handlers) > 0 {
			types = append(types, hookType)
		}
	}
	return types
}

// GetAllHandlers returns a map of all registered handlers grouped by hook type.
func (r *Registry) GetAllHandlers() map[HookType][]*Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[HookType][]*Handler)
	for hookType, handlers := range r.handlers {
		if len(handlers) > 0 {
			handlersCopy := make([]*Handler, len(handlers))
			copy(handlersCopy, handlers)
			result[hookType] = handlersCopy
		}
	}
	return result
}
