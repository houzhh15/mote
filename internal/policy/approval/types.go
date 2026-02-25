// Package approval provides the approval subsystem for tool execution.
package approval

import (
	"context"
	"time"

	"mote/internal/policy"
)

// ApprovalDecision represents the type of approval decision.
type ApprovalDecision string

const (
	// DecisionApproved indicates the request was approved.
	DecisionApproved ApprovalDecision = "approved"

	// DecisionRejected indicates the request was rejected.
	DecisionRejected ApprovalDecision = "rejected"

	// DecisionTimeout indicates the request timed out.
	DecisionTimeout ApprovalDecision = "timeout"
)

// ApprovalRequest represents a pending approval request.
type ApprovalRequest struct {
	// ID is the unique identifier for this request.
	ID string `json:"id"`

	// ToolName is the name of the tool being called.
	ToolName string `json:"tool_name"`

	// Arguments contains the serialized tool arguments.
	Arguments string `json:"arguments"`

	// Reason explains why approval is required.
	Reason string `json:"reason"`

	// SessionID is the session this request belongs to.
	SessionID string `json:"session_id"`

	// AgentID is the agent making the request.
	AgentID string `json:"agent_id"`

	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the request will timeout.
	ExpiresAt time.Time `json:"expires_at"`
}

// ApprovalResult represents the outcome of an approval request.
type ApprovalResult struct {
	// Approved indicates whether the request was approved.
	Approved bool `json:"approved"`

	// Message contains additional information about the decision.
	Message string `json:"message,omitempty"`

	// ApprovedBy identifies who made the decision.
	ApprovedBy string `json:"approved_by,omitempty"`

	// DecidedAt is when the decision was made.
	DecidedAt time.Time `json:"decided_at"`

	// Decision is the type of decision: approved, rejected, timeout.
	Decision ApprovalDecision `json:"decision"`

	// ModifiedArguments contains user-edited tool call arguments.
	// If non-empty, the runner should use these instead of the original arguments.
	ModifiedArguments string `json:"modified_arguments,omitempty"`
}

// ApprovalHandler manages approval requests.
type ApprovalHandler interface {
	// RequestApproval creates a new approval request and waits for decision.
	// Blocks until approved, rejected, or timeout.
	RequestApproval(ctx context.Context, call *policy.ToolCall, reason string) (*ApprovalResult, error)

	// HandleResponse processes an approval response from UI.
	// Optional modifiedArguments allows the user to edit tool arguments before execution.
	HandleResponse(requestID string, approved bool, message string, modifiedArguments ...string) error

	// GetPending returns a pending approval request by ID.
	GetPending(requestID string) (*ApprovalRequest, bool)

	// ListPending returns all pending approval requests.
	ListPending() []*ApprovalRequest
}

// ApprovalNotifier sends approval notifications via WebSocket.
type ApprovalNotifier interface {
	// NotifyRequest broadcasts a new approval request.
	NotifyRequest(req *ApprovalRequest) error

	// NotifyResolved broadcasts the resolution of an approval request.
	NotifyResolved(req *ApprovalRequest, result *ApprovalResult) error
}

// ApprovalLogger records approval events for audit.
type ApprovalLogger interface {
	// LogRequest logs the creation of an approval request.
	LogRequest(req *ApprovalRequest) error

	// LogDecision logs the decision made on an approval request.
	LogDecision(req *ApprovalRequest, result *ApprovalResult) error
}

// ApprovalRequestMessage is the WebSocket message for approval requests.
type ApprovalRequestMessage struct {
	Type string           `json:"type"`
	Data *ApprovalRequest `json:"data"`
}

// ApprovalResponseMessage is the WebSocket message for approval responses.
type ApprovalResponseMessage struct {
	Type string                  `json:"type"`
	Data ApprovalResponsePayload `json:"data"`
}

// ApprovalResponsePayload is the payload for approval response messages.
type ApprovalResponsePayload struct {
	ID       string `json:"id"`
	Approved bool   `json:"approved"`
	Message  string `json:"message,omitempty"`
}

// ApprovalResolvedMessage is the WebSocket message for resolved approvals.
type ApprovalResolvedMessage struct {
	Type string                  `json:"type"`
	Data ApprovalResolvedPayload `json:"data"`
}

// ApprovalResolvedPayload is the payload for resolved approval messages.
type ApprovalResolvedPayload struct {
	ID         string    `json:"id"`
	Approved   bool      `json:"approved"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	DecidedAt  time.Time `json:"decided_at"`
}

// NewApprovalRequestMessage creates a new approval request message.
func NewApprovalRequestMessage(req *ApprovalRequest) *ApprovalRequestMessage {
	return &ApprovalRequestMessage{
		Type: "approval_request",
		Data: req,
	}
}

// NewApprovalResolvedMessage creates a new approval resolved message.
func NewApprovalResolvedMessage(req *ApprovalRequest, result *ApprovalResult) *ApprovalResolvedMessage {
	return &ApprovalResolvedMessage{
		Type: "approval_resolved",
		Data: ApprovalResolvedPayload{
			ID:         req.ID,
			Approved:   result.Approved,
			ApprovedBy: result.ApprovedBy,
			DecidedAt:  result.DecidedAt,
		},
	}
}
