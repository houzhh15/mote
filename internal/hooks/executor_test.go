package hooks

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestExecutor_Execute_EmptyHandlers(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	result := e.Execute(ctx, nil, hookCtx)
	if !result.Continue {
		t.Error("expected Continue=true for empty handlers")
	}
}

func TestExecutor_Execute_SingleHandler(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	called := false
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			called = true
			return ContinueResult(), nil
		},
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)
	if !called {
		t.Error("handler was not called")
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestExecutor_Execute_ChainInterruption(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handler1Called := false
	handler2Called := false

	handlers := []*Handler{
		{
			ID:       "first",
			Priority: 100,
			Enabled:  true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				handler1Called = true
				return StopResult(), nil
			},
		},
		{
			ID:       "second",
			Priority: 50,
			Enabled:  true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				handler2Called = true
				return ContinueResult(), nil
			},
		},
	}

	result := e.Execute(ctx, handlers, hookCtx)

	if !handler1Called {
		t.Error("first handler should have been called")
	}
	if handler2Called {
		t.Error("second handler should not have been called after chain interruption")
	}
	if result.Continue {
		t.Error("expected Continue=false after chain interruption")
	}
}

func TestExecutor_Execute_ModifiedData(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handlers := []*Handler{
		{
			ID:      "modifier1",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				return ModifiedResult(map[string]any{"key1": "value1"}), nil
			},
		},
		{
			ID:      "modifier2",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				return ModifiedResult(map[string]any{"key2": "value2"}), nil
			},
		},
	}

	result := e.Execute(ctx, handlers, hookCtx)

	if !result.Modified {
		t.Error("expected Modified=true")
	}
	if result.Data["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %v", result.Data["key1"])
	}
	if result.Data["key2"] != "value2" {
		t.Errorf("expected key2=value2, got %v", result.Data["key2"])
	}
}

func TestExecutor_Execute_DisabledHandler(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	called := false
	handler := &Handler{
		ID:      "disabled",
		Enabled: false,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			called = true
			return ContinueResult(), nil
		},
	}

	e.Execute(ctx, []*Handler{handler}, hookCtx)
	if called {
		t.Error("disabled handler should not have been called")
	}
}

func TestExecutor_Execute_PanicRecovery(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handler2Called := false
	handlers := []*Handler{
		{
			ID:      "panicker",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				panic("test panic")
			},
		},
		{
			ID:      "after-panic",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				handler2Called = true
				return ContinueResult(), nil
			},
		},
	}

	// Should not panic
	result := e.Execute(ctx, handlers, hookCtx)

	if !handler2Called {
		t.Error("handler after panic should have been called")
	}
	if !result.Continue {
		t.Error("expected Continue=true after panic recovery")
	}
}

func TestExecutor_Execute_HandlerError(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	testErr := errors.New("test error")
	handler2Called := false

	handlers := []*Handler{
		{
			ID:      "error-handler",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				return ContinueResult(), testErr
			},
		},
		{
			ID:      "after-error",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				handler2Called = true
				return ContinueResult(), nil
			},
		},
	}

	result := e.Execute(ctx, handlers, hookCtx)

	if !handler2Called {
		t.Error("handler after error should have been called")
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestExecutor_Execute_NilHandlerFunc(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handler := &Handler{
		ID:      "nil-func",
		Enabled: true,
		Handler: nil, // nil handler function
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)
	if !result.Continue {
		t.Error("expected Continue=true for nil handler function")
	}
}

func TestExecutor_DataPropagation(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handlers := []*Handler{
		{
			ID:      "setter",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				return ModifiedResult(map[string]any{"shared": "data"}), nil
			},
		},
		{
			ID:      "reader",
			Enabled: true,
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				val, ok := hookCtx.GetData("shared")
				if !ok || val != "data" {
					return ErrorResult(errors.New("data not propagated")), nil
				}
				return ContinueResult(), nil
			},
		},
	}

	result := e.Execute(ctx, handlers, hookCtx)
	if !result.Continue {
		t.Error("expected data to propagate between handlers")
	}
}

