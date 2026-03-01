package delegate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DelegationTracker records delegation invocations for audit and monitoring.
type DelegationTracker struct {
	db *sql.DB
}

// DelegationRecord represents a single delegation invocation.
type DelegationRecord struct {
	ID              string     `json:"id"`
	ParentSessionID string     `json:"parent_session_id"`
	ChildSessionID  string     `json:"child_session_id"`
	AgentName       string     `json:"agent_name"`
	Depth           int        `json:"depth"`
	Chain           string     `json:"chain"` // JSON array: ["main","researcher"]
	Prompt          string     `json:"prompt"`
	Status          string     `json:"status"` // "running", "completed", "failed", "timeout"
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ResultLength    int        `json:"result_length"`
	TokensUsed      int        `json:"tokens_used"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	Mode            string     `json:"mode"`           // "legacy" or "structured"
	ExecutedSteps   []string   `json:"executed_steps"` // PDA step labels executed
	PDAStackDepth   int        `json:"pda_stack_depth"`
}

// NewTracker creates a new DelegationTracker.
func NewTracker(db *sql.DB) *DelegationTracker {
	return &DelegationTracker{db: db}
}

// StartDelegation records the start of a delegation invocation.
func (t *DelegationTracker) StartDelegation(record DelegationRecord) error {
	mode := record.Mode
	if mode == "" {
		mode = "legacy"
	}
	_, err := t.db.Exec(`
		INSERT INTO delegate_invocations 
		(id, parent_session_id, child_session_id, agent_name, depth, chain, prompt, status, started_at, mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.ParentSessionID, record.ChildSessionID,
		record.AgentName, record.Depth, record.Chain, record.Prompt,
		"running", record.StartedAt, mode)
	return err
}

// CompleteDelegation updates a delegation record with completion status.
func (t *DelegationTracker) CompleteDelegation(
	id string,
	status string,
	resultLength int,
	tokensUsed int,
	errorMsg *string,
) error {
	return t.CompleteDelegationWithPDA(id, status, resultLength, tokensUsed, errorMsg, nil, 0)
}

// CompleteDelegationWithPDA updates a delegation record with PDA execution details.
func (t *DelegationTracker) CompleteDelegationWithPDA(
	id string,
	status string,
	resultLength int,
	tokensUsed int,
	errorMsg *string,
	executedSteps []string,
	pdaStackDepth int,
) error {
	now := time.Now()
	var stepsJSON *string
	if len(executedSteps) > 0 {
		data, _ := json.Marshal(executedSteps)
		s := string(data)
		stepsJSON = &s
	}
	_, err := t.db.Exec(`
		UPDATE delegate_invocations 
		SET status = ?, completed_at = ?, result_length = ?, tokens_used = ?, 
		    error_message = ?, executed_steps = ?, pda_stack_depth = ?
		WHERE id = ?`,
		status, now, resultLength, tokensUsed, errorMsg, stepsJSON, pdaStackDepth, id)
	return err
}

// GetByParentSession returns all delegation records for a parent session.
func (t *DelegationTracker) GetByParentSession(parentSessionID string) ([]DelegationRecord, error) {
	rows, err := t.db.Query(`
		SELECT id, parent_session_id, child_session_id, agent_name, depth, chain, prompt,
		       status, started_at, completed_at, result_length, tokens_used, error_message
		FROM delegate_invocations
		WHERE parent_session_id = ?
		ORDER BY started_at ASC`, parentSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// GetByID returns a single delegation record by ID.
func (t *DelegationTracker) GetByID(id string) (*DelegationRecord, error) {
	row := t.db.QueryRow(`
		SELECT id, parent_session_id, child_session_id, agent_name, depth, chain, prompt,
		       status, started_at, completed_at, result_length, tokens_used, error_message
		FROM delegate_invocations
		WHERE id = ?`, id)

	var r DelegationRecord
	var completedAt sql.NullTime
	var errorMsg sql.NullString
	err := row.Scan(
		&r.ID, &r.ParentSessionID, &r.ChildSessionID, &r.AgentName,
		&r.Depth, &r.Chain, &r.Prompt, &r.Status, &r.StartedAt,
		&completedAt, &r.ResultLength, &r.TokensUsed, &errorMsg)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		r.ErrorMessage = &errorMsg.String
	}
	return &r, nil
}

// GetRecent returns the most recent delegation records across all sessions,
// ordered by started_at descending.  limit controls the maximum number of
// records returned (capped at 200).
func (t *DelegationTracker) GetRecent(limit int) ([]DelegationRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := t.db.Query(`
		SELECT id, parent_session_id, child_session_id, agent_name, depth, chain, prompt,
		       status, started_at, completed_at, result_length, tokens_used, error_message
		FROM delegate_invocations
		ORDER BY started_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// DeleteByIDs deletes delegation records by their IDs.
// Returns the number of rows deleted.
func (t *DelegationTracker) DeleteByIDs(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := "(" + strings.Join(placeholders, ",") + ")"
	result, err := t.db.Exec("DELETE FROM delegate_invocations WHERE id IN "+inClause, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ChainToJSON converts a string slice to a JSON array string.
func ChainToJSON(chain []string) string {
	data, _ := json.Marshal(chain)
	return string(data)
}

// GenerateInvocationID creates a unique ID for a delegation invocation.
func GenerateInvocationID(parentSession, agentName string) string {
	return fmt.Sprintf("dlg_%s_%s_%d", parentSession[:min(8, len(parentSession))], agentName, time.Now().UnixMilli())
}

func scanRecords(rows *sql.Rows) ([]DelegationRecord, error) {
	var records []DelegationRecord
	for rows.Next() {
		var r DelegationRecord
		var completedAt sql.NullTime
		var errorMsg sql.NullString
		err := rows.Scan(
			&r.ID, &r.ParentSessionID, &r.ChildSessionID, &r.AgentName,
			&r.Depth, &r.Chain, &r.Prompt, &r.Status, &r.StartedAt,
			&completedAt, &r.ResultLength, &r.TokensUsed, &errorMsg)
		if err != nil {
			return nil, err
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		if errorMsg.Valid {
			r.ErrorMessage = &errorMsg.String
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
