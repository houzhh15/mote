package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 验证路径
	if db.Path() != dbPath {
		t.Errorf("Path() = %q, want %q", db.Path(), dbPath)
	}

	// 验证可以查询
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Errorf("query failed: %v", err)
	}
	if result != 1 {
		t.Errorf("result = %d, want 1", result)
	}
}

func TestOpen_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if db.Path() != dbPath {
		t.Errorf("Path() = %q, want %q", db.Path(), dbPath)
	}
}

func TestOpen_WALMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var fkEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Errorf("foreign_keys = %d, want 1", fkEnabled)
	}
}

func TestWithTx_Commit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 在事务中插入数据
	err = db.WithTx(func(tx *Tx) error {
		_, err := tx.Exec("INSERT INTO kv_store (key, value) VALUES (?, ?)", "test_key", "test_value")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx failed: %v", err)
	}

	// 验证数据已提交
	var value string
	if err := db.QueryRow("SELECT value FROM kv_store WHERE key = ?", "test_key").Scan(&value); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if value != "test_value" {
		t.Errorf("value = %q, want test_value", value)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 在事务中插入数据后返回错误
	testErr := errors.New("test error")
	err = db.WithTx(func(tx *Tx) error {
		_, err := tx.Exec("INSERT INTO kv_store (key, value) VALUES (?, ?)", "rollback_key", "rollback_value")
		if err != nil {
			return err
		}
		return testErr
	})
	if err != testErr {
		t.Errorf("WithTx error = %v, want %v", err, testErr)
	}

	// 验证数据已回滚
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM kv_store WHERE key = ?", "rollback_key").Scan(&count); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (should be rolled back)", count)
	}
}

func TestBegin(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	_, err = tx.Exec("INSERT INTO kv_store (key, value) VALUES (?, ?)", "manual_key", "manual_value")
	if err != nil {
		tx.Rollback()
		t.Fatalf("insert failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// 验证数据
	var value string
	if err := db.QueryRow("SELECT value FROM kv_store WHERE key = ?", "manual_key").Scan(&value); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if value != "manual_value" {
		t.Errorf("value = %q, want manual_value", value)
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 关闭后查询应该失败
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err == nil {
		t.Error("query should fail after close")
	}
}
