package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"mote/internal/config"
	"mote/internal/storage/migrations"

	_ "modernc.org/sqlite"
)

// DB 封装数据库连接
type DB struct {
	*sql.DB
	path string
}

// Open 打开数据库连接
func Open(path string) (*DB, error) {
	// 展开路径
	expandedPath, err := config.ExpandPath(path)
	if err != nil {
		return nil, fmt.Errorf("expand path: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// Build DSN with _pragma parameters so that every new connection in
	// the pool is configured identically.  Previously PRAGMAs were set
	// via db.Exec() which only applies to one pooled connection — any
	// subsequent connections lacked WAL/busy_timeout, causing SQLITE_BUSY
	// errors under concurrent load (e.g. two chat windows).
	dsn := buildDSN(expandedPath)

	// 打开数据库连接
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Limit connection pool size.  SQLite allows only one concurrent
	// writer; keeping pool small prevents SQLITE_BUSY contention while
	// still allowing concurrent reads via WAL mode.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	// 执行迁移
	if err := migrations.Run(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{DB: db, path: expandedPath}, nil
}

// buildDSN constructs a modernc.org/sqlite DSN with _pragma parameters.
// This ensures every pooled connection inherits the same configuration.
func buildDSN(path string) string {
	v := url.Values{}
	v.Set("_pragma", "journal_mode=WAL")
	v.Add("_pragma", "foreign_keys=ON")
	v.Add("_pragma", "busy_timeout=30000") // 30s — generous for concurrent tool execution
	v.Add("_pragma", "synchronous=NORMAL") // Safe with WAL; reduces fsync pressure
	v.Add("_txlock", "immediate")          // Acquire write lock at BEGIN, fail fast instead of deadlock
	return path + "?" + v.Encode()
}

// Path 返回数据库文件路径
func (db *DB) Path() string {
	return db.path
}

// Tx 封装事务
type Tx struct {
	*sql.Tx
}

// Begin 开启事务
func (db *DB) Begin() (*Tx, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx}, nil
}

// WithTx 在事务中执行函数，自动处理提交或回滚
func (db *DB) WithTx(fn func(*Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
