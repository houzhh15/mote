package scheduler

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"mote/internal/storage"
)

func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "scheduler_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	db, err := storage.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func TestSessionManager(t *testing.T) {
	t.Run("NewSessionManager", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)
		if sm == nil {
			t.Fatal("expected non-nil session manager")
		}
		if sm.Len() != 0 {
			t.Errorf("expected empty cache, got %d", sm.Len())
		}
	})

	t.Run("Create and Get", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		meta := json.RawMessage(`{"key": "value"}`)
		cached, err := sm.Create(meta)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		if cached.Session.ID == "" {
			t.Error("expected non-empty session ID")
		}

		got, err := sm.Get(cached.Session.ID)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if got.Session.ID != cached.Session.ID {
			t.Error("session ID mismatch")
		}
	})

	t.Run("Get cache miss", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		session, err := db.CreateSession(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		cached, err := sm.Get(session.ID)
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if cached.Session.ID != session.ID {
			t.Error("session ID mismatch")
		}

		if sm.Len() != 1 {
			t.Errorf("expected 1 cached session, got %d", sm.Len())
		}
	})

	t.Run("Get not found", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		_, err := sm.Get("nonexistent")
		if err != ErrSessionNotFound {
			t.Errorf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("GetOrCreate existing", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		got, err := sm.GetOrCreate(cached.Session.ID, nil)
		if err != nil {
			t.Fatalf("failed to get or create: %v", err)
		}
		if got.Session.ID != cached.Session.ID {
			t.Error("should return existing session")
		}
	})

	t.Run("AddMessage", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		msg, err := sm.AddMessage(cached.Session.ID, "user", "hello", nil, "")
		if err != nil {
			t.Fatalf("failed to add message: %v", err)
		}
		if msg.Content != "hello" {
			t.Errorf("expected content 'hello', got %q", msg.Content)
		}

		cached, _ = sm.Get(cached.Session.ID)
		if len(cached.Messages) != 1 {
			t.Errorf("expected 1 message in cache, got %d", len(cached.Messages))
		}
	})

	t.Run("GetMessages", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		for i := 0; i < 3; i++ {
			_, err := sm.AddMessage(cached.Session.ID, "user", "message", nil, "")
			if err != nil {
				t.Fatalf("failed to add message: %v", err)
			}
		}

		messages, err := sm.GetMessages(cached.Session.ID)
		if err != nil {
			t.Fatalf("failed to get messages: %v", err)
		}
		if len(messages) != 3 {
			t.Errorf("expected 3 messages, got %d", len(messages))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		id := cached.Session.ID

		err = sm.Delete(id)
		if err != nil {
			t.Fatalf("failed to delete session: %v", err)
		}

		if sm.Len() != 0 {
			t.Error("expected session to be removed from cache")
		}

		_, err = sm.Get(id)
		if err != ErrSessionNotFound {
			t.Errorf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("Invalidate", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		sm.Invalidate(cached.Session.ID)

		if sm.Len() != 0 {
			t.Error("expected session to be removed from cache")
		}

		got, err := sm.Get(cached.Session.ID)
		if err != nil {
			t.Fatalf("expected session to still exist: %v", err)
		}
		if got.Session.ID != cached.Session.ID {
			t.Error("session should be reloaded from database")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		db := setupTestDB(t)
		sm := NewSessionManager(db, 10)

		for i := 0; i < 5; i++ {
			_, err := sm.Create(nil)
			if err != nil {
				t.Fatalf("failed to create session: %v", err)
			}
		}

		sm.Clear()

		if sm.Len() != 0 {
			t.Errorf("expected empty cache, got %d", sm.Len())
		}
	})
}

func TestSessionManagerEviction(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db, 3)

	var ids []string
	for i := 0; i < 5; i++ {
		cached, err := sm.Create(nil)
		if err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		ids = append(ids, cached.Session.ID) //nolint:staticcheck // SA4010: Accumulating ids for later use
		time.Sleep(10 * time.Millisecond)
	}

	if sm.Len() > 3 {
		t.Errorf("expected cache size <= 3, got %d", sm.Len())
	}
}

func TestCachedSession(t *testing.T) {
	session := &storage.Session{ID: "test123"}
	cached := &CachedSession{
		Session:    session,
		Messages:   []*storage.Message{},
		Dirty:      false,
		LastAccess: time.Now(),
	}

	if cached.Session.ID != "test123" {
		t.Error("expected session ID test123")
	}
	if cached.Dirty {
		t.Error("expected Dirty to be false")
	}
}

func TestRunState(t *testing.T) {
	run := &Run{
		ID:        "run1",
		SessionID: "session1",
		State:     RunStatePending,
	}

	if !run.IsPending() {
		t.Error("expected IsPending to be true")
	}
	if run.IsRunning() {
		t.Error("expected IsRunning to be false")
	}
	if run.IsCompleted() {
		t.Error("expected IsCompleted to be false")
	}

	run.State = RunStateRunning
	if !run.IsRunning() {
		t.Error("expected IsRunning to be true")
	}

	run.State = RunStateCompleted
	if !run.IsCompleted() {
		t.Error("expected IsCompleted to be true for completed state")
	}

	run.State = RunStateFailed
	if !run.IsCompleted() {
		t.Error("expected IsCompleted to be true for failed state")
	}

	run.State = RunStateCancelled
	if !run.IsCompleted() {
		t.Error("expected IsCompleted to be true for cancelled state")
	}
}
