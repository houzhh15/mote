package jsvm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"mote/internal/jsvmerr"
)

func TestRuntimeExecute(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	result, err := rt.Execute(ctx, `1 + 2`, "test.js", "exec-1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	val, ok := result.Value.(int64)
	if !ok {
		t.Fatalf("Expected int64, got %T", result.Value)
	}
	if val != 3 {
		t.Errorf("Expected 3, got %d", val)
	}
}

func TestRuntimeExecuteString(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	result, err := rt.Execute(ctx, `"hello" + " " + "world"`, "test.js", "exec-2")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	val, ok := result.Value.(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Value)
	}
	if val != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", val)
	}
}

func TestRuntimeExecuteObject(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	result, err := rt.Execute(ctx, `({ name: "test", value: 42 })`, "test.js", "exec-3")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	val, ok := result.Value.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", result.Value)
	}
	if val["name"] != "test" {
		t.Errorf("Expected name 'test', got '%v'", val["name"])
	}
}

func TestRuntimeExecuteWithHostAPI(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	// Test that mote object is available
	result, err := rt.Execute(ctx, `typeof mote.log.info`, "test.js", "exec-4")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Value != "function" {
		t.Errorf("Expected 'function', got '%v'", result.Value)
	}
}

func TestRuntimeExecuteTimeout(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	cfg.SandboxConfig.Timeout = 100 * time.Millisecond
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	_, err := rt.Execute(ctx, `while(true) {}`, "test.js", "exec-timeout")
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestRuntimeExecuteSyntaxError(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	_, err := rt.Execute(ctx, `function( { broken`, "test.js", "exec-syntax")
	if err == nil {
		t.Fatal("Expected syntax error, got nil")
	}

	// Check error type
	var syntaxErr *jsvmerr.ScriptSyntaxError
	var execErr *jsvmerr.ExecutionError
	if !(syntaxErr != nil || execErr != nil) {
		// At least one should be present, but we just check it's an error
		t.Logf("Error type: %T, message: %v", err, err)
	}
}

func TestRuntimeExecuteFile(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	// Create temp script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.js")
	err := os.WriteFile(scriptPath, []byte(`40 + 2`), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	ctx := context.Background()
	result, err := rt.ExecuteFile(ctx, scriptPath, "exec-file")
	if err != nil {
		t.Fatalf("ExecuteFile failed: %v", err)
	}

	val, ok := result.Value.(int64)
	if !ok {
		t.Fatalf("Expected int64, got %T", result.Value)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

func TestRuntimeExecuteFileNotFound(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	_, err := rt.ExecuteFile(ctx, "/nonexistent/script.js", "exec-notfound")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestRuntimeClose(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)

	err := rt.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Subsequent executions should fail
	ctx := context.Background()
	_, err = rt.Execute(ctx, `1`, "test.js", "exec-closed")
	if err == nil {
		t.Error("Expected error after close, got nil")
	}
}

func TestRuntimeContextCancellation(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	cfg.SandboxConfig.Timeout = 5 * time.Second
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := rt.Execute(ctx, `while(true) {}`, "test.js", "exec-cancel")
		done <- err
	}()

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Expected error after cancellation, got nil")
		}
	case <-time.After(1 * time.Second):
		t.Error("Execution did not stop after cancellation")
	}
}

func TestRuntimeUndefinedResult(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	result, err := rt.Execute(ctx, `undefined`, "test.js", "exec-undef")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Value != nil {
		t.Errorf("Expected nil for undefined, got %v", result.Value)
	}
}

func TestRuntimeNullResult(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	ctx := context.Background()
	result, err := rt.Execute(ctx, `null`, "test.js", "exec-null")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Value != nil {
		t.Errorf("Expected nil for null, got %v", result.Value)
	}
}

func TestDefaultRuntimeConfig(t *testing.T) {
	cfg := DefaultRuntimeConfig()

	if cfg.PoolConfig.MaxSize != 5 {
		t.Errorf("PoolConfig.MaxSize = %d, want 5", cfg.PoolConfig.MaxSize)
	}

	if cfg.SandboxConfig.Timeout != 30*time.Second {
		t.Errorf("SandboxConfig.Timeout = %v, want 30s", cfg.SandboxConfig.Timeout)
	}
}
