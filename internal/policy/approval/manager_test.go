package approval

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mote/internal/policy"
)

// mockNotifier is a mock implementation of ApprovalNotifier.
type mockNotifier struct {
	mu          sync.Mutex
	requests    []*ApprovalRequest
	resolutions []*ApprovalRequest
}

func (m *mockNotifier) NotifyRequest(req *ApprovalRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	return nil
}

func (m *mockNotifier) NotifyResolved(req *ApprovalRequest, result *ApprovalResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolutions = append(m.resolutions, req)
	return nil
}

// mockLogger is a mock implementation of ApprovalLogger.
type mockLogger struct {
	mu        sync.Mutex
	requests  []*ApprovalRequest
	decisions []*ApprovalResult
}

func (m *mockLogger) LogRequest(req *ApprovalRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
}

func (m *mockLogger) LogDecision(req *ApprovalRequest, result *ApprovalResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisions = append(m.decisions, result)
}

func TestManager_RequestApproval_Approve(t *testing.T) {
	notifier := &mockNotifier{}
	logger := &mockLogger{}

	manager := NewManager(&ManagerConfig{
		Notifier:   notifier,
		Logger:     logger,
		Timeout:    5 * time.Second,
		MaxPending: 10,
	})
	defer manager.Close()

	call := &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command": "sudo apt update"}`,
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	var result *ApprovalResult
	var err error
	done := make(chan struct{})

	go func() {
		result, err = manager.RequestApproval(context.Background(), call, "sudo requires approval")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	pending := manager.ListPending()
	require.Len(t, pending, 1)

	err2 := manager.HandleResponse(pending[0].ID, true, "approved by admin")
	require.NoError(t, err2)

	<-done
	require.NoError(t, err)
	assert.True(t, result.Approved)
	assert.Equal(t, DecisionApproved, result.Decision)
	assert.Equal(t, "approved by admin", result.Message)

	assert.Len(t, notifier.requests, 1)
	assert.Len(t, notifier.resolutions, 1)
	assert.Len(t, logger.requests, 1)
	assert.Len(t, logger.decisions, 1)
}

func TestManager_RequestApproval_Reject(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 10,
	})
	defer manager.Close()

	call := &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command": "rm -rf /"}`,
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	var result *ApprovalResult
	done := make(chan struct{})

	go func() {
		result, _ = manager.RequestApproval(context.Background(), call, "dangerous operation")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	pending := manager.ListPending()
	require.Len(t, pending, 1)

	err := manager.HandleResponse(pending[0].ID, false, "denied by admin")
	require.NoError(t, err)

	<-done
	assert.False(t, result.Approved)
	assert.Equal(t, DecisionRejected, result.Decision)
}

func TestManager_RequestApproval_Timeout(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    100 * time.Millisecond,
		MaxPending: 10,
	})
	defer manager.Close()

	call := &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command": "sudo apt update"}`,
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	result, err := manager.RequestApproval(context.Background(), call, "test timeout")
	require.NoError(t, err)

	assert.False(t, result.Approved)
	assert.Equal(t, DecisionTimeout, result.Decision)
	assert.Contains(t, result.Message, "timed out")
}

func TestManager_RequestApproval_ContextCancel(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 10,
	})
	defer manager.Close()

	call := &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command": "test"}`,
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	ctx, cancel := context.WithCancel(context.Background())

	var result *ApprovalResult
	var err error
	done := make(chan struct{})

	go func() {
		result, err = manager.RequestApproval(ctx, call, "test cancel")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	<-done
	assert.Error(t, err)
	assert.False(t, result.Approved)
	assert.Equal(t, DecisionRejected, result.Decision)
}

func TestManager_RequestApproval_MaxPending(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 2,
	})
	defer manager.Close()

	for i := 0; i < 2; i++ {
		go func() {
			call := &policy.ToolCall{Name: "shell", SessionID: "session"}
			manager.RequestApproval(context.Background(), call, "test")
		}()
	}

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, manager.PendingCount())

	call := &policy.ToolCall{Name: "shell", SessionID: "session"}
	_, err := manager.RequestApproval(context.Background(), call, "test")
	assert.ErrorIs(t, err, policy.ErrMaxPendingExceeded)
}

func TestManager_HandleResponse_NotFound(t *testing.T) {
	manager := NewManager(nil)
	defer manager.Close()

	err := manager.HandleResponse("nonexistent-id", true, "")
	assert.ErrorIs(t, err, policy.ErrRequestNotFound)
}

func TestManager_GetPending(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 10,
	})
	defer manager.Close()

	call := &policy.ToolCall{
		Name:      "shell",
		SessionID: "session-1",
		AgentID:   "agent-1",
	}

	go func() {
		manager.RequestApproval(context.Background(), call, "test")
	}()

	time.Sleep(50 * time.Millisecond)

	pending := manager.ListPending()
	require.Len(t, pending, 1)

	req, ok := manager.GetPending(pending[0].ID)
	assert.True(t, ok)
	assert.Equal(t, "shell", req.ToolName)

	_, ok = manager.GetPending("nonexistent")
	assert.False(t, ok)
}

func TestManager_Close(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 10,
	})

	call := &policy.ToolCall{Name: "shell", SessionID: "session"}

	var result *ApprovalResult
	done := make(chan struct{})

	go func() {
		result, _ = manager.RequestApproval(context.Background(), call, "test")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, manager.PendingCount())

	manager.Close()

	<-done
	assert.False(t, result.Approved)
	assert.Equal(t, 0, manager.PendingCount())
}

func TestManager_ConcurrentAccess(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		Timeout:    5 * time.Second,
		MaxPending: 100,
	})
	defer manager.Close()

	var wg sync.WaitGroup
	const numRequests = 20

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			call := &policy.ToolCall{
				Name:      "shell",
				SessionID: "session",
				AgentID:   "agent",
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			manager.RequestApproval(ctx, call, "test")
		}(i)
	}

	time.Sleep(30 * time.Millisecond)
	pending := manager.ListPending()
	for _, req := range pending[:len(pending)/2] {
		go func(id string) {
			manager.HandleResponse(id, true, "approved")
		}(req.ID)
	}

	wg.Wait()
	assert.Equal(t, 0, manager.PendingCount())
}
