package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestKVSet(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	if err := db.KVSet("key1", "value1", 0); err != nil {
		t.Fatalf("KVSet failed: %v", err)
	}
	value, err := db.KVGet("key1")
	if err != nil || value != "value1" {
		t.Error("KVSet/KVGet failed")
	}
}

func TestKVSet_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_ = db.KVSet("key1", "value1", 0)
	_ = db.KVSet("key1", "value2", 0)
	value, _ := db.KVGet("key1")
	if value != "value2" {
		t.Error("KVSet overwrite failed")
	}
}

func TestKVGet_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_, err := db.KVGet("nonexistent")
	if err != ErrNotFound {
		t.Error("want ErrNotFound")
	}
}

func TestKVGet_Expired(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_ = db.KVSet("expired", "value", time.Nanosecond)
	time.Sleep(time.Millisecond)
	_, err := db.KVGet("expired")
	if err != ErrNotFound {
		t.Error("expired key should not be found")
	}
}

func TestKVDelete(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_ = db.KVSet("del_key", "value", 0)
	if err := db.KVDelete("del_key"); err != nil {
		t.Fatalf("KVDelete failed: %v", err)
	}
	_, err := db.KVGet("del_key")
	if err != ErrNotFound {
		t.Error("key should be deleted")
	}
}

func TestKVList(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_ = db.KVSet("prefix:a", "va", 0)
	_ = db.KVSet("prefix:b", "vb", 0)
	_ = db.KVSet("other:c", "vc", 0)

	result, err := db.KVList("prefix:")
	if err != nil || len(result) != 2 {
		t.Error("KVList failed")
	}
}

func TestKVCleanExpired(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	_ = db.KVSet("valid", "v", 0)
	_ = db.KVSet("exp1", "v", time.Nanosecond)
	_ = db.KVSet("exp2", "v", time.Nanosecond)
	time.Sleep(time.Millisecond)

	deleted, err := db.KVCleanExpired()
	if err != nil || deleted != 2 {
		t.Errorf("KVCleanExpired: deleted=%d, want 2", deleted)
	}
}

func TestKVExists(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()

	exists, _ := db.KVExists("nonexistent")
	if exists {
		t.Error("nonexistent key should not exist")
	}

	_ = db.KVSet("exists_key", "v", 0)
	exists, _ = db.KVExists("exists_key")
	if !exists {
		t.Error("exists_key should exist")
	}
}
