# Runner Package

This package provides the core agent execution loop for the Mote agent runtime.

## Overview

The runner package orchestrates:
- LLM provider interaction (streaming and non-streaming)
- Tool execution and result handling
- Conversation history management
- System prompt generation

## Runner

The main execution engine:

```go
// Create runner dependencies
provider := copilot.NewProvider(auth)
registry := tools.NewRegistryWithBuiltins()
sessions := scheduler.NewSessionManager(db, 100)
config := runner.DefaultConfig()

// Create runner
r := runner.NewRunner(provider, registry, sessions, config)

// Start a run
events, err := r.Run(ctx, sessionID, "Hello, what can you do?")
if err != nil {
    log.Fatal(err)
}

// Process events
for event := range events {
    switch event.Type {
    case runner.EventTypeContent:
        fmt.Print(event.Content)
    case runner.EventTypeToolCall:
        fmt.Printf("\nCalling tool: %s\n", event.ToolCall.ID)
    case runner.EventTypeToolResult:
        fmt.Printf("Tool result: %s\n", event.ToolResult.Output)
    case runner.EventTypeDone:
        fmt.Printf("\nDone! Tokens: %d\n", event.Usage.TotalTokens)
    case runner.EventTypeError:
        fmt.Printf("Error: %s\n", event.ErrorMsg)
    }
}
```

## Configuration

```go
config := runner.DefaultConfig().
    WithMaxIterations(10).    // Max tool call loops
    WithMaxTokens(8000).      // Max output tokens
    WithMaxMessages(100).     // Max history messages
    WithTimeout(5*time.Minute).
    WithStreamOutput(true).
    WithTemperature(0.7).
    WithSystemPrompt("Custom system prompt")
```

### Default Values

| Option | Default | Description |
|--------|---------|-------------|
| MaxIterations | 10 | Maximum tool call iterations |
| MaxTokens | 8000 | Maximum output tokens |
| MaxMessages | 100 | Maximum history messages |
| Timeout | 5 minutes | Run timeout |
| StreamOutput | true | Enable streaming |
| Temperature | 0.7 | Model temperature |

## Events

Events are emitted during execution:

### EventTypeContent
Text content from the model.
```go
event.Content // string
```

### EventTypeToolCall
Model requests a tool execution.
```go
event.ToolCall.ID        // string
event.ToolCall.Name      // string (via Function)
event.ToolCall.Arguments // string (JSON)
```

### EventTypeToolResult
Tool execution completed.
```go
event.ToolResult.ToolCallID // string
event.ToolResult.ToolName   // string
event.ToolResult.Output     // string
event.ToolResult.IsError    // bool
event.ToolResult.DurationMs // int64
```

### EventTypeDone
Run completed successfully.
```go
event.Usage.PromptTokens     // int
event.Usage.CompletionTokens // int
event.Usage.TotalTokens      // int
```

### EventTypeError
An error occurred.
```go
event.Error    // error
event.ErrorMsg // string
```

## HistoryManager

Manages conversation history with compression:

```go
hm := runner.NewHistoryManager(maxMessages, maxTokens)

// Estimate tokens
tokens := hm.EstimateTokens("Hello world")

// Check if compression needed
if hm.ShouldCompress(messages) {
    compressed, _ := hm.Compress(messages)
}
```

Compression preserves:
- All system messages
- Most recent conversation messages
- Respects token and message limits

## PromptBuilder

Generates system prompts with tool information:

```go
pb := runner.NewPromptBuilder(registry)
pb.SetBasePrompt("You are a helpful assistant.")
prompt := pb.Build() // Includes tool descriptions
```

## Error Types

- `ErrMaxIterations`: Reached iteration limit
- `ErrContextCanceled`: Context was cancelled
- `ErrNoProvider`: No LLM provider configured
- `ErrNoMessages`: Empty message list
- `ErrSessionNotFound`: Session doesn't exist
