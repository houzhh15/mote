package tools

import (
	"context"
	"encoding/json"
	"sync"

	"mote/internal/provider"
)

// Registry manages a collection of tools.
// It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
// Returns ErrToolAlreadyExists if a tool with the same name is already registered.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return NewInvalidArgsError("registry", "tool cannot be nil", nil)
	}

	name := tool.Name()
	if name == "" {
		return NewInvalidArgsError("registry", "tool name cannot be empty", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return NewToolAlreadyExistsError(name)
	}

	r.tools[name] = tool
	return nil
}

// MustRegister adds a tool to the registry and panics on error.
// Useful for registering built-in tools during initialization.
func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

// Get retrieves a tool by name.
// Returns the tool and true if found, nil and false otherwise.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.tools))
	for name := range r.tools {
		result = append(result, name)
	}
	return result
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Execute runs a tool by name with the given arguments.
// Returns ErrToolNotFound if the tool is not registered.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return ToolResult{}, NewToolNotFoundError(name)
	}

	return tool.Execute(ctx, args)
}

// Unregister removes a tool from the registry.
// Returns ErrToolNotFound if the tool is not registered.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return NewToolNotFoundError(name)
	}

	delete(r.tools, name)
	return nil
}

// Clear removes all tools from the registry.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = make(map[string]Tool)
}

// ToProviderTools converts all registered tools to provider.Tool format.
// This is used when sending tool definitions to the LLM.
func (r *Registry) ToProviderTools() ([]provider.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]provider.Tool, 0, len(r.tools))

	for _, tool := range r.tools {
		params := tool.Parameters()

		// Marshal parameters to JSON
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return nil, NewInvalidArgsError(tool.Name(), "failed to marshal parameters", err)
		}

		pt := provider.Tool{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  paramsJSON,
			},
		}

		result = append(result, pt)
	}

	return result, nil
}

// Clone creates a shallow copy of the registry.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clone := NewRegistry()
	for name, tool := range r.tools {
		clone.tools[name] = tool
	}
	return clone
}
