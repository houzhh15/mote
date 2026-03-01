package vllm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// ProcessStream processes an SSE stream from the vLLM API (OpenAI-compatible format).
// Each event is prefixed with "data: " and terminated with "\n\n".
// The stream ends with "data: [DONE]".
func ProcessStream(reader io.ReadCloser) <-chan provider.ChatEvent {
	events := make(chan provider.ChatEvent, 32)

	go func() {
		defer close(events)
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		// Increase buffer size for large streaming chunks
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Handle SSE data lines
			if !strings.HasPrefix(line, "data: ") && !strings.HasPrefix(line, "data:") {
				continue
			}

			// Extract data payload
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimPrefix(data, "data:")
			data = strings.TrimSpace(data)

			// Check for stream termination
			if data == "[DONE]" {
				events <- provider.ChatEvent{
					Type: provider.EventTypeDone,
				}
				return
			}

			// Parse the JSON chunk
			var chunk chatStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				logger.Error().Err(err).Str("data", data).Msg("Failed to parse vLLM stream chunk")
				continue
			}

			// Handle error in stream
			if chunk.Error != nil {
				events <- provider.ChatEvent{
					Type:  provider.EventTypeError,
					Error: fmt.Errorf("[%s] %s", chunk.Error.Type, chunk.Error.Message),
				}
				return
			}

			// Process choices
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			delta := choice.Delta

			// Emit content tokens
			if delta.Content != "" {
				events <- provider.ChatEvent{
					Type:  provider.EventTypeContent,
					Delta: delta.Content,
				}
			}

			// Emit tool calls
			for _, tc := range delta.ToolCalls {
				events <- provider.ChatEvent{
					Type: provider.EventTypeToolCall,
					ToolCall: &provider.ToolCall{
						Index:     0, // vLLM streams tool calls one at a time
						ID:        tc.ID,
						Type:      "function",
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}

			// Handle finish reason
			if choice.FinishReason == "stop" || choice.FinishReason == "tool_calls" || choice.FinishReason == "length" {
				doneEvent := provider.ChatEvent{
					Type:         provider.EventTypeDone,
					FinishReason: choice.FinishReason,
				}
				if chunk.Usage != nil {
					doneEvent.Usage = &provider.Usage{
						PromptTokens:     chunk.Usage.PromptTokens,
						CompletionTokens: chunk.Usage.CompletionTokens,
						TotalTokens:      chunk.Usage.TotalTokens,
					}
				}
				events <- doneEvent
			}
		}

		if err := scanner.Err(); err != nil {
			logger.Error().Err(err).Msg("vLLM stream scanner error")
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
		}
	}()

	return events
}
