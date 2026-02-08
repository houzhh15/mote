package copilot

import (
	"strings"
	"testing"

	"mote/internal/provider"
)

func TestProcessSSE_Content(t *testing.T) {
	input := `data: {"id":"123","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data: {"id":"123","choices":[{"index":0,"delta":{"content":" World"}}]}

data: [DONE]
`
	r := strings.NewReader(input)
	events := ProcessSSE(r)

	// Collect events
	var received []provider.ChatEvent
	for event := range events {
		received = append(received, event)
	}

	if len(received) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(received))
	}

	// Check content events
	if received[0].Type != provider.EventTypeContent || received[0].Delta != "Hello" {
		t.Errorf("first event = %+v, want content 'Hello'", received[0])
	}
	if received[1].Type != provider.EventTypeContent || received[1].Delta != " World" {
		t.Errorf("second event = %+v, want content ' World'", received[1])
	}

	// Check done event
	last := received[len(received)-1]
	if last.Type != provider.EventTypeDone {
		t.Errorf("last event type = %s, want done", last.Type)
	}
}

func TestProcessSSE_ToolCall(t *testing.T) {
	input := `data: {"id":"123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","function":{"name":"get_weather"}}]}}]}

data: {"id":"123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}

data: {"id":"123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]}}]}

data: [DONE]
`
	r := strings.NewReader(input)
	events := ProcessSSE(r)

	// Collect events
	var received []provider.ChatEvent
	for event := range events {
		received = append(received, event)
	}

	// Should have tool_call event when arguments form valid JSON
	foundToolCall := false
	for _, event := range received {
		if event.Type == provider.EventTypeToolCall {
			foundToolCall = true
			if event.ToolCall.ID != "call_123" {
				t.Errorf("tool_call.id = %s, want call_123", event.ToolCall.ID)
			}
			if event.ToolCall.Name != "get_weather" {
				t.Errorf("tool_call.name = %s, want get_weather", event.ToolCall.Name)
			}
			if event.ToolCall.Arguments != `{"city":"NYC"}` {
				t.Errorf("tool_call.arguments = %s, want {\"city\":\"NYC\"}", event.ToolCall.Arguments)
			}
		}
	}

	if !foundToolCall {
		t.Error("expected tool_call event, got none")
	}
}

func TestProcessSSE_EmptyLines(t *testing.T) {
	input := `
event: message

data: {"id":"123","choices":[{"index":0,"delta":{"content":"Test"}}]}



data: [DONE]
`
	r := strings.NewReader(input)
	events := ProcessSSE(r)

	var received []provider.ChatEvent
	for event := range events {
		received = append(received, event)
	}

	// Should still process correctly despite empty lines and event lines
	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
}

func TestIsValidJSON(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`{"key":"value"}`, true},
		{`{"key":"value"`, false},
		{`[1,2,3]`, true},
		{`null`, true},
		{`"string"`, true},
		{`{"incomplete":`, false},
		{``, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidJSON(tt.input)
			if got != tt.want {
				t.Errorf("isValidJSON(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewStreamProcessor(t *testing.T) {
	r := strings.NewReader("")
	p := NewStreamProcessor(r)

	if p == nil {
		t.Fatal("NewStreamProcessor returned nil")
	}

	if p.toolCalls == nil {
		t.Error("toolCalls map is nil")
	}
}
