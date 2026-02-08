package cron

import (
	"database/sql"
	"fmt"
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

	now := time.Now()
	result := &Job{
		Name:      job.Name,
		Schedule:  job.Schedule,
		Type:      job.Type,
		Payload:   job.Payload,
		Enabled:   job.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	query := `
		INSERT INTO cron_jobs (name, schedule, type, payload, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, result.Name, result.Schedule, result.Type, result.Payload,
		result.Enabled, result.CreatedAt, result.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	return result, nil
}

// Get retrieves a job by name.
func (s *JobStore) Get(name string) (*Job, error) {
	query := `
		SELECT name, schedule, type, payload, enabled, last_run, next_run, created_at, updated_at
		FROM cron_jobs
		WHERE name = ?
	`
	row := s.db.QueryRow(query, name)

	var job Job
	err := row.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
		&job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
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
	existing.UpdatedAt = time.Now()

	query := `
		UPDATE cron_jobs
		SET schedule = ?, type = ?, payload = ?, enabled = ?, updated_at = ?
		WHERE name = ?
	`
	_, err = s.db.Exec(query, existing.Schedule, existing.Type, existing.Payload,
		existing.Enabled, existing.UpdatedAt, name)
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
		SELECT name, schedule, type, payload, enabled, last_run, next_run, created_at, updated_at
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
		err := rows.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
			&job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}

// ListEnabled retrieves all enabled jobs.
func (s *JobStore) ListEnabled() ([]*Job, error) {
	query := `
		SELECT name, schedule, type, payload, enabled, last_run, next_run, created_at, updated_at
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
		err := rows.Scan(&job.Name, &job.Schedule, &job.Type, &job.Payload, &job.Enabled,
			&job.LastRun, &job.NextRun, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
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
