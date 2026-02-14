package migrations

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// 首次执行迁移
	if err := Run(db); err != nil {
		t.Fatalf("first migration run: %v", err)
	}

	// 验证版本 (should be the number of migration scripts)
	version, err := Version(db)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	// Currently we have 6 migrations: 001_init.sql, 002_cron.sql, 003_session_model.sql, 004_session_title.sql, 005_session_skills.sql, 006_cron_workspace.sql
	expectedVersion := 6
	if version != expectedVersion {
		t.Errorf("version = %d, want %d", version, expectedVersion)
	}

	// 验证表已创建
	tables := []string{"sessions", "messages", "kv_store", "_migrations"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestRun_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// 执行两次迁移
	if err := Run(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := Run(db); err != nil {
		t.Fatalf("second run: %v", err)
	}

	// 版本应该不变
	version, err := Version(db)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	// Should be the number of migration scripts
	expectedVersion := 6
	if version != expectedVersion {
		t.Errorf("version = %d, want %d", version, expectedVersion)
	}

	// 确认只有对应数量的迁移记录
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != expectedVersion {
		t.Errorf("migration count = %d, want %d", count, expectedVersion)
	}
}

func TestPending(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// 创建迁移表但不执行迁移
	if err := ensureMigrationsTable(db); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}

	// 应该有待执行的迁移
	pending, err := Pending(db)
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	// Number of migration scripts
	expectedPending := 6
	if len(pending) != expectedPending {
		t.Errorf("pending count = %d, want %d", len(pending), expectedPending)
	}

	// 执行迁移后
	if err := Run(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pending, err = Pending(db)
	if err != nil {
		t.Fatalf("get pending after run: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending count after run = %d, want 0", len(pending))
	}
}

func TestVersion_EmptyDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// 创建迁移表
	if err := ensureMigrationsTable(db); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}

	// 空数据库版本应该是 0
	version, err := Version(db)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version != 0 {
		t.Errorf("version = %d, want 0", version)
	}
}
