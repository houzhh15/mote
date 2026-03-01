package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"mote/internal/workspace"
)

// Runner executes prompts through the LLM.
type Runner interface {
	Run(ctx context.Context, prompt string, opts ...interface{}) (string, error)
}

// CancellableRunner is an optional interface that runners can implement
// to support forceful cancellation of stuck sessions.
type CancellableRunner interface {
	Runner
	CancelSession(sessionID string)
}

// AgentRunner is an optional interface that runners can implement
// to support direct delegation to a specific sub-agent (bypassing the main LLM).
type AgentRunner interface {
	RunAgent(ctx context.Context, sessionID, agentName, prompt string) (string, error)
}

// ToolRegistry provides access to registered tools.
type ToolRegistry interface {
	Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
}

// JSExecutor executes JavaScript code.
type JSExecutor interface {
	Execute(ctx context.Context, script, scriptName, executionID string) (interface{}, error)
	ExecuteFile(ctx context.Context, filePath, executionID string) (interface{}, error)
}

// WorkspaceManager manages workspace bindings for sessions.
type WorkspaceManager interface {
	Bind(sessionID, workspacePath string, readOnly bool) error
	BindWithAlias(sessionID, workspacePath, alias string, readOnly bool) error
	Get(sessionID string) (*workspace.WorkspaceBinding, bool)
}

// Executor handles running cron jobs.
type Executor struct {
	runner           Runner
	toolRegistry     ToolRegistry
	jsExecutor       JSExecutor
	historyStore     *HistoryStore
	workspaceManager WorkspaceManager
	retryPolicy      RetryPolicy
	timeout          time.Duration
	logger           zerolog.Logger
}

// ExecutorConfig holds configuration for the executor.
type ExecutorConfig struct {
	Timeout     time.Duration
	RetryPolicy RetryPolicy
}

// DefaultExecutorConfig returns default executor configuration.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		Timeout:     30 * time.Minute,
		RetryPolicy: DefaultRetryPolicy(),
	}
}

// NewExecutor creates a new job executor.
func NewExecutor(
	runner Runner,
	toolRegistry ToolRegistry,
	jsExecutor JSExecutor,
	historyStore *HistoryStore,
	workspaceManager WorkspaceManager,
	cfg ExecutorConfig,
	logger zerolog.Logger,
) *Executor {
	return &Executor{
		runner:           runner,
		toolRegistry:     toolRegistry,
		jsExecutor:       jsExecutor,
		historyStore:     historyStore,
		workspaceManager: workspaceManager,
		retryPolicy:      cfg.RetryPolicy,
		timeout:          cfg.Timeout,
		logger:           logger,
	}
}

// ExecuteResult holds the result of job execution.
type ExecuteResult struct {
	Success   bool
	Result    string
	Error     error
	Retries   int
	Duration  time.Duration
	HistoryID int64
}

// Execute runs a job and records the result in history.
func (e *Executor) Execute(ctx context.Context, job *Job) *ExecuteResult {
	startTime := time.Now()

	// Bind workspace if specified
	if job.WorkspacePath != "" && e.workspaceManager != nil {
		sessionID := deriveCronSessionID(job.Name)
		var err error
		if job.WorkspaceAlias != "" {
			err = e.workspaceManager.BindWithAlias(sessionID, job.WorkspacePath, job.WorkspaceAlias, false)
		} else {
			err = e.workspaceManager.Bind(sessionID, job.WorkspacePath, false)
		}
		if err != nil {
			e.logger.Warn().
				Err(err).
				Str("job", job.Name).
				Str("workspace", job.WorkspacePath).
				Msg("failed to bind workspace, continuing without workspace context")
		} else {
			e.logger.Debug().
				Str("job", job.Name).
				Str("workspace", job.WorkspacePath).
				Str("session_id", sessionID).
				Msg("workspace bound to cron job session")
		}
	}

	// Create history entry
	entry, err := e.historyStore.StartExecution(job.Name)
	if err != nil {
		e.logger.Error().Err(err).Str("job", job.Name).Msg("failed to create history entry")
		return &ExecuteResult{
			Success: false,
			Error:   err,
		}
	}

	// Execute with retry
	result, execErr, retries := e.executeWithRetry(ctx, job)

	// Update history
	if err := e.historyStore.FinishExecution(entry, result, execErr); err != nil {
		e.logger.Error().Err(err).Str("job", job.Name).Msg("failed to update history entry")
	}

	return &ExecuteResult{
		Success:   execErr == nil,
		Result:    result,
		Error:     execErr,
		Retries:   retries,
		Duration:  time.Since(startTime),
		HistoryID: entry.ID,
	}
}

