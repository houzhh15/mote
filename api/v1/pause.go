package v1

import (
	"encoding/json"
	"net/http"

	"mote/internal/gateway/handlers"
)

// PauseRequest represents a pause request.
type PauseRequest struct {
	SessionID string `json:"session_id"`
}

// ResumeRequest represents a resume request.
type ResumeRequest struct {
	SessionID string `json:"session_id"`
	UserInput string `json:"user_input,omitempty"` // Optional user input to inject
}

// PauseStatusResponse represents the pause status response.
type PauseStatusResponse struct {
	SessionID        string   `json:"session_id"`
	Paused           bool     `json:"paused"`
	PausedAt         string   `json:"paused_at,omitempty"`
	TimeoutRemaining int      `json:"timeout_remaining,omitempty"` // Seconds remaining before timeout
	PendingTools     []string `json:"pending_tools,omitempty"`     // Names of tools about to be executed
}

// HandlePause pauses the execution of a session before the next tool call.
// POST /api/v1/pause
func (r *Router) HandlePause(w http.ResponseWriter, req *http.Request) {
	var pauseReq PauseRequest
	if err := json.NewDecoder(req.Body).Decode(&pauseReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if pauseReq.SessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "session_id is required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	// Call runner's pause method
	if err := r.runner.PauseSession(pauseReq.SessionID); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message":    "Session pause flag set. Execution will pause before next tool call.",
		"session_id": pauseReq.SessionID,
	})
}

// HandleResume resumes the execution of a paused session.
// POST /api/v1/resume
func (r *Router) HandleResume(w http.ResponseWriter, req *http.Request) {
	var resumeReq ResumeRequest
	if err := json.NewDecoder(req.Body).Decode(&resumeReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if resumeReq.SessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "session_id is required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	// Call runner's resume method with optional user input
	if err := r.runner.ResumeSession(resumeReq.SessionID, resumeReq.UserInput); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	msg := "Session resumed"
	if resumeReq.UserInput != "" {
		msg = "Session resumed with user input injected as tool result"
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message":    msg,
		"session_id": resumeReq.SessionID,
	})
}

// HandlePauseStatus returns the current pause status of a session.
// GET /api/v1/pause/status?session_id=xxx
func (r *Router) HandlePauseStatus(w http.ResponseWriter, req *http.Request) {
	sessionID := req.URL.Query().Get("session_id")
	if sessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "session_id query parameter is required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	// Get pause status from runner
	status, err := r.runner.GetPauseStatus(sessionID)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Convert to response format
	resp := PauseStatusResponse{
		SessionID: sessionID,
		Paused:    status.Paused,
	}

	if status.Paused {
		resp.PausedAt = status.PausedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.TimeoutRemaining = status.TimeoutIn

		// Extract tool names from pending tools
		if len(status.PendingTools) > 0 {
			resp.PendingTools = make([]string, len(status.PendingTools))
			for i, tool := range status.PendingTools {
				resp.PendingTools[i] = tool.Name
			}
		}
	}

	handlers.SendJSON(w, http.StatusOK, resp)
}
