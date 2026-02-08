# Tools Package

This package provides the tool interface and built-in tools for the Mote agent runtime.

## Overview

The tools system allows AI agents to interact with external systems through a well-defined interface. Each tool:
- Has a unique name
- Provides a description for the AI model
- Defines input parameters via JSON Schema
- Implements an `Execute` method

## Core Types

### Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any
    Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}
```

### ToolResult

```go
type ToolResult struct {
    Content  string         // Main output
    IsError  bool           // Whether this is an error result
    Metadata map[string]any // Optional metadata
}
```

## Registry

The `Registry` manages tool registration and execution:

```go
// Create a new registry
registry := tools.NewRegistry()

// Register a tool
registry.Register(myTool)

// Execute a tool
result, err := registry.Execute(ctx, "tool_name", args)

// Get tool list for LLM
providerTools, err := registry.ToProviderTools()
```

## Built-in Tools

### shell
Execute shell commands.

Parameters:
- `command` (required): The command to execute
- `timeout`: Optional timeout in seconds

### read_file
Read file contents.

Parameters:
- `path` (required): File path
- `start_line`: Start line (1-indexed)
- `end_line`: End line (1-indexed)

### write_file
Write content to a file.

Parameters:
- `path` (required): File path
- `content` (required): Content to write
- `append`: Append mode (default: false)

### list_dir
List directory contents.

Parameters:
- `path` (required): Directory path
- `pattern`: Glob pattern filter
- `recursive`: Recursive listing

### http
Make HTTP requests.

Parameters:
- `url` (required): Request URL
- `method`: HTTP method (default: GET)
- `headers`: Request headers
- `body`: Request body

## Creating Custom Tools

```go
type MyTool struct {
    tools.BaseTool
}

func NewMyTool() *MyTool {
    return &MyTool{
        BaseTool: tools.BaseTool{
            ToolName:        "my_tool",
            ToolDescription: "Does something useful",
            ToolParameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "input": map[string]any{
                        "type":        "string",
                        "description": "Input value",
                    },
                },
                "required": []string{"input"},
            },
        },
    }
}

func (t *MyTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
    input := args["input"].(string)
    // ... do something
    return tools.NewSuccessResult("output"), nil
}
```

## Error Handling

The package defines several error types:
- `ErrToolNotFound`: Tool not registered
- `ErrToolAlreadyExists`: Duplicate registration
- `ErrInvalidArgs`: Invalid arguments
- `ErrToolTimeout`: Execution timeout
