package storage

import (
"database/sql"
"encoding/json"
"time"

"github.com/google/uuid"
)

// Context represents a compressed conversation context
type Context struct {
ID               string    `json:"id"`
SessionID        string    `json:"session_id"`
Version          int       `json:"version"`
Summary          string    `json:"summary"`
KeptMessageIDs   []string  `json:"kept_message_ids"`
TotalTokens      int       `json:"total_tokens"`
OriginalTokens   int       `json:"original_tokens"`
CreatedAt        time.Time `json:"created_at"`
}

// SaveContext saves a compressed context to the database
func (db *DB) SaveContext(ctx *Context) error {
if ctx.ID == "" {
ctx.ID = uuid.New().String()
}
if ctx.CreatedAt.IsZero() {
ctx.CreatedAt = time.Now()
}

// Encode kept_message_ids as JSON
keptIDsJSON, err := json.Marshal(ctx.KeptMessageIDs)
if err != nil {
return err
}

_, err = db.Exec(`
INSERT INTO contexts (id, session_id, version, summary, kept_message_ids, total_tokens, original_tokens, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, ctx.ID, ctx.SessionID, ctx.Version, ctx.Summary, string(keptIDsJSON), ctx.TotalTokens, ctx.OriginalTokens, ctx.CreatedAt)

return err
}

// GetLatestContext retrieves the latest compressed context for a session
func (db *DB) GetLatestContext(sessionID string) (*Context, error) {
var ctx Context
var keptIDsJSON string

err := db.QueryRow(`
SELECT id, session_id, version, summary, kept_message_ids, total_tokens, original_tokens, created_at
FROM contexts
WHERE session_id = ?
ORDER BY version DESC
LIMIT 1
`, sessionID).Scan(&ctx.ID, &ctx.SessionID, &ctx.Version, &ctx.Summary, &keptIDsJSON, &ctx.TotalTokens, &ctx.OriginalTokens, &ctx.CreatedAt)

if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}

// Decode kept_message_ids from JSON
if keptIDsJSON != "" {
if err := json.Unmarshal([]byte(keptIDsJSON), &ctx.KeptMessageIDs); err != nil {
return nil, err
}
}

return &ctx, nil
}

// ListContexts retrieves all contexts for a session
func (db *DB) ListContexts(sessionID string, limit int) ([]*Context, error) {
if limit <= 0 {
limit = 10
}

rows, err := db.Query(`
SELECT id, session_id, version, summary, kept_message_ids, total_tokens, original_tokens, created_at
FROM contexts
WHERE session_id = ?
ORDER BY version DESC
LIMIT ?
`, sessionID, limit)
if err != nil {
return nil, err
}
defer rows.Close()

var contexts []*Context
for rows.Next() {
var ctx Context
var keptIDsJSON string

err := rows.Scan(&ctx.ID, &ctx.SessionID, &ctx.Version, &ctx.Summary, &keptIDsJSON, &ctx.TotalTokens, &ctx.OriginalTokens, &ctx.CreatedAt)
if err != nil {
return nil, err
}

// Decode kept_message_ids from JSON
if keptIDsJSON != "" {
if err := json.Unmarshal([]byte(keptIDsJSON), &ctx.KeptMessageIDs); err != nil {
return nil, err
}
}

contexts = append(contexts, &ctx)
}

return contexts, rows.Err()
}

// GetMaxContextVersion returns the maximum version number for a session's contexts
func (db *DB) GetMaxContextVersion(sessionID string) (int, error) {
var maxVersion sql.NullInt64
err := db.QueryRow(`
SELECT MAX(version) FROM contexts WHERE session_id = ?
`, sessionID).Scan(&maxVersion)

if err != nil {
return 0, err
}

if !maxVersion.Valid {
return 0, nil
}

return int(maxVersion.Int64), nil
}
