// Package approval provides audit logging for approval events.
package approval

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogEntry represents a single audit log entry.
type LogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	EventType  string    `json:"event_type"` // "request" or "decision"
	RequestID  string    `json:"request_id"`
	ToolName   string    `json:"tool_name"`
	Arguments  string    `json:"arguments,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	AgentID    string    `json:"agent_id,omitempty"`
	Decision   string    `json:"decision,omitempty"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	Message    string    `json:"message,omitempty"`
}

// FileLogger implements ApprovalLogger using JSON lines file.
type FileLogger struct {
	mu   sync.Mutex
	path string
	file *os.File
}

// NewFileLogger creates a new file-based logger.
func NewFileLogger(path string) (*FileLogger, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &FileLogger{
		path: path,
		file: file,
	}, nil
}

// LogRequest logs an approval request event.
func (l *FileLogger) LogRequest(req *ApprovalRequest) error {
	if l == nil || l.file == nil {
		return nil
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		EventType: "request",
		RequestID: req.ID,
		ToolName:  req.ToolName,
		Arguments: req.Arguments,
		Reason:    req.Reason,
		SessionID: req.SessionID,
		AgentID:   req.AgentID,
	}

	return l.writeEntry(entry)
}

// LogDecision logs an approval decision event.
func (l *FileLogger) LogDecision(req *ApprovalRequest, result *ApprovalResult) error {
	if l == nil || l.file == nil {
		return nil
	}

	entry := LogEntry{
		Timestamp:  time.Now(),
		EventType:  "decision",
		RequestID:  req.ID,
		ToolName:   req.ToolName,
		Decision:   string(result.Decision),
		ApprovedBy: result.ApprovedBy,
		Message:    result.Message,
		SessionID:  req.SessionID,
		AgentID:    req.AgentID,
	}

	return l.writeEntry(entry)
}

// writeEntry writes a log entry to the file.
func (l *FileLogger) writeEntry(entry LogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("logger: failed to marshal log entry", "error", err)
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	if _, err := l.file.Write(append(data, '\n')); err != nil {
		slog.Error("logger: failed to write log entry", "error", err)
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// Close closes the log file.
func (l *FileLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}

// Path returns the log file path.
func (l *FileLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// MemoryLogger implements ApprovalLogger using in-memory storage.
// Useful for testing and short-lived sessions.
type MemoryLogger struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

// NewMemoryLogger creates a new in-memory logger with optional size limit.
func NewMemoryLogger(maxSize int) *MemoryLogger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemoryLogger{
		entries: make([]LogEntry, 0),
		maxSize: maxSize,
	}
}

// LogRequest logs an approval request event.
func (l *MemoryLogger) LogRequest(req *ApprovalRequest) error {
	if l == nil {
		return nil
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		EventType: "request",
		RequestID: req.ID,
		ToolName:  req.ToolName,
		Arguments: req.Arguments,
		Reason:    req.Reason,
		SessionID: req.SessionID,
		AgentID:   req.AgentID,
	}

	return l.addEntry(entry)
}

// LogDecision logs an approval decision event.
func (l *MemoryLogger) LogDecision(req *ApprovalRequest, result *ApprovalResult) error {
	if l == nil {
		return nil
	}

	entry := LogEntry{
		Timestamp:  time.Now(),
		EventType:  "decision",
		RequestID:  req.ID,
		ToolName:   req.ToolName,
		Decision:   string(result.Decision),
		ApprovedBy: result.ApprovedBy,
		Message:    result.Message,
		SessionID:  req.SessionID,
		AgentID:    req.AgentID,
	}

	return l.addEntry(entry)
}

// addEntry adds an entry with size limit enforcement.
func (l *MemoryLogger) addEntry(entry LogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Enforce size limit by removing oldest entries
	if len(l.entries) >= l.maxSize {
		l.entries = l.entries[1:]
	}

	l.entries = append(l.entries, entry)
	return nil
}

// Entries returns all logged entries.
func (l *MemoryLogger) Entries() []LogEntry {
	if l == nil {
		return nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

// Clear removes all logged entries.
func (l *MemoryLogger) Clear() {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = l.entries[:0]
}

// Count returns the number of logged entries.
func (l *MemoryLogger) Count() int {
	if l == nil {
		return 0
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	return len(l.entries)
}

// NopLogger is a no-op logger that discards all entries.
type NopLogger struct{}

// NewNopLogger creates a no-op logger.
func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

// LogRequest is a no-op.
func (l *NopLogger) LogRequest(_ *ApprovalRequest) error {
	return nil
}

// LogDecision is a no-op.
func (l *NopLogger) LogDecision(_ *ApprovalRequest, _ *ApprovalResult) error {
	return nil
}
