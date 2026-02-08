package builtin

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"mote/internal/tools"
)

// ReadFileArgs defines the parameters for the read_file tool.
type ReadFileArgs struct {
	Path      string `json:"path" jsonschema:"description=The file path to read,required"`
	StartLine int    `json:"start_line" jsonschema:"description=Start line number (1-based). If not specified starts from beginning"`
	EndLine   int    `json:"end_line" jsonschema:"description=End line number (1-based inclusive). If not specified reads to end"`
}

// ReadFileTool reads files from disk.
type ReadFileTool struct {
	tools.BaseTool
	// MaxFileSize is the maximum file size to read in bytes.
	MaxFileSize int64
}

// NewReadFileTool creates a new read file tool.
func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{
		BaseTool: tools.BaseTool{
			ToolName:        "read_file",
			ToolDescription: "Read the contents of a file from disk. Supports reading specific line ranges for large files.",
			ToolParameters:  tools.BuildSchema(ReadFileArgs{}),
		},
		MaxFileSize: 10 * 1024 * 1024, // 10MB default
	}
}

// Execute reads the file.
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return tools.ToolResult{}, tools.NewInvalidArgsError(t.Name(), "path is required", nil)
	}

	startLine := 0
	if v, ok := args["start_line"].(float64); ok && v > 0 {
		startLine = int(v)
	}

	endLine := 0
	if v, ok := args["end_line"].(float64); ok && v > 0 {
		endLine = int(v)
	}

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	default:
	}

	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.NewErrorResult(fmt.Sprintf("file not found: %s", path)), nil
		}
		return tools.NewErrorResult(fmt.Sprintf("failed to stat file: %v", err)), nil
	}

	if info.IsDir() {
		return tools.NewErrorResult(fmt.Sprintf("path is a directory: %s", path)), nil
	}

	// Check file size
	var warning string
	if info.Size() > t.MaxFileSize {
		warning = fmt.Sprintf("Warning: file is large (%d bytes). Consider using line ranges.\n\n", info.Size())
	}

	// If line range specified, read line by line
	if startLine > 0 || endLine > 0 {
		return t.readLines(path, startLine, endLine, warning)
	}

	// Read entire file
	data, err := os.ReadFile(path)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	content := string(data)
	if len(content) > int(t.MaxFileSize) {
		content = content[:t.MaxFileSize] + "\n... (content truncated)"
	}

	return tools.NewSuccessResult(warning + content), nil
}

// readLines reads specific lines from a file.
func (t *ReadFileTool) readLines(path string, startLine, endLine int, warning string) (tools.ToolResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to open file: %v", err)), nil
	}
	defer file.Close()

	var result strings.Builder
	if warning != "" {
		result.WriteString(warning)
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++

		if startLine > 0 && lineNum < startLine {
			continue
		}
		if endLine > 0 && lineNum > endLine {
			break
		}

		if linesRead > 0 {
			result.WriteString("\n")
		}
		result.WriteString(scanner.Text())
		linesRead++

		// Check size limit
		if result.Len() > int(t.MaxFileSize) {
			result.WriteString("\n... (content truncated)")
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("error reading file: %v", err)), nil
	}

	if linesRead == 0 {
		if startLine > lineNum {
			return tools.NewErrorResult(fmt.Sprintf("start_line %d exceeds file length (%d lines)", startLine, lineNum)), nil
		}
		return tools.NewSuccessResult("(empty result)"), nil
	}

	metadata := map[string]any{
		"lines_read": linesRead,
		"start_line": startLine,
		"end_line":   endLine,
	}

	return tools.NewResultWithMetadata(result.String(), metadata), nil
}
