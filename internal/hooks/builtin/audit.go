package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"mote/internal/hooks"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// AuditRecord represents a single audit log entry.
type AuditRecord struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	HookType   hooks.HookType `json:"hook_type"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolID     string         `json:"tool_id,omitempty"`
	ParamHash  string         `json:"param_hash,omitempty"`
	ParamCount int            `json:"param_count,omitempty"`
	Duration   time.Duration  `json:"duration,omitempty"`
	Success    bool           `json:"success"`
	Error      string         `json:"error,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
}

// AuditStore defines the interface for storing audit records.
type AuditStore interface {
	// Store saves an audit record.
	Store(record *AuditRecord) error
	// Close releases resources.
	Close() error
}

// LogAuditStore stores audit records to a logger.
type LogAuditStore struct {
	logger zerolog.Logger
}

// NewLogAuditStore creates a new log-based audit store.
func NewLogAuditStore(logger *zerolog.Logger) *LogAuditStore {
	l := log.Logger
	if logger != nil {
		l = *logger
	}
	return &LogAuditStore{logger: l}
}

// Store implements AuditStore.
func (s *LogAuditStore) Store(record *AuditRecord) error {
	event := s.logger.Info()

	event = event.
		Str("audit_id", record.ID).
		Str("hook_type", string(record.HookType)).
		Time("timestamp", record.Timestamp).
		Bool("success", record.Success)

	if record.ToolName != "" {
		event = event.Str("tool_name", record.ToolName)
	}
	if record.ToolID != "" {
		event = event.Str("tool_id", record.ToolID)
	}
	if record.ParamCount > 0 {
		event = event.Int("param_count", record.ParamCount)
	}
	if record.Duration > 0 {
		event = event.Dur("duration", record.Duration)
	}
	if record.Error != "" {
		event = event.Str("error", record.Error)
	}
	if record.SessionID != "" {
		event = event.Str("session_id", record.SessionID)
	}

	event.Msg("audit record")
	return nil
}

// Close implements AuditStore.
func (s *LogAuditStore) Close() error {
	return nil
}

// MemoryAuditStore stores audit records in memory (useful for testing).
type MemoryAuditStore struct {
	records []*AuditRecord
	mu      sync.RWMutex
	maxSize int
}

// NewMemoryAuditStore creates a new in-memory audit store.
func NewMemoryAuditStore(maxSize int) *MemoryAuditStore {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemoryAuditStore{
		records: make([]*AuditRecord, 0),
		maxSize: maxSize,
	}
}

// Store implements AuditStore.
func (s *MemoryAuditStore) Store(record *AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest records if at capacity
	if len(s.records) >= s.maxSize {
		s.records = s.records[1:]
	}
	s.records = append(s.records, record)
	return nil
}

// Close implements AuditStore.
func (s *MemoryAuditStore) Close() error {
	return nil
}

// GetRecords returns all stored records.
func (s *MemoryAuditStore) GetRecords() []*AuditRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AuditRecord, len(s.records))
	copy(result, s.records)
	return result
}

// Clear removes all records.
func (s *MemoryAuditStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = s.records[:0]
}

// AuditConfig configures the audit hook.
type AuditConfig struct {
	// Store is the audit record store.
	Store AuditStore
	// IncludeParams indicates whether to include parameter details.
	IncludeParams bool
	// IncludeResults indicates whether to include result details.
	IncludeResults bool
}

// AuditHook provides tool call auditing functionality.
type AuditHook struct {
	store          AuditStore
	includeParams  bool
	includeResults bool
	counter        uint64
	mu             sync.Mutex
}

// NewAuditHook creates a new audit hook with the given configuration.
func NewAuditHook(cfg AuditConfig) *AuditHook {
	store := cfg.Store
	if store == nil {
		store = NewLogAuditStore(nil)
	}
	return &AuditHook{
		store:          store,
		includeParams:  cfg.IncludeParams,
		includeResults: cfg.IncludeResults,
	}
}

