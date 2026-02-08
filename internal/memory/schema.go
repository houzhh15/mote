package memory

import (
	"database/sql"
	"fmt"
)

// CreateMemoryTables creates the memory storage tables.
func CreateMemoryTables(db *sql.DB, enableFTS bool) error {
	// Main memories table with P0 embedding columns
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			metadata TEXT,
			source TEXT NOT NULL,
			session_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			embedding BLOB,
			embedding_model TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("create memories table: %w", err)
	}

	// Create indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// Create FTS5 virtual table if enabled
	if enableFTS {
		_, err = db.Exec(`
			CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
				id,
				content,
				tokenize='porter unicode61'
			)
		`)
		if err != nil {
			return fmt.Errorf("create FTS table: %w", err)
		}
	}

	return nil
}

// MigrateMemorySchema migrates existing tables to add new columns.
// This is safe to call multiple times - it will skip columns that already exist.
func MigrateMemorySchema(db *sql.DB) error {
	// P0: Add embedding columns if they don't exist
	migrations := []struct {
		column string
		ddl    string
	}{
		{"embedding", "ALTER TABLE memories ADD COLUMN embedding BLOB"},
		{"embedding_model", "ALTER TABLE memories ADD COLUMN embedding_model TEXT"},
		// P1: Chunk metadata columns (for future use)
		{"chunk_index", "ALTER TABLE memories ADD COLUMN chunk_index INTEGER"},
		{"chunk_total", "ALTER TABLE memories ADD COLUMN chunk_total INTEGER"},
		{"source_file", "ALTER TABLE memories ADD COLUMN source_file TEXT"},
		{"source_line_start", "ALTER TABLE memories ADD COLUMN source_line_start INTEGER"},
		{"source_line_end", "ALTER TABLE memories ADD COLUMN source_line_end INTEGER"},
		// P2: Auto-capture/recall classification columns
		{"category", "ALTER TABLE memories ADD COLUMN category TEXT DEFAULT 'other'"},
		{"importance", "ALTER TABLE memories ADD COLUMN importance REAL DEFAULT 0.7"},
		{"capture_method", "ALTER TABLE memories ADD COLUMN capture_method TEXT DEFAULT 'manual'"},
	}

	for _, m := range migrations {
		if !columnExists(db, "memories", m.column) {
			if _, err := db.Exec(m.ddl); err != nil {
				return fmt.Errorf("migrate column %s: %w", m.column, err)
			}
		}
	}

	// Create index on source_file for P1
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_source_file ON memories(source_file)`)

	// Create index on category for P2
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category)`)

	return nil
}

// columnExists checks if a column exists in a table.
func columnExists(db *sql.DB, table, column string) bool {
	query := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table)
	var count int
	if err := db.QueryRow(query, column).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
