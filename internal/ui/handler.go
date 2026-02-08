package ui

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"mote/internal/gateway/handlers"
	"mote/internal/storage"
)

// Handler manages UI-related HTTP endpoints.
type Handler struct {
	registry *Registry
	state    *StateManager
	static   *StaticServer
	db       *storage.DB
}

// NewHandler creates a new UI handler.
func NewHandler(registry *Registry, state *StateManager, static *StaticServer, db *storage.DB) *Handler {
	return &Handler{
		registry: registry,
		state:    state,
		static:   static,
		db:       db,
	}
}

// RegisterRoutes registers UI routes on the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// API routes
	api := r.PathPrefix("/api/ui").Subrouter()
	api.HandleFunc("/components", h.HandleComponents).Methods(http.MethodGet)
	api.HandleFunc("/state", h.HandleGetState).Methods(http.MethodGet)
	api.HandleFunc("/state", h.HandleUpdateState).Methods(http.MethodPut)

	// Session routes
	r.HandleFunc("/api/sessions", h.HandleListSessions).Methods(http.MethodGet)
	r.HandleFunc("/api/sessions/{id}", h.HandleGetSession).Methods(http.MethodGet)
	r.HandleFunc("/api/sessions/{id}", h.HandleDeleteSession).Methods(http.MethodDelete)

	// Config routes
	r.HandleFunc("/api/config", h.HandleGetConfig).Methods(http.MethodGet)
	r.HandleFunc("/api/config", h.HandleUpdateConfig).Methods(http.MethodPut)

	// Static files (must be last, as it's a catch-all)
	r.PathPrefix("/").Handler(h.static)
}

// HandleComponents returns the list of registered UI components.
func (h *Handler) HandleComponents(w http.ResponseWriter, r *http.Request) {
	components := h.registry.List()
	handlers.SendJSON(w, http.StatusOK, ComponentsResponse{
		Components: components,
	})
}

// HandleGetState returns the current UI state.
func (h *Handler) HandleGetState(w http.ResponseWriter, r *http.Request) {
	state := h.state.Get()
	handlers.SendJSON(w, http.StatusOK, state)
}

// HandleUpdateState updates the UI state with partial data.
func (h *Handler) HandleUpdateState(w http.ResponseWriter, r *http.Request) {
	var partial UIState
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	h.state.Update(partial)

	state := h.state.Get()
	handlers.SendJSON(w, http.StatusOK, state)
}

// HandleListSessions returns a list of sessions.
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	rows, err := h.db.Query(`
		SELECT id, created_at, updated_at, 
		       (SELECT COUNT(*) FROM messages WHERE session_id = sessions.id) as message_count
		FROM sessions 
		ORDER BY updated_at DESC
		LIMIT 100
	`)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to query sessions")
		return
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var s SessionSummary
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.MessageCount); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []SessionSummary{}
	}

	handlers.SendJSON(w, http.StatusOK, SessionsResponse{
		Sessions: sessions,
	})
}

// HandleGetSession returns details of a specific session.
func (h *Handler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Session ID required")
		return
	}

	var s SessionSummary
	err := h.db.QueryRow(`
		SELECT id, created_at, updated_at,
		       (SELECT COUNT(*) FROM messages WHERE session_id = sessions.id) as message_count
		FROM sessions 
		WHERE id = ?
	`, id).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.MessageCount)

	if err != nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, s)
}

// HandleDeleteSession deletes a specific session.
func (h *Handler) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Session ID required")
		return
	}

	// Delete messages first (foreign key)
	_, _ = h.db.Exec("DELETE FROM messages WHERE session_id = ?", id)

	result, err := h.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to delete session")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "Session deleted",
	})
}

// HandleGetConfig returns the current configuration (safe view).
func (h *Handler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	// Return a safe subset of config
	config := ConfigView{
		Gateway: GatewayConfigView{
			Port: 8080, // TODO: get from actual config
			Host: "localhost",
		},
		Memory: MemoryConfigView{
			Enabled: false, // TODO: get from actual config
		},
		Cron: CronConfigView{
			Enabled: false, // TODO: get from actual config
		},
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

// HandleUpdateConfig updates configuration settings.
func (h *Handler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	// TODO: implement config update
	// For now, return method not allowed as config update is sensitive
	handlers.SendError(w, http.StatusMethodNotAllowed, "CONFIG_UPDATE_DISABLED", "Configuration updates are not enabled")
}
