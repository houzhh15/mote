package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound 表示记录不存在
var ErrNotFound = errors.New("not found")

// Session 会话实体
type Session struct {
	ID             string          `json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Metadata       json.RawMessage `json:"metadata"`
	Model          string          `json:"model"`           // 使用的模型
	Scenario       string          `json:"scenario"`        // 场景类型: chat/cron/channel
	SelectedSkills []string        `json:"selected_skills"` // 选中的技能列表，空表示全部
}

// CreateSession 创建新会话
// 可选参数: model (string), scenario (string)
func (db *DB) CreateSession(metadata json.RawMessage, opts ...interface{}) (*Session, error) {
	return db.CreateSessionWithID(uuid.New().String(), metadata, opts...)
}

// CreateSessionWithID 使用指定 ID 创建新会话
// 可选参数: model (string), scenario (string)
func (db *DB) CreateSessionWithID(id string, metadata json.RawMessage, opts ...interface{}) (*Session, error) {
	now := time.Now()

	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	// 解析可选参数
	model := ""
	scenario := "chat" // 默认为 chat 场景
	for i := 0; i < len(opts); i += 2 {
		if i+1 >= len(opts) {
			break
		}
		key, ok1 := opts[i].(string)
		val, ok2 := opts[i+1].(string)
		if !ok1 || !ok2 {
			continue
		}
		switch key {
		case "model":
			model = val
		case "scenario":
			scenario = val
		}
	}

	_, err := db.Exec(
		"INSERT INTO sessions (id, created_at, updated_at, metadata, model, scenario) VALUES (?, ?, ?, ?, ?, ?)",
		id, now, now, string(metadata), model, scenario,
	)
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
		Model:     model,
		Scenario:  scenario,
	}, nil
}

// CreateSession 在事务中创建会话
func (tx *Tx) CreateSession(metadata json.RawMessage) (*Session, error) {
	return tx.CreateSessionWithID(uuid.New().String(), metadata)
}

// CreateSessionWithID 在事务中使用指定 ID 创建会话
func (tx *Tx) CreateSessionWithID(id string, metadata json.RawMessage) (*Session, error) {
	now := time.Now()

	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	_, err := tx.Exec(
		"INSERT INTO sessions (id, created_at, updated_at, metadata) VALUES (?, ?, ?, ?)",
		id, now, now, string(metadata),
	)
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
	}, nil
}

// GetSession 获取会话
func (db *DB) GetSession(id string) (*Session, error) {
	var s Session
	var metadataStr string
	var selectedSkillsStr string

	err := db.QueryRow(
		"SELECT id, created_at, updated_at, metadata, COALESCE(model, ''), COALESCE(scenario, 'chat'), COALESCE(selected_skills, '') FROM sessions WHERE id = ?",
		id,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &metadataStr, &s.Model, &s.Scenario, &selectedSkillsStr)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.Metadata = json.RawMessage(metadataStr)
	s.SelectedSkills = parseSelectedSkills(selectedSkillsStr)
	return &s, nil
}

// UpdateSession 更新会话
func (db *DB) UpdateSession(id string, metadata json.RawMessage) error {
	now := time.Now()

	result, err := db.Exec(
		"UPDATE sessions SET metadata = ?, updated_at = ? WHERE id = ?",
		string(metadata), now, id,
	)
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

// UpdateSessionModel 更新会话的模型
func (db *DB) UpdateSessionModel(id string, model string) error {
	now := time.Now()

	result, err := db.Exec(
		"UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?",
		model, now, id,
	)
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

// DeleteSession 删除会话
func (db *DB) DeleteSession(id string) error {
	result, err := db.Exec("DELETE FROM sessions WHERE id = ?", id)
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

// ListSessions 列出会话
func (db *DB) ListSessions(limit, offset int) ([]*Session, error) {
	query := "SELECT id, created_at, updated_at, metadata, COALESCE(model, ''), COALESCE(scenario, 'chat'), COALESCE(selected_skills, '') FROM sessions ORDER BY updated_at DESC"
	args := []any{}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		var metadataStr string
		var selectedSkillsStr string

		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &metadataStr, &s.Model, &s.Scenario, &selectedSkillsStr); err != nil {
			return nil, err
		}

		s.Metadata = json.RawMessage(metadataStr)
		s.SelectedSkills = parseSelectedSkills(selectedSkillsStr)
		sessions = append(sessions, &s)
	}

	return sessions, rows.Err()
}

// UpdateSessionSkills 更新会话的选中技能列表
func (db *DB) UpdateSessionSkills(id string, skillIDs []string) error {
	now := time.Now()

	skillsStr := formatSelectedSkills(skillIDs)

	result, err := db.Exec(
		"UPDATE sessions SET selected_skills = ?, updated_at = ? WHERE id = ?",
		skillsStr, now, id,
	)
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

// parseSelectedSkills 从数据库字符串解析技能列表
// 空字符串返回nil（表示全部），JSON数组返回解析后的列表
func parseSelectedSkills(s string) []string {
	if s == "" {
		return nil
	}
	var skills []string
	if err := json.Unmarshal([]byte(s), &skills); err != nil {
		// Fallback: 尝试逗号分隔格式
		parts := strings.Split(s, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				skills = append(skills, p)
			}
		}
		return skills
	}
	return skills
}

// formatSelectedSkills 将技能列表格式化为数据库存储字符串
// nil或空列表返回空字符串（表示全部），非空列表返回JSON数组
func formatSelectedSkills(skillIDs []string) string {
	if len(skillIDs) == 0 {
		return ""
	}
	data, err := json.Marshal(skillIDs)
	if err != nil {
		return ""
	}
	return string(data)
}
