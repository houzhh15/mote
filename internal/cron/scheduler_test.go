package cron

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

// setupSchedulerTest creates test dependencies for scheduler tests.
func setupSchedulerTest(t *testing.T) (*sql.DB, *JobStore, *HistoryStore, *Executor) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Initialize schema
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS cron_jobs (
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
		)`,
		`CREATE TABLE IF NOT EXISTS cron_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_name TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			status TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			retry_count INTEGER NOT NULL DEFAULT 0
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}

	logger := zerolog.Nop()
	store := NewJobStore(db)
	history := NewHistoryStore(db)

	runner := &mockRunner{result: "ok"}
	executor := NewExecutor(runner, nil, nil, history, nil, DefaultExecutorConfig(), logger)

	return db, store, history, executor
}

func TestSchedulerStartStop(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)

	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Start again should fail
	if err := scheduler.Start(ctx); err == nil {
		t.Error("expected error starting already running scheduler")
	}

	// Stop
	scheduler.Stop()

	// Stop again should be idempotent
	scheduler.Stop()
}

func TestSchedulerAddJob(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	if job.Name == "" {
		t.Error("expected job name")
	}
	if job.Name != "test-job" {
		t.Errorf("got name %q, want %q", job.Name, "test-job")
	}

	// Job should be registered
	if scheduler.Entries() != 1 {
		t.Errorf("got %d entries, want 1", scheduler.Entries())
	}
}

func TestSchedulerAddJobInvalidCron(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	_, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "bad-cron",
		Schedule: "invalid cron",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestSchedulerUpdateJob(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// Update schedule
	newSchedule := "*/10 * * * *"
	updated, err := scheduler.UpdateJob(ctx, job.Name, JobPatch{Schedule: &newSchedule})
	if err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	if updated.Schedule != newSchedule {
		t.Errorf("got schedule %q, want %q", updated.Schedule, newSchedule)
	}
}

func TestSchedulerRemoveJob(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	if err := scheduler.RemoveJob(ctx, job.Name); err != nil {
		t.Fatalf("RemoveJob failed: %v", err)
	}

	// Job should be removed
	if scheduler.Entries() != 0 {
		t.Errorf("got %d entries, want 0", scheduler.Entries())
	}

	// Should be gone from store
	_, err = scheduler.GetJob(ctx, job.Name)
	if err == nil {
		t.Error("expected error getting removed job")
	}
}

func TestSchedulerEnableDisable(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// Initially enabled
	if !job.Enabled {
		t.Error("expected job to be enabled initially")
	}

	// Disable
	disabled, err := scheduler.DisableJob(ctx, job.Name)
	if err != nil {
		t.Fatalf("DisableJob failed: %v", err)
	}
	if disabled.Enabled {
		t.Error("expected job to be disabled")
	}
	if scheduler.Entries() != 0 {
		t.Errorf("disabled job should not be scheduled, got %d entries", scheduler.Entries())
	}

	// Enable
	enabled, err := scheduler.EnableJob(ctx, job.Name)
	if err != nil {
		t.Fatalf("EnableJob failed: %v", err)
	}
	if !enabled.Enabled {
		t.Error("expected job to be enabled")
	}
	if scheduler.Entries() != 1 {
		t.Errorf("enabled job should be scheduled, got %d entries", scheduler.Entries())
	}
}

func TestSchedulerListJobs(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	// Add some jobs
	for i := 0; i < 3; i++ {
		payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
		_, err := scheduler.AddJob(ctx, JobCreate{
			Name:     fmt.Sprintf("test-job-%d", i),
			Schedule: "*/5 * * * *",
			Type:     JobTypePrompt,
			Payload:  string(payload),
			Enabled:  true,
		})
		if err != nil {
			t.Fatalf("AddJob failed: %v", err)
		}
	}

	jobs, err := scheduler.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3", len(jobs))
	}
}

func TestSchedulerRunNow(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test prompt"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	result, err := scheduler.RunNow(ctx, job.Name)
	if err != nil {
		t.Fatalf("RunNow failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.Result != "ok" {
		t.Errorf("got result %q, want %q", result.Result, "ok")
	}
}

func TestSchedulerGetNextRun(t *testing.T) {
	_, store, history, executor := setupSchedulerTest(t)

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "test-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	nextRun, ok := scheduler.GetNextRun(job.Name)
	if !ok {
		t.Fatal("expected next run time")
	}

	if nextRun.IsZero() {
		t.Error("expected non-zero next run time")
	}

	// Should be in the future
	if nextRun.Before(time.Now()) {
		t.Error("expected next run to be in the future")
	}
}

func TestSchedulerLoadsEnabledJobsOnStart(t *testing.T) {
	db, store, history, executor := setupSchedulerTest(t)
	ctx := context.Background()

	// Create jobs before starting scheduler
	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job1, err := store.Create(&JobCreate{
		Name:     "enabled-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = store.Create(&JobCreate{
		Name:     "disabled-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create new scheduler (simulating restart)
	_ = db // keep db alive
	scheduler := NewScheduler(store, history, executor, nil, nil)

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	// Only enabled job should be registered
	if scheduler.Entries() != 1 {
		t.Errorf("got %d entries, want 1 (only enabled)", scheduler.Entries())
	}

	// Verify it's the right job
	_, ok := scheduler.GetNextRun(job1.Name)
	if !ok {
		t.Error("enabled job should have next run time")
	}
}

func TestSchedulerScheduledExecution(t *testing.T) {
	_, store, history, _ := setupSchedulerTest(t)

	// Use an atomic counter to track executions
	var execCount atomic.Int32
	runner := &mockRunner{
		runFunc: func(ctx context.Context, prompt string) (string, error) {
			execCount.Add(1)
			return "executed", nil
		},
	}
	executor := NewExecutor(runner, nil, nil, history, nil, DefaultExecutorConfig(), zerolog.Nop())

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	// Create a job that runs every minute
	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	_, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "every-minute",
		Schedule: "* * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// Verify the job is registered correctly
	if scheduler.Entries() != 1 {
		t.Errorf("got %d entries, want 1", scheduler.Entries())
	}
}

func TestSchedulerGracefulShutdown(t *testing.T) {
	_, store, history, _ := setupSchedulerTest(t)

	// Create a slow runner
	var execCount atomic.Int32
	runner := &mockRunner{
		runFunc: func(ctx context.Context, prompt string) (string, error) {
			execCount.Add(1)
			time.Sleep(100 * time.Millisecond)
			return "done", nil
		},
	}
	executor := NewExecutor(runner, nil, nil, history, nil, DefaultExecutorConfig(), zerolog.Nop())

	scheduler := NewScheduler(store, history, executor, nil, nil)
	ctx := context.Background()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Add and run a job
	payload, _ := json.Marshal(PromptPayload{Prompt: "test"})
	job, err := scheduler.AddJob(ctx, JobCreate{
		Name:     "slow-job",
		Schedule: "*/5 * * * *",
		Type:     JobTypePrompt,
		Payload:  string(payload),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// Start execution in background
	go func() {
		_, _ = scheduler.RunNow(ctx, job.Name)
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop should wait for running jobs
	stopCtx := scheduler.Stop()

	// Wait for stop to complete
	select {
	case <-stopCtx.Done():
		// Stopped
	case <-time.After(1 * time.Second):
		t.Error("stop took too long")
	}

	// Execution should have completed
	if execCount.Load() != 1 {
		t.Errorf("got %d executions, want 1", execCount.Load())
	}
}
