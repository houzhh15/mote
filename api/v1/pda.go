package v1

import (
	"encoding/json"
	"fmt"
	"net/http"

	"mote/internal/gateway/handlers"
	"mote/internal/runner"
	"mote/pkg/logger"

	"github.com/gorilla/mux"
)

// PDAControlRequest represents a request to control PDA execution.
type PDAControlRequest struct {
	Action string `json:"action"` // "continue", "restart", "status"
}

// PDACheckpointInfo represents checkpoint information returned by the status action.
type PDACheckpointInfo struct {
	HasCheckpoint   bool     `json:"has_checkpoint"`
	AgentName       string   `json:"agent_name,omitempty"`
	InterruptStep   int      `json:"interrupt_step,omitempty"`
	InterruptAgent  string   `json:"interrupt_agent,omitempty"`
	InterruptReason string   `json:"interrupt_reason,omitempty"`
	ExecutedSteps   []string `json:"executed_steps,omitempty"`
	InitialPrompt   string   `json:"initial_prompt,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
}

// HandlePDAControl handles PDA checkpoint control operations for a session.
// POST /api/v1/sessions/{id}/pda
//
// Actions:
//   - "status": Returns checkpoint info without modifying state
//   - "continue": Resumes PDA from checkpoint (SSE streaming response)
//   - "restart": Clears the checkpoint
func (r *Router) HandlePDAControl(w http.ResponseWriter, req *http.Request) {
	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "session ID required")
		return
	}

	var pdaReq PDAControlRequest
	if err := json.NewDecoder(req.Body).Decode(&pdaReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}

	switch pdaReq.Action {
	case "status":
		r.handlePDAStatus(w, sessionID)

	case "restart":
		r.handlePDARestart(w, sessionID)

	case "continue":
		r.handlePDAResume(w, req, sessionID)

	default:
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest,
			fmt.Sprintf("unknown action %q, use 'status', 'continue', or 'restart'", pdaReq.Action))
	}
}

// handlePDAStatus returns checkpoint information for a session.
func (r *Router) handlePDAStatus(w http.ResponseWriter, sessionID string) {
	if r.runner == nil {
		handlers.SendJSON(w, http.StatusOK, PDACheckpointInfo{HasCheckpoint: false})
		return
	}

	info := r.runner.GetPDACheckpointInfo(sessionID)
	if info == nil {
		handlers.SendJSON(w, http.StatusOK, PDACheckpointInfo{HasCheckpoint: false})
		return
	}

	resp := PDACheckpointInfo{
		HasCheckpoint: true,
	}
	if v, ok := info["agent_name"].(string); ok {
		resp.AgentName = v
	}
	if v, ok := info["interrupt_step"].(int); ok {
		resp.InterruptStep = v
	}
	if v, ok := info["interrupt_agent"].(string); ok {
		resp.InterruptAgent = v
	}
	if v, ok := info["interrupt_reason"].(string); ok {
		resp.InterruptReason = v
	}
	if v, ok := info["executed_steps"].([]string); ok {
		resp.ExecutedSteps = v
	}
	if v, ok := info["initial_prompt"].(string); ok {
		resp.InitialPrompt = v
	}
	if v, ok := info["created_at"]; ok {
		resp.CreatedAt = fmt.Sprintf("%v", v)
	}

	handlers.SendJSON(w, http.StatusOK, resp)
}

// handlePDARestart clears a PDA checkpoint.
func (r *Router) handlePDARestart(w http.ResponseWriter, sessionID string) {
	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeInternalError, "runner not available")
		return
	}

	if err := r.runner.ClearPDACheckpoint(sessionID); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "PDA checkpoint cleared",
	})
}

// handlePDAResume resumes PDA execution via SSE streaming. Uses the same SSE
// format as HandleChatStream so the frontend can reuse its event processing logic.
func (r *Router) handlePDAResume(w http.ResponseWriter, req *http.Request, sessionID string) {
	lgr := logger.Get()

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeInternalError, "runner not available")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, "streaming not supported")
		return
	}

	ctx := req.Context()

	events, err := r.runner.ResumePDA(ctx, sessionID)
	if err != nil {
		sendSSEError(w, flusher, err)
		return
	}

	lgr.Info().Str("sessionID", sessionID).Msg("PDA resume: streaming events")

	// Stream events using the same format as HandleChatStream
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
		case runner.EventTypeToolCallUpdate:
			if event.ToolCallUpdate != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_call_update",
					ToolCallUpdate: &ToolCallUpdateEvent{
						ToolCallID: event.ToolCallUpdate.ToolCallID,
						ToolName:   event.ToolCallUpdate.ToolName,
						Status:     event.ToolCallUpdate.Status,
						Arguments:  event.ToolCallUpdate.Arguments,
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
		case runner.EventTypePDAProgress:
			if event.PDAProgress != nil {
				var parentSteps []PDAParentStepSSE
				for _, ps := range event.PDAProgress.ParentSteps {
					parentSteps = append(parentSteps, PDAParentStepSSE{
						AgentName:  ps.AgentName,
						StepIndex:  ps.StepIndex,
						TotalSteps: ps.TotalSteps,
						StepLabel:  ps.StepLabel,
					})
				}
				sseEvent = ChatStreamEvent{
					Type: "pda_progress",
					PDAProgress: &PDAProgressSSEEvent{
						AgentName:     event.PDAProgress.AgentName,
						StepIndex:     event.PDAProgress.StepIndex,
						TotalSteps:    event.PDAProgress.TotalSteps,
						StepLabel:     event.PDAProgress.StepLabel,
						StepType:      event.PDAProgress.StepType,
						Phase:         event.PDAProgress.Phase,
						StackDepth:    event.PDAProgress.StackDepth,
						ExecutedSteps: event.PDAProgress.ExecutedSteps,
						TotalTokens:   event.PDAProgress.TotalTokens,
						Model:         event.PDAProgress.Model,
						ParentSteps:   parentSteps,
					},
				}
			} else {
				continue
			}
		case runner.EventTypeDone:
			continue
		case runner.EventTypeHeartbeat:
			sseEvent = ChatStreamEvent{
				Type: "heartbeat",
			}
		default:
			continue
		}

		// Propagate sub-agent identity
		if event.AgentName != "" {
			sseEvent.AgentName = event.AgentName
			sseEvent.AgentDepth = event.AgentDepth
		}

		sseEvent.SessionID = sessionID
		data, _ := json.Marshal(sseEvent)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			lgr.Error().Err(err).Msg("PDA resume: failed to write SSE event")
			go func() {
				for range events {
				}
			}()
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
