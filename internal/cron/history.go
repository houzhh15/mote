package cron

import (
	"database/sql"
	"fmt"
	"time"
)

// HistoryStore handles persistence of job execution history.
type HistoryStore struct {
	db *sql.DB
}

// NewHistoryStore creates a new history store.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// Create inserts a new history entry and returns the ID.
func (s *HistoryStore) Create(entry *HistoryEntry) (int64, error) {
	query := `
		INSERT INTO cron_history (job_name, started_at, status, retry_count)
		VALUES (?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, entry.JobName, entry.StartedAt, entry.Status, entry.RetryCount)
	if err != nil {
		return 0, fmt.Errorf("insert history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}

	return id, nil
}

// Update modifies an existing history entry.
func (s *HistoryStore) Update(entry *HistoryEntry) error {
	query := `
		UPDATE cron_history
		SET finished_at = ?, status = ?, result = ?, error = ?, retry_count = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query, entry.FinishedAt, entry.Status, entry.Result, entry.Error,
		entry.RetryCount, entry.ID)
	if err != nil {
		return fmt.Errorf("update history: %w", err)
	}
	return nil
}

// Get retrieves a history entry by ID.
func (s *HistoryStore) Get(id int64) (*HistoryEntry, error) {
	query := `
		SELECT id, job_name, started_at, finished_at, status, result, error, retry_count
		FROM cron_history
		WHERE id = ?
	`
	row := s.db.QueryRow(query, id)

	var entry HistoryEntry
	err := row.Scan(&entry.ID, &entry.JobName, &entry.StartedAt, &entry.FinishedAt,
		&entry.Status, &entry.Result, &entry.Error, &entry.RetryCount)
	if err == sql.ErrNoRows {
		return nil, ErrHistoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan history: %w", err)
	}

	return &entry, nil
}

// List retrieves all history entries with limit.
func (s *HistoryStore) List(limit int) ([]*HistoryEntry, error) {
	query := `
		SELECT id, job_name, started_at, finished_at, status, result, error, retry_count
		FROM cron_history
		ORDER BY started_at DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []*HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		err := rows.Scan(&entry.ID, &entry.JobName, &entry.StartedAt, &entry.FinishedAt,
			&entry.Status, &entry.Result, &entry.Error, &entry.RetryCount)
		if err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// ListByJob retrieves history entries for a job with limit.
func (s *HistoryStore) ListByJob(jobName string, limit int) ([]*HistoryEntry, error) {
	query := `
		SELECT id, job_name, started_at, finished_at, status, result, error, retry_count
		FROM cron_history
		WHERE job_name = ?
		ORDER BY started_at DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, jobName, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []*HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		err := rows.Scan(&entry.ID, &entry.JobName, &entry.StartedAt, &entry.FinishedAt,
			&entry.Status, &entry.Result, &entry.Error, &entry.RetryCount)
		if err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// GetLatest retrieves the most recent history entry for a job.
func (s *HistoryStore) GetLatest(jobName string) (*HistoryEntry, error) {
	query := `
		SELECT id, job_name, started_at, finished_at, status, result, error, retry_count
		FROM cron_history
		WHERE job_name = ?
		ORDER BY started_at DESC
		LIMIT 1
	`
	row := s.db.QueryRow(query, jobName)

	var entry HistoryEntry
	err := row.Scan(&entry.ID, &entry.JobName, &entry.StartedAt, &entry.FinishedAt,
		&entry.Status, &entry.Result, &entry.Error, &entry.RetryCount)
	if err == sql.ErrNoRows {
		return nil, nil // No history yet
	}
	if err != nil {
		return nil, fmt.Errorf("scan history: %w", err)
	}

	return &entry, nil
}

// Cleanup removes old history entries beyond the retention limit.
func (s *HistoryStore) Cleanup(jobName string, keep int) (int64, error) {
	// First get the ID threshold
	query := `
		SELECT id FROM cron_history
		WHERE job_name = ?
		ORDER BY started_at DESC
		LIMIT 1 OFFSET ?
	`
	row := s.db.QueryRow(query, jobName, keep)

	var thresholdID int64
	err := row.Scan(&thresholdID)
	if err == sql.ErrNoRows {
		return 0, nil // Not enough entries to cleanup
	}
	if err != nil {
		return 0, fmt.Errorf("get threshold id: %w", err)
	}

	// Delete entries older than threshold
	deleteQuery := `
		DELETE FROM cron_history
		WHERE job_name = ? AND id <= ?
	`
	result, err := s.db.Exec(deleteQuery, jobName, thresholdID)
	if err != nil {
		return 0, fmt.Errorf("delete old history: %w", err)
	}

	return result.RowsAffected()
}

// CleanupAll removes old history entries for all jobs.
func (s *HistoryStore) CleanupAll(keep int) (int64, error) {
	// Get all unique job names
	query := `SELECT DISTINCT job_name FROM cron_history`
	rows, err := s.db.Query(query)
	if err != nil {
		return 0, fmt.Errorf("query job names: %w", err)
	}
	defer rows.Close()

	var totalDeleted int64
	for rows.Next() {
		var jobName string
		if err := rows.Scan(&jobName); err != nil {
			return totalDeleted, fmt.Errorf("scan job name: %w", err)
		}

		deleted, err := s.Cleanup(jobName, keep)
		if err != nil {
			return totalDeleted, err
		}
		totalDeleted += deleted
	}

	return totalDeleted, rows.Err()
}

// StartExecution creates a running history entry and returns it.
func (s *HistoryStore) StartExecution(jobName string) (*HistoryEntry, error) {
	entry := &HistoryEntry{
		JobName:   jobName,
		StartedAt: time.Now(),
		Status:    StatusRunning,
	}

	id, err := s.Create(entry)
	if err != nil {
		return nil, err
	}
	entry.ID = id

	return entry, nil
}

// FinishExecution updates a history entry with the result.
func (s *HistoryStore) FinishExecution(entry *HistoryEntry, result string, err error) error {
	now := time.Now()
	entry.FinishedAt = &now

	if err != nil {
		entry.Status = StatusFailed
		entry.Error = err.Error()
	} else {
		entry.Status = StatusSuccess
		entry.Result = result
	}

	return s.Update(entry)
}
