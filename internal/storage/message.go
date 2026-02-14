package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ToolCall 工具调用
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function json.RawMessage `json:"function"`
}

// GetName returns the name of the tool being called.
func (tc *ToolCall) GetName() string {
	if len(tc.Function) == 0 {
		return ""
	}
	var fn struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(tc.Function, &fn); err != nil {
		return ""
	}
	return fn.Name
}

// GetArguments returns the arguments of the tool call.
func (tc *ToolCall) GetArguments() string {
	if len(tc.Function) == 0 {
		return ""
	}
	var fn struct {
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(tc.Function, &fn); err != nil {
		return ""
	}
	return fn.Arguments
}

// Message 消息实体
type Message struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AppendMessage 添加消息
func (db *DB) AppendMessage(sessionID, role, content string, toolCalls []ToolCall, toolCallID string) (*Message, error) {
	id := uuid.New().String()
	now := time.Now()

	var toolCallsJSON *string
	if len(toolCalls) > 0 {
		data, err := json.Marshal(toolCalls)
		if err != nil {
			return nil, err
		}
		s := string(data)
		toolCallsJSON = &s
	}

	var toolCallIDPtr *string
	if toolCallID != "" {
		toolCallIDPtr = &toolCallID
	}

	_, err := db.Exec(
		"INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, sessionID, role, content, toolCallsJSON, toolCallIDPtr, now,
	)
	if err != nil {
		return nil, err
	}

	// Update session's updated_at timestamp
	_, _ = db.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", now, sessionID)

	return &Message{
		ID:         id,
		SessionID:  sessionID,
		Role:       role,
		Content:    content,
		ToolCalls:  toolCalls,
		ToolCallID: toolCallID,
		CreatedAt:  now,
	}, nil
}

// AppendMessage 在事务中添加消息
func (tx *Tx) AppendMessage(sessionID, role, content string, toolCalls []ToolCall, toolCallID string) (*Message, error) {
	id := uuid.New().String()
	now := time.Now()

	var toolCallsJSON *string
	if len(toolCalls) > 0 {
		data, err := json.Marshal(toolCalls)
		if err != nil {
			return nil, err
		}
		s := string(data)
		toolCallsJSON = &s
	}

	var toolCallIDPtr *string
	if toolCallID != "" {
		toolCallIDPtr = &toolCallID
	}

	_, err := tx.Exec(
		"INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, sessionID, role, content, toolCallsJSON, toolCallIDPtr, now,
	)
	if err != nil {
		return nil, err
	}

	// Update session's updated_at timestamp
	_, _ = tx.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", now, sessionID)

	return &Message{
		ID:         id,
		SessionID:  sessionID,
		Role:       role,
		Content:    content,
		ToolCalls:  toolCalls,
		ToolCallID: toolCallID,
		CreatedAt:  now,
	}, nil
}

// GetMessages 获取会话消息列表
func (db *DB) GetMessages(sessionID string, limit int) ([]*Message, error) {
	query := "SELECT id, session_id, role, content, tool_calls, tool_call_id, created_at FROM messages WHERE session_id = ? ORDER BY created_at ASC"
	args := []any{sessionID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var m Message
		var toolCallsJSON sql.NullString
		var toolCallID sql.NullString

		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &toolCallsJSON, &toolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}

		if toolCallsJSON.Valid {
			if err := json.Unmarshal([]byte(toolCallsJSON.String), &m.ToolCalls); err != nil {
				return nil, err
			}
		}

		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}

		messages = append(messages, &m)
	}

	return messages, rows.Err()
}

// DeleteMessage 删除消息
func (db *DB) DeleteMessage(id string) error {
	result, err := db.Exec("DELETE FROM messages WHERE id = ?", id)
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

// CountMessages 统计会话消息数
func (db *DB) CountMessages(sessionID string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&count)
	return count, err
}

// ReplaceMessages atomically replaces all messages for a session with new ones.
// This is used to persist compaction results: the old messages are deleted and
// the compacted messages are inserted in a single transaction.
func (db *DB) ReplaceMessages(sessionID string, messages []*Message) error {
	return db.WithTx(func(tx *Tx) error {
		// Delete all existing messages for this session
		if _, err := tx.Exec("DELETE FROM messages WHERE session_id = ?", sessionID); err != nil {
			return err
		}

		// Insert new (compacted) messages
		for _, msg := range messages {
			id := msg.ID
			if id == "" {
				id = uuid.New().String()
			}

			var toolCallsJSON *string
			if len(msg.ToolCalls) > 0 {
				data, err := json.Marshal(msg.ToolCalls)
				if err != nil {
					return err
				}
				s := string(data)
				toolCallsJSON = &s
			}

			var toolCallIDPtr *string
			if msg.ToolCallID != "" {
				toolCallIDPtr = &msg.ToolCallID
			}

			createdAt := msg.CreatedAt
			if createdAt.IsZero() {
				createdAt = time.Now()
			}

			if _, err := tx.Exec(
				"INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
				id, sessionID, msg.Role, msg.Content, toolCallsJSON, toolCallIDPtr, createdAt,
			); err != nil {
				return err
			}
		}

		// Update session timestamp
		_, _ = tx.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", time.Now(), sessionID)
		return nil
	})
}

// GetMessage 获取单条消息
func (db *DB) GetMessage(id string) (*Message, error) {
	var m Message
	var toolCallsJSON sql.NullString
	var toolCallID sql.NullString

	err := db.QueryRow(
		"SELECT id, session_id, role, content, tool_calls, tool_call_id, created_at FROM messages WHERE id = ?",
		id,
	).Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &toolCallsJSON, &toolCallID, &m.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if toolCallsJSON.Valid {
		if err := json.Unmarshal([]byte(toolCallsJSON.String), &m.ToolCalls); err != nil {
			return nil, err
		}
	}

	if toolCallID.Valid {
		m.ToolCallID = toolCallID.String
	}

	return &m, nil
}
