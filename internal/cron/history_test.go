package cron

import (
	"errors"
	"testing"
	"time"
)

func TestHistoryStoreCreate(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	entry := &HistoryEntry{
		JobName:   "test-job",
		StartedAt: time.Now(),
		Status:    StatusRunning,
	}

	id, err := store.Create(entry)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestHistoryStoreGet(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	// Create entry
	entry := &HistoryEntry{
		JobName:   "test-job",
		StartedAt: time.Now(),
		Status:    StatusRunning,
	}
	id, err := store.Create(entry)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get entry
	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	if got.JobName != "test-job" {
		t.Errorf("JobName = %s, want test-job", got.JobName)
	}
	if got.Status != StatusRunning {
		t.Errorf("Status = %s, want running", got.Status)
	}
}

func TestHistoryStoreUpdate(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	// Create entry
	entry := &HistoryEntry{
		JobName:   "test-job",
		StartedAt: time.Now(),
		Status:    StatusRunning,
	}
	id, err := store.Create(entry)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	entry.ID = id

	// Update entry
	now := time.Now()
	entry.FinishedAt = &now
	entry.Status = StatusSuccess
	entry.Result = "completed"

	err = store.Update(entry)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Status != StatusSuccess {
		t.Errorf("Status = %s, want success", got.Status)
	}
	if got.Result != "completed" {
		t.Errorf("Result = %s, want completed", got.Result)
	}
}

func TestHistoryStoreList(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	// Create multiple entries
	for i := 0; i < 5; i++ {
		entry := &HistoryEntry{
			JobName:   "test-job",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    StatusSuccess,
		}
		_, err := store.Create(entry)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with limit
	entries, err := store.ListByJob("test-job", 3)
	if err != nil {
		t.Fatalf("ListByJob failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestHistoryStoreGetLatest(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	baseTime := time.Now()
	// Create multiple entries with distinct timestamps and update with results
	for i := 0; i < 3; i++ {
		entry := &HistoryEntry{
			JobName:   "test-job",
			StartedAt: baseTime.Add(time.Duration(i) * time.Minute), // Use minutes for clear separation
			Status:    StatusRunning,
		}
		id, err := store.Create(entry)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Update with result
		entry.ID = id
		entry.Status = StatusSuccess
		entry.Result = "run-" + string(rune('0'+i))
		now := time.Now()
		entry.FinishedAt = &now
		if err := store.Update(entry); err != nil {
			t.Fatalf("Update failed: %v", err)
		}
	}

	// Get latest
	latest, err := store.GetLatest("test-job")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}

	if latest == nil {
		t.Fatal("expected latest entry, got nil")
	}
	if latest.Result != "run-2" { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Errorf("expected latest entry 'run-2', got %s", latest.Result)
	}
}

func TestHistoryStoreGetLatestNoHistory(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	latest, err := store.GetLatest("nonexistent-job")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}

	if latest != nil {
		t.Error("expected nil for job with no history")
	}
}

func TestHistoryStoreCleanup(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	// Create many entries
	for i := 0; i < 10; i++ {
		entry := &HistoryEntry{
			JobName:   "test-job",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    StatusSuccess,
		}
		_, err := store.Create(entry)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Cleanup keeping only 5
	deleted, err := store.Cleanup("test-job", 5)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if deleted != 5 {
		t.Errorf("expected 5 deleted, got %d", deleted)
	}

	// Verify remaining
	entries, err := store.ListByJob("test-job", 100)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("expected 5 remaining, got %d", len(entries))
	}
}

func TestHistoryStoreCleanupNotEnough(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	// Create only 3 entries
	for i := 0; i < 3; i++ {
		entry := &HistoryEntry{
			JobName:   "test-job",
			StartedAt: time.Now(),
			Status:    StatusSuccess,
		}
		_, err := store.Create(entry)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Try to cleanup keeping 5 (more than we have)
	deleted, err := store.Cleanup("test-job", 5)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestHistoryStoreStartExecution(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	entry, err := store.StartExecution("test-job")
	if err != nil {
		t.Fatalf("StartExecution failed: %v", err)
	}

	if entry.ID <= 0 {
		t.Error("expected positive ID")
	}
	if entry.JobName != "test-job" {
		t.Errorf("JobName = %s, want test-job", entry.JobName)
	}
	if entry.Status != StatusRunning {
		t.Errorf("Status = %s, want running", entry.Status)
	}
}

func TestHistoryStoreFinishExecutionSuccess(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	entry, err := store.StartExecution("test-job")
	if err != nil {
		t.Fatalf("StartExecution failed: %v", err)
	}

	err = store.FinishExecution(entry, "done", nil)
	if err != nil {
		t.Fatalf("FinishExecution failed: %v", err)
	}

	// Verify
	got, err := store.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Status != StatusSuccess {
		t.Errorf("Status = %s, want success", got.Status)
	}
	if got.Result != "done" {
		t.Errorf("Result = %s, want done", got.Result)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
}

func TestHistoryStoreFinishExecutionError(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)

	entry, err := store.StartExecution("test-job")
	if err != nil {
		t.Fatalf("StartExecution failed: %v", err)
	}

	err = store.FinishExecution(entry, "", errors.New("something went wrong"))
	if err != nil {
		t.Fatalf("FinishExecution failed: %v", err)
	}

	// Verify
	got, err := store.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Status != StatusFailed {
		t.Errorf("Status = %s, want failed", got.Status)
	}
	if got.Error != "something went wrong" {
		t.Errorf("Error = %s, want 'something went wrong'", got.Error)
	}
}
