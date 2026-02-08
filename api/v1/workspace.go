package v1

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

// WorkspaceBindRequest is the request to bind a workspace.
type WorkspaceBindRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Alias     string `json:"alias,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// WorkspaceResponse represents a workspace binding response.
type WorkspaceResponse struct {
	SessionID  string `json:"session_id"`
	Path       string `json:"path"`
	Alias      string `json:"alias,omitempty"`
	ReadOnly   bool   `json:"read_only"`
	BoundAt    string `json:"bound_at,omitempty"`
	LastAccess string `json:"last_access,omitempty"`
}

// FileInfoResponse represents file info in a workspace.
type FileInfoResponse struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// HandleListWorkspaces returns all workspace bindings.
func (r *Router) HandleListWorkspaces(w http.ResponseWriter, req *http.Request) {
	if r.workspaceManager == nil {
		http.Error(w, "workspace manager not initialized", http.StatusServiceUnavailable)
		return
	}

	// Clean up orphan bindings (sessions that no longer exist)
	if r.db != nil {
		cleaned := r.workspaceManager.CleanupOrphanBindings(func(sessionID string) bool {
			_, err := r.db.GetSession(sessionID)
			return err == nil
		})
		if cleaned > 0 {
			slog.Info("cleaned up orphan workspace bindings", "count", cleaned)
		}
	}

	bindings := r.workspaceManager.List()
	responses := make([]WorkspaceResponse, 0, len(bindings))
	for _, b := range bindings {
		responses = append(responses, WorkspaceResponse{
			SessionID:  b.SessionID,
			Path:       b.Path,
			Alias:      b.Alias,
			ReadOnly:   b.ReadOnly,
			BoundAt:    b.BoundAt.Format("2006-01-02T15:04:05Z07:00"),
			LastAccess: b.LastAccess.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workspaces": responses,
	})
}

// HandleGetWorkspace returns a specific workspace binding.
func (r *Router) HandleGetWorkspace(w http.ResponseWriter, req *http.Request) {
	if r.workspaceManager == nil {
		http.Error(w, "workspace manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	binding, exists := r.workspaceManager.Get(sessionID)
	if !exists {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}

	resp := WorkspaceResponse{
		SessionID:  binding.SessionID,
		Path:       binding.Path,
		Alias:      binding.Alias,
		ReadOnly:   binding.ReadOnly,
		BoundAt:    binding.BoundAt.Format("2006-01-02T15:04:05Z07:00"),
		LastAccess: binding.LastAccess.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleBindWorkspace binds a workspace to a session.
func (r *Router) HandleBindWorkspace(w http.ResponseWriter, req *http.Request) {
	if r.workspaceManager == nil {
		http.Error(w, "workspace manager not initialized", http.StatusServiceUnavailable)
		return
	}

	var bindReq WorkspaceBindRequest
	if err := json.NewDecoder(req.Body).Decode(&bindReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if bindReq.Alias != "" {
		err = r.workspaceManager.BindWithAlias(bindReq.SessionID, bindReq.Path, bindReq.Alias, bindReq.ReadOnly)
	} else {
		err = r.workspaceManager.Bind(bindReq.SessionID, bindReq.Path, bindReq.ReadOnly)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	binding, _ := r.workspaceManager.Get(bindReq.SessionID)
	resp := WorkspaceResponse{
		SessionID:  binding.SessionID,
		Path:       binding.Path,
		Alias:      binding.Alias,
		ReadOnly:   binding.ReadOnly,
		BoundAt:    binding.BoundAt.Format("2006-01-02T15:04:05Z07:00"),
		LastAccess: binding.LastAccess.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleUnbindWorkspace removes a workspace binding.
func (r *Router) HandleUnbindWorkspace(w http.ResponseWriter, req *http.Request) {
	if r.workspaceManager == nil {
		http.Error(w, "workspace manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	if err := r.workspaceManager.Unbind(sessionID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListWorkspaceFiles lists files in a workspace directory.
func (r *Router) HandleListWorkspaceFiles(w http.ResponseWriter, req *http.Request) {
	if r.workspaceManager == nil {
		http.Error(w, "workspace manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	relativePath := req.URL.Query().Get("path")
	if relativePath == "" {
		relativePath = "."
	}

	files, err := r.workspaceManager.ListFiles(sessionID, relativePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	responses := make([]FileInfoResponse, 0, len(files))
	for _, f := range files {
		responses = append(responses, FileInfoResponse{
			Name:    f.Name,
			Path:    f.Path,
			IsDir:   f.IsDir,
			Size:    f.Size,
			ModTime: f.ModTime.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}
