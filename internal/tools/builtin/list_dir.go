package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mote/internal/tools"
)

// ListDirArgs defines the parameters for the list_dir tool.
type ListDirArgs struct {
	Path      string `json:"path" jsonschema:"description=The directory path to list,required"`
	Recursive bool   `json:"recursive" jsonschema:"description=If true list files recursively"`
	Pattern   string `json:"pattern" jsonschema:"description=Glob pattern to filter files (e.g. *.go)"`
	MaxDepth  int    `json:"max_depth" jsonschema:"description=Maximum depth for recursive listing (default: 10)"`
}

// ListDirTool lists directory contents.
type ListDirTool struct {
	tools.BaseTool
	// MaxEntries is the maximum number of entries to return.
	MaxEntries int
}

// NewListDirTool creates a new list directory tool.
func NewListDirTool() *ListDirTool {
	return &ListDirTool{
		BaseTool: tools.BaseTool{
			ToolName:        "list_dir",
			ToolDescription: "List the contents of a directory. Returns file names, types, sizes, and modification times.",
			ToolParameters:  tools.BuildSchema(ListDirArgs{}),
		},
		MaxEntries: 1000,
	}
}

// Execute lists the directory contents.
func (t *ListDirTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "path is required", nil)
	}

	recursive := false
	if v, ok := args["recursive"].(bool); ok {
		recursive = v
	}

	pattern, _ := args["pattern"].(string)

	maxDepth := 10
	if v, ok := args["max_depth"].(float64); ok && v > 0 {
		maxDepth = int(v)
	}

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	default:
	}

	// Check if path exists and is a directory
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.NewErrorResult(fmt.Sprintf("directory not found: %s", path)), nil
		}
		return tools.NewErrorResult(fmt.Sprintf("failed to stat path: %v", err)), nil
	}

	if !info.IsDir() {
		return tools.NewErrorResult(fmt.Sprintf("path is not a directory: %s", path)), nil
	}

	if recursive {
		return t.listRecursive(ctx, path, pattern, maxDepth)
	}

	return t.listFlat(path, pattern)
}

// listFlat lists a single directory level.
func (t *ListDirTool) listFlat(path, pattern string) (tools.ToolResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to read directory: %v", err)), nil
	}

	var result strings.Builder
	count := 0

	for _, entry := range entries {
		if count >= t.MaxEntries {
			result.WriteString(fmt.Sprintf("\n... (%d more entries)", len(entries)-count))
			break
		}

		name := entry.Name()

		// Apply pattern filter
		if pattern != "" {
			matched, err := filepath.Match(pattern, name)
			if err != nil {
				return tools.NewErrorResult(fmt.Sprintf("invalid pattern: %v", err)), nil
			}
			if !matched {
				continue
			}
		}

		if count > 0 {
			result.WriteString("\n")
		}

		info, err := entry.Info()
		if err != nil {
			result.WriteString(fmt.Sprintf("%s [error getting info]", name))
			count++
			continue
		}

		typeStr := "file"
		if entry.IsDir() {
			typeStr = "dir"
			name += "/"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typeStr = "link"
		}

		result.WriteString(fmt.Sprintf("%s  %s  %d bytes  %s",
			name, typeStr, info.Size(), info.ModTime().Format("2006-01-02 15:04:05")))
		count++
	}

	if count == 0 {
		if pattern != "" {
			return tools.NewSuccessResult(fmt.Sprintf("No files matching pattern '%s' in %s", pattern, path)), nil
		}
		return tools.NewSuccessResult(fmt.Sprintf("Directory is empty: %s", path)), nil
	}

	return tools.NewResultWithMetadata(result.String(), map[string]any{"count": count}), nil
}

// listRecursive lists directory contents recursively.
func (t *ListDirTool) listRecursive(ctx context.Context, root, pattern string, maxDepth int) (tools.ToolResult, error) {
	var result strings.Builder
	count := 0
	baseDepth := strings.Count(root, string(os.PathSeparator))

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		// Check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Skip errors
		}

		if count >= t.MaxEntries {
			return filepath.SkipAll
		}

		// Check depth
		depth := strings.Count(path, string(os.PathSeparator)) - baseDepth
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip root directory
		if path == root {
			return nil
		}

		name := d.Name()

		// Apply pattern filter (only to files, not directories)
		if pattern != "" && !d.IsDir() {
			matched, err := filepath.Match(pattern, name)
			if err != nil {
				return nil
			}
			if !matched {
				return nil
			}
		}

		if count > 0 {
			result.WriteString("\n")
		}

		relPath, _ := filepath.Rel(root, path)
		if d.IsDir() {
			relPath += "/"
		}

		info, err := d.Info()
		if err != nil {
			result.WriteString(fmt.Sprintf("%s [error]", relPath))
			count++
			return nil
		}

		typeStr := "file"
		if d.IsDir() {
			typeStr = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typeStr = "link"
		}

		result.WriteString(fmt.Sprintf("%s  %s  %d bytes", relPath, typeStr, info.Size()))
		count++

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return tools.NewErrorResult(fmt.Sprintf("error walking directory: %v", err)), nil
	}

	if count == 0 {
		return tools.NewSuccessResult(fmt.Sprintf("No entries found in %s", root)), nil
	}

	if count >= t.MaxEntries {
		result.WriteString("\n... (more entries truncated)")
	}

	return tools.NewResultWithMetadata(result.String(), map[string]any{"count": count}), nil
}
