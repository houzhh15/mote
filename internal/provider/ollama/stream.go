package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// ProcessStream processes JSON line stream from Ollama and returns a channel of ChatEvents.
// Ollama uses newline-delimited JSON (NDJSON), not SSE format.
func ProcessStream(r io.ReadCloser) <-chan provider.ChatEvent {
	events := make(chan provider.ChatEvent)

	go func() {
		defer close(events)
		defer r.Close()

		scanner := bufio.NewScanner(r)
		// Increase buffer size for large responses
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024) // 1MB max

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var resp ollamaResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				logger.Error().Err(err).Str("line", string(line)).Msg("Failed to parse Ollama stream line")
				events <- provider.ChatEvent{
					Type:  provider.EventTypeError,
					Error: err,
				}
				continue
			}

			// Check for inline error (Ollama may return {"error":"..."} in stream body)
			if resp.Error != "" {
				logger.Error().Str("error", resp.Error).Msg("Ollama stream returned inline error")
				events <- provider.ChatEvent{
					Type:  provider.EventTypeError,
					Error: fmt.Errorf("ollama error: %s", resp.Error),
				}
				return
			}

			// Process content delta
			if resp.Message.Content != "" {
				events <- provider.ChatEvent{
					Type:  provider.EventTypeContent,
					Delta: resp.Message.Content,
				}
			}

			// Process tool calls
			for i, tc := range resp.Message.ToolCalls {
				// Convert arguments map back to string for provider interface
				var argsStr string
				if tc.Function.Arguments != nil {
					if argsBytes, err := json.Marshal(tc.Function.Arguments); err == nil {
						argsStr = string(argsBytes)
					}
				}
				events <- provider.ChatEvent{
					Type: provider.EventTypeToolCall,
					ToolCall: &provider.ToolCall{
						Index:     i,
						ID:        tc.ID,
						Type:      "function",
						Name:      tc.Function.Name,
						Arguments: argsStr,
					},
				}
			}

			// Check for completion
			if resp.Done {
				// Build usage from eval counts
				var usage *provider.Usage
				if resp.PromptEvalCount > 0 || resp.EvalCount > 0 {
					usage = &provider.Usage{
						PromptTokens:     resp.PromptEvalCount,
						CompletionTokens: resp.EvalCount,
						TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
					}
				}

				// Determine finish reason based on accumulated state
				finishReason := provider.FinishReasonStop
				// Note: Tool calls are determined after stream ends in processStreamResponse

				events <- provider.ChatEvent{
					Type:         provider.EventTypeDone,
					Usage:        usage,
					FinishReason: finishReason,
				}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			logger.Error().Err(err).Msg("Error reading Ollama stream")
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
		}
	}()

	return events
}

// StreamAccumulator accumulates streaming events into a complete response.
// Useful for testing or when you need the full response from a stream.
type StreamAccumulator struct {
	Content   string
	ToolCalls []provider.ToolCall
	Usage     *provider.Usage
}

// NewStreamAccumulator creates a new StreamAccumulator.
func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		ToolCalls: make([]provider.ToolCall, 0),
	}
}

// Process accumulates events from a channel and returns the final response.
func (a *StreamAccumulator) Process(events <-chan provider.ChatEvent) (*provider.ChatResponse, error) {
	for event := range events {
		switch event.Type {
		case provider.EventTypeContent:
			a.Content += event.Delta
		case provider.EventTypeToolCall:
			if event.ToolCall != nil {
				a.ToolCalls = append(a.ToolCalls, *event.ToolCall)
			}
		case provider.EventTypeDone:
			a.Usage = event.Usage
		case provider.EventTypeError:
			return nil, event.Error
		}
	}

	finishReason := provider.FinishReasonStop
	if len(a.ToolCalls) > 0 {
		finishReason = provider.FinishReasonToolCalls
	}

	return &provider.ChatResponse{
		Content:      a.Content,
		ToolCalls:    a.ToolCalls,
		Usage:        a.Usage,
		FinishReason: finishReason,
	}, nil
}
