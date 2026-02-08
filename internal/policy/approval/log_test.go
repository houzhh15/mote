package approval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLogger_LogRequest(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	logger, err := NewFileLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	req := &ApprovalRequest{
		ID:        "req-1",
		ToolName:  "shell",
		Arguments: `{"command": "sudo apt update"}`,
		Reason:    "sudo requires approval",
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	err = logger.LogRequest(req)
	require.NoError(t, err)

	// Verify file content
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var entry LogEntry
	err = json.Unmarshal(content, &entry)
	require.NoError(t, err)

	assert.Equal(t, "request", entry.EventType)
	assert.Equal(t, "req-1", entry.RequestID)
	assert.Equal(t, "shell", entry.ToolName)
	assert.Equal(t, "sudo requires approval", entry.Reason)
}

func TestFileLogger_LogDecision(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	logger, err := NewFileLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	req := &ApprovalRequest{
		ID:        "req-1",
		ToolName:  "shell",
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	result := &ApprovalResult{
		Approved:   true,
		Decision:   DecisionApproved,
		ApprovedBy: "admin",
		Message:    "approved by admin",
	}

	err = logger.LogDecision(req, result)
	require.NoError(t, err)

	// Verify file content
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var entry LogEntry
	err = json.Unmarshal(content, &entry)
	require.NoError(t, err)

	assert.Equal(t, "decision", entry.EventType)
	assert.Equal(t, "req-1", entry.RequestID)
	assert.Equal(t, "approved", entry.Decision)
	assert.Equal(t, "admin", entry.ApprovedBy)
}

func TestFileLogger_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	logger, err := NewFileLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	// Log multiple entries
	for i := 0; i < 3; i++ {
		req := &ApprovalRequest{
			ID:       "req-" + string(rune('1'+i)),
			ToolName: "shell",
		}
		err = logger.LogRequest(req)
		require.NoError(t, err)
	}

	// Verify line count
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 3)
}

func TestFileLogger_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nested", "dir", "audit.jsonl")

	logger, err := NewFileLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	assert.FileExists(t, logPath)
}

func TestFileLogger_NilLogger(t *testing.T) {
	var logger *FileLogger

	err := logger.LogRequest(&ApprovalRequest{ID: "test"})
	assert.NoError(t, err)

	err = logger.LogDecision(&ApprovalRequest{ID: "test"}, &ApprovalResult{})
	assert.NoError(t, err)
}

func TestFileLogger_Path(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	logger, err := NewFileLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	assert.Equal(t, logPath, logger.Path())

	var nilLogger *FileLogger
	assert.Empty(t, nilLogger.Path())
}

func TestMemoryLogger_LogRequest(t *testing.T) {
	logger := NewMemoryLogger(100)

	req := &ApprovalRequest{
		ID:        "req-1",
		ToolName:  "shell",
		Arguments: `{"command": "test"}`,
		Reason:    "requires approval",
	}

	err := logger.LogRequest(req)
	require.NoError(t, err)

	entries := logger.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "request", entries[0].EventType)
	assert.Equal(t, "req-1", entries[0].RequestID)
}

func TestMemoryLogger_LogDecision(t *testing.T) {
	logger := NewMemoryLogger(100)

	req := &ApprovalRequest{
		ID:       "req-1",
		ToolName: "shell",
	}

	result := &ApprovalResult{
		Approved:   true,
		Decision:   DecisionApproved,
		ApprovedBy: "admin",
	}

	err := logger.LogDecision(req, result)
	require.NoError(t, err)

	entries := logger.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "decision", entries[0].EventType)
	assert.Equal(t, "approved", entries[0].Decision)
}

func TestMemoryLogger_SizeLimit(t *testing.T) {
	logger := NewMemoryLogger(5)

	// Log 10 entries
	for i := 0; i < 10; i++ {
		req := &ApprovalRequest{
			ID:       "req-" + string(rune('0'+i)),
			ToolName: "shell",
		}
		_ = logger.LogRequest(req)
	}

	// Should only have 5 entries
	assert.Equal(t, 5, logger.Count())

	// Should have the last 5 entries (req-5 to req-9)
	entries := logger.Entries()
	assert.Equal(t, "req-5", entries[0].RequestID)
}

func TestMemoryLogger_DefaultMaxSize(t *testing.T) {
	logger := NewMemoryLogger(0) // Should default to 1000
	assert.NotNil(t, logger)
}

func TestMemoryLogger_Clear(t *testing.T) {
	logger := NewMemoryLogger(100)

	_ = logger.LogRequest(&ApprovalRequest{ID: "req-1"})
	_ = logger.LogRequest(&ApprovalRequest{ID: "req-2"})

	assert.Equal(t, 2, logger.Count())

	logger.Clear()
	assert.Equal(t, 0, logger.Count())
}

func TestMemoryLogger_NilLogger(t *testing.T) {
	var logger *MemoryLogger

	err := logger.LogRequest(&ApprovalRequest{ID: "test"})
	assert.NoError(t, err)

	err = logger.LogDecision(&ApprovalRequest{ID: "test"}, &ApprovalResult{})
	assert.NoError(t, err)

	assert.Nil(t, logger.Entries())
	assert.Equal(t, 0, logger.Count())

	// Should not panic
	logger.Clear()
}

func TestMemoryLogger_Concurrent(t *testing.T) {
	logger := NewMemoryLogger(1000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := &ApprovalRequest{
				ID:       "req-" + string(rune(id)),
				ToolName: "shell",
			}
			_ = logger.LogRequest(req)
			_ = logger.Count()
			_ = logger.Entries()
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 100, logger.Count())
}

func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()

	err := logger.LogRequest(&ApprovalRequest{ID: "test"})
	assert.NoError(t, err)

	err = logger.LogDecision(&ApprovalRequest{ID: "test"}, &ApprovalResult{})
	assert.NoError(t, err)
}

func TestLogEntry_Timestamp(t *testing.T) {
	logger := NewMemoryLogger(100)

	before := time.Now()
	_ = logger.LogRequest(&ApprovalRequest{ID: "req-1"})
	after := time.Now()

	entries := logger.Entries()
	require.Len(t, entries, 1)

	assert.True(t, entries[0].Timestamp.After(before) || entries[0].Timestamp.Equal(before))
	assert.True(t, entries[0].Timestamp.Before(after) || entries[0].Timestamp.Equal(after))
}