// deriveCronSessionID generates a session ID for a cron job.
func deriveCronSessionID(jobName string) string {
	return "cron-" + jobName
}

// executeWithRetry handles retry logic.
func (e *Executor) executeWithRetry(ctx context.Context, job *Job) (string, error, int) {
	var lastErr error
	var result string

	for attempt := 0; attempt <= e.retryPolicy.MaxAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			delay := e.retryPolicy.NextDelay(attempt - 1)
			select {
			case <-ctx.Done():
				return "", ctx.Err(), attempt
			case <-time.After(delay):
			}
		}

		result, lastErr = e.executeOnce(ctx, job)
		if lastErr == nil {
			return result, nil, attempt
		}

		if !e.retryPolicy.ShouldRetry(attempt, lastErr) {
			return "", lastErr, attempt
		}

		e.logger.Warn().
			Err(lastErr).
			Str("job", job.Name).
			Int("attempt", attempt+1).
			Msg("job execution failed, retrying")
	}

	return "", lastErr, e.retryPolicy.MaxAttempts
}

// executeOnce runs the job once without retry.
func (e *Executor) executeOnce(ctx context.Context, job *Job) (string, error) {
	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	switch job.Type {
	case JobTypePrompt:
		return e.executePrompt(execCtx, job)
	case JobTypeTool:
		return e.executeTool(execCtx, job)
	case JobTypeScript:
		return e.executeScript(execCtx, job)
	default:
		return "", fmt.Errorf("unknown job type: %s", job.Type)
	}
}

// executePrompt runs a prompt job.
func (e *Executor) executePrompt(ctx context.Context, job *Job) (string, error) {
	if e.runner == nil {
		return "", fmt.Errorf("runner not configured")
	}

	var payload PromptPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return "", NonRetryable(fmt.Errorf("invalid prompt payload: %w", err))
	}

	if payload.Prompt == "" {
		return "", NonRetryable(fmt.Errorf("prompt is required"))
	}

	// If AgentID is specified, route directly to that sub-agent
	if payload.AgentID != "" {
		agentRunner, ok := e.runner.(AgentRunner)
		if !ok {
			return "", NonRetryable(fmt.Errorf("runner does not support direct agent execution"))
		}
		sessionID := deriveCronSessionID(job.Name)
		return agentRunner.RunAgent(ctx, sessionID, payload.AgentID, payload.Prompt)
	}

	// Pass derived session ID and per-job model to runner for workspace-aware execution
	sessionID := deriveCronSessionID(job.Name)
	opts := []interface{}{sessionID}
	if payload.Model != "" {
		opts = append(opts, payload.Model)
	}
	result, err := e.runner.Run(ctx, payload.Prompt, opts...)
	if err != nil {
		return "", err
	}

	return result, nil
}

// executeTool runs a tool job.
func (e *Executor) executeTool(ctx context.Context, job *Job) (string, error) {
	if e.toolRegistry == nil {
		return "", fmt.Errorf("tool registry not configured")
	}

	var payload ToolPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return "", NonRetryable(fmt.Errorf("invalid tool payload: %w", err))
	}

	if payload.Tool == "" {
		return "", NonRetryable(fmt.Errorf("tool name is required"))
	}

	result, err := e.toolRegistry.Execute(ctx, payload.Tool, payload.Args)
	if err != nil {
		return "", err
	}

	// Convert result to string
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}

	return string(resultJSON), nil
}

// executeScript runs a script job.
func (e *Executor) executeScript(ctx context.Context, job *Job) (string, error) {
	if e.jsExecutor == nil {
		return "", fmt.Errorf("js executor not configured")
	}

	var payload ScriptPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return "", NonRetryable(fmt.Errorf("invalid script payload: %w", err))
	}

	executionID := fmt.Sprintf("cron-%s-%d", job.Name, time.Now().UnixNano())

	var result interface{}
	var err error

	if payload.ScriptPath != "" {
		result, err = e.jsExecutor.ExecuteFile(ctx, payload.ScriptPath, executionID)
	} else if payload.Script != "" {
		result, err = e.jsExecutor.Execute(ctx, payload.Script, job.Name+".js", executionID)
	} else {
		return "", NonRetryable(fmt.Errorf("script or script_path is required"))
	}

	if err != nil {
		return "", err
	}

	// Convert result to string
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}

	return string(resultJSON), nil
}
