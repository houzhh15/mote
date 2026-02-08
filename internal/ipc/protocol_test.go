package ipc

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	msg := NewMessage(MsgStatusUpdate, RoleMain)

	if msg.ID == "" {
		t.Error("expected non-empty ID")
	}
	if msg.Version != ProtocolVersion {
		t.Errorf("expected version %s, got %s", ProtocolVersion, msg.Version)
	}
	if msg.Type != MsgStatusUpdate {
		t.Errorf("expected type %s, got %s", MsgStatusUpdate, msg.Type)
	}
	if msg.Source != RoleMain {
		t.Errorf("expected source %s, got %s", RoleMain, msg.Source)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestMessageWithTarget(t *testing.T) {
	msg := NewMessage(MsgShowNotification, RoleMain).WithTarget(RoleBubble)

	if msg.Target != RoleBubble {
		t.Errorf("expected target %s, got %s", RoleBubble, msg.Target)
	}
}

func TestMessageWithPayload(t *testing.T) {
	payload := &StatusUpdatePayload{
		Status:    "running",
		SessionID: "session-123",
		TaskCount: 5,
	}

	msg := NewMessage(MsgStatusUpdate, RoleMain).WithPayload(payload)

	if msg.Payload == nil {
		t.Fatal("expected non-nil payload")
	}

	var decoded StatusUpdatePayload
	if err := msg.ParsePayload(&decoded); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if decoded.Status != payload.Status {
		t.Errorf("expected status %s, got %s", payload.Status, decoded.Status)
	}
	if decoded.SessionID != payload.SessionID {
		t.Errorf("expected session ID %s, got %s", payload.SessionID, decoded.SessionID)
	}
	if decoded.TaskCount != payload.TaskCount {
		t.Errorf("expected task count %d, got %d", payload.TaskCount, decoded.TaskCount)
	}
}

func TestNotificationPayload(t *testing.T) {
	payload := &NotificationPayload{
		ID:       "notif-1",
		Title:    "Test Notification",
		Body:     "This is a test",
		Icon:     "info",
		Duration: 5 * time.Second,
		Actions: []NotificationAction{
			{ID: "action-1", Label: "OK"},
			{ID: "action-2", Label: "Cancel"},
		},
		Data: map[string]any{
			"key": "value",
		},
	}

	msg := NewMessage(MsgShowNotification, RoleMain).WithPayload(payload)

	var decoded NotificationPayload
	if err := msg.ParsePayload(&decoded); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if decoded.ID != payload.ID {
		t.Errorf("expected ID %s, got %s", payload.ID, decoded.ID)
	}
	if decoded.Title != payload.Title {
		t.Errorf("expected title %s, got %s", payload.Title, decoded.Title)
	}
	if len(decoded.Actions) != len(payload.Actions) {
		t.Errorf("expected %d actions, got %d", len(payload.Actions), len(decoded.Actions))
	}
}

func TestEncodeDecodeMessage(t *testing.T) {
	original := NewMessage(MsgStatusUpdate, RoleMain).
		WithTarget(RoleTray).
		WithPayload(&StatusUpdatePayload{
			Status: "running",
		})

	frame, err := EncodeMessage(original)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	if len(frame) <= HeaderSize {
		t.Fatalf("frame too short: %d bytes", len(frame))
	}

	decoded, err := DecodeMessage(frame)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, decoded.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: expected %s, got %s", original.Type, decoded.Type)
	}
	if decoded.Source != original.Source {
		t.Errorf("Source mismatch: expected %s, got %s", original.Source, decoded.Source)
	}
	if decoded.Target != original.Target {
		t.Errorf("Target mismatch: expected %s, got %s", original.Target, decoded.Target)
	}
}

func TestEncoderDecoder(t *testing.T) {
	buf := new(bytes.Buffer)

	encoder := NewEncoder(buf)

	messages := []*Message{
		NewMessage(MsgRegister, RoleTray).WithPayload(&RegisterPayload{Role: RoleTray, PID: 123}),
		NewMessage(MsgStatusUpdate, RoleMain).WithPayload(&StatusUpdatePayload{Status: "running"}),
		NewMessage(MsgShowNotification, RoleMain).WithPayload(&NotificationPayload{ID: "1", Title: "Test"}),
	}

	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			t.Fatalf("failed to encode message: %v", err)
		}
	}

	decoder := NewDecoder(buf)

	for i, original := range messages {
		decoded, err := decoder.Decode()
		if err != nil {
			t.Fatalf("failed to decode message %d: %v", i, err)
		}

		if decoded.ID != original.ID {
			t.Errorf("message %d: ID mismatch", i)
		}
		if decoded.Type != original.Type {
			t.Errorf("message %d: Type mismatch", i)
		}
	}
}

func TestFrameReader(t *testing.T) {
	fr := NewFrameReader()

	msg := NewMessage(MsgStatusUpdate, RoleMain)
	frame, _ := EncodeMessage(msg)

	fr.Write(frame[:5])
	m, err := fr.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil message for partial data")
	}

	fr.Write(frame[5:])
	m, err = fr.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil message")
	}

	if m.ID != msg.ID {
		t.Errorf("ID mismatch: expected %s, got %s", msg.ID, m.ID)
	}
}

func TestMessageSerialization(t *testing.T) {
	msg := NewMessage(MsgAction, RoleBubble).
		WithTarget(RoleMain).
		WithReplyTo("original-id").
		WithPayload(&ActionPayload{
			Source:         "notification",
			Action:         "click",
			NotificationID: "notif-1",
			Data:           map[string]any{"key": "value"},
		})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ReplyTo != msg.ReplyTo {
		t.Errorf("ReplyTo mismatch: expected %s, got %s", msg.ReplyTo, decoded.ReplyTo)
	}
}

func TestMaxMessageSize(t *testing.T) {
	largePayload := make([]byte, MaxMessageSize+1)
	for i := range largePayload {
		largePayload[i] = 'a'
	}

	msg := NewMessage(MsgStatusUpdate, RoleMain)
	msg.Payload = largePayload

	_, err := EncodeMessage(msg)
	if err == nil {
		t.Error("expected error for oversized message")
	}
}
