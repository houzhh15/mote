package minimax

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// ProcessStream processes an SSE stream from the MiniMax API (OpenAI-compatible format).
// Each event is prefixed with "data: " and terminated with "\n\n".
// The stream ends with "data: [DONE]".
//
// MiniMax models support two modes for thinking content:
//  1. reasoning_split=True (preferred): Thinking content is separated into
//     reasoning_content or reasoning_details fields in the delta.
//  2. Fallback: Thinking content wrapped in <think>...</think> tags within the
//     content field, parsed by thinkTagParser.
func ProcessStream(reader io.ReadCloser) <-chan provider.ChatEvent {
	events := make(chan provider.ChatEvent, 32)

	go func() {
		defer close(events)
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		// Increase buffer size for large streaming chunks
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		// State machine for <think>...</think> tag parsing
		var thinkState thinkTagParser

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
				logger.Error().Err(err).Str("data", data).Msg("Failed to parse MiniMax stream chunk")
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

			// Handle reasoning_content field (when reasoning_split=True)
			if delta.ReasoningContent != "" {
				events <- provider.ChatEvent{
					Type:     provider.EventTypeThinking,
					Thinking: delta.ReasoningContent,
				}
			}

			// Handle reasoning_details field (structured reasoning details)
			for _, detail := range delta.ReasoningDetails {
				if detail.Text != "" {
					events <- provider.ChatEvent{
						Type:     provider.EventTypeThinking,
						Thinking: detail.Text,
					}
				}
			}

			// Emit content tokens — parse <think>...</think> tags as fallback
			// (used when reasoning_split is not enabled)
			if delta.Content != "" {
				thinkState.Process(delta.Content, events)
			}

			// Emit tool calls
			for _, tc := range delta.ToolCalls {
				events <- provider.ChatEvent{
					Type: provider.EventTypeToolCall,
					ToolCall: &provider.ToolCall{
						Index:     tc.Index,
						ID:        tc.ID,
						Type:      "function",
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}

			// Handle finish reason — always emit Done event when finish_reason is present.
			// Usage may or may not be included in the final chunk.
			if choice.FinishReason == "stop" || choice.FinishReason == "tool_calls" {
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
			logger.Error().Err(err).Msg("MiniMax stream scanner error")
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
		}
	}()

	return events
}

// thinkTagParser is a streaming state machine that separates <think>...</think>
// content from regular content in a token-by-token stream.
//
// MiniMax models (e.g., MiniMax-M2.5) embed reasoning/thinking in the content
// field wrapped in <think>...</think> tags. This parser intercepts those tags
// and emits EventTypeThinking events instead of EventTypeContent.
type thinkTagParser struct {
	inThink bool   // currently inside <think>...</think>
	tagBuf  string // buffer for partial tag matching (e.g., "<", "<th", "</thi")
}

// Process handles a content delta, splitting it into thinking vs content events.
// It batches consecutive characters of the same type into a single event.
func (p *thinkTagParser) Process(delta string, events chan<- provider.ChatEvent) {
	var contentBuf strings.Builder
	var thinkBuf strings.Builder

	flush := func() {
		if contentBuf.Len() > 0 {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeContent,
				Delta: contentBuf.String(),
			}
			contentBuf.Reset()
		}
		if thinkBuf.Len() > 0 {
			events <- provider.ChatEvent{
				Type:     provider.EventTypeThinking,
				Thinking: thinkBuf.String(),
			}
			thinkBuf.Reset()
		}
	}

	for i := 0; i < len(delta); i++ {
		ch := delta[i]

		if p.tagBuf != "" {
			// We're in the middle of matching a potential tag
			p.tagBuf += string(ch)

			if p.inThink {
				// Trying to match </think>
				const closeTag = "</think>"
				if strings.HasPrefix(closeTag, p.tagBuf) {
					if p.tagBuf == closeTag {
						// Full close tag matched — exit thinking mode
						flush()
						p.inThink = false
						p.tagBuf = ""
					}
					// Partial match, continue accumulating
				} else {
					// Not a close tag — flush buffer as thinking content
					thinkBuf.WriteString(p.tagBuf)
					p.tagBuf = ""
				}
			} else {
				// Trying to match <think>
				const openTag = "<think>"
				if strings.HasPrefix(openTag, p.tagBuf) {
					if p.tagBuf == openTag {
						// Full open tag matched — enter thinking mode
						flush()
						p.inThink = true
						p.tagBuf = ""
					}
					// Partial match, continue accumulating
				} else {
					// Not an open tag — flush buffer as regular content
					contentBuf.WriteString(p.tagBuf)
					p.tagBuf = ""
				}
			}
		} else if ch == '<' {
			// Start of a potential tag
			p.tagBuf = "<"
		} else if p.inThink {
			thinkBuf.WriteByte(ch)
		} else {
			contentBuf.WriteByte(ch)
		}
	}

	flush()
}
