package builtin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mote/internal/tools"
)

// EditFileArgs defines the parameters for the edit_file tool.
type EditFileArgs struct {
	Path    string `json:"path" jsonschema:"description=The file path to edit,required"`
	OldText string `json:"old_text" jsonschema:"description=The exact text to find and replace (must match exactly including whitespace),required"`
	NewText string `json:"new_text" jsonschema:"description=The text to replace old_text with,required"`
}

// EditFileTool edits existing files by replacing text.
type EditFileTool struct {
	tools.BaseTool
}

// NewEditFileTool creates a new edit file tool.
func NewEditFileTool() *EditFileTool {
	return &EditFileTool{
		BaseTool: tools.BaseTool{
			ToolName:        "edit_file",
			ToolDescription: "Edit an existing file by replacing specific text. The old_text must match exactly (including whitespace and indentation). Use this for precise edits instead of rewriting entire files.",
			ToolParameters:  tools.BuildSchema(EditFileArgs{}),
		},
	}
}

// Execute edits the file by replacing old_text with new_text.
func (t *EditFileTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "path is required", nil)
	}

	oldText, _ := args["old_text"].(string)
	if oldText == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "old_text is required", nil)
	}

	newText, _ := args["new_text"].(string)
	// newText can be empty (for deletion)

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	default:
	}

	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.NewErrorResult(fmt.Sprintf("file not found: %s", path)), nil
		}
		return tools.NewErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	contentStr := string(content)

	// Check if old_text exists in file
	count := strings.Count(contentStr, oldText)
	if count == 0 {
		// Provide helpful error with snippet
		preview := contentStr
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return tools.NewErrorResult(fmt.Sprintf(
			"old_text not found in file. File starts with:\n%s\n\nMake sure old_text matches exactly including whitespace.",
			preview,
		)), nil
	}

	if count > 1 {
		return tools.NewErrorResult(fmt.Sprintf(
			"old_text matches %d locations in file. Please provide more context to make the match unique.",
			count,
		)), nil
	}

	// Replace the text
	newContent := strings.Replace(contentStr, oldText, newText, 1)

	// Write back
	info, err := os.Stat(path)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to stat file: %v", err)), nil
	}

	err = os.WriteFile(path, []byte(newContent), info.Mode())
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return tools.NewResultWithMetadata(
		fmt.Sprintf("Successfully edited %s: replaced %d characters with %d characters",
			path, len(oldText), len(newText)),
		map[string]any{
			"path":         path,
			"old_length":   len(oldText),
			"new_length":   len(newText),
			"delta":        len(newText) - len(oldText),
			"total_length": len(newContent),
		},
	), nil
}
