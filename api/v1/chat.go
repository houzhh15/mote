package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mote/internal/gateway/handlers"
	"mote/internal/provider"
	"mote/internal/runner"
	"mote/pkg/logger"
)

// HandleChat handles synchronous chat requests.
func (r *Router) HandleChat(w http.ResponseWriter, req *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if chatReq.Message == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Message is required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	ctx := req.Context()

	// Get or create session
	sessionID := chatReq.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Run agent and collect response
	events, err := r.runner.Run(ctx, sessionID, chatReq.Message)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Collect events into response
	var message string
	var toolCalls []ToolCallResult

	for event := range events {
		switch event.Type {
		case runner.EventTypeContent:
			message += event.Content
		case runner.EventTypeToolResult:
			if event.ToolResult != nil {
				toolCalls = append(toolCalls, ToolCallResult{
					Name:   event.ToolResult.ToolName,
					Result: event.ToolResult.Output,
				})
			}
		case runner.EventTypeError:
			handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, event.ErrorMsg)
			return
		}
	}

	handlers.SendJSON(w, http.StatusOK, ChatResponse{
		SessionID: sessionID,
		Message:   message,
		ToolCalls: toolCalls,
	})
}

// HandleChatStream handles streaming chat requests using SSE.
func (r *Router) HandleChatStream(w http.ResponseWriter, req *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if chatReq.Message == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Message is required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, "Streaming not supported")
		return
	}

	ctx := req.Context()

	// Get or create session
	sessionID := chatReq.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Create a context with timeout (30 minutes for complex tasks)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Run agent with streaming
	events, err := r.runner.Run(ctx, sessionID, chatReq.Message)
	if err != nil {
		sendSSEError(w, flusher, err)
		return
	}

	for event := range events {
		var sseEvent ChatStreamEvent

		switch event.Type {
		case runner.EventTypeContent:
			sseEvent = ChatStreamEvent{
				Type:  "content",
				Delta: event.Content,
			}
		case runner.EventTypeToolCall:
			if event.ToolCall != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_call",
					ToolCall: &ToolCallResult{
						Name:      event.ToolCall.GetName(),
						Arguments: event.ToolCall.GetArguments(),
					},
				}
			} else {
				continue
			}
		case runner.EventTypeToolResult:
			if event.ToolResult != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_result",
					ToolResult: &ToolResultEvent{
						ToolCallID: event.ToolResult.ToolCallID,
						ToolName:   event.ToolResult.ToolName,
						Output:     event.ToolResult.Output,
						IsError:    event.ToolResult.IsError,
						DurationMs: event.ToolResult.DurationMs,
					},
				}
			} else {
				continue
			}
		case runner.EventTypeError:
			sseEvent = ChatStreamEvent{
				Type:  "error",
				Error: event.ErrorMsg,
			}
		case runner.EventTypeDone:
			// Skip internal done event, we'll send our own
			continue
		case runner.EventTypeHeartbeat:
			// Send heartbeat to keep connection alive
			sseEvent = ChatStreamEvent{
				Type: "heartbeat",
			}
			// Log heartbeat being sent to client
			logger.Info().Msg("Sending heartbeat to client")
		default:
			continue
		}

		data, _ := json.Marshal(sseEvent)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			logger.Error().Err(err).Msg("Failed to write SSE event to client")
			return
		}
		flusher.Flush()
	}

	// Send done event
	doneEvent := ChatStreamEvent{
		Type:      "done",
		SessionID: sessionID,
	}
	data, _ := json.Marshal(doneEvent)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// sendSSEError sends an error event via SSE.
func sendSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
	errEvent := ChatStreamEvent{
		Type:  "error",
		Error: err.Error(), // Legacy field for backward compatibility
	}

	// Try to extract detailed error info from ProviderError
	if pe, ok := err.(*provider.ProviderError); ok {
		errEvent.ErrorDetail = &ErrorDetail{
			Code:       string(pe.Code),
			Message:    pe.Message,
			Provider:   pe.Provider,
			Retryable:  pe.Retryable,
			RetryAfter: pe.RetryAfter,
		}
	} else {
		// Generic error - classify it
		errEvent.ErrorDetail = &ErrorDetail{
			Code:      string(provider.ErrCodeUnknown),
			Message:   err.Error(),
			Retryable: false,
		}
	}

	data, _ := json.Marshal(errEvent)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
