package cron

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// JobStore handles persistence of cron jobs.
type JobStore struct {
	db *sql.DB
}

// NewJobStore creates a new job store.
func NewJobStore(db *sql.DB) *JobStore {
	return &JobStore{db: db}
}

// Create inserts a new job into the database.
func (s *JobStore) Create(job *JobCreate) (*Job, error) {
	if err := job.Validate(); err != nil {
		return nil, err
	}

	// Validate and normalize workspace path
	normalizedPath, err := validateAndNormalizePath(job.WorkspacePath)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result := &Job{
		Name:           job.Name,
		Schedule:       job.Schedule,
		Type:           job.Type,
		Payload:        job.Payload,
		Enabled:        job.Enabled,
		WorkspacePath:  normalizedPath,
		WorkspaceAlias: job.WorkspaceAlias,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	query := `
		INSERT INTO cron_jobs (name, schedule, type, payload, enabled, workspace_path, workspace_alias, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = s.db.Exec(query, result.Name, result.Schedule, result.Type, result.Payload,
		result.Enabled, nullString(result.WorkspacePath), nullString(result.WorkspaceAlias),
		result.CreatedAt, result.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	return result, nil
}

// Get retrieves a job by name.
func (s *JobStore) Get(name string) (*Job, error) {
	query := `
		SELECT name, schedule, type, payload, enabled, workspace_path, workspace_alias, last_run, next_run, created_at, updated_at
		FROM cron_jobs
		WHERE name = ?
	`
	row := s.db.QueryRow(query, name)

	var job Job
	var workspacePath, workspaceAlias sql.NullString
	err := row.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
		&workspacePath, &workspaceAlias, &job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}

	if workspacePath.Valid {
		job.WorkspacePath = workspacePath.String
	}
	if workspaceAlias.Valid {
		job.WorkspaceAlias = workspaceAlias.String
	}

	return &job, nil
}

// Update modifies an existing job.
func (s *JobStore) Update(name string, patch *JobPatch) (*Job, error) {
	// Get existing job first
	existing, err := s.Get(name)
	if err != nil {
		return nil, err
	}

	// Apply patch
	if patch.Schedule != nil {
		existing.Schedule = *patch.Schedule
	}
	if patch.Type != nil {
		existing.Type = *patch.Type
	}
	if patch.Payload != nil {
		existing.Payload = *patch.Payload
	}
	if patch.Enabled != nil {
		existing.Enabled = *patch.Enabled
	}
	if patch.WorkspacePath != nil {
		normalizedPath, err := validateAndNormalizePath(*patch.WorkspacePath)
		if err != nil {
			return nil, err
		}
		existing.WorkspacePath = normalizedPath
	}
	if patch.WorkspaceAlias != nil {
		existing.WorkspaceAlias = *patch.WorkspaceAlias
	}
	existing.UpdatedAt = time.Now()

	query := `
		UPDATE cron_jobs
		SET schedule = ?, type = ?, payload = ?, enabled = ?, workspace_path = ?, workspace_alias = ?, updated_at = ?
		WHERE name = ?
	`
	_, err = s.db.Exec(query, existing.Schedule, existing.Type, existing.Payload,
		existing.Enabled, nullString(existing.WorkspacePath), nullString(existing.WorkspaceAlias),
		existing.UpdatedAt, name)
	if err != nil {
		return nil, fmt.Errorf("update job: %w", err)
	}

	return existing, nil
}

// Delete removes a job by name.
func (s *JobStore) Delete(name string) error {
	query := `DELETE FROM cron_jobs WHERE name = ?`
	result, err := s.db.Exec(query, name)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrJobNotFound
	}

	return nil
}

// List retrieves all jobs.
func (s *JobStore) List() ([]*Job, error) {
	query := `
		SELECT name, schedule, type, payload, enabled, workspace_path, workspace_alias, last_run, next_run, created_at, updated_at
		FROM cron_jobs
		ORDER BY name
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var job Job
		var workspacePath, workspaceAlias sql.NullString
		err := rows.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
			&workspacePath, &workspaceAlias, &job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if workspacePath.Valid {
			job.WorkspacePath = workspacePath.String
		}
		if workspaceAlias.Valid {
			job.WorkspaceAlias = workspaceAlias.String
		}
		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}

// ListEnabled retrieves all enabled jobs.
func (s *JobStore) ListEnabled() ([]*Job, error) {
	query := `
		SELECT name, schedule, type, payload, enabled, workspace_path, workspace_alias, last_run, next_run, created_at, updated_at
		FROM cron_jobs
		WHERE enabled = 1
		ORDER BY next_run
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query enabled jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var job Job
		var workspacePath, workspaceAlias sql.NullString
		err := rows.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
			&workspacePath, &workspaceAlias, &job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if workspacePath.Valid {
			job.WorkspacePath = workspacePath.String
		}
		if workspaceAlias.Valid {
			job.WorkspaceAlias = workspaceAlias.String
		}
		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}

// UpdateLastRun updates the last_run and next_run timestamps.
func (s *JobStore) UpdateLastRun(name string, lastRun, nextRun time.Time) error {
	query := `
		UPDATE cron_jobs
		SET last_run = ?, next_run = ?, updated_at = ?
		WHERE name = ?
	`
	_, err := s.db.Exec(query, lastRun, nextRun, time.Now(), name)
	if err != nil {
		return fmt.Errorf("update last run: %w", err)
	}
	return nil
}

// validateAndNormalizePath validates and normalizes a workspace path.
// It ensures the path exists, is a directory, and returns the absolute path.
func validateAndNormalizePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid workspace path: %w", err)
	}

	// Check existence
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("workspace path does not exist: %w", err)
	}

	// Must be a directory
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory")
	}

	return absPath, nil
}

// nullString converts empty string to NULL for SQL.
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
