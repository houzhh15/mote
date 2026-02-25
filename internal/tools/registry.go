package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"mote/internal/provider"
)

// Registry manages a collection of tools.
// It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]Tool
	agentID string
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
	clone.agentID = r.agentID
	return clone
}

// Filter keeps only the tools whose names appear in allowList.
// Supports special syntax:
//   - "*" — keep all tools (no filtering)
//   - "!tool_name" — exclude a specific tool (applied after inclusion)
//   - "prefix_*" — glob pattern matching tools with the given prefix
func (r *Registry) Filter(allowList []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Separate inclusions (positive) and exclusions (negative)
	var includes []string
	var excludes []string
	for _, name := range allowList {
		if name == "*" {
			// Wildcard: only process exclusions
			includes = nil
			for _, n := range allowList {
				if strings.HasPrefix(n, "!") {
					excludes = append(excludes, n[1:])
				}
			}
			for _, ex := range excludes {
				delete(r.tools, ex)
			}
			return
		}
		if strings.HasPrefix(name, "!") {
			excludes = append(excludes, name[1:])
		} else {
			includes = append(includes, name)
		}
	}

	// Build allowed set from includes (may contain glob patterns)
	allowed := make(map[string]struct{})
	for _, pattern := range includes {
		if strings.HasSuffix(pattern, "*") {
			// Glob: match all tools with this prefix
			prefix := pattern[:len(pattern)-1]
			for name := range r.tools {
				if strings.HasPrefix(name, prefix) {
					allowed[name] = struct{}{}
				}
			}
		} else {
			allowed[pattern] = struct{}{}
		}
	}
	for name := range r.tools {
		if _, ok := allowed[name]; !ok {
			delete(r.tools, name)
		}
	}

	// Apply exclusions
	for _, ex := range excludes {
		delete(r.tools, ex)
	}
}

// Remove deletes a tool from the registry by name.
// Unlike Unregister, it does not return an error if the tool does not exist.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// SetAgentID sets the agent identifier for this registry.
func (r *Registry) SetAgentID(id string) {
	r.agentID = id
}

// GetAgentID returns the agent identifier for this registry.
func (r *Registry) GetAgentID() string {
	return r.agentID
}
