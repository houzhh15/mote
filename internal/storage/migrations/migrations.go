package migrations

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Run 执行所有待执行的迁移
func Run(db *sql.DB) error {
	// 确保迁移表存在
	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// 获取已应用的版本
	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("get applied versions: %w", err)
	}

	// 获取所有迁移文件
	migrations, err := getMigrationFiles()
	if err != nil {
		return fmt.Errorf("get migration files: %w", err)
	}

	// 按版本号排序
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// 执行待执行的迁移
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		if err := executeMigration(db, m); err != nil {
			return fmt.Errorf("execute migration %d: %w", m.version, err)
		}
	}

	return nil
}

// Version 返回当前数据库版本
func Version(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM _migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Pending 返回待执行的迁移版本列表
func Pending(db *sql.DB) ([]int, error) {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return nil, err
	}

	migrations, err := getMigrationFiles()
	if err != nil {
		return nil, err
	}

	var pending []int
	for _, m := range migrations {
		if !applied[m.version] {
			pending = append(pending, m.version)
		}
	}

	sort.Ints(pending)
	return pending, nil
}

type migration struct {
	version int
	name    string
	content string
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func getAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM _migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}

	return applied, rows.Err()
}

func getMigrationFiles() ([]migration, error) {
	entries, err := fs.ReadDir(FS, "scripts")
	if err != nil {
		return nil, err
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			continue
		}

		// NOTE: embed.FS always uses forward slashes, even on Windows.
		// Do NOT use filepath.Join here as it would use backslashes on Windows.
		content, err := fs.ReadFile(FS, "scripts/"+entry.Name())
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migration{
			version: version,
			name:    entry.Name(),
			content: string(content),
		})
	}

	return migrations, nil
}

func parseVersion(filename string) (int, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	return strconv.Atoi(parts[0])
}

func executeMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 执行迁移 SQL
	if _, err := tx.Exec(m.content); err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}

	// 记录迁移版本
	if _, err := tx.Exec("INSERT INTO _migrations (version) VALUES (?)", m.version); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	return tx.Commit()
}
