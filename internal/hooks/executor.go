package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
)

// JSExecutor is the interface for executing JavaScript code in hooks.
type JSExecutor interface {
	Execute(ctx context.Context, script, scriptName, executionID string) (*JSExecuteResult, error)
}

// JSExecuteResult holds the result of JS script execution.
type JSExecuteResult struct {
	Value interface{}
	Logs  []string
}

// Executor executes hook handlers in sequence.
type Executor struct {
	// recoverPanic determines whether to recover from handler panics.
	recoverPanic bool
	// jsRuntime is the JavaScript runtime for executing script handlers.
	jsRuntime JSExecutor
}

// NewExecutor creates a new hook executor.
func NewExecutor() *Executor {
	return &Executor{
		recoverPanic: true,
	}
}

// ExecutorOption configures the executor.
type ExecutorOption func(*Executor)

// WithPanicRecovery sets whether the executor should recover from panics.
func WithPanicRecovery(recover bool) ExecutorOption {
	return func(e *Executor) {
		e.recoverPanic = recover
	}
}

// WithJSExecutor sets the JavaScript executor for script handlers.
func WithJSExecutor(js JSExecutor) ExecutorOption {
	return func(e *Executor) {
		e.jsRuntime = js
	}
}

// SetJSRuntime sets the JavaScript runtime for executing script handlers.
func (e *Executor) SetJSRuntime(js JSExecutor) {
	e.jsRuntime = js
}

// NewExecutorWithOptions creates a new hook executor with options.
func NewExecutorWithOptions(opts ...ExecutorOption) *Executor {
	e := NewExecutor()
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute runs handlers in sequence, respecting priority order.
// If a handler returns Continue=false, execution stops.
// Modified data from handlers is merged into the final result.
func (e *Executor) Execute(ctx context.Context, handlers []*Handler, hookCtx *Context) *Result {
	if len(handlers) == 0 {
		return ContinueResult()
	}

	finalResult := &Result{
		Continue: true,
		Modified: false,
		Data:     make(map[string]any),
	}

	for _, handler := range handlers {
		// Skip disabled handlers
		if !handler.Enabled {
			continue
		}

		// Execute handler with panic recovery
		result, err := e.executeHandler(ctx, handler, hookCtx)
		if err != nil {
			log.Error().
				Err(err).
				Str("handler_id", handler.ID).
				Str("hook_type", string(hookCtx.Type)).
				Msg("handler execution error")

			// Continue to next handler on error (unless it's a panic)
			if result != nil && !result.Continue {
				finalResult.Continue = false
				finalResult.Error = err
				break
			}
			continue
		}

		// Merge modified data
		if result != nil && result.Modified && result.Data != nil {
			finalResult.Modified = true
			for k, v := range result.Data {
				finalResult.Data[k] = v
			}
			// Also update hook context for subsequent handlers
			for k, v := range result.Data {
				hookCtx.SetData(k, v)
			}
		}

		// Check if chain should be interrupted
		if result != nil && !result.Continue {
			finalResult.Continue = false
			log.Debug().
				Str("handler_id", handler.ID).
				Str("hook_type", string(hookCtx.Type)).
				Msg("hook chain interrupted by handler")
			break
		}
	}

	return finalResult
}

// executeHandler executes a single handler with optional panic recovery.
func (e *Executor) executeHandler(ctx context.Context, handler *Handler, hookCtx *Context) (result *Result, err error) {
	if e.recoverPanic {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Str("handler_id", handler.ID).
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Msg("handler panicked")
				err = fmt.Errorf("%w: %v", ErrHandlerPanic, r)
				result = ContinueResult() // Allow chain to continue after panic
			}
		}()
	}

	// Check if this is a script handler
	if handler.ScriptPath != "" {
		return e.executeScriptHandler(ctx, handler, hookCtx)
	}

	if handler.Handler == nil {
		return ContinueResult(), nil
	}

	return handler.Handler(ctx, hookCtx)
}

// executeScriptHandler executes a JavaScript script handler.
func (e *Executor) executeScriptHandler(ctx context.Context, handler *Handler, hookCtx *Context) (*Result, error) {
	if e.jsRuntime == nil {
		return nil, fmt.Errorf("JavaScript runtime not configured")
	}

	// Read script file
	scriptContent, err := os.ReadFile(handler.ScriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}

	// Serialize hook context to JSON for passing to script
	ctxJSON, err := json.Marshal(hookCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize hook context: %w", err)
	}

	// Wrap script with context injection and result extraction
	wrappedScript := fmt.Sprintf(`
(function() {
	const hookContext = %s;
	
	// Execute the handler script
	%s
	
	// If there's a handler function, call it
	if (typeof handler === 'function') {
		const result = handler(hookContext);
		return JSON.stringify(result || { continue: true });
	}
	
	return JSON.stringify({ continue: true });
})();
`, string(ctxJSON), string(scriptContent))

	// Generate execution ID
	execID := fmt.Sprintf("%s-%d", handler.ID, time.Now().UnixNano())

	// Execute script
	jsResult, err := e.jsRuntime.Execute(ctx, wrappedScript, handler.ScriptPath, execID)
	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w", err)
	}

	// Parse result
	return e.parseScriptResult(jsResult)
}

// parseScriptResult parses the JavaScript execution result into a hook Result.
func (e *Executor) parseScriptResult(jsResult *JSExecuteResult) (*Result, error) {
	if jsResult == nil || jsResult.Value == nil {
		return ContinueResult(), nil
	}

	// Try to parse as JSON string
	resultStr, ok := jsResult.Value.(string)
	if !ok {
		// Not a string, return continue result
		return ContinueResult(), nil
	}

	var parsed struct {
		Continue bool           `json:"continue"`
		Modified bool           `json:"modified"`
		Data     map[string]any `json:"data"`
	}

	if err := json.Unmarshal([]byte(resultStr), &parsed); err != nil {
		// Failed to parse, return continue result
		return ContinueResult(), nil
	}

	return &Result{
		Continue: parsed.Continue,
		Modified: parsed.Modified,
		Data:     parsed.Data,
	}, nil
}

// ExecuteFunc is a convenience function to execute a single handler function.
func ExecuteFunc(ctx context.Context, fn HandlerFunc, hookCtx *Context) (*Result, error) {
	if fn == nil {
		return ContinueResult(), nil
	}
	return fn(ctx, hookCtx)
}
