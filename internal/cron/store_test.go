package cron

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create tables
	schema := `
		CREATE TABLE IF NOT EXISTS cron_jobs (
			name TEXT PRIMARY KEY,
			schedule TEXT NOT NULL,
			type TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			model TEXT,
			session_id TEXT,
			workspace_path TEXT,
			workspace_alias TEXT,
			last_run DATETIME,
			next_run DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cron_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_name TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			status TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			retry_count INTEGER NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_cron_history_job_name ON cron_history(job_name, started_at DESC);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tmpDir)
	})

	return db
}

func TestJobStoreCreate(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	job, err := store.Create(&JobCreate{
		Name:     "test-job",
		Schedule: "0 * * * *",
		Type:     JobTypePrompt,
		Payload:  `{"prompt": "hello"}`,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if job.Name != "test-job" {
		t.Errorf("Name = %s, want test-job", job.Name)
	}
	if job.Schedule != "0 * * * *" {
		t.Errorf("Schedule = %s, want 0 * * * *", job.Schedule)
	}
	if job.Type != JobTypePrompt {
		t.Errorf("Type = %s, want prompt", job.Type)
	}
}

func TestJobStoreCreateValidation(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	_, err := store.Create(&JobCreate{
		Name: "", // Missing name
	})
	if err == nil {
		t.Error("expected validation error for missing name")
	}
}

func TestJobStoreGet(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	// Create job
	_, err := store.Create(&JobCreate{
		Name:     "get-test",
		Schedule: "0 * * * *",
		Type:     JobTypeTool,
		Payload:  `{"tool": "echo"}`,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get job
	job, err := store.Get("get-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if job.Name != "get-test" {
		t.Errorf("Name = %s, want get-test", job.Name)
	}
}

func TestJobStoreGetNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	_, err := store.Get("nonexistent")
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestJobStoreUpdate(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	// Create job
	_, err := store.Create(&JobCreate{
		Name:     "update-test",
		Schedule: "0 * * * *",
		Type:     JobTypePrompt,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update job
	newSchedule := "*/5 * * * *"
	enabled := false
	job, err := store.Update("update-test", &JobPatch{
		Schedule: &newSchedule,
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if job.Schedule != "*/5 * * * *" {
		t.Errorf("Schedule = %s, want */5 * * * *", job.Schedule)
	}
	if job.Enabled {
		t.Error("Enabled should be false")
	}
}

func TestJobStoreUpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	_, err := store.Update("nonexistent", &JobPatch{})
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestJobStoreDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	// Create job
	_, err := store.Create(&JobCreate{
		Name:     "delete-test",
		Schedule: "0 * * * *",
		Type:     JobTypePrompt,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete job
	err = store.Delete("delete-test")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get("delete-test")
	if err != ErrJobNotFound {
		t.Error("expected job to be deleted")
	}
}

func TestJobStoreDeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	err := store.Delete("nonexistent")
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestJobStoreList(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	// Create jobs
	for i := 1; i <= 3; i++ {
		_, err := store.Create(&JobCreate{
			Name:     "job-" + string(rune('0'+i)),
			Schedule: "0 * * * *",
			Type:     JobTypePrompt,
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List jobs
	jobs, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestJobStoreListEnabled(t *testing.T) {
	db := setupTestDB(t)
	store := NewJobStore(db)

	// Create enabled job
	_, err := store.Create(&JobCreate{
		Name:     "enabled-job",
		Schedule: "0 * * * *",
		Type:     JobTypePrompt,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create disabled job
	_, err = store.Create(&JobCreate{
		Name:     "disabled-job",
		Schedule: "0 * * * *",
		Type:     JobTypePrompt,
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List enabled jobs
	jobs, err := store.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("expected 1 enabled job, got %d", len(jobs))
	}
	if jobs[0].Name != "enabled-job" {
		t.Errorf("expected enabled-job, got %s", jobs[0].Name)
	}
}
