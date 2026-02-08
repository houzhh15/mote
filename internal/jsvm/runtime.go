package jsvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"

	"mote/internal/jsvmerr"
	"mote/internal/storage"
)

// RuntimeConfig holds configuration for the Runtime.
type RuntimeConfig struct {
	// Pool configuration
	PoolConfig PoolConfig
	// Sandbox configuration
	SandboxConfig SandboxConfig
}

// DefaultRuntimeConfig returns default runtime configuration.
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		PoolConfig:    DefaultPoolConfig(),
		SandboxConfig: DefaultSandboxConfig(),
	}
}

// Runtime provides JavaScript execution capabilities.
type Runtime struct {
	pool   *VMPool
	config RuntimeConfig
	db     *storage.DB
	logger zerolog.Logger
	closed bool
}

// NewRuntime creates a new JavaScript runtime.
func NewRuntime(cfg RuntimeConfig, db *storage.DB, logger zerolog.Logger) *Runtime {
	return &Runtime{
		pool:   NewVMPool(cfg.PoolConfig),
		config: cfg,
		db:     db,
		logger: logger,
	}
}

// ExecuteResult holds the result of script execution.
type ExecuteResult struct {
	// Value is the return value of the script.
	Value interface{}
	// Logs contains any log messages captured during execution.
	Logs []string
}

// Execute runs a JavaScript script and returns the result.
func (r *Runtime) Execute(ctx context.Context, script, scriptName, executionID string) (*ExecuteResult, error) {
	if r.closed {
		return nil, fmt.Errorf("runtime is closed")
	}

	// Acquire VM from pool
	vm, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer r.pool.Release(vm)

	// Setup sandbox
	sandbox := NewSandbox(r.config.SandboxConfig, r.db, r.logger)
	execCtx, err := sandbox.Setup(vm, ctx, scriptName, executionID)
	if err != nil {
		return nil, err
	}
	defer sandbox.Cleanup(vm)

	// Execute script
	val, err := vm.RunString(script)
	if err != nil {
		return nil, wrapExecutionError(err, scriptName)
	}

	// Check context cancellation
	select {
	case <-execCtx.Done():
		return nil, &jsvmerr.ExecutionError{
			Script: scriptName,
			Cause:  execCtx.Err(),
		}
	default:
	}

	return &ExecuteResult{
		Value: exportValue(val),
	}, nil
}

// ExecuteFile reads a file and executes its contents.
func (r *Runtime) ExecuteFile(ctx context.Context, filePath, executionID string) (*ExecuteResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}

	scriptName := filepath.Base(filePath)
	return r.Execute(ctx, string(content), scriptName, executionID)
}

// Close shuts down the runtime and releases resources.
func (r *Runtime) Close() error {
	r.closed = true
	return r.pool.Close()
}

// wrapExecutionError converts goja errors to structured errors.
func wrapExecutionError(err error, scriptName string) error {
	// Check for interrupt
	if interrupted, ok := err.(*goja.InterruptedError); ok {
		return &jsvmerr.ExecutionError{
			Script: scriptName,
			Cause:  fmt.Errorf("interrupted: %v", interrupted.Value()),
		}
	}

	// Check for syntax error
	if exception, ok := err.(*goja.Exception); ok {
		return &jsvmerr.ExecutionError{
			Script: scriptName,
			Cause:  fmt.Errorf("exception: %s", exception.String()),
		}
	}

	// Check for compile error
	if compileErr, ok := err.(*goja.CompilerSyntaxError); ok {
		return &jsvmerr.ScriptSyntaxError{
			File:    scriptName,
			Message: compileErr.Error(),
		}
	}

	return &jsvmerr.ExecutionError{
		Script: scriptName,
		Cause:  err,
	}
}

// exportValue converts goja values to Go values.
func exportValue(val goja.Value) interface{} {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	return val.Export()
}
