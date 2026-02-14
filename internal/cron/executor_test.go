package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockRunner implements Runner for testing.
type mockRunner struct {
	result  string
	err     error
	calls   int
	runFunc func(ctx context.Context, prompt string) (string, error)
}

func (r *mockRunner) Run(ctx context.Context, prompt string, opts ...interface{}) (string, error) {
	r.calls++
	if r.runFunc != nil {
		return r.runFunc(ctx, prompt)
	}
	return r.result, r.err
}

// mockToolRegistry implements ToolRegistry for testing.
type mockToolRegistry struct {
	result interface{}
	err    error
	calls  int
}

func (r *mockToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	r.calls++
	return r.result, r.err
}

// mockJSExecutor implements JSExecutor for testing.
type mockJSExecutor struct {
	result interface{}
	err    error
	calls  int
}

func (e *mockJSExecutor) Execute(ctx context.Context, script, scriptName, executionID string) (interface{}, error) {
	e.calls++
	return e.result, e.err
}

func (e *mockJSExecutor) ExecuteFile(ctx context.Context, filePath, executionID string) (interface{}, error) {
	e.calls++
	return e.result, e.err
}

func TestExecutorExecutePrompt(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	runner := &mockRunner{result: "Hello, world!"}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(runner, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-prompt",
		Type:    JobTypePrompt,
		Payload: `{"prompt": "Say hello"}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if result.Result != "Hello, world!" {
		t.Errorf("Result = %s, want 'Hello, world!'", result.Result)
	}
	if runner.calls != 1 {
		t.Errorf("Runner called %d times, want 1", runner.calls)
	}
}

func TestExecutorExecuteTool(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	toolRegistry := &mockToolRegistry{result: map[string]string{"message": "done"}}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, toolRegistry, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-tool",
		Type:    JobTypeTool,
		Payload: `{"tool": "echo", "args": {"input": "hello"}}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if toolRegistry.calls != 1 {
		t.Errorf("ToolRegistry called %d times, want 1", toolRegistry.calls)
	}
}

func TestExecutorExecuteScript(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	jsExecutor := &mockJSExecutor{result: 42}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, nil, jsExecutor, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-script",
		Type:    JobTypeScript,
		Payload: `{"script": "1 + 1"}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if jsExecutor.calls != 1 {
		t.Errorf("JSExecutor called %d times, want 1", jsExecutor.calls)
	}
}

func TestExecutorExecuteScriptFile(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	jsExecutor := &mockJSExecutor{result: "file result"}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, nil, jsExecutor, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-script-file",
		Type:    JobTypeScript,
		Payload: `{"script_path": "/path/to/script.js"}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if jsExecutor.calls != 1 {
		t.Errorf("JSExecutor called %d times, want 1", jsExecutor.calls)
	}
}

func TestExecutorRetryOnError(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)

	callCount := 0
	runner := &mockRunner{
		runFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			if callCount < 3 {
				return "", errors.New("temporary error")
			}
			return "success", nil
		},
	}

	logger := zerolog.Nop()
	cfg := ExecutorConfig{
		Timeout: 5 * time.Minute,
		RetryPolicy: RetryPolicy{
			MaxAttempts:  3,
			InitialDelay: 1 * time.Millisecond, // Fast for tests
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		},
	}
	executor := NewExecutor(runner, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-retry",
		Type:    JobTypePrompt,
		Payload: `{"prompt": "test"}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}
	if result.Retries != 2 {
		t.Errorf("Retries = %d, want 2", result.Retries)
	}
}

func TestExecutorNonRetryableError(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	jsExecutor := &mockJSExecutor{}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, nil, jsExecutor, historyStore, nil, cfg, logger)

	// Invalid payload - non-retryable
	job := &Job{
		Name:    "test-invalid",
		Type:    JobTypeScript,
		Payload: `{invalid json`,
	}

	result := executor.Execute(context.Background(), job)

	if result.Success {
		t.Error("Expected failure for invalid payload")
	}
	if result.Retries != 0 {
		t.Errorf("Retries = %d, want 0 (non-retryable)", result.Retries)
	}
}

func TestExecutorTimeout(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)

	// Runner that blocks
	slowRunner := &mockRunner{
		runFunc: func(ctx context.Context, prompt string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(10 * time.Second):
				return "done", nil
			}
		},
	}

	logger := zerolog.Nop()
	cfg := ExecutorConfig{
		Timeout: 50 * time.Millisecond, // Very short timeout
		RetryPolicy: RetryPolicy{
			MaxAttempts: 0, // No retries
		},
	}
	executor := NewExecutor(slowRunner, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-timeout",
		Type:    JobTypePrompt,
		Payload: `{"prompt": "slow"}`,
	}

	result := executor.Execute(context.Background(), job)

	if result.Success {
		t.Error("Expected failure for timeout")
	}
	if !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Errorf("Error = %v, want context.DeadlineExceeded", result.Error)
	}
}

func TestExecutorHistoryRecording(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	runner := &mockRunner{result: "done"}
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(runner, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-history",
		Type:    JobTypePrompt,
		Payload: `{"prompt": "test"}`,
	}

	result := executor.Execute(context.Background(), job)

	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}

	// Verify history was recorded
	entry, err := historyStore.Get(result.HistoryID)
	if err != nil {
		t.Fatalf("Get history failed: %v", err)
	}

	if entry.JobName != "test-history" {
		t.Errorf("JobName = %s, want test-history", entry.JobName)
	}
	if entry.Status != StatusSuccess {
		t.Errorf("Status = %s, want success", entry.Status)
	}
	if entry.Result != "done" {
		t.Errorf("Result = %s, want done", entry.Result)
	}
}

func TestExecutorMissingRunner(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name:    "test-no-runner",
		Type:    JobTypePrompt,
		Payload: `{"prompt": "test"}`,
	}

	result := executor.Execute(context.Background(), job)

	if result.Success {
		t.Error("Expected failure when runner is not configured")
	}
}

func TestExecutorUnknownJobType(t *testing.T) {
	db := setupTestDB(t)
	historyStore := NewHistoryStore(db)
	logger := zerolog.Nop()

	cfg := DefaultExecutorConfig()
	executor := NewExecutor(nil, nil, nil, historyStore, nil, cfg, logger)

	job := &Job{
		Name: "test-unknown",
		Type: JobType("unknown"),
	}

	result := executor.Execute(context.Background(), job)

	if result.Success {
		t.Error("Expected failure for unknown job type")
	}
}
