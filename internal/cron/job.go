package cron

import (
	"time"
)

// JobType defines the type of cron job.
type JobType string

const (
	// JobTypePrompt runs a prompt through the LLM.
	JobTypePrompt JobType = "prompt"
	// JobTypeTool invokes a registered tool.
	JobTypeTool JobType = "tool"
	// JobTypeScript executes a JavaScript script.
	JobTypeScript JobType = "script"
)

// Job represents a scheduled task.
type Job struct {
	// Name is the unique identifier for the job.
	Name string `json:"name" db:"name"`
	// Schedule is the cron expression (e.g., "0 * * * *" for hourly).
	Schedule string `json:"schedule" db:"schedule"`
	// Type is the job type (prompt, tool, or script).
	Type JobType `json:"type" db:"type"`
	// Payload contains job-specific configuration as JSON.
	Payload string `json:"payload" db:"payload"`
	// Enabled indicates if the job is active.
	Enabled bool `json:"enabled" db:"enabled"`
	// WorkspacePath is the absolute path to the workspace directory for this job.
	WorkspacePath string `json:"workspace_path,omitempty" db:"workspace_path"`
	// WorkspaceAlias is the display name for the workspace.
	WorkspaceAlias string `json:"workspace_alias,omitempty" db:"workspace_alias"`
	// LastRun is the timestamp of the last execution.
	LastRun *time.Time `json:"last_run,omitempty" db:"last_run"`
	// NextRun is the scheduled time for the next execution.
	NextRun *time.Time `json:"next_run,omitempty" db:"next_run"`
	// CreatedAt is when the job was created.
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// UpdatedAt is when the job was last modified.
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// JobCreate is the input for creating a new job.
type JobCreate struct {
	Name           string  `json:"name"`
	Schedule       string  `json:"schedule"`
	Type           JobType `json:"type"`
	Payload        string  `json:"payload"`
	Enabled        bool    `json:"enabled"`
	WorkspacePath  string  `json:"workspace_path,omitempty"`
	WorkspaceAlias string  `json:"workspace_alias,omitempty"`
}

// Validate checks if the create input is valid.
func (c *JobCreate) Validate() error {
	if c.Name == "" {
		return &InvalidScheduleError{Message: "name is required"}
	}
	if c.Schedule == "" {
		return &InvalidScheduleError{Message: "schedule is required"}
	}
	switch c.Type {
	case JobTypePrompt, JobTypeTool, JobTypeScript:
		// Valid
	default:
		return &InvalidScheduleError{Message: "invalid job type"}
	}
	return nil
}

// JobPatch is the input for updating a job.
type JobPatch struct {
	Schedule       *string  `json:"schedule,omitempty"`
	Type           *JobType `json:"type,omitempty"`
	Payload        *string  `json:"payload,omitempty"`
	Enabled        *bool    `json:"enabled,omitempty"`
	WorkspacePath  *string  `json:"workspace_path,omitempty"`
	WorkspaceAlias *string  `json:"workspace_alias,omitempty"`
}

// HistoryStatus represents the execution status of a job run.
type HistoryStatus string

const (
	// StatusRunning indicates the job is currently executing.
	StatusRunning HistoryStatus = "running"
	// StatusSuccess indicates the job completed successfully.
	StatusSuccess HistoryStatus = "success"
	// StatusFailed indicates the job failed.
	StatusFailed HistoryStatus = "failed"
)

// HistoryEntry represents a single execution of a job.
type HistoryEntry struct {
	// ID is the unique identifier for this execution.
	ID int64 `json:"id" db:"id"`
	// JobName is the name of the job that was executed.
	JobName string `json:"job_name" db:"job_name"`
	// StartedAt is when execution started.
	StartedAt time.Time `json:"started_at" db:"started_at"`
	// FinishedAt is when execution completed (nil if still running).
	FinishedAt *time.Time `json:"finished_at,omitempty" db:"finished_at"`
	// Status is the current execution status.
	Status HistoryStatus `json:"status" db:"status"`
	// Result contains the execution result (if successful).
	Result string `json:"result,omitempty" db:"result"`
	// Error contains the error message (if failed).
	Error string `json:"error,omitempty" db:"error"`
	// RetryCount is the number of retry attempts made.
	RetryCount int `json:"retry_count" db:"retry_count"`
}

// PromptPayload is the payload structure for prompt-type jobs.
type PromptPayload struct {
	Prompt    string `json:"prompt"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

// ToolPayload is the payload structure for tool-type jobs.
type ToolPayload struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args,omitempty"`
}

// ScriptPayload is the payload structure for script-type jobs.
type ScriptPayload struct {
	Script     string                 `json:"script"`
	ScriptPath string                 `json:"script_path,omitempty"`
	Args       map[string]interface{} `json:"args,omitempty"`
}
