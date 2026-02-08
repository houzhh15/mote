package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mote/internal/workspace"

	"github.com/gorilla/mux"
)

func TestHandleListWorkspaces(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()
	err := wm.Bind("session-1", tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to bind workspace: %v", err)
	}

	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	w := httptest.NewRecorder()

	router.HandleListWorkspaces(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp []WorkspaceResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(resp))
	}
}

func TestHandleListWorkspaces_NilManager(t *testing.T) {
	router := &Router{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	w := httptest.NewRecorder()

	router.HandleListWorkspaces(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleGetWorkspace(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()
	wm.BindWithAlias("session-1", tmpDir, "myalias", true)

	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/session-1", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionId": "session-1"})
	w := httptest.NewRecorder()

	router.HandleGetWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp WorkspaceResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", resp.SessionID)
	}
	if resp.Alias != "myalias" {
		t.Errorf("expected alias myalias, got %s", resp.Alias)
	}
}

func TestHandleGetWorkspace_NotFound(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/nonexistent", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionId": "nonexistent"})
	w := httptest.NewRecorder()

	router.HandleGetWorkspace(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleBindWorkspace(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()

	router := &Router{workspaceManager: wm}
	body := WorkspaceBindRequest{
		SessionID: "session-1",
		Path:      tmpDir,
		ReadOnly:  false,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	router.HandleBindWorkspace(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp WorkspaceResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", resp.SessionID)
	}
}

func TestHandleBindWorkspace_WithAlias(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()

	router := &Router{workspaceManager: wm}
	body := WorkspaceBindRequest{
		SessionID: "session-1",
		Path:      tmpDir,
		Alias:     "myproject",
		ReadOnly:  true,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	router.HandleBindWorkspace(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp WorkspaceResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Alias != "myproject" {
		t.Errorf("expected alias myproject, got %s", resp.Alias)
	}
	if !resp.ReadOnly {
		t.Error("expected read-only to be true")
	}
}

func TestHandleBindWorkspace_InvalidBody(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()

	router.HandleBindWorkspace(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleUnbindWorkspace(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()
	wm.Bind("session-1", tmpDir, false)

	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/session-1", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionId": "session-1"})
	w := httptest.NewRecorder()

	router.HandleUnbindWorkspace(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestHandleUnbindWorkspace_NotFound(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/nonexistent", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionId": "nonexistent"})
	w := httptest.NewRecorder()

	router.HandleUnbindWorkspace(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleListWorkspaceFiles(t *testing.T) {
	wm := workspace.NewWorkspaceManager()
	tmpDir := t.TempDir()
	wm.Bind("session-1", tmpDir, false)

	router := &Router{workspaceManager: wm}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/session-1/files", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionId": "session-1"})
	w := httptest.NewRecorder()

	router.HandleListWorkspaceFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