// Handler returns a hook handler that audits tool calls.
func (h *AuditHook) Handler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    50, // Medium priority
		Source:      "_builtin",
		Description: "Audits tool calls and session events",
		Enabled:     true,
		Handler:     h.handle,
	}
}

func (h *AuditHook) handle(_ context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	// Only audit tool calls and sessions
	switch hookCtx.Type {
	case hooks.HookBeforeToolCall:
		// Record tool call start in context data for duration calculation
		hookCtx.SetData("_audit_start", time.Now())

	case hooks.HookAfterToolCall:
		h.auditToolCall(hookCtx)

	case hooks.HookSessionCreate, hooks.HookSessionEnd:
		h.auditSession(hookCtx)
	}

	return hooks.ContinueResult(), nil
}

func (h *AuditHook) auditToolCall(hookCtx *hooks.Context) {
	if hookCtx.ToolCall == nil {
		return
	}

	h.mu.Lock()
	h.counter++
	recordID := fmt.Sprintf("audit-%d-%d", hookCtx.Timestamp.UnixNano(), h.counter)
	h.mu.Unlock()

	record := &AuditRecord{
		ID:        recordID,
		Timestamp: hookCtx.Timestamp,
		HookType:  hookCtx.Type,
		ToolName:  hookCtx.ToolCall.ToolName,
		ToolID:    hookCtx.ToolCall.ID,
		Duration:  hookCtx.ToolCall.Duration,
		Success:   hookCtx.ToolCall.Error == "",
		Error:     hookCtx.ToolCall.Error,
	}

	if hookCtx.ToolCall.Params != nil {
		record.ParamCount = len(hookCtx.ToolCall.Params)
		if h.includeParams {
			record.ParamHash = hashParams(hookCtx.ToolCall.Params)
		}
	}

	if hookCtx.Session != nil {
		record.SessionID = hookCtx.Session.ID
	}

	if err := h.store.Store(record); err != nil {
		log.Error().Err(err).Msg("failed to store audit record")
	}
}

func (h *AuditHook) auditSession(hookCtx *hooks.Context) {
	if hookCtx.Session == nil {
		return
	}

	h.mu.Lock()
	h.counter++
	recordID := fmt.Sprintf("audit-%d-%d", hookCtx.Timestamp.UnixNano(), h.counter)
	h.mu.Unlock()

	eventType := "session_start"
	if hookCtx.Type == hooks.HookSessionEnd {
		eventType = "session_end"
	}

	record := &AuditRecord{
		ID:        recordID,
		Timestamp: hookCtx.Timestamp,
		HookType:  hookCtx.Type,
		SessionID: hookCtx.Session.ID,
		Success:   true,
	}

	log.Debug().
		Str("event_type", eventType).
		Str("session_id", hookCtx.Session.ID).
		Msg("session audit")

	if err := h.store.Store(record); err != nil {
		log.Error().Err(err).Msg("failed to store audit record")
	}
}

// Close releases resources.
func (h *AuditHook) Close() error {
	return h.store.Close()
}

// RegisterAuditHooks registers audit hooks for tool calls.
func RegisterAuditHooks(manager *hooks.Manager, cfg AuditConfig) error {
	hook := NewAuditHook(cfg)

	// Register for tool call hooks
	hookTypes := []hooks.HookType{
		hooks.HookBeforeToolCall,
		hooks.HookAfterToolCall,
		hooks.HookSessionCreate,
		hooks.HookSessionEnd,
	}

	for _, hookType := range hookTypes {
		id := fmt.Sprintf("builtin:audit:%s", hookType)
		handler := hook.Handler(id)
		if err := manager.Register(hookType, handler); err != nil {
			return fmt.Errorf("failed to register audit hook for %s: %w", hookType, err)
		}
	}

	return nil
}

// hashParams creates a simple hash of parameters for auditing.
func hashParams(params map[string]any) string {
	// Create a simple summary instead of a real hash
	data, err := json.Marshal(params)
	if err != nil {
		return "error"
	}
	if len(data) > 100 {
		return fmt.Sprintf("len=%d", len(data))
	}
	return string(data)
}
