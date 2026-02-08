package builtin

import (
	"mote/internal/tools"
)

// RegisterBuiltins registers all built-in tools to the given registry.
func RegisterBuiltins(r *tools.Registry) error {
	builtins := []tools.Tool{
		NewShellTool(),
		NewReadFileTool(),
		NewWriteFileTool(),
		NewEditFileTool(),
		NewListDirTool(),
		NewHTTPTool(),
	}

	for _, tool := range builtins {
		if err := r.Register(tool); err != nil {
			return err
		}
	}

	return nil
}

// MustRegisterBuiltins registers all built-in tools and panics on error.
func MustRegisterBuiltins(r *tools.Registry) {
	if err := RegisterBuiltins(r); err != nil {
		panic(err)
	}
}

// NewRegistryWithBuiltins creates a new registry with all built-in tools registered.
func NewRegistryWithBuiltins() *tools.Registry {
	r := tools.NewRegistry()
	MustRegisterBuiltins(r)
	return r
}

// ToolNames returns the names of all built-in tools.
func ToolNames() []string {
	return []string{
		"shell",
		"read_file",
		"write_file",
		"edit_file",
		"list_dir",
		"http",
	}
}
