package ollama

import (
	"io"
	"strings"
	"testing"

	"mote/internal/provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessStream(t *testing.T) {
	// Simulate Ollama NDJSON stream
	streamData := `{"model":"llama3.2","message":{"role":"assistant","content":"Hello"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":" there"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":"!"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":10,"eval_count":5}
`

	r := io.NopCloser(strings.NewReader(streamData))
	events := ProcessStream(r)

	// Collect all events
	var collected []provider.ChatEvent
	for event := range events {
		collected = append(collected, event)
	}

	// Should have 4 events: 3 content + 1 done
	require.Len(t, collected, 4)

	assert.Equal(t, provider.EventTypeContent, collected[0].Type)
	assert.Equal(t, "Hello", collected[0].Delta)

	assert.Equal(t, provider.EventTypeContent, collected[1].Type)
	assert.Equal(t, " there", collected[1].Delta)

	assert.Equal(t, provider.EventTypeContent, collected[2].Type)
	assert.Equal(t, "!", collected[2].Delta)

	assert.Equal(t, provider.EventTypeDone, collected[3].Type)
	require.NotNil(t, collected[3].Usage)
	assert.Equal(t, 10, collected[3].Usage.PromptTokens)
	assert.Equal(t, 5, collected[3].Usage.CompletionTokens)
}

func TestProcessStream_WithToolCalls(t *testing.T) {
	streamData := `{"model":"llama3.2","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true}
`

	r := io.NopCloser(strings.NewReader(streamData))
	events := ProcessStream(r)

	var collected []provider.ChatEvent
	for event := range events {
		collected = append(collected, event)
	}

	require.Len(t, collected, 2)

	assert.Equal(t, provider.EventTypeToolCall, collected[0].Type)
	require.NotNil(t, collected[0].ToolCall)
	assert.Equal(t, "call_1", collected[0].ToolCall.ID)
	assert.Equal(t, "get_weather", collected[0].ToolCall.Name)
	assert.Equal(t, `{"city":"NYC"}`, collected[0].ToolCall.Arguments)

	assert.Equal(t, provider.EventTypeDone, collected[1].Type)
}

func TestProcessStream_EmptyContent(t *testing.T) {
	// Sometimes Ollama sends empty content chunks
	streamData := `{"model":"llama3.2","message":{"role":"assistant","content":""},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":"Hi"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true}
`

	r := io.NopCloser(strings.NewReader(streamData))
	events := ProcessStream(r)

	var contentEvents []provider.ChatEvent
	for event := range events {
		if event.Type == provider.EventTypeContent {
			contentEvents = append(contentEvents, event)
		}
	}

	// Should only have 1 content event (empty strings are skipped)
	require.Len(t, contentEvents, 1)
	assert.Equal(t, "Hi", contentEvents[0].Delta)
}

func TestProcessStream_InvalidJSON(t *testing.T) {
	streamData := `{"model":"llama3.2","message":{"role":"assistant","content":"Hi"},"done":false}
invalid json line
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true}
`

	r := io.NopCloser(strings.NewReader(streamData))
	events := ProcessStream(r)

	var errors []provider.ChatEvent
	for event := range events {
		if event.Type == provider.EventTypeError {
			errors = append(errors, event)
		}
	}

	// Should have exactly 1 error event
	require.Len(t, errors, 1)
	require.NotNil(t, errors[0].Error)
}

func TestStreamAccumulator(t *testing.T) {
	events := make(chan provider.ChatEvent, 10)

	// Send events
	events <- provider.ChatEvent{Type: provider.EventTypeContent, Delta: "Hello"}
	events <- provider.ChatEvent{Type: provider.EventTypeContent, Delta: " "}
	events <- provider.ChatEvent{Type: provider.EventTypeContent, Delta: "World"}
	events <- provider.ChatEvent{
		Type: provider.EventTypeDone,
		Usage: &provider.Usage{
			PromptTokens:     5,
			CompletionTokens: 3,
			TotalTokens:      8,
		},
	}
	close(events)

	acc := NewStreamAccumulator()
	resp, err := acc.Process(events)

	require.NoError(t, err)
	assert.Equal(t, "Hello World", resp.Content)
	assert.Equal(t, provider.FinishReasonStop, resp.FinishReason)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 8, resp.Usage.TotalTokens)
}

func TestStreamAccumulator_WithToolCalls(t *testing.T) {
	events := make(chan provider.ChatEvent, 10)

	events <- provider.ChatEvent{
		Type: provider.EventTypeToolCall,
		ToolCall: &provider.ToolCall{
			ID:        "call_1",
			Name:      "test_tool",
			Arguments: `{"arg":"value"}`,
		},
	}
	events <- provider.ChatEvent{Type: provider.EventTypeDone}
	close(events)

	acc := NewStreamAccumulator()
	resp, err := acc.Process(events)

	require.NoError(t, err)
	assert.Equal(t, provider.FinishReasonToolCalls, resp.FinishReason)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_1", resp.ToolCalls[0].ID)
}

func TestStreamAccumulator_Error(t *testing.T) {
	events := make(chan provider.ChatEvent, 10)

	events <- provider.ChatEvent{Type: provider.EventTypeContent, Delta: "Partial"}
	events <- provider.ChatEvent{
		Type:  provider.EventTypeError,
		Error: ErrConnectionFailed,
	}
	close(events)

	acc := NewStreamAccumulator()
	_, err := acc.Process(events)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectionFailed)
}
