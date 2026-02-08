package v1

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/gorilla/mux"

	"mote/internal/gateway/handlers"
	"mote/internal/tools"
)

// HandleListTools returns all available tools.
func (r *Router) HandleListTools(w http.ResponseWriter, req *http.Request) {
	if r.tools == nil {
		handlers.SendJSON(w, http.StatusOK, ToolsListResponse{
			Tools: []ToolInfo{},
			Count: 0,
		})
		return
	}

	tools := r.tools.List()

	var toolInfos []ToolInfo
	for _, t := range tools {
		toolInfos = append(toolInfos, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Parameters(),
			Type:        getToolType(t),
		})
	}

	if toolInfos == nil {
		toolInfos = []ToolInfo{}
	}

	handlers.SendJSON(w, http.StatusOK, ToolsListResponse{
		Tools: toolInfos,
		Count: len(toolInfos),
	})
}

// HandleGetTool returns details of a specific tool.
func (r *Router) HandleGetTool(w http.ResponseWriter, req *http.Request) {
	if r.tools == nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Tool registry not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	tool, ok := r.tools.Get(name)
	if !ok {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Tool not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, ToolInfo{
		Name:        tool.Name(),
		Description: tool.Description(),
		Schema:      tool.Parameters(),
		Type:        getToolType(tool),
	})
}

// HandleExecuteTool executes a specific tool.
func (r *Router) HandleExecuteTool(w http.ResponseWriter, req *http.Request) {
	if r.tools == nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Tool registry not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	tool, ok := r.tools.Get(name)
	if !ok {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Tool not found")
		return
	}

	var execReq ToolExecuteRequest
	if err := json.NewDecoder(req.Body).Decode(&execReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	ctx := req.Context()
	result, err := tool.Execute(ctx, execReq.Params)
	if err != nil {
		handlers.SendJSON(w, http.StatusOK, ToolExecuteResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Convert result to response format
	var resultData any
	if result.Content != "" {
		resultData = result.Content
	}
	if result.Metadata != nil {
		resultData = map[string]any{
			"content":  result.Content,
			"metadata": result.Metadata,
		}
	}

	handlers.SendJSON(w, http.StatusOK, ToolExecuteResponse{
		Success: true,
		Result:  resultData,
	})
}

// toolTyper is an interface for tools that can report their type.
type toolTyper interface {
	Type() string
}

// getToolType returns the type of a tool.
func getToolType(t any) string {
	if typed, ok := t.(toolTyper); ok {
		return typed.Type()
	}
	return "builtin"
}

// CreateToolRequest is the request to create a new tool template.
type CreateToolRequest struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime"` // "javascript", "python", "shell", "powershell"
	Target  string `json:"target"`  // "user" or "workspace"
}

// CreateToolResponse is the response for creating a tool template.
type CreateToolResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// HandleCreateTool creates a new tool template.
func (r *Router) HandleCreateTool(w http.ResponseWriter, req *http.Request) {
	var createReq CreateToolRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if createReq.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Default to user target if not specified
	target := tools.TargetUser
	if createReq.Target == "workspace" {
		target = tools.TargetWorkspace
	} else if createReq.Target != "" && createReq.Target != "user" {
		http.Error(w, "target must be 'user' or 'workspace'", http.StatusBadRequest)
		return
	}

	// Default to javascript runtime if not specified
	runtimeType := tools.RuntimeJavaScript
	switch createReq.Runtime {
	case "python":
		runtimeType = tools.RuntimePython
	case "shell", "bash":
		runtimeType = tools.RuntimeBash
	case "powershell":
		runtimeType = tools.RuntimePowerShell
	case "javascript", "":
		runtimeType = tools.RuntimeJavaScript
	default:
		http.Error(w, "runtime must be 'javascript', 'python', 'shell', or 'powershell'", http.StatusBadRequest)
		return
	}

	path, err := tools.CreateToolTemplate(createReq.Name, runtimeType, target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	handlers.SendJSON(w, http.StatusOK, CreateToolResponse{
		Status:  "ok",
		Message: "tool template created",
		Path:    path,
	})
}

// OpenToolsDirRequest is the request to open tools directory.
type OpenToolsDirRequest struct {
	Target string `json:"target"` // "user" or "workspace"
}

// HandleOpenToolsDir opens the tools directory in the system file manager.
func (r *Router) HandleOpenToolsDir(w http.ResponseWriter, req *http.Request) {
	var openReq OpenToolsDirRequest
	if err := json.NewDecoder(req.Body).Decode(&openReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default to user target if not specified
	target := tools.TargetUser
	if openReq.Target == "workspace" {
		target = tools.TargetWorkspace
	} else if openReq.Target != "" && openReq.Target != "user" {
		http.Error(w, "target must be 'user' or 'workspace'", http.StatusBadRequest)
		return
	}

	// Get the tools directory path
	dir, err := tools.GetToolsDir(target)
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

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "opened tools directory",
		"path":    dir,
	})
}
