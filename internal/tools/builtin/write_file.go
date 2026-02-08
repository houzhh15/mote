package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"mote/internal/tools"
)

// WriteFileArgs defines the parameters for the write_file tool.
type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"description=The file path to write to,required"`
	Content string `json:"content" jsonschema:"description=The content to write to the file,required"`
	Append  bool   `json:"append" jsonschema:"description=If true append to existing file instead of overwriting"`
	Mode    int    `json:"mode" jsonschema:"description=File permission mode (default: 0644)"`
}

// WriteFileTool writes files to disk.
type WriteFileTool struct {
	tools.BaseTool
}

// NewWriteFileTool creates a new write file tool.
func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{
		BaseTool: tools.BaseTool{
			ToolName:        "write_file",
			ToolDescription: "Write content to a file. Creates the file if it doesn't exist, and creates parent directories as needed.",
			ToolParameters:  tools.BuildSchema(WriteFileArgs{}),
		},
	}
}

// Execute writes to the file.
func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "path is required", nil)
	}

	content, _ := args["content"].(string)
	// Content can be empty, that's valid

	appendMode := false
	if v, ok := args["append"].(bool); ok {
		appendMode = v
	}

	mode := os.FileMode(0644)
	if v, ok := args["mode"].(float64); ok && v > 0 {
		mode = os.FileMode(int(v))
	}

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	default:
	}

	// Create parent directories
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
		}
	}

	if appendMode {
		return t.appendToFile(path, content, mode)
	}

	return t.writeFile(path, content, mode)
}

// writeFile writes content to a file, overwriting existing content.
func (t *WriteFileTool) writeFile(path, content string, mode os.FileMode) (tools.ToolResult, error) {
	err := os.WriteFile(path, []byte(content), mode)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return tools.NewResultWithMetadata(
		fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path),
		map[string]any{
			"path":  path,
			"bytes": len(content),
		},
	), nil
}

// appendToFile appends content to a file.
func (t *WriteFileTool) appendToFile(path, content string, mode os.FileMode) (tools.ToolResult, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to open file for append: %v", err)), nil
	}
	defer file.Close()

	n, err := file.WriteString(content)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to append to file: %v", err)), nil
	}

	return tools.NewResultWithMetadata(
		fmt.Sprintf("Successfully appended %d bytes to %s", n, path),
		map[string]any{
			"path":  path,
			"bytes": n,
		},
	), nil
}
