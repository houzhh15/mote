package approval

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"mote/internal/policy"
)

// pendingRequest holds the state for a pending approval request.
type pendingRequest struct {
	request *ApprovalRequest
	done    chan *ApprovalResult
	timer   *time.Timer
}

// Manager implements the ApprovalHandler interface.
type Manager struct {
	mu sync.RWMutex

	// pending holds all pending approval requests.
	pending map[string]*pendingRequest

	// notifier sends approval notifications.
	notifier ApprovalNotifier

	// logger records approval events.
	logger ApprovalLogger

	// slogger is the structured logger.
	slogger *slog.Logger

	// timeout is the default approval timeout.
	timeout time.Duration

	// maxPending is the maximum number of pending requests.
	maxPending int
}

// ManagerConfig configures the Manager.
type ManagerConfig struct {
	Notifier   ApprovalNotifier
	Logger     ApprovalLogger
	Timeout    time.Duration
	MaxPending int
}

// NewManager creates a new Manager.
func NewManager(config *ManagerConfig) *Manager {
	timeout := 5 * time.Minute
	maxPending := 100

	if config != nil {
		if config.Timeout > 0 {
			timeout = config.Timeout
		}
		if config.MaxPending > 0 {
			maxPending = config.MaxPending
		}
	}

	m := &Manager{
		pending:    make(map[string]*pendingRequest),
		slogger:    slog.Default(),
		timeout:    timeout,
		maxPending: maxPending,
	}

	if config != nil {
		m.notifier = config.Notifier
		m.logger = config.Logger
	}

	return m
}

// SetNotifier sets the notifier.
func (m *Manager) SetNotifier(n ApprovalNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = n
}

// SetLogger sets the logger.
func (m *Manager) SetLogger(l ApprovalLogger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = l
}

// RequestApproval creates a new approval request and waits for decision.
// Blocks until approved, rejected, or timeout.
func (m *Manager) RequestApproval(ctx context.Context, call *policy.ToolCall, reason string) (*ApprovalResult, error) {
	// Check max pending
	m.mu.RLock()
	if len(m.pending) >= m.maxPending {
		m.mu.RUnlock()
		return nil, policy.ErrMaxPendingExceeded
	}
	m.mu.RUnlock()

	// Create approval request
	now := time.Now()
	req := &ApprovalRequest{
		ID:        uuid.New().String(),
		ToolName:  call.Name,
		Arguments: call.Arguments,
		Reason:    reason,
		SessionID: call.SessionID,
		AgentID:   call.AgentID,
		CreatedAt: now,
		ExpiresAt: now.Add(m.timeout),
	}

	// Create pending request
	pr := &pendingRequest{
		request: req,
		done:    make(chan *ApprovalResult, 1),
	}

	// Set up timeout timer
	pr.timer = time.AfterFunc(m.timeout, func() {
		m.handleTimeout(req.ID)
	})

	// Store pending request
	m.mu.Lock()
	m.pending[req.ID] = pr
	m.mu.Unlock()

	// Log request creation
	m.slogger.Info("approval request created",
		"request_id", req.ID,
		"tool", req.ToolName,
		"reason", req.Reason,
	)

	if m.logger != nil {
		m.logger.LogRequest(req)
	}

	// Notify via WebSocket
	if m.notifier != nil {
		if err := m.notifier.NotifyRequest(req); err != nil {
			m.slogger.Warn("failed to send approval notification",
				"request_id", req.ID,
				"error", err,
			)
		}
	}

	// Wait for result
	select {
	case result := <-pr.done:
		return result, nil
	case <-ctx.Done():
		// Context cancelled - clean up
		m.cleanup(req.ID)
		return &ApprovalResult{
			Approved:  false,
			Message:   "request cancelled",
			DecidedAt: time.Now(),
			Decision:  DecisionRejected,
		}, ctx.Err()
	}
}

// HandleResponse processes an approval response from UI.
func (m *Manager) HandleResponse(requestID string, approved bool, message string) error {
	m.mu.Lock()
	pr, ok := m.pending[requestID]
	if !ok {
		m.mu.Unlock()
		return policy.ErrRequestNotFound
	}

	// Stop timeout timer
	if pr.timer != nil {
		pr.timer.Stop()
	}

	// Remove from pending
	delete(m.pending, requestID)
	m.mu.Unlock()

	// Create result
	decision := DecisionRejected
	if approved {
		decision = DecisionApproved
	}

	result := &ApprovalResult{
		Approved:   approved,
		Message:    message,
		ApprovedBy: "user",
		DecidedAt:  time.Now(),
		Decision:   decision,
	}

	// Log decision
	m.slogger.Info("approval decision",
		"request_id", requestID,
		"decision", decision,
		"approved", approved,
	)

	if m.logger != nil {
		m.logger.LogDecision(pr.request, result)
	}

	// Notify resolution
	if m.notifier != nil {
		if err := m.notifier.NotifyResolved(pr.request, result); err != nil {
			m.slogger.Warn("failed to send resolution notification",
				"request_id", requestID,
				"error", err,
			)
		}
	}

	// Send result to waiting goroutine
	select {
	case pr.done <- result:
	default:
		// Channel full or closed - request was already resolved
	}

	return nil
}

// handleTimeout handles approval request timeout.
func (m *Manager) handleTimeout(requestID string) {
	m.mu.Lock()
	pr, ok := m.pending[requestID]
	if !ok {
		m.mu.Unlock()
		return
	}

	// Remove from pending
	delete(m.pending, requestID)
	m.mu.Unlock()

	// Create timeout result
	result := &ApprovalResult{
		Approved:  false,
		Message:   "approval request timed out",
		DecidedAt: time.Now(),
		Decision:  DecisionTimeout,
	}

	// Log timeout
	m.slogger.Warn("approval request timed out",
		"request_id", requestID,
		"tool", pr.request.ToolName,
	)

	if m.logger != nil {
		m.logger.LogDecision(pr.request, result)
	}

	// Notify resolution
	if m.notifier != nil {
		if err := m.notifier.NotifyResolved(pr.request, result); err != nil {
			m.slogger.Warn("failed to send timeout notification",
				"request_id", requestID,
				"error", err,
			)
		}
	}

	// Send result to waiting goroutine
	select {
	case pr.done <- result:
	default:
	}
}

// cleanup removes a pending request without sending a result.
func (m *Manager) cleanup(requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pr, ok := m.pending[requestID]; ok {
		if pr.timer != nil {
			pr.timer.Stop()
		}
		delete(m.pending, requestID)
	}
}

// GetPending returns a pending approval request by ID.
func (m *Manager) GetPending(requestID string) (*ApprovalRequest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pr, ok := m.pending[requestID]; ok {
		return pr.request, true
	}
	return nil, false
}

// ListPending returns all pending approval requests.
func (m *Manager) ListPending() []*ApprovalRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ApprovalRequest, 0, len(m.pending))
	for _, pr := range m.pending {
		result = append(result, pr.request)
	}
	return result
}

// PendingCount returns the number of pending requests.
func (m *Manager) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pending)
}

// Close cancels all pending requests and cleans up resources.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, pr := range m.pending {
		if pr.timer != nil {
			pr.timer.Stop()
		}
		// Send cancellation result
		select {
		case pr.done <- &ApprovalResult{
			Approved:  false,
			Message:   "manager closed",
			DecidedAt: time.Now(),
			Decision:  DecisionRejected,
		}:
		default:
		}
		delete(m.pending, id)
	}
}
