package v1

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	_ = json.NewEncoder(w).Encode(resp)
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
	_ = json.NewEncoder(w).Encode(resp)
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
	if relativePath == "" || relativePath == "/" {
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"files": responses,
	})
}

// DirectoryEntry represents a directory item for the directory browser.
type DirectoryEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// HandleBrowseDirectory lists directories (and optionally files) at a given system path.
// This is used by the frontend directory picker to browse the filesystem.
//
//	GET /api/v1/browse-directory?path=/some/path&dirs_only=true
//
// If path is empty, returns the user's home directory contents.
// On Windows with an empty path, returns available drive letters.
func (r *Router) HandleBrowseDirectory(w http.ResponseWriter, req *http.Request) {
	requestedPath := req.URL.Query().Get("path")
	dirsOnly := req.URL.Query().Get("dirs_only") != "false" // default true

	// If no path specified, use home directory (or drive roots on Windows)
	if requestedPath == "" {
		if runtime.GOOS == "windows" {
			// List available drive letters on Windows
			drives := listWindowsDrives()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"path":    "",
				"parent":  "",
				"entries": drives,
			})
			return
		}
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "cannot determine home directory", http.StatusInternalServerError)
			return
		}
		requestedPath = home
	}

	// Clean and resolve the path
	absPath, err := filepath.Abs(requestedPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Verify the path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "path does not exist", http.StatusNotFound)
		} else {
			http.Error(w, "cannot access path: "+err.Error(), http.StatusForbidden)
		}
		return
	}
	if !info.IsDir() {
		http.Error(w, "path is not a directory", http.StatusBadRequest)
		return
	}

	// Read directory entries
	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		http.Error(w, "cannot read directory: "+err.Error(), http.StatusForbidden)
		return
	}

	entries := make([]DirectoryEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		// Skip hidden files/directories (starting with .)
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		isDir := entry.IsDir()

		// If dirs_only, skip non-directory entries
		if dirsOnly && !isDir {
			continue
		}

		entries = append(entries, DirectoryEntry{
			Name:  entry.Name(),
			Path:  filepath.Join(absPath, entry.Name()),
			IsDir: isDir,
		})
	}

	// Sort: directories first, then by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	// Compute parent directory
	parent := filepath.Dir(absPath)
	if parent == absPath {
		// We're at the root (/ on Unix, C:\ on Windows)
		if runtime.GOOS == "windows" {
			parent = "" // signal to show drive list
		} else {
			parent = "" // no parent for /
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"path":    absPath,
		"parent":  parent,
		"entries": entries,
	})
}

// listWindowsDrives returns available drive letters on Windows.
func listWindowsDrives() []DirectoryEntry {
	drives := make([]DirectoryEntry, 0)
	if runtime.GOOS != "windows" {
		return drives
	}
	// Check drives A-Z
	for letter := 'A'; letter <= 'Z'; letter++ {
		drivePath := string(letter) + ":\\"
		if _, err := os.Stat(drivePath); err == nil {
			drives = append(drives, DirectoryEntry{
				Name:  string(letter) + ":",
				Path:  drivePath,
				IsDir: true,
			})
		}
	}
	return drives
}

// HandleOpenWorkspaceDir opens a workspace directory in the system file manager.
func (r *Router) HandleOpenWorkspaceDir(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	dir := body.Path

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		http.Error(w, "directory does not exist: "+dir, http.StatusBadRequest)
		return
	}

	// Open in file manager using platform-specific command
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "linux":
		cmd = exec.Command("xdg-open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		http.Error(w, "unsupported operating system", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, "failed to open directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"message": "opened directory in file manager",
		"path":    dir,
	})
}
