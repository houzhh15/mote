package v1

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gorilla/mux"

	"mote/internal/gateway/handlers"
	"mote/internal/prompts"
)

// PromptInfo represents prompt information for API response.
type PromptInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// PromptListResponse is the response for listing prompts.
type PromptListResponse struct {
	Prompts []*PromptInfo `json:"prompts"`
	Count   int           `json:"count"`
}

// CreatePromptRequest is the request to create a prompt.
type CreatePromptRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// UpdatePromptRequest is the request to update a prompt.
type UpdatePromptRequest struct {
	Content  *string `json:"content,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

// HandleListPrompts returns all user prompts.
func (r *Router) HandleListPrompts(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	promptList := r.promptManager.ListPrompts()
	infos := make([]*PromptInfo, 0, len(promptList))

	for _, p := range promptList {
		info := &PromptInfo{
			ID:        p.ID,
			Name:      p.Name,
			Type:      string(p.Type),
			Content:   p.Content,
			Priority:  p.Priority,
			Enabled:   p.Enabled,
			CreatedAt: p.CreatedAt.Format(time.RFC3339),
			UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
		}
		infos = append(infos, info)
	}

	resp := PromptListResponse{
		Prompts: infos,
		Count:   len(infos),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleGetPrompt returns a specific prompt.
func (r *Router) HandleGetPrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	promptID := vars["id"]

	prompt, err := r.promptManager.GetPrompt(promptID)
	if err != nil {
		http.Error(w, "prompt not found", http.StatusNotFound)
		return
	}

	info := &PromptInfo{
		ID:        prompt.ID,
		Name:      prompt.Name,
		Type:      string(prompt.Type),
		Content:   prompt.Content,
		Priority:  prompt.Priority,
		Enabled:   prompt.Enabled,
		CreatedAt: prompt.CreatedAt.Format(time.RFC3339),
		UpdatedAt: prompt.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// HandleCreatePrompt creates a new prompt.
func (r *Router) HandleCreatePrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	var createReq CreatePromptRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if createReq.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if createReq.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	promptType := prompts.PromptType(createReq.Type)
	if promptType == "" {
		promptType = prompts.PromptTypeSystem
	}

	prompt, err := r.promptManager.AddPrompt(prompts.PromptConfig{
		Name:     createReq.Name,
		Type:     promptType,
		Content:  createReq.Content,
		Priority: createReq.Priority,
		Enabled:  createReq.Enabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info := &PromptInfo{
		ID:        prompt.ID,
		Name:      prompt.Name,
		Type:      string(prompt.Type),
		Content:   prompt.Content,
		Priority:  prompt.Priority,
		Enabled:   prompt.Enabled,
		CreatedAt: prompt.CreatedAt.Format(time.RFC3339),
		UpdatedAt: prompt.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(info)
}

// HandleUpdatePrompt updates an existing prompt.
func (r *Router) HandleUpdatePrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	promptID := vars["id"]

	var updateReq UpdatePromptRequest
	if err := json.NewDecoder(req.Body).Decode(&updateReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Update content if provided
	if updateReq.Content != nil {
		if err := r.promptManager.UpdatePrompt(promptID, *updateReq.Content); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	// Update priority if provided
	if updateReq.Priority != nil {
		if err := r.promptManager.SetPriority(promptID, *updateReq.Priority); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	// Update enabled if provided
	if updateReq.Enabled != nil {
		if *updateReq.Enabled {
			if err := r.promptManager.EnablePrompt(promptID); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
		} else {
			if err := r.promptManager.DisablePrompt(promptID); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
		}
	}

	// Return updated prompt
	prompt, err := r.promptManager.GetPrompt(promptID)
	if err != nil {
		http.Error(w, "prompt not found", http.StatusNotFound)
		return
	}

	info := &PromptInfo{
		ID:        prompt.ID,
		Name:      prompt.Name,
		Type:      string(prompt.Type),
		Content:   prompt.Content,
		Priority:  prompt.Priority,
		Enabled:   prompt.Enabled,
		CreatedAt: prompt.CreatedAt.Format(time.RFC3339),
		UpdatedAt: prompt.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// HandleDeletePrompt deletes a prompt.
func (r *Router) HandleDeletePrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	promptID := vars["id"]

	if err := r.promptManager.RemovePrompt(promptID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTogglePrompt enables or disables a prompt.
func (r *Router) HandleTogglePrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	promptID := vars["id"]

	// Get current state
	prompt, err := r.promptManager.GetPrompt(promptID)
	if err != nil {
		http.Error(w, "prompt not found", http.StatusNotFound)
		return
	}

	// Toggle
	if prompt.Enabled {
		if err := r.promptManager.DisablePrompt(promptID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := r.promptManager.EnablePrompt(promptID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"enabled": !prompt.Enabled,
	})
}

// OpenPromptsDirRequest is the request to open prompts directory.
type OpenPromptsDirRequest struct {
	Target string `json:"target"` // "user" or "workspace"
}

// HandleOpenPromptsDir opens the prompts directory in the file manager.
func (r *Router) HandleOpenPromptsDir(w http.ResponseWriter, req *http.Request) {
	var openReq OpenPromptsDirRequest
	if err := json.NewDecoder(req.Body).Decode(&openReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var dir string
	switch openReq.Target {
	case "workspace":
		dir = filepath.Join(".", ".mote", "prompts")
	case "user", "":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "failed to get home directory", http.StatusInternalServerError)
			return
		}
		dir = filepath.Join(homeDir, ".mote", "prompts")
	default:
		http.Error(w, "target must be 'user' or 'workspace'", http.StatusBadRequest)
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Open in file manager
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "linux":
		cmd = exec.Command("xdg-open", dir)
	default:
		http.Error(w, "unsupported platform", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"path":   dir,
	})
}

// RenderPromptRequest is the request to render a prompt with variables.
type RenderPromptRequest struct {
	Variables map[string]string `json:"variables"`
}

// HandleRenderPrompt renders a prompt with variables.
func (r *Router) HandleRenderPrompt(w http.ResponseWriter, req *http.Request) {
	if r.promptManager == nil {
		http.Error(w, "prompt manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	promptID := vars["id"]

	var renderReq RenderPromptRequest
	if err := json.NewDecoder(req.Body).Decode(&renderReq); err != nil {
		// Allow empty body (no variables)
		renderReq.Variables = make(map[string]string)
	}

	prompt, err := r.promptManager.GetPrompt(promptID)
	if err != nil {
		http.Error(w, "prompt not found", http.StatusNotFound)
		return
	}

	// Simple variable replacement ({{variable}})
	content := prompt.Content
	for key, value := range renderReq.Variables {
		content = replaceVariable(content, key, value)
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"content": content,
	})
}

// replaceVariable replaces {{key}} with value in content
func replaceVariable(content, key, value string) string {
	placeholder := "{{" + key + "}}"
	result := content
	for i := 0; i < 100; i++ { // Prevent infinite loop
		idx := indexOf(result, placeholder)
		if idx == -1 {
			break
		}
		result = result[:idx] + value + result[idx+len(placeholder):]
	}
	return result
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
