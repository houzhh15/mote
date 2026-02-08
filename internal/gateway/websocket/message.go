// Package websocket provides WebSocket hub and client management.
package websocket

import "encoding/json"

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Session string          `json:"session,omitempty"`
	Delta   string          `json:"delta,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Path    string          `json:"path,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`

	// Approval-related fields (M08)
	RequestID  string `json:"request_id,omitempty"`
	Approved   bool   `json:"approved,omitempty"`
	ApprovedBy string `json:"approved_by,omitempty"`

	// UI-related fields (M09)
	State     json.RawMessage `json:"state,omitempty"`     // UI state data
	Action    string          `json:"action,omitempty"`    // UI action type
	Payload   json.RawMessage `json:"payload,omitempty"`   // Action payload
	Component string          `json:"component,omitempty"` // Component name
	Event     string          `json:"event,omitempty"`     // Component event type
}

// BroadcastMessage wraps a message with its target session.
type BroadcastMessage struct {
	Session string
	Data    []byte
}

// Message types.
const (
	TypeSubscribe   = "subscribe"
	TypeUnsubscribe = "unsubscribe"
	TypePing        = "ping"
	TypePong        = "pong"
	TypeStream      = "stream"
	TypeToolCall    = "tool_call"
	TypeReload      = "reload"
	TypeError       = "error"

	// Approval message types (M08)
	TypeApprovalRequest  = "approval_request"
	TypeApprovalResponse = "approval_response"
	TypeApprovalResolved = "approval_resolved"

	// UI message types (M09)
	TypeChat        = "chat"
	TypeUIState     = "ui_state"
	TypeUIAction    = "ui_action"
	TypeUIComponent = "ui_component"
)
