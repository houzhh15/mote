package approval

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBroadcaster is a mock implementation of Broadcaster.
type mockBroadcaster struct {
	mu       sync.Mutex
	messages []broadcastMessage
	err      error
}

type broadcastMessage struct {
	Type string
	Data any
}

func (m *mockBroadcaster) BroadcastAll(messageType string, data any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, broadcastMessage{Type: messageType, Data: data})
	return nil
}

func TestNotifier_NotifyRequest(t *testing.T) {
	broadcaster := &mockBroadcaster{}
	notifier := NewNotifier(broadcaster)

	req := &ApprovalRequest{
		ID:        "test-id",
		ToolName:  "shell",
		Arguments: `{"command": "sudo apt update"}`,
		Reason:    "sudo requires approval",
		SessionID: "session-1",
		AgentID:   "agent-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	err := notifier.NotifyRequest(req)
	require.NoError(t, err)

	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()
	require.Len(t, broadcaster.messages, 1)
	assert.Equal(t, "approval_request", broadcaster.messages[0].Type)
	assert.Equal(t, req, broadcaster.messages[0].Data)
}

func TestNotifier_NotifyResolved(t *testing.T) {
	broadcaster := &mockBroadcaster{}
	notifier := NewNotifier(broadcaster)

	req := &ApprovalRequest{
		ID:       "test-id",
		ToolName: "shell",
	}

	result := &ApprovalResult{
		Approved:   true,
		Message:    "approved",
		ApprovedBy: "admin",
		DecidedAt:  time.Now(),
		Decision:   DecisionApproved,
	}

	err := notifier.NotifyResolved(req, result)
	require.NoError(t, err)

	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()
	require.Len(t, broadcaster.messages, 1)
	assert.Equal(t, "approval_resolved", broadcaster.messages[0].Type)

	payload, ok := broadcaster.messages[0].Data.(ApprovalResolvedPayload)
	require.True(t, ok)
	assert.Equal(t, "test-id", payload.ID)
	assert.True(t, payload.Approved)
	assert.Equal(t, "admin", payload.ApprovedBy)
}

func TestNotifier_BroadcastError(t *testing.T) {
	broadcaster := &mockBroadcaster{err: errors.New("broadcast failed")}
	notifier := NewNotifier(broadcaster)

	req := &ApprovalRequest{
		ID:       "test-id",
		ToolName: "shell",
	}

	err := notifier.NotifyRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast failed")
}

func TestNotifier_NilBroadcaster(t *testing.T) {
	notifier := NewNotifier(nil)

	req := &ApprovalRequest{
		ID:       "test-id",
		ToolName: "shell",
	}

	// Should not error, just skip
	err := notifier.NotifyRequest(req)
	assert.NoError(t, err)

	err = notifier.NotifyResolved(req, &ApprovalResult{Approved: true})
	assert.NoError(t, err)
}

func TestNewApprovalRequestMessage(t *testing.T) {
	req := &ApprovalRequest{
		ID:        "test-id",
		ToolName:  "shell",
		Arguments: `{"command": "test"}`,
	}

	msg := NewApprovalRequestMessage(req)

	assert.Equal(t, "approval_request", msg.Type)
	assert.Equal(t, req, msg.Data)
}

func TestNewApprovalResolvedMessage(t *testing.T) {
	req := &ApprovalRequest{ID: "test-id"}
	result := &ApprovalResult{
		Approved:   true,
		ApprovedBy: "admin",
		DecidedAt:  time.Now(),
	}

	msg := NewApprovalResolvedMessage(req, result)

	assert.Equal(t, "approval_resolved", msg.Type)
	assert.Equal(t, "test-id", msg.Data.ID)
	assert.True(t, msg.Data.Approved)
	assert.Equal(t, "admin", msg.Data.ApprovedBy)
}

func TestFormatApprovalRequestJSON(t *testing.T) {
	req := &ApprovalRequest{
		ID:       "test-id",
		ToolName: "shell",
	}

	json := FormatApprovalRequestJSON(req)

	assert.Contains(t, json, `"id":"test-id"`)
	assert.Contains(t, json, `"tool_name":"shell"`)
}

func TestFormatApprovalResultJSON(t *testing.T) {
	result := &ApprovalResult{
		Approved: true,
		Message:  "approved",
	}

	json := FormatApprovalResultJSON(result)

	assert.Contains(t, json, `"approved":true`)
	assert.Contains(t, json, `"message":"approved"`)
}
