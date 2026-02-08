package websocket

import (
	"encoding/json"
	"testing"
)

func TestWSMessage_UIFields_JSONSerialization(t *testing.T) {
	stateData := json.RawMessage(`{"theme":"dark","sidebar_open":true}`)
	payloadData := json.RawMessage(`{"action":"navigate","target":"/settings"}`)

	msg := WSMessage{
		Type:      TypeUIState,
		Session:   "test-session",
		State:     stateData,
		Action:    "state_update",
		Payload:   payloadData,
		Component: "sidebar",
		Event:     "toggle",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal WSMessage: %v", err)
	}

	var decoded WSMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}

	if decoded.Type != TypeUIState {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, TypeUIState)
	}
	if decoded.Action != msg.Action {
		t.Errorf("Action mismatch: got %q, want %q", decoded.Action, msg.Action)
	}
	if decoded.Component != msg.Component {
		t.Errorf("Component mismatch: got %q, want %q", decoded.Component, msg.Component)
	}
	if decoded.Event != msg.Event {
		t.Errorf("Event mismatch: got %q, want %q", decoded.Event, msg.Event)
	}
	if string(decoded.State) != string(msg.State) {
		t.Errorf("State mismatch: got %s, want %s", decoded.State, msg.State)
	}
	if string(decoded.Payload) != string(msg.Payload) {
		t.Errorf("Payload mismatch: got %s, want %s", decoded.Payload, msg.Payload)
	}
}

func TestWSMessage_UIFields_OmitEmpty(t *testing.T) {
	msg := WSMessage{
		Type:    TypeChat,
		Session: "test-session",
		Message: "hello",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal WSMessage: %v", err)
	}

	str := string(data)

	// UI fields should be omitted when empty
	if containsStr(str, "state") {
		t.Error("empty state should be omitted")
	}
	if containsStr(str, "action") {
		t.Error("empty action should be omitted")
	}
	if containsStr(str, "payload") {
		t.Error("empty payload should be omitted")
	}
	if containsStr(str, "component") {
		t.Error("empty component should be omitted")
	}
	if containsStr(str, "event") {
		t.Error("empty event should be omitted")
	}
}

func TestUIMessageTypes(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"TypeChat", TypeChat, "chat"},
		{"TypeUIState", TypeUIState, "ui_state"},
		{"TypeUIAction", TypeUIAction, "ui_action"},
		{"TypeUIComponent", TypeUIComponent, "ui_component"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestWSMessage_UIComponent(t *testing.T) {
	eventData := json.RawMessage(`{"x":100,"y":200}`)
	msg := WSMessage{
		Type:      TypeUIComponent,
		Component: "weather-widget",
		Event:     "click",
		Payload:   eventData,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal WSMessage: %v", err)
	}

	var decoded WSMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}

	if decoded.Type != TypeUIComponent {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, TypeUIComponent)
	}
	if decoded.Component != "weather-widget" {
		t.Errorf("Component mismatch: got %q, want %q", decoded.Component, "weather-widget")
	}
	if decoded.Event != "click" {
		t.Errorf("Event mismatch: got %q, want %q", decoded.Event, "click")
	}
}

func TestWSMessage_UIAction(t *testing.T) {
	payloadData := json.RawMessage(`{"page":"/settings","params":{"tab":"general"}}`)
	msg := WSMessage{
		Type:    TypeUIAction,
		Action:  "navigate",
		Payload: payloadData,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal WSMessage: %v", err)
	}

	var decoded WSMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WSMessage: %v", err)
	}

	if decoded.Type != TypeUIAction {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, TypeUIAction)
	}
	if decoded.Action != "navigate" {
		t.Errorf("Action mismatch: got %q, want %q", decoded.Action, "navigate")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
