package storage

import (
	"database/sql"
	"errors"
	"time"
)

// KVSet 设置键值，ttl 为 0 表示永不过期
func (db *DB) KVSet(key, value string, ttl time.Duration) error {
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}

	_, err := db.Exec(
		"INSERT OR REPLACE INTO kv_store (key, value, expires_at) VALUES (?, ?, ?)",
		key, value, expiresAt,
	)
	return err
}

// KVGet 获取键值
func (db *DB) KVGet(key string) (string, error) {
	var value string
	var expiresAt sql.NullTime

	err := db.QueryRow(
		"SELECT value, expires_at FROM kv_store WHERE key = ?",
		key,
	).Scan(&value, &expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}

	// 检查是否过期
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		// 过期了，删除并返回 not found
		db.Exec("DELETE FROM kv_store WHERE key = ?", key)
		return "", ErrNotFound
	}

	return value, nil
}

// KVDelete 删除键值
func (db *DB) KVDelete(key string) error {
	result, err := db.Exec("DELETE FROM kv_store WHERE key = ?", key)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// KVList 按前缀列出键值对
func (db *DB) KVList(prefix string) (map[string]string, error) {
	rows, err := db.Query(
		"SELECT key, value, expires_at FROM kv_store WHERE key LIKE ? || '%'",
		prefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	result := make(map[string]string)

	for rows.Next() {
		var key, value string
		var expiresAt sql.NullTime

		if err := rows.Scan(&key, &value, &expiresAt); err != nil {
			return nil, err
		}

		// 跳过已过期的
		if expiresAt.Valid && expiresAt.Time.Before(now) {
			continue
		}

		result[key] = value
	}

	return result, rows.Err()
}

// KVCleanExpired 清理过期的键值对
func (db *DB) KVCleanExpired() (int64, error) {
	result, err := db.Exec(
		"DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?",
		time.Now(),
	)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// KVExists 检查键是否存在且未过期
func (db *DB) KVExists(key string) (bool, error) {
	var expiresAt sql.NullTime

	err := db.QueryRow(
		"SELECT expires_at FROM kv_store WHERE key = ?",
		key,
	).Scan(&expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// 检查是否过期
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return false, nil
	}

	return true, nil
}
