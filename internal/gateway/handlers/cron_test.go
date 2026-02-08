package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"

	"mote/internal/cron"
)

// setupCronHandlerTest creates test dependencies.
func setupCronHandlerTest(t *testing.T) (*CronHandler, *mux.Router) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Initialize schema
	schema := `
		CREATE TABLE IF NOT EXISTS cron_jobs (
			name TEXT PRIMARY KEY,
			schedule TEXT NOT NULL,
			type TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
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
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	store := cron.NewJobStore(db)
	history := cron.NewHistoryStore(db)

	runner := &mockRunner{result: "executed"}
	executor := cron.NewExecutor(runner, nil, nil, history, cron.DefaultExecutorConfig(), zerolog.Nop())
	scheduler := cron.NewScheduler(store, history, executor, nil, nil)

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	t.Cleanup(func() { scheduler.Stop() })

	handler := NewCronHandler(scheduler, history)

	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return handler, router
}

// mockRunner is a test runner for cron jobs.
type mockRunner struct {
	result string
	err    error
}

func (m *mockRunner) Run(ctx context.Context, prompt string, opts ...interface{}) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestCronHandlerListJobsEmpty(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	req := httptest.NewRequest("GET", "/api/cron/jobs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	jobs, ok := resp["jobs"].([]any)
	if !ok {
		// jobs can be nil if empty
		if resp["jobs"] != nil {
			t.Errorf("expected jobs to be nil or array, got %T", resp["jobs"])
		}
	} else if len(jobs) != 0 {
		t.Errorf("got %d jobs, want 0", len(jobs))
	}
}

func TestCronHandlerCreateJob(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{\"prompt\": \"hello\"}",
		"enabled": true
	}`

	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var job cron.Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if job.Name != "test-job" {
		t.Errorf("got name %q, want %q", job.Name, "test-job")
	}
}

func TestCronHandlerCreateJobInvalidCron(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	body := `{
		"name": "bad-cron",
		"schedule": "invalid",
		"type": "prompt",
		"payload": "{}",
		"enabled": true
	}`

	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCronHandlerGetJob(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create a job first
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Get the job
	req = httptest.NewRequest("GET", "/api/cron/jobs/test-job", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	job := resp["job"].(map[string]any)
	if job["name"] != "test-job" {
		t.Errorf("got name %v, want test-job", job["name"])
	}
}

func TestCronHandlerGetJobNotFound(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	req := httptest.NewRequest("GET", "/api/cron/jobs/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCronHandlerUpdateJob(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create a job first
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Update the job
	updateBody := `{"schedule": "*/10 * * * *"}`
	req = httptest.NewRequest("PUT", "/api/cron/jobs/test-job", bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var job cron.Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if job.Schedule != "*/10 * * * *" {
		t.Errorf("got schedule %q, want %q", job.Schedule, "*/10 * * * *")
	}
}

func TestCronHandlerDeleteJob(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create a job first
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Delete the job
	req = httptest.NewRequest("DELETE", "/api/cron/jobs/test-job", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify it's gone
	req = httptest.NewRequest("GET", "/api/cron/jobs/test-job", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCronHandlerRunJob(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create a job first
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{\"prompt\": \"hello\"}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Run the job
	req = httptest.NewRequest("POST", "/api/cron/jobs/test-job/run", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
	if resp["result"] != "executed" {
		t.Errorf("got result %v, want 'executed'", resp["result"])
	}
}

func TestCronHandlerEnableDisable(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create a job first
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Disable the job
	req = httptest.NewRequest("POST", "/api/cron/jobs/test-job/disable", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var job cron.Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if job.Enabled {
		t.Error("expected job to be disabled")
	}

	// Enable the job
	req = httptest.NewRequest("POST", "/api/cron/jobs/test-job/enable", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}

	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !job.Enabled {
		t.Error("expected job to be enabled")
	}
}

func TestCronHandlerListHistory(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create and run a job to generate history
	body := `{
		"name": "test-job",
		"schedule": "*/5 * * * *",
		"type": "prompt",
		"payload": "{\"prompt\": \"hello\"}",
		"enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Run the job
	req = httptest.NewRequest("POST", "/api/cron/jobs/test-job/run", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Get history
	req = httptest.NewRequest("GET", "/api/cron/history?job=test-job", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	entries, ok := resp["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries to be array, got %T", resp["entries"])
	}

	if len(entries) < 1 {
		t.Errorf("got %d entries, want at least 1", len(entries))
	}
}

func TestCronHandlerListJobs(t *testing.T) {
	_, router := setupCronHandlerTest(t)

	// Create some jobs
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{
			"name": "test-job-%d",
			"schedule": "*/5 * * * *",
			"type": "prompt",
			"payload": "{}",
			"enabled": true
		}`, i)
		req := httptest.NewRequest("POST", "/api/cron/jobs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// List jobs
	req := httptest.NewRequest("GET", "/api/cron/jobs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	jobs := resp["jobs"].([]any)
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3", len(jobs))
	}
}
