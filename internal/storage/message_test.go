package storage

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAppendMessage(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	msg, err := db.AppendMessage(session.ID, "user", "Hello", nil, "")
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if msg.Role != "user" || msg.Content != "Hello" {
		t.Error("message content mismatch")
	}
}

func TestAppendMessage_WithToolCalls(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	toolCalls := []ToolCall{{ID: "call_1", Type: "function", Function: json.RawMessage(`{}`)}}
	msg, err := db.AppendMessage(session.ID, "assistant", "", toolCalls, "")
	if err != nil || len(msg.ToolCalls) != 1 {
		t.Error("tool calls mismatch")
	}
}

func TestGetMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	for i := 0; i < 3; i++ {
		_, _ = db.AppendMessage(session.ID, "user", "msg", nil, "")
	}

	messages, err := db.GetMessages(session.ID, 0)
	if err != nil || len(messages) != 3 {
		t.Error("GetMessages failed")
	}
}

func TestGetMessage(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	created, _ := db.AppendMessage(session.ID, "user", "Hello", nil, "")
	got, err := db.GetMessage(created.ID)
	if err != nil || got.ID != created.ID {
		t.Error("GetMessage failed")
	}
}

func TestDeleteMessage(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	msg, _ := db.AppendMessage(session.ID, "user", "Hello", nil, "")
	if err := db.DeleteMessage(msg.ID); err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}
	_, err := db.GetMessage(msg.ID)
	if err != ErrNotFound {
		t.Error("message should be deleted")
	}
}

func TestCountMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	_, _ = db.AppendMessage(session.ID, "user", "msg1", nil, "")
	_, _ = db.AppendMessage(session.ID, "user", "msg2", nil, "")

	count, err := db.CountMessages(session.ID)
	if err != nil || count != 2 {
		t.Error("CountMessages failed")
	}
}

func TestCascadeDelete(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	session, _ := db.CreateSession(nil)
	msg, _ := db.AppendMessage(session.ID, "user", "Hello", nil, "")
	_ = db.DeleteSession(session.ID)

	_, err := db.GetMessage(msg.ID)
	if err != ErrNotFound {
		t.Error("message should be cascade deleted")
	}
}
