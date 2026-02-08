package storage

import (
	"database/sql"
	"fmt"
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

	// 打开数据库连接
	db, err := sql.Open("sqlite", expandedPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// 配置 SQLite
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}

	// 执行迁移
	if err := migrations.Run(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{DB: db, path: expandedPath}, nil
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
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
