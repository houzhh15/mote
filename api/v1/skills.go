package v1

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/gorilla/mux"

	"mote/internal/skills"
)

// SkillInfo represents skill information for API response.
type SkillInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	State       string   `json:"state"`
	Source      string   `json:"source"`
	Tools       []string `json:"tools,omitempty"`
	Hooks       []string `json:"hooks,omitempty"`
	Prompts     int      `json:"prompts,omitempty"`
}

// SkillListResponse is the response for listing skills.
type SkillListResponse struct {
	Skills []*SkillInfo `json:"skills"`
	Count  int          `json:"count"`
}

// SkillConfigRequest is the request to update skill config.
type SkillConfigRequest struct {
	Config map[string]any `json:"config"`
}

// HandleListSkills returns all available skills.
func (r *Router) HandleListSkills(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	skillList := r.skillManager.ListSkills()
	infos := make([]*SkillInfo, 0, len(skillList))

	for _, status := range skillList {
		if status.Skill == nil {
			continue
		}
		skill := status.Skill

		toolNames := make([]string, 0)
		for _, t := range skill.Tools {
			toolNames = append(toolNames, t.Name)
		}

		hookTypes := make([]string, 0)
		for _, h := range skill.Hooks {
			hookTypes = append(hookTypes, h.Type)
		}

		info := &SkillInfo{
			ID:          skill.ID,
			Name:        skill.Name,
			Description: skill.Description,
			Version:     skill.Version,
			State:       string(status.State),
			Source:      "manifest",
			Tools:       toolNames,
			Hooks:       hookTypes,
			Prompts:     len(skill.Prompts),
		}
		infos = append(infos, info)
	}

	resp := SkillListResponse{
		Skills: infos,
		Count:  len(infos),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleGetSkill returns a specific skill.
func (r *Router) HandleGetSkill(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	skillID := vars["id"]

	skill, found := r.skillManager.GetSkill(skillID)
	if !found {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	toolNames := make([]string, 0)
	for _, t := range skill.Tools {
		toolNames = append(toolNames, t.Name)
	}

	hookTypes := make([]string, 0)
	for _, h := range skill.Hooks {
		hookTypes = append(hookTypes, h.Type)
	}

	info := &SkillInfo{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Version:     skill.Version,
		State:       "registered", // Default state
		Source:      "manifest",
		Tools:       toolNames,
		Hooks:       hookTypes,
		Prompts:     len(skill.Prompts),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// HandleActivateSkill activates a skill.
func (r *Router) HandleActivateSkill(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	skillID := vars["id"]

	// Parse optional config
	var configReq SkillConfigRequest
	if req.Body != nil && req.ContentLength > 0 {
		json.NewDecoder(req.Body).Decode(&configReq)
	}

	if err := r.skillManager.Activate(skillID, configReq.Config); err != nil {
		if err == skills.ErrSkillNotFound {
			http.Error(w, "skill not found", http.StatusNotFound)
			return
		}
		if err == skills.ErrSkillAlreadyActive {
			http.Error(w, "skill already active", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "skill activated",
	})
}

// HandleDeactivateSkill deactivates a skill.
func (r *Router) HandleDeactivateSkill(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	skillID := vars["id"]

	if err := r.skillManager.Deactivate(skillID); err != nil {
		if err == skills.ErrSkillNotFound {
			http.Error(w, "skill not found", http.StatusNotFound)
			return
		}
		if err == skills.ErrSkillNotActive {
			http.Error(w, "skill not active", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "skill deactivated",
	})
}

// HandleGetSkillConfig returns skill configuration.
func (r *Router) HandleGetSkillConfig(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	skillID := vars["id"]

	config, err := r.skillManager.GetSkillConfig(skillID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"skill_id": skillID,
		"config":   config,
	})
}

// HandleSetSkillConfig updates skill configuration.
func (r *Router) HandleSetSkillConfig(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(req)
	skillID := vars["id"]

	var configReq SkillConfigRequest
	if err := json.NewDecoder(req.Body).Decode(&configReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := r.skillManager.SetSkillConfig(skillID, configReq.Config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "config updated",
	})
}

// HandleReloadSkills reloads all skills from disk.
func (r *Router) HandleReloadSkills(w http.ResponseWriter, req *http.Request) {
	if r.skillManager == nil {
		http.Error(w, "skill manager not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := r.skillManager.ScanAllPaths(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	skillList := r.skillManager.ListSkills()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"message": "skills reloaded",
		"count":   len(skillList),
	})
}

// CreateSkillRequest is the request to create a new skill template.
type CreateSkillRequest struct {
	Name   string `json:"name"`
	Target string `json:"target"` // "user" or "workspace"
}

// CreateSkillResponse is the response for creating a skill template.
type CreateSkillResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// HandleCreateSkill creates a new skill template.
func (r *Router) HandleCreateSkill(w http.ResponseWriter, req *http.Request) {
	var createReq CreateSkillRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if createReq.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Default to user target if not specified
	target := skills.TargetUser
	if createReq.Target == "workspace" {
		target = skills.TargetWorkspace
	} else if createReq.Target != "" && createReq.Target != "user" {
		http.Error(w, "target must be 'user' or 'workspace'", http.StatusBadRequest)
		return
	}

	path, err := skills.CreateTemplate(createReq.Name, target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateSkillResponse{
		Status:  "ok",
		Message: "skill template created",
		Path:    path,
	})
}

// OpenSkillsDirRequest is the request to open skills directory.
type OpenSkillsDirRequest struct {
	Target string `json:"target"` // "user" or "workspace"
}

// HandleOpenSkillsDir opens the skills directory in the system file manager.
func (r *Router) HandleOpenSkillsDir(w http.ResponseWriter, req *http.Request) {
	var openReq OpenSkillsDirRequest
	if err := json.NewDecoder(req.Body).Decode(&openReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default to user target if not specified
	target := skills.TargetUser
	if openReq.Target == "workspace" {
		target = skills.TargetWorkspace
	} else if openReq.Target != "" && openReq.Target != "user" {
		http.Error(w, "target must be 'user' or 'workspace'", http.StatusBadRequest)
		return
	}

	// Get the skills directory path
	dir, err := skills.GetSkillsDir(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"message": "opened skills directory",
		"path":    dir,
	})
}
