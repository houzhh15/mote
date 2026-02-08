package copilot

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// SSE event prefixes.
const (
	sseDataPrefix  = "data: "
	sseDoneMarker  = "[DONE]"
	sseEventPrefix = "event:"
)

// streamDelta represents a delta object in a streaming response.
type streamDelta struct {
	Content   string           `json:"content,omitempty"`
	ToolCalls []streamToolCall `json:"tool_calls,omitempty"`
}

// streamToolCall represents a tool call in a streaming response.
type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// streamChoice represents a choice in a streaming response.
type streamChoice struct {
	Index        int         `json:"index"`
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason,omitempty"`
}

// streamResponse represents a streaming response from the API.
type streamResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []streamChoice  `json:"choices"`
	Usage   *provider.Usage `json:"usage,omitempty"`
}

// StreamProcessor processes SSE events from a streaming response.
type StreamProcessor struct {
	reader    io.Reader
	toolCalls map[int]*provider.ToolCall // Accumulates tool calls by index
}

// NewStreamProcessor creates a new StreamProcessor.
func NewStreamProcessor(r io.Reader) *StreamProcessor {
	return &StreamProcessor{
		reader:    r,
		toolCalls: make(map[int]*provider.ToolCall),
	}
}

// ProcessSSE processes SSE events and returns a channel of ChatEvents.
func ProcessSSE(r io.Reader) <-chan provider.ChatEvent {
	events := make(chan provider.ChatEvent)

	go func() {
		defer close(events)

		processor := NewStreamProcessor(r)
		scanner := bufio.NewScanner(r)

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and event lines
			if line == "" || strings.HasPrefix(line, sseEventPrefix) {
				continue
			}

			// Process data lines
			if !strings.HasPrefix(line, sseDataPrefix) {
				continue
			}

			data := strings.TrimPrefix(line, sseDataPrefix)

			// Check for done marker
			if data == sseDoneMarker {
				// Emit any remaining pending tool calls before done
				for idx, tc := range processor.toolCalls {
					logger.Info().
						Int("index", idx).
						Str("name", tc.Name).
						Int("argsLen", len(tc.Arguments)).
						Msg("Emitting pending tool call on [DONE]")
					events <- provider.ChatEvent{
						Type:     provider.EventTypeToolCall,
						ToolCall: tc,
					}
				}
				events <- provider.ChatEvent{Type: provider.EventTypeDone}
				return
			}

			// Parse JSON response
			var resp streamResponse
			if err := json.Unmarshal([]byte(data), &resp); err != nil {
				logger.Error().Err(err).Str("data", data).Msg("Failed to parse SSE data")
				continue
			}

			// Process choices
			for _, choice := range resp.Choices {
				// Process content delta
				if choice.Delta.Content != "" {
					events <- provider.ChatEvent{
						Type:  provider.EventTypeContent,
						Delta: choice.Delta.Content,
					}
				}

				// Process tool calls
				for _, tc := range choice.Delta.ToolCalls {
					logger.Debug().
						Int("index", tc.Index).
						Str("id", tc.ID).
						Str("name", tc.Function.Name).
						Msg("Processing tool call delta")
					event := processor.processToolCall(tc)
					if event != nil {
						logger.Info().
							Str("toolName", event.ToolCall.Name).
							Msg("Tool call complete, emitting event")
						events <- *event
					}
				}

				// Check for finish reason
				if choice.FinishReason != nil {
					logger.Info().
						Str("finishReason", *choice.FinishReason).
						Bool("hasUsage", resp.Usage != nil).
						Int("pendingToolCalls", len(processor.toolCalls)).
						Msg("Stream finish reason received")

					// Emit any remaining pending tool calls before sending done event
					for idx, tc := range processor.toolCalls {
						logger.Info().
							Int("index", idx).
							Str("name", tc.Name).
							Int("argsLen", len(tc.Arguments)).
							Msg("Emitting pending tool call on finish")
						events <- provider.ChatEvent{
							Type:     provider.EventTypeToolCall,
							ToolCall: tc,
						}
					}

					// Send done event with usage and finish reason
					events <- provider.ChatEvent{
						Type:         provider.EventTypeDone,
						Usage:        resp.Usage, // May be nil, that's okay
						FinishReason: *choice.FinishReason,
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			logger.Error().Err(err).Msg("Stream scanner error")
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
		} else {
			// Scanner finished normally without explicit finish_reason
			// This can happen if:
			// 1. Connection closes before [DONE] marker (Copilot token limit reached)
			// 2. Network interruption
			// We should still emit pending tool calls and done event
			logger.Warn().
				Int("pendingToolCalls", len(processor.toolCalls)).
				Msg("Stream ended without explicit finish_reason or [DONE] marker - possible token limit")

			// Emit any remaining pending tool calls
			for idx, tc := range processor.toolCalls {
				logger.Info().
					Int("index", idx).
					Str("name", tc.Name).
					Int("argsLen", len(tc.Arguments)).
					Msg("Emitting pending tool call on unexpected stream end")
				events <- provider.ChatEvent{
					Type:     provider.EventTypeToolCall,
					ToolCall: tc,
				}
			}

			// Send done event to allow the runner to continue
			events <- provider.ChatEvent{
				Type: provider.EventTypeDone,
			}
		}
	}()

	return events
}

// processToolCall accumulates and processes tool call deltas.
func (p *StreamProcessor) processToolCall(tc streamToolCall) *provider.ChatEvent {
	// Get or create tool call accumulator
	existing, ok := p.toolCalls[tc.Index]
	if !ok {
		p.toolCalls[tc.Index] = &provider.ToolCall{
			Index: tc.Index,
		}
		existing = p.toolCalls[tc.Index]
	}

	// Accumulate ID (usually comes with the first chunk)
	if tc.ID != "" {
		existing.ID = tc.ID
	}

	// Accumulate function name (usually comes with the first chunk)
	if tc.Function.Name != "" {
		existing.Name = tc.Function.Name
	}

	// Accumulate arguments
	if tc.Function.Arguments != "" {
		existing.Arguments += tc.Function.Arguments
	}

	// Check if the arguments form valid JSON (indicates completion)
	if existing.Arguments != "" && isValidJSON(existing.Arguments) {
		event := &provider.ChatEvent{
			Type:     provider.EventTypeToolCall,
			ToolCall: existing,
		}
		// Clear accumulated tool call
		delete(p.toolCalls, tc.Index)
		return event
	}

	return nil
}

// isValidJSON checks if a string is valid JSON.
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
