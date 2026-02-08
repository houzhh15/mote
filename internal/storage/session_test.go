package storage

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCreateSession(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
}

func TestGetSession(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	created, _ := db.CreateSession(nil)
	got, err := db.GetSession(created.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_, err := db.GetSession("nonexistent")
	if err != ErrNotFound {
		t.Errorf("want ErrNotFound")
	}
}

func TestUpdateSession(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	newMeta := json.RawMessage(`{"key":"value"}`)
	if err := db.UpdateSession(session.ID, newMeta); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	if err := db.DeleteSession(session.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	_, err := db.GetSession(session.ID)
	if err != ErrNotFound {
		t.Error("session should be deleted")
	}
}

func TestListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	for i := 0; i < 3; i++ {
		_, _ = db.CreateSession(nil)
	}

	sessions, err := db.ListSessions(2, 0)
	if err != nil || len(sessions) != 2 {
		t.Errorf("ListSessions failed")
	}
}
