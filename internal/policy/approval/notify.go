package approval

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// Broadcaster defines the interface for broadcasting messages via WebSocket.
type Broadcaster interface {
	// BroadcastAll sends a message to all connected clients.
	BroadcastAll(messageType string, data any) error
}

// Notifier implements the ApprovalNotifier interface.
type Notifier struct {
	broadcaster Broadcaster
	logger      *slog.Logger
}

// NewNotifier creates a new Notifier.
func NewNotifier(broadcaster Broadcaster) *Notifier {
	return &Notifier{
		broadcaster: broadcaster,
		logger:      slog.Default(),
	}
}

// SetLogger sets a custom logger.
func (n *Notifier) SetLogger(l *slog.Logger) {
	n.logger = l
}

// NotifyRequest broadcasts a new approval request to all clients.
func (n *Notifier) NotifyRequest(req *ApprovalRequest) error {
	if n.broadcaster == nil {
		n.logger.Warn("notifier: broadcaster not configured, skipping notification")
		return nil
	}

	msg := NewApprovalRequestMessage(req)

	if err := n.broadcaster.BroadcastAll(msg.Type, msg.Data); err != nil {
		n.logger.Error("notifier: failed to broadcast approval request",
			"request_id", req.ID,
			"error", err,
		)
		return fmt.Errorf("notifier: failed to broadcast: %w", err)
	}

	n.logger.Debug("notifier: broadcast approval request",
		"request_id", req.ID,
		"tool", req.ToolName,
	)

	return nil
}

// NotifyResolved broadcasts the resolution of an approval request.
func (n *Notifier) NotifyResolved(req *ApprovalRequest, result *ApprovalResult) error {
	if n.broadcaster == nil {
		n.logger.Warn("notifier: broadcaster not configured, skipping notification")
		return nil
	}

	msg := NewApprovalResolvedMessage(req, result)

	if err := n.broadcaster.BroadcastAll(msg.Type, msg.Data); err != nil {
		n.logger.Error("notifier: failed to broadcast approval resolution",
			"request_id", req.ID,
			"error", err,
		)
		return fmt.Errorf("notifier: failed to broadcast: %w", err)
	}

	n.logger.Debug("notifier: broadcast approval resolved",
		"request_id", req.ID,
		"approved", result.Approved,
	)

	return nil
}

// FormatApprovalRequestJSON formats an approval request as JSON for logging.
func FormatApprovalRequestJSON(req *ApprovalRequest) string {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(data)
}

// FormatApprovalResultJSON formats an approval result as JSON for logging.
func FormatApprovalResultJSON(result *ApprovalResult) string {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("{\"error\": \"%s\"}", err.Error())
	}
	return string(data)
}
