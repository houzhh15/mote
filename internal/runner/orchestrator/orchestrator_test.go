package orchestrator

import (
	"testing"

	"mote/internal/provider"
	"mote/internal/storage"
)

func TestStandardOrchestrator_ConvertToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    []provider.ToolCall
		expected int
	}{
		{
			name: "simple tool call",
			input: []provider.ToolCall{
				{
					ID:        "call_123",
					Name:      "test_tool",
					Arguments: "{}",
				},
			},
			expected: 1,
		},
		{
			name: "multiple tool calls",
			input: []provider.ToolCall{
				{
					ID:        "call_123",
					Name:      "test_tool",
					Arguments: "{}",
				},
				{
					ID: "call_456",
					Function: &struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      "another_tool",
						Arguments: `{"arg":"value"}`,
					},
				},
			},
			expected: 2,
		},
		{
			name:     "empty list",
			input:    []provider.ToolCall{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToolCalls(tt.input)

			if len(result) != tt.expected {
				t.Errorf("expected %d tool calls, got %d", tt.expected, len(result))
			}

			// Verify structure of first tool call if present
			if len(result) > 0 && len(tt.input) > 0 {
				if result[0].Type != "function" {
					t.Errorf("expected type 'function', got '%s'", result[0].Type)
				}
				if result[0].ID != tt.input[0].ID {
					t.Errorf("expected ID '%s', got '%s'", tt.input[0].ID, result[0].ID)
				}
			}
		})
	}
}

func TestLoopState_Initial(t *testing.T) {
	state := &LoopState{
		Iteration:              0,
		ConsecutiveErrors:      0,
		TotalConsecutiveErrors: 0,
		LastResponse:           nil,
		TotalTokens:            0,
		ContextRetried:         false,
		TransientRetries:       0,
		UseChat:                false,
	}

	if state.Iteration != 0 {
		t.Errorf("expected initial iteration 0, got %d", state.Iteration)
	}
	if state.ContextRetried {
		t.Error("expected ContextRetried to be false initially")
	}
	if state.TotalTokens != 0 {
		t.Errorf("expected initial total tokens 0, got %d", state.TotalTokens)
	}
	if state.TransientRetries != 0 {
		t.Errorf("expected initial transient retries 0, got %d", state.TransientRetries)
	}
}

func TestUsage_Accumulation(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	// Simulate adding more usage
	usage.PromptTokens += 50
	usage.CompletionTokens += 25
	usage.TotalTokens += 75

	if usage.PromptTokens != 150 {
		t.Errorf("expected prompt tokens 150, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 75 {
		t.Errorf("expected completion tokens 75, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 225 {
		t.Errorf("expected total tokens 225, got %d", usage.TotalTokens)
	}
}

func TestStorageToolCall_Conversion(t *testing.T) {
	// Test that storage.ToolCall structure is correct
	functionJSON := []byte(`{"name":"test_tool","arguments":"{\"arg\":\"value\"}"}`)
	tc := storage.ToolCall{
		ID:       "call_123",
		Type:     "function",
		Function: functionJSON,
	}

	if tc.ID != "call_123" {
		t.Errorf("expected ID 'call_123', got '%s'", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("expected type 'function', got '%s'", tc.Type)
	}
	
	// Test GetName method
	name := tc.GetName()
	if name != "test_tool" {
		t.Errorf("expected function name 'test_tool', got '%s'", name)
	}
}

