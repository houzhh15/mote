// Package ipc provides inter-process communication primitives for Mote's
// multi-process architecture (Main, Tray, Bubble).
package ipc

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Socket/Pipe paths for different platforms
const (
	// UnixSocketPath is the default Unix Domain Socket path (macOS/Linux)
	UnixSocketPath = "/tmp/mote.sock"

	// WindowsPipeName is the Windows Named Pipe path
	WindowsPipeName = `\\.\pipe\mote-ipc`

	// ProtocolVersion is the current IPC protocol version
	ProtocolVersion = "1.0"

	// MaxMessageSize is the maximum allowed message size (1MB)
	MaxMessageSize = 1024 * 1024

	// HeaderSize is the size of the length header (4 bytes)
	HeaderSize = 4
)

// MessageType defines the type of IPC message
type MessageType string

const (
	// MsgRegister is sent by client to register its identity
	MsgRegister MessageType = "register"

	// MsgStatusUpdate is sent by main process to update status (e.g., to tray)
	MsgStatusUpdate MessageType = "status_update"

	// MsgShowNotification is sent to bubble process to show a notification
	MsgShowNotification MessageType = "show_notification"

	// MsgCloseNotification is sent to bubble process to close a notification
	MsgCloseNotification MessageType = "close_notification"

	// MsgAction is sent by bubble/tray when user interacts (e.g., click)
	MsgAction MessageType = "action"

	// MsgExit is sent by main process to signal child processes to exit
	MsgExit MessageType = "exit"

	// MsgPing is sent for health check
	MsgPing MessageType = "ping"

	// MsgPong is the response to ping
	MsgPong MessageType = "pong"

	// MsgError is sent when an error occurs
	MsgError MessageType = "error"
)

// ProcessRole defines the role of a process in the IPC system
type ProcessRole string

const (
	// RoleMain is the main GUI process (IPC server)
	RoleMain ProcessRole = "main"

	// RoleTray is the system tray process
	RoleTray ProcessRole = "tray"

	// RoleBubble is the notification bubble process
	RoleBubble ProcessRole = "bubble"
)

// Message represents an IPC message between processes
type Message struct {
	// ID is the unique identifier for this message
	ID string `json:"id"`

	// Version is the protocol version
	Version string `json:"version"`

	// Type is the message type
	Type MessageType `json:"type"`

	// Source is the role of the sender
	Source ProcessRole `json:"source"`

	// Target is the role of the intended receiver (empty for broadcast)
	Target ProcessRole `json:"target,omitempty"`

	// Payload is the message-specific data
	Payload json.RawMessage `json:"payload,omitempty"`

	// Timestamp is when the message was created
	Timestamp int64 `json:"timestamp"`

	// ReplyTo is the ID of the message this is replying to (for request-response)
	ReplyTo string `json:"reply_to,omitempty"`
}

// NewMessage creates a new message with the given type and source
func NewMessage(msgType MessageType, source ProcessRole) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Version:   ProtocolVersion,
		Type:      msgType,
		Source:    source,
		Timestamp: time.Now().UnixMilli(),
	}
}

// WithTarget sets the target role
func (m *Message) WithTarget(target ProcessRole) *Message {
	m.Target = target
	return m
}

// WithPayload sets the payload from any serializable value
func (m *Message) WithPayload(payload any) *Message {
	data, err := json.Marshal(payload)
	if err == nil {
		m.Payload = data
	}
	return m
}

// WithReplyTo sets the reply-to field
func (m *Message) WithReplyTo(replyTo string) *Message {
	m.ReplyTo = replyTo
	return m
}

// ParsePayload unmarshals the payload into the given target
func (m *Message) ParsePayload(target any) error {
	if m.Payload == nil {
		return nil
	}
	return json.Unmarshal(m.Payload, target)
}

// RegisterPayload is the payload for MsgRegister
type RegisterPayload struct {
	Role    ProcessRole `json:"role"`
	PID     int         `json:"pid"`
	Version string      `json:"version,omitempty"`
}

// StatusUpdatePayload is the payload for MsgStatusUpdate
type StatusUpdatePayload struct {
	Status    string         `json:"status"`
	SessionID string         `json:"session_id,omitempty"`
	TaskCount int            `json:"task_count,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// NotificationPayload is the payload for MsgShowNotification
type NotificationPayload struct {
	ID       string               `json:"id"`
	Title    string               `json:"title"`
	Body     string               `json:"body"`
	Icon     string               `json:"icon,omitempty"`
	Duration time.Duration        `json:"duration,omitempty"`
	Actions  []NotificationAction `json:"actions,omitempty"`
	Data     map[string]any       `json:"data,omitempty"`
}

// NotificationAction represents a clickable action in a notification
type NotificationAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// CloseNotificationPayload is the payload for MsgCloseNotification
type CloseNotificationPayload struct {
	ID string `json:"id"`
}

// ActionPayload is the payload for MsgAction
type ActionPayload struct {
	Source         string         `json:"source"`
	Action         string         `json:"action"`
	NotificationID string         `json:"notification_id,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
}

// ErrorPayload is the payload for MsgError
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Handler is a function that handles incoming messages
type Handler interface {
	Handle(msg *Message) error
}

// HandlerFunc is an adapter to allow ordinary functions to be used as handlers
type HandlerFunc func(msg *Message) error

// Handle implements Handler interface
func (f HandlerFunc) Handle(msg *Message) error {
	return f(msg)
}