func TestExecuteFunc(t *testing.T) {
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	called := false
	fn := func(ctx context.Context, hookCtx *Context) (*Result, error) {
		called = true
		return ContinueResult(), nil
	}

	result, err := ExecuteFunc(ctx, fn, hookCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestExecuteFunc_Nil(t *testing.T) {
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	result, err := ExecuteFunc(ctx, nil, hookCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for nil function")
	}
}

// mockJSExecutor is a mock implementation of JSExecutor for testing.
type mockJSExecutor struct {
	result     *JSExecuteResult
	err        error
	callCount  int
	lastScript string
}

func (m *mockJSExecutor) Execute(ctx context.Context, script, scriptName, executionID string) (*JSExecuteResult, error) {
	m.callCount++
	m.lastScript = script
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestExecutor_SetJSRuntime(t *testing.T) {
	e := NewExecutor()
	mock := &mockJSExecutor{}

	e.SetJSRuntime(mock)

	if e.jsRuntime != mock {
		t.Error("expected jsRuntime to be set")
	}
}

func TestExecutor_WithJSExecutor(t *testing.T) {
	mock := &mockJSExecutor{}
	e := NewExecutorWithOptions(WithJSExecutor(mock))

	if e.jsRuntime != mock {
		t.Error("expected jsRuntime to be set via option")
	}
}

func TestExecutor_ScriptHandler_NoRuntime(t *testing.T) {
	e := NewExecutor()
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	handler := &Handler{
		ID:         "script-handler",
		Enabled:    true,
		ScriptPath: "/tmp/test.js",
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)

	// Should continue (error is logged but chain continues)
	if !result.Continue {
		t.Error("expected chain to continue even when JS runtime is missing")
	}
}

func TestExecutor_ScriptHandler_Success(t *testing.T) {
	mock := &mockJSExecutor{
		result: &JSExecuteResult{
			Value: `{"continue": true, "modified": true, "data": {"key": "value"}}`,
		},
	}
	e := NewExecutorWithOptions(WithJSExecutor(mock))
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	// Create temp script file
	tmpFile := t.TempDir() + "/test.js"
	if err := writeTestFile(tmpFile, "function handler(ctx) { return { continue: true }; }"); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	handler := &Handler{
		ID:         "script-handler",
		Enabled:    true,
		ScriptPath: tmpFile,
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)

	if mock.callCount != 1 {
		t.Errorf("expected JS executor to be called once, got %d", mock.callCount)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
	if !result.Modified {
		t.Error("expected Modified=true")
	}
	if result.Data["key"] != "value" {
		t.Errorf("expected data key='value', got %v", result.Data["key"])
	}
}

func TestExecutor_ScriptHandler_StopChain(t *testing.T) {
	mock := &mockJSExecutor{
		result: &JSExecuteResult{
			Value: `{"continue": false, "modified": false}`,
		},
	}
	e := NewExecutorWithOptions(WithJSExecutor(mock))
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	tmpFile := t.TempDir() + "/test.js"
	if err := writeTestFile(tmpFile, "function handler(ctx) { return { continue: false }; }"); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	handler := &Handler{
		ID:         "script-handler",
		Enabled:    true,
		ScriptPath: tmpFile,
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)

	if result.Continue {
		t.Error("expected Continue=false when script returns false")
	}
}

func TestExecutor_ScriptHandler_Error(t *testing.T) {
	mock := &mockJSExecutor{
		err: errors.New("script error"),
	}
	e := NewExecutorWithOptions(WithJSExecutor(mock))
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	tmpFile := t.TempDir() + "/test.js"
	if err := writeTestFile(tmpFile, "broken script"); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	handler := &Handler{
		ID:         "script-handler",
		Enabled:    true,
		ScriptPath: tmpFile,
	}

	result := e.Execute(ctx, []*Handler{handler}, hookCtx)

	// Chain should continue after error
	if !result.Continue {
		t.Error("expected chain to continue after script error")
	}
}

func TestExecutor_ParseScriptResult_NilResult(t *testing.T) {
	e := NewExecutor()
	result, err := e.parseScriptResult(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for nil result")
	}
}

func TestExecutor_ParseScriptResult_InvalidJSON(t *testing.T) {
	e := NewExecutor()
	result, err := e.parseScriptResult(&JSExecuteResult{Value: "not json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for invalid JSON")
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
