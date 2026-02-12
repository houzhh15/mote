package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	internalChannel "mote/internal/channel"
	"mote/internal/cron"
	"mote/internal/gateway/handlers"
	"mote/internal/mcp/client"
	"mote/internal/mcp/server"
	"mote/internal/memory"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/prompts"
	"mote/internal/provider"
	"mote/internal/provider/copilot"
	"mote/internal/runner"
	"mote/internal/skills"
	"mote/internal/storage"
	"mote/internal/tools"
	"mote/internal/ui"
	"mote/internal/workspace"
)

// EmbeddedServerInterface defines the interface for the embedded server that supports hot reload.
type EmbeddedServerInterface interface {
	ReloadProviders() error
}

// RouterDeps holds dependencies for the v1 API router.
type RouterDeps struct {
	Runner           *runner.Runner
	Tools            *tools.Registry
	Memory           *memory.MemoryIndex
	MCPClient        *client.Manager
	MCPServer        *server.Server
	DB               *storage.DB
	CronScheduler    *cron.Scheduler
	UIHandler        *ui.Handler
	PolicyExecutor   *policy.PolicyExecutor
	ApprovalManager  *approval.Manager
	ChannelRegistry  *internalChannel.Registry
	WorkspaceManager *workspace.WorkspaceManager
	SkillManager     *skills.Manager
	PromptManager    *prompts.Manager
	MultiPool        *provider.MultiProviderPool // Multi-provider support
	EmbeddedServer   EmbeddedServerInterface     // Embedded server for hot reload
}

// Router wraps v1 API dependencies.
type Router struct {
	runner           *runner.Runner
	tools            *tools.Registry
	memory           *memory.MemoryIndex
	mcpClient        *client.Manager
	mcpServer        *server.Server
	db               *storage.DB
	cronScheduler    *cron.Scheduler
	uiHandler        *ui.Handler
	policyExecutor   *policy.PolicyExecutor
	approvalManager  *approval.Manager
	channelRegistry  *internalChannel.Registry
	workspaceManager *workspace.WorkspaceManager
	skillManager     *skills.Manager
	promptManager    *prompts.Manager
	multiPool        *provider.MultiProviderPool
	embeddedServer   EmbeddedServerInterface
	versionChecker   interface{} // *skills.VersionChecker
	skillUpdater     interface{} // *skills.SkillUpdater
}

// NewRouter creates a new v1 API router.
func NewRouter(deps *RouterDeps) *Router {
	if deps == nil {
		deps = &RouterDeps{}
	}
	return &Router{
		runner:           deps.Runner,
		tools:            deps.Tools,
		memory:           deps.Memory,
		mcpClient:        deps.MCPClient,
		mcpServer:        deps.MCPServer,
		db:               deps.DB,
		cronScheduler:    deps.CronScheduler,
		uiHandler:        deps.UIHandler,
		policyExecutor:   deps.PolicyExecutor,
		approvalManager:  deps.ApprovalManager,
		channelRegistry:  deps.ChannelRegistry,
		workspaceManager: deps.WorkspaceManager,
		skillManager:     deps.SkillManager,
		promptManager:    deps.PromptManager,
		multiPool:        deps.MultiPool,
		embeddedServer:   deps.EmbeddedServer,
	}
}

// SetRunner updates the runner dependency.
func (r *Router) SetRunner(runner *runner.Runner) {
	r.runner = runner
}

// SetTools updates the tools registry dependency.
func (r *Router) SetTools(tools *tools.Registry) {
	r.tools = tools
}

// SetMCPClient updates the MCP client manager dependency.
func (r *Router) SetMCPClient(mcpClient *client.Manager) {
	r.mcpClient = mcpClient
}

// SetMemory updates the memory index dependency.
func (r *Router) SetMemory(m *memory.MemoryIndex) {
	r.memory = m
}

// SetCronScheduler updates the cron scheduler dependency.
func (r *Router) SetCronScheduler(c *cron.Scheduler) {
	r.cronScheduler = c
}

// SetSkillManager updates the skill manager dependency.
func (r *Router) SetSkillManager(m *skills.Manager) {
	r.skillManager = m
}

// SetPromptManager updates the prompt manager dependency.
func (r *Router) SetPromptManager(m *prompts.Manager) {
	r.promptManager = m
}

// SetMultiPool updates the multi-provider pool dependency.
func (r *Router) SetMultiPool(pool *provider.MultiProviderPool) {
	r.multiPool = pool
}

// SetVersionChecker sets the version checker dependency.
func (r *Router) SetVersionChecker(vc interface{}) {
	r.versionChecker = vc
}

// SetSkillUpdater sets the skill updater dependency.
func (r *Router) SetSkillUpdater(su interface{}) {
	r.skillUpdater = su
}

// RegisterRoutes registers all v1 API routes.
func (r *Router) RegisterRoutes(router *mux.Router) {
	v1 := router.PathPrefix("/api/v1").Subrouter()

	// Health
	v1.HandleFunc("/health", r.HandleHealth).Methods(http.MethodGet)

	// Auth status (for web mode)
	v1.HandleFunc("/auth/status", r.HandleAuthStatus).Methods(http.MethodGet)

	// Chat
	v1.HandleFunc("/chat", r.HandleChat).Methods(http.MethodPost)
	v1.HandleFunc("/chat/stream", r.HandleChatStream).Methods(http.MethodPost)

	// Pause control
	v1.HandleFunc("/pause", r.HandlePause).Methods(http.MethodPost)
	v1.HandleFunc("/resume", r.HandleResume).Methods(http.MethodPost)
	v1.HandleFunc("/pause/status", r.HandlePauseStatus).Methods(http.MethodGet)

	// Sessions
	v1.HandleFunc("/sessions", r.HandleListSessions).Methods(http.MethodGet)
	v1.HandleFunc("/sessions", r.HandleCreateSession).Methods(http.MethodPost)
	v1.HandleFunc("/sessions/{id}", r.HandleGetSession).Methods(http.MethodGet)
	v1.HandleFunc("/sessions/{id}", r.HandleUpdateSession).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}", r.HandleDeleteSession).Methods(http.MethodDelete)
	v1.HandleFunc("/sessions/{id}/messages", r.HandleGetMessages).Methods(http.MethodGet)
	v1.HandleFunc("/sessions/{id}/model", r.HandleUpdateSessionModel).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}/skills", r.HandleUpdateSessionSkills).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}/reconfigure", r.HandleReconfigureSession).Methods(http.MethodPost)

	// Tools
	v1.HandleFunc("/tools", r.HandleListTools).Methods(http.MethodGet)
	v1.HandleFunc("/tools/create", r.HandleCreateTool).Methods(http.MethodPost)
	v1.HandleFunc("/tools/open", r.HandleOpenToolsDir).Methods(http.MethodPost)
	v1.HandleFunc("/tools/{name}", r.HandleGetTool).Methods(http.MethodGet)
	v1.HandleFunc("/tools/{name}/execute", r.HandleExecuteTool).Methods(http.MethodPost)

	// Memory
	v1.HandleFunc("/memory", r.HandleListMemory).Methods(http.MethodGet)
	v1.HandleFunc("/memory/search", r.HandleMemorySearch).Methods(http.MethodPost)
	v1.HandleFunc("/memory", r.HandleAddMemory).Methods(http.MethodPost)
	// P1: Memory sync, daily, export, import, batch delete (must be before /memory/{id})
	v1.HandleFunc("/memory/sync", r.HandleMemorySync).Methods(http.MethodPost)
	v1.HandleFunc("/memory/daily", r.HandleGetDaily).Methods(http.MethodGet)
	v1.HandleFunc("/memory/daily", r.HandleAppendDaily).Methods(http.MethodPost)
	v1.HandleFunc("/memory/export", r.HandleMemoryExport).Methods(http.MethodGet)
	v1.HandleFunc("/memory/import", r.HandleMemoryImport).Methods(http.MethodPost)
	v1.HandleFunc("/memory/batch", r.HandleBatchDelete).Methods(http.MethodDelete)
	// P2: Memory stats
	v1.HandleFunc("/memory/stats", r.HandleMemoryStats).Methods(http.MethodGet)
	// Generic ID routes (must be after specific paths)
	v1.HandleFunc("/memory/{id}", r.HandleGetMemory).Methods(http.MethodGet)
	v1.HandleFunc("/memory/{id}", r.HandleUpdateMemory).Methods(http.MethodPut)
	v1.HandleFunc("/memory/{id}", r.HandleDeleteMemory).Methods(http.MethodDelete)

	// Cron
	v1.HandleFunc("/cron/jobs", r.HandleListCronJobs).Methods(http.MethodGet)
	v1.HandleFunc("/cron/jobs", r.HandleCreateCronJob).Methods(http.MethodPost)
	v1.HandleFunc("/cron/jobs/{name}", r.HandleGetCronJob).Methods(http.MethodGet)
	v1.HandleFunc("/cron/jobs/{name}", r.HandleUpdateCronJob).Methods(http.MethodPut)
	v1.HandleFunc("/cron/jobs/{name}", r.HandleDeleteCronJob).Methods(http.MethodDelete)
	v1.HandleFunc("/cron/jobs/{name}/run", r.HandleRunCronJob).Methods(http.MethodPost)
	v1.HandleFunc("/cron/history", r.HandleCronHistory).Methods(http.MethodGet)

	// Config
	v1.HandleFunc("/config", r.HandleGetConfig).Methods(http.MethodGet)
	v1.HandleFunc("/config", r.HandleUpdateConfig).Methods(http.MethodPut)
	v1.HandleFunc("/config/reload", r.HandleReloadConfig).Methods(http.MethodPost)

	// Models
	v1.HandleFunc("/models", r.HandleListModels).Methods(http.MethodGet)
	v1.HandleFunc("/models/current", r.HandleSetCurrentModel).Methods(http.MethodPut)

	// Providers - Status and Recovery
	v1.HandleFunc("/providers/status", r.HandleProviderStatus).Methods(http.MethodGet)
	v1.HandleFunc("/providers/{name}/recover", r.HandleProviderRecover).Methods(http.MethodPost)

	// Settings - Scenario Models
	v1.HandleFunc("/settings/models", r.HandleGetScenarioModels).Methods(http.MethodGet)
	v1.HandleFunc("/settings/models", r.HandleUpdateScenarioModels).Methods(http.MethodPut)

	// MCP
	v1.HandleFunc("/mcp/servers", r.HandleListMCPServers).Methods(http.MethodGet)
	v1.HandleFunc("/mcp/servers", r.HandleAddMCPServer).Methods(http.MethodPost)
	v1.HandleFunc("/mcp/servers/import", r.HandleImportMCPServers).Methods(http.MethodPost)
	v1.HandleFunc("/mcp/servers/{name}", r.HandleGetMCPServer).Methods(http.MethodGet)
	v1.HandleFunc("/mcp/servers/{name}", r.HandleUpdateMCPServer).Methods(http.MethodPut)
	v1.HandleFunc("/mcp/servers/{name}", r.HandleDeleteMCPServer).Methods(http.MethodDelete)
	v1.HandleFunc("/mcp/servers/{name}/stop", r.HandleStopMCPServer).Methods(http.MethodPost)
	v1.HandleFunc("/mcp/servers/{name}/restart", r.HandleRestartMCPServer).Methods(http.MethodPost)
	v1.HandleFunc("/mcp/tools", r.HandleListMCPTools).Methods(http.MethodGet)
	v1.HandleFunc("/mcp/prompts", r.HandleListMCPPrompts).Methods(http.MethodGet)
	v1.HandleFunc("/mcp/prompts/{server}/{name}", r.HandleGetMCPPrompt).Methods(http.MethodPost)

	// UI
	v1.HandleFunc("/ui/components", r.HandleUIComponents).Methods(http.MethodGet)
	v1.HandleFunc("/ui/state", r.HandleGetUIState).Methods(http.MethodGet)
	v1.HandleFunc("/ui/state", r.HandleUpdateUIState).Methods(http.MethodPut)

	// Policy (M08)
	v1.HandleFunc("/policy/status", r.HandlePolicyStatus).Methods(http.MethodGet)
	v1.HandleFunc("/policy/check", r.HandlePolicyCheck).Methods(http.MethodPost)

	// Approvals (M08)
	v1.HandleFunc("/approvals", r.HandleApprovalList).Methods(http.MethodGet)
	v1.HandleFunc("/approvals/{id}/respond", r.HandleApprovalRespond).Methods(http.MethodPost)

	// Channels
	v1.HandleFunc("/channels", r.HandleListChannels).Methods(http.MethodGet)
	v1.HandleFunc("/channels/{type}/config", r.HandleGetChannelConfig).Methods(http.MethodGet)
	v1.HandleFunc("/channels/{type}/config", r.HandleUpdateChannelConfig).Methods(http.MethodPut)
	v1.HandleFunc("/channels/{type}/start", r.HandleStartChannel).Methods(http.MethodPost)
	v1.HandleFunc("/channels/{type}/stop", r.HandleStopChannel).Methods(http.MethodPost)

	// Workspace
	v1.HandleFunc("/workspaces", r.HandleListWorkspaces).Methods(http.MethodGet)
	v1.HandleFunc("/workspaces", r.HandleBindWorkspace).Methods(http.MethodPost)
	v1.HandleFunc("/workspaces/{sessionId}", r.HandleGetWorkspace).Methods(http.MethodGet)
	v1.HandleFunc("/workspaces/{sessionId}", r.HandleUnbindWorkspace).Methods(http.MethodDelete)
	v1.HandleFunc("/workspaces/{sessionId}/files", r.HandleListWorkspaceFiles).Methods(http.MethodGet)
	v1.HandleFunc("/browse-directory", r.HandleBrowseDirectory).Methods(http.MethodGet)

	// Skills
	v1.HandleFunc("/skills", r.HandleListSkills).Methods(http.MethodGet)
	v1.HandleFunc("/skills/create", r.HandleCreateSkill).Methods(http.MethodPost)
	v1.HandleFunc("/skills/open", r.HandleOpenSkillsDir).Methods(http.MethodPost)
	v1.HandleFunc("/skills/reload", r.HandleReloadSkills).Methods(http.MethodPost)
	v1.HandleFunc("/skills/check-updates", r.HandleCheckSkillUpdates).Methods(http.MethodPost)
	v1.HandleFunc("/skills/{id}", r.HandleGetSkill).Methods(http.MethodGet)
	v1.HandleFunc("/skills/{id}/activate", r.HandleActivateSkill).Methods(http.MethodPost)
	v1.HandleFunc("/skills/{id}/deactivate", r.HandleDeactivateSkill).Methods(http.MethodPost)
	v1.HandleFunc("/skills/{id}/config", r.HandleGetSkillConfig).Methods(http.MethodGet)
	v1.HandleFunc("/skills/{id}/config", r.HandleSetSkillConfig).Methods(http.MethodPut)
	v1.HandleFunc("/skills/{id}/update", r.HandleUpdateSkill).Methods(http.MethodPost)

	// Prompts
	v1.HandleFunc("/prompts", r.HandleListPrompts).Methods(http.MethodGet)
	v1.HandleFunc("/prompts", r.HandleCreatePrompt).Methods(http.MethodPost)
	v1.HandleFunc("/prompts/open", r.HandleOpenPromptsDir).Methods(http.MethodPost)
	v1.HandleFunc("/prompts/{id}", r.HandleGetPrompt).Methods(http.MethodGet)
	v1.HandleFunc("/prompts/{id}", r.HandleUpdatePrompt).Methods(http.MethodPut)
	v1.HandleFunc("/prompts/{id}", r.HandleDeletePrompt).Methods(http.MethodDelete)
	v1.HandleFunc("/prompts/{id}/toggle", r.HandleTogglePrompt).Methods(http.MethodPost)
	v1.HandleFunc("/prompts/reload", r.HandleReloadPrompts).Methods(http.MethodPost)
	v1.HandleFunc("/prompts/{id}/render", r.HandleRenderPrompt).Methods(http.MethodPost)
}

// SetupLegacyRedirects sets up redirects from old /api/ to /api/v1/.
func SetupLegacyRedirects(router *mux.Router) {
	legacyRedirects := map[string]string{
		"/api/health":    "/api/v1/health",
		"/api/cron/jobs": "/api/v1/cron/jobs",
		"/api/sessions":  "/api/v1/sessions",
		"/api/config":    "/api/v1/config",
	}

	for old, newPath := range legacyRedirects {
		target := newPath // Capture for closure
		router.HandleFunc(old, func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, target+req.URL.RawQuery, http.StatusPermanentRedirect)
		}).Methods(http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete)
	}
}

// HandleHealth returns the health status of the API.
func (r *Router) HandleHealth(w http.ResponseWriter, req *http.Request) {
	components := make(map[string]ComponentHealth)

	// Check database
	if r.db != nil {
		if err := r.db.Ping(); err != nil {
			components["database"] = ComponentHealth{Status: "unhealthy", Message: err.Error()}
		} else {
			components["database"] = ComponentHealth{Status: "healthy"}
		}
	}

	// Check memory
	if r.memory != nil {
		components["memory"] = ComponentHealth{Status: "healthy"}
	} else {
		components["memory"] = ComponentHealth{Status: "disabled"}
	}

	// Check cron
	if r.cronScheduler != nil {
		components["cron"] = ComponentHealth{Status: "healthy"}
	} else {
		components["cron"] = ComponentHealth{Status: "disabled"}
	}

	// Determine overall status
	status := "healthy"
	for _, comp := range components {
		if comp.Status == "unhealthy" {
			status = "degraded"
			break
		}
	}

	timestamp := time.Now().Format(time.RFC3339)
	if ts := req.Context().Value("request_time"); ts != nil {
		if s, ok := ts.(string); ok {
			timestamp = s
		}
	}

	handlers.SendJSON(w, http.StatusOK, HealthResponse{
		Status:     status,
		Version:    "0.1.0", // TODO: get from build info
		Timestamp:  timestamp,
		Components: components,
	})
}

// AuthStatusResponse represents the response for auth status check.
type AuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	TokenMasked   string `json:"token_masked,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	Message       string `json:"message,omitempty"`
}

// HandleAuthStatus returns the current authentication status.
func (r *Router) HandleAuthStatus(w http.ResponseWriter, req *http.Request) {
	// Check if token is configured
	token := viper.GetString("provider.api_key")
	if token == "" {
		token = viper.GetString("copilot.token")
	}

	response := AuthStatusResponse{
		Provider: viper.GetString("provider.type"),
		Model:    viper.GetString("provider.model"),
	}

	if token == "" {
		response.Authenticated = false
		response.Message = "未配置认证 Token，请运行 mote auth login 进行认证"
	} else {
		response.Authenticated = true
		// Mask token for display
		if len(token) > 8 {
			response.TokenMasked = token[:4] + "..." + token[len(token)-4:]
		} else {
			response.TokenMasked = "***"
		}
	}

	handlers.SendJSON(w, http.StatusOK, response)
}

// HandleListSessions returns a list of sessions.
func (r *Router) HandleListSessions(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	rows, err := r.db.Query(`
		SELECT s.id, s.created_at, s.updated_at, 
		       COALESCE(s.title, '') as title,
		       COALESCE(s.model, '') as model,
		       COALESCE(s.scenario, 'chat') as scenario,
		       COALESCE(s.selected_skills, '') as selected_skills,
		       (SELECT COUNT(*) FROM messages WHERE session_id = s.id) as message_count,
		       COALESCE(
		           (SELECT SUBSTR(content, 1, 50) FROM messages 
		            WHERE session_id = s.id AND role = 'user' 
		            ORDER BY created_at ASC LIMIT 1),
		           ''
		       ) as preview
		FROM sessions s
		ORDER BY s.updated_at DESC
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
		var preview string
		var selectedSkillsStr string
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.Title, &s.Model, &s.Scenario, &selectedSkillsStr, &s.MessageCount, &preview); err != nil {
			continue
		}
		if preview != "" {
			s.Preview = preview
		}
		s.SelectedSkills = parseSkillsJSON(selectedSkillsStr)
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []SessionSummary{}
	}

	handlers.SendJSON(w, http.StatusOK, SessionsListResponse{Sessions: sessions})
}

// HandleCreateSession creates a new session.
func (r *Router) HandleCreateSession(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	// Parse request body for optional scenario
	var body CreateSessionRequest
	if req.Body != nil {
		_ = json.NewDecoder(req.Body).Decode(&body) // Ignore errors, use defaults
	}

	// Generate a new session ID
	sessionID := generateSessionID()

	// Determine scenario and default model
	scenario := body.Scenario
	if scenario == "" {
		scenario = "chat"
	}

	// Get default model for the scenario
	var model string
	switch scenario {
	case "chat":
		model = viper.GetString("copilot.chat_model")
	case "cron":
		model = viper.GetString("cron.model")
	case "channel":
		model = viper.GetString("channels.model")
	default:
		model = viper.GetString("copilot.chat_model")
	}

	// Allow explicit model override
	if body.Model != "" {
		model = body.Model
	}

	// Insert session with scenario and model
	_, err := r.db.Exec(`INSERT INTO sessions (id, scenario, model) VALUES (?, ?, ?)`, sessionID, scenario, model)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to create session")
		return
	}

	handlers.SendJSON(w, http.StatusCreated, CreateSessionResponse{ID: sessionID, Model: model, Scenario: scenario})
}

// HandleGetSession returns details of a specific session.
func (r *Router) HandleGetSession(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	var s SessionSummary
	var title, model, scenario, selectedSkills sql.NullString
	err := r.db.QueryRow(`
		SELECT id, created_at, updated_at, title, model, scenario, selected_skills,
		       (SELECT COUNT(*) FROM messages WHERE session_id = sessions.id) as message_count
		FROM sessions 
		WHERE id = ?
	`, id).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &title, &model, &scenario, &selectedSkills, &s.MessageCount)

	if err != nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	if title.Valid {
		s.Title = title.String
	}
	if model.Valid {
		s.Model = model.String
	}
	if scenario.Valid {
		s.Scenario = scenario.String
	}
	if selectedSkills.Valid {
		s.SelectedSkills = parseSkillsJSON(selectedSkills.String)
	}

	handlers.SendJSON(w, http.StatusOK, s)
}

// HandleDeleteSession deletes a specific session.
func (r *Router) HandleDeleteSession(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	// Delete messages first (foreign key)
	_, _ = r.db.Exec("DELETE FROM messages WHERE session_id = ?", id)

	result, err := r.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to delete session")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "Session deleted"})
}

// HandleGetMessages returns messages for a session.
func (r *Router) HandleGetMessages(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["id"]

	rows, err := r.db.Query(`
		SELECT id, role, content, tool_calls, tool_call_id, created_at
		FROM messages 
		WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to query messages")
		return
	}
	defer rows.Close()

	// First pass: collect all messages and build tool result map
	type rawMessage struct {
		ID            string
		Role          string
		Content       string
		ToolCallsJSON *string
		ToolCallID    *string
		CreatedAt     time.Time
	}

	var rawMessages []rawMessage
	toolResultMap := make(map[string]string) // tool_call_id -> output

	for rows.Next() {
		var rm rawMessage
		if err := rows.Scan(&rm.ID, &rm.Role, &rm.Content, &rm.ToolCallsJSON, &rm.ToolCallID, &rm.CreatedAt); err != nil {
			continue
		}
		rawMessages = append(rawMessages, rm)

		// If this is a tool role message, map its result
		if rm.Role == "tool" && rm.ToolCallID != nil && *rm.ToolCallID != "" {
			toolResultMap[*rm.ToolCallID] = rm.Content
		}
	}

	// Second pass: build API messages, skip tool messages, and populate tool results
	var messages []Message
	for _, rm := range rawMessages {
		// Skip tool role messages - they're embedded in assistant tool_calls
		if rm.Role == "tool" {
			continue
		}

		m := Message{
			ID:        rm.ID,
			Role:      rm.Role,
			Content:   rm.Content,
			CreatedAt: rm.CreatedAt,
		}

		// Parse tool_calls JSON if present
		if rm.ToolCallsJSON != nil && *rm.ToolCallsJSON != "" {
			var toolCalls []storage.ToolCall
			if err := json.Unmarshal([]byte(*rm.ToolCallsJSON), &toolCalls); err == nil {
				// Convert storage.ToolCall to ToolCallResult and populate results
				for _, tc := range toolCalls {
					tcr := ToolCallResult{
						Name:      tc.GetName(),
						Arguments: tc.GetArguments(),
					}
					// Look up the result from the tool result map
					if result, ok := toolResultMap[tc.ID]; ok {
						tcr.Result = result
					}
					m.ToolCalls = append(m.ToolCalls, tcr)
				}
			}
		}

		messages = append(messages, m)
	}

	if messages == nil {
		messages = []Message{}
	}

	handlers.SendJSON(w, http.StatusOK, MessagesListResponse{Messages: messages})
}

// HandleUpdateSessionModel updates the model for a specific session.
// Deprecated: Use HandleReconfigureSession for active sessions.
// This handler only updates the DB without cleaning up runtime resources (caches, ACP sessions).
// Kept for backward compatibility with initial session setup (NewChatPage).
func (r *Router) HandleUpdateSessionModel(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	var body UpdateSessionModelRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if body.Model == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Model is required")
		return
	}

	// Validate model exists - support both Copilot and Ollama models
	modelValid := false
	// Check Copilot models
	for _, m := range copilot.ListModels() {
		if m == body.Model {
			modelValid = true
			break
		}
	}
	// Check Ollama models (with "ollama:" prefix)
	if !modelValid && strings.HasPrefix(body.Model, "ollama:") {
		modelValid = true
	}
	// Check via multiPool if available
	if !modelValid && r.multiPool != nil {
		_, _, err := r.multiPool.GetProvider(body.Model)
		modelValid = err == nil
	}
	if !modelValid {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid model name")
		return
	}

	// Update session model
	result, err := r.db.Exec("UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?", body.Model, time.Now(), id)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to update session model")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, UpdateSessionModelResponse{ID: id, Model: body.Model})
}

// HandleUpdateSessionSkills updates the selected skills for a specific session.
// Deprecated: Use HandleReconfigureSession for active sessions.
// This handler only updates the DB without cleaning up runtime resources (caches, ACP sessions).
// Kept for backward compatibility with initial session setup (NewChatPage).
func (r *Router) HandleUpdateSessionSkills(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	var body UpdateSessionSkillsRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Serialize skills to JSON for storage
	var skillsStr string
	if len(body.SelectedSkills) > 0 {
		data, err := json.Marshal(body.SelectedSkills)
		if err != nil {
			handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to serialize skills")
			return
		}
		skillsStr = string(data)
	}

	result, err := r.db.Exec("UPDATE sessions SET selected_skills = ?, updated_at = ? WHERE id = ?", skillsStr, time.Now(), id)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to update session skills")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, UpdateSessionSkillsResponse{ID: id, SelectedSkills: body.SelectedSkills})
}

// parseSkillsJSON parses a JSON array string into a string slice.
// Returns nil for empty string (meaning "all skills").
func parseSkillsJSON(s string) []string {
	if s == "" {
		return nil
	}
	var skills []string
	if err := json.Unmarshal([]byte(s), &skills); err != nil {
		return nil
	}
	return skills
}

// HandleReconfigureSession atomically reconfigures a session's model, workspace,
// and/or skills. This is a major operation that cleans up all runtime resources.
//
// Flow:
//  1. Validate all parameters upfront
//  2. Update DB fields (model, skills, workspace)
//  3. Clean up ALL runtime resources via Runner.ResetSession
//  4. Return the new session configuration
//
// This replaces the old separate model/skills update endpoints for cases where
// resource cleanup is needed (i.e., during active chat sessions).
func (r *Router) HandleReconfigureSession(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["id"]

	var body ReconfigureSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// At least one field must be specified
	if body.Model == nil && body.WorkspacePath == nil && body.SelectedSkills == nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest,
			"At least one of model, workspace_path, or selected_skills must be specified")
		return
	}

	log.Info().
		Str("sessionID", sessionID).
		Interface("model", body.Model).
		Interface("workspace_path", body.WorkspacePath).
		Interface("selected_skills", body.SelectedSkills).
		Msg("Session reconfigure requested")

	// --- Phase 1: Validate model if specified ---
	if body.Model != nil && *body.Model != "" {
		modelValid := false
		for _, m := range copilot.ListModels() {
			if m == *body.Model {
				modelValid = true
				break
			}
		}
		if !modelValid && strings.HasPrefix(*body.Model, "ollama:") {
			modelValid = true
		}
		if !modelValid && r.multiPool != nil {
			_, _, err := r.multiPool.GetProvider(*body.Model)
			modelValid = err == nil
		}
		if !modelValid {
			handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid model name: "+*body.Model)
			return
		}
	}

	// --- Phase 2: Verify session exists ---
	var currentModel, currentSkills sql.NullString
	err := r.db.QueryRow("SELECT COALESCE(model,''), COALESCE(selected_skills,'') FROM sessions WHERE id = ?", sessionID).
		Scan(&currentModel, &currentSkills)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	// --- Phase 3: Update DB fields ---
	now := time.Now()

	if body.Model != nil {
		if _, err := r.db.Exec("UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?", *body.Model, now, sessionID); err != nil {
			handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to update model")
			return
		}
		log.Info().Str("sessionID", sessionID).Str("model", *body.Model).Msg("Session model updated in DB")
	}

	if body.SelectedSkills != nil {
		var skillsStr string
		if len(*body.SelectedSkills) > 0 {
			data, _ := json.Marshal(*body.SelectedSkills)
			skillsStr = string(data)
		}
		if _, err := r.db.Exec("UPDATE sessions SET selected_skills = ?, updated_at = ? WHERE id = ?", skillsStr, now, sessionID); err != nil {
			handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to update skills")
			return
		}
		log.Info().Str("sessionID", sessionID).Strs("skills", *body.SelectedSkills).Msg("Session skills updated in DB")
	}

	// --- Phase 3b: Handle workspace binding ---
	var workspacePath string
	if body.WorkspacePath != nil {
		if *body.WorkspacePath == "" {
			// Unbind workspace
			if r.workspaceManager != nil {
				_ = r.workspaceManager.Unbind(sessionID)
				log.Info().Str("sessionID", sessionID).Msg("Workspace unbound")
			}
		} else {
			// Bind workspace
			if r.workspaceManager != nil {
				alias := ""
				if body.WorkspaceAlias != nil {
					alias = *body.WorkspaceAlias
				}
				if alias != "" {
					err = r.workspaceManager.BindWithAlias(sessionID, *body.WorkspacePath, alias, false)
				} else {
					err = r.workspaceManager.Bind(sessionID, *body.WorkspacePath, false)
				}
				if err != nil {
					handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Failed to bind workspace: "+err.Error())
					return
				}
				workspacePath = *body.WorkspacePath
				log.Info().Str("sessionID", sessionID).Str("path", workspacePath).Msg("Workspace bound")
			}
		}
	}

	// --- Phase 4: Clean up ALL runtime resources ---
	if r.runner != nil {
		r.runner.ResetSession(sessionID)
		log.Info().Str("sessionID", sessionID).Msg("Runtime resources cleaned up via Runner.ResetSession")
	}

	// --- Phase 5: Read back final state and return ---
	var finalModel, finalSkills sql.NullString
	_ = r.db.QueryRow("SELECT COALESCE(model,''), COALESCE(selected_skills,'') FROM sessions WHERE id = ?", sessionID).
		Scan(&finalModel, &finalSkills)

	// Get workspace path
	if workspacePath == "" && r.workspaceManager != nil {
		if binding, ok := r.workspaceManager.Get(sessionID); ok {
			workspacePath = binding.Path
		}
	}

	resp := ReconfigureSessionResponse{
		ID:             sessionID,
		Model:          finalModel.String,
		WorkspacePath:  workspacePath,
		SelectedSkills: parseSkillsJSON(finalSkills.String),
		Message:        "Session reconfigured successfully. All runtime resources have been reset.",
	}

	log.Info().
		Str("sessionID", sessionID).
		Str("model", resp.Model).
		Str("workspace", resp.WorkspacePath).
		Strs("skills", resp.SelectedSkills).
		Msg("Session reconfigure completed successfully")

	handlers.SendJSON(w, http.StatusOK, resp)
}

// HandleUpdateSession updates the properties of a specific session.
func (r *Router) HandleUpdateSession(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	var body UpdateSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if body.Title == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Title is required")
		return
	}

	// Update session title (without updating updated_at - only messages should update it)
	result, err := r.db.Exec("UPDATE sessions SET title = ? WHERE id = ?", body.Title, id)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to update session")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	// Query and return full session info
	var session SessionSummary
	var title sql.NullString
	err = r.db.QueryRow(
		`SELECT id, created_at, updated_at, title, model, scenario FROM sessions WHERE id = ?`,
		id,
	).Scan(&session.ID, &session.CreatedAt, &session.UpdatedAt, &title, &session.Model, &session.Scenario)

	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to fetch updated session")
		return
	}

	if title.Valid {
		session.Title = title.String
	}

	handlers.SendJSON(w, http.StatusOK, session)
}

// HandleGetScenarioModels returns the default models for each scenario.
func (r *Router) HandleGetScenarioModels(w http.ResponseWriter, req *http.Request) {
	response := ScenarioModelsResponse{
		Chat:    viper.GetString("copilot.chat_model"),
		Cron:    viper.GetString("cron.model"),
		Channel: viper.GetString("channels.model"),
	}

	// Apply defaults if empty
	if response.Chat == "" {
		response.Chat = viper.GetString("copilot.model")
	}
	if response.Chat == "" {
		response.Chat = copilot.DefaultModel
	}
	if response.Cron == "" {
		response.Cron = "gpt-4o-mini"
	}
	if response.Channel == "" {
		response.Channel = "gpt-4o-mini"
	}

	handlers.SendJSON(w, http.StatusOK, response)
}

// HandleUpdateScenarioModels updates the default models for scenarios.
func (r *Router) HandleUpdateScenarioModels(w http.ResponseWriter, req *http.Request) {
	var body UpdateScenarioModelsRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Validate models
	validModels := copilot.ListModels()
	validateModel := func(model string) bool {
		if model == "" {
			return true // Empty means no change
		}
		for _, m := range validModels {
			if m == model {
				return true
			}
		}
		return false
	}

	if !validateModel(body.Chat) || !validateModel(body.Cron) || !validateModel(body.Channel) {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid model name")
		return
	}

	// Update viper config and persist to file
	if body.Chat != "" {
		viper.Set("copilot.chat_model", body.Chat)
	}
	if body.Cron != "" {
		viper.Set("cron.model", body.Cron)
	}
	if body.Channel != "" {
		viper.Set("channels.model", body.Channel)
	}

	// Persist config changes to file
	if err := viper.WriteConfig(); err != nil {
		log.Warn().Err(err).Msg("Failed to persist scenario models config")
		// Continue anyway - runtime config is updated
	}

	// Return updated values
	response := ScenarioModelsResponse{
		Chat:    viper.GetString("copilot.chat_model"),
		Cron:    viper.GetString("cron.model"),
		Channel: viper.GetString("channels.model"),
	}

	handlers.SendJSON(w, http.StatusOK, response)
}

// HandleGetConfig returns the current configuration.
func (r *Router) HandleGetConfig(w http.ResponseWriter, req *http.Request) {
	// Get values with defaults
	gatewayHost := viper.GetString("gateway.host")
	if gatewayHost == "" {
		gatewayHost = "localhost"
	}
	gatewayPort := viper.GetInt("gateway.port")
	if gatewayPort == 0 {
		gatewayPort = 18788
	}
	providerDefault := viper.GetString("provider.default")
	if providerDefault == "" {
		providerDefault = "copilot"
	}

	config := ConfigResponse{
		Gateway: GatewayConfigView{
			Host: gatewayHost,
			Port: gatewayPort,
		},
		Provider: ProviderConfigView{
			Default: providerDefault,
			Enabled: viper.GetStringSlice("provider.enabled"),
		},
		Ollama: OllamaConfigView{
			Endpoint: viper.GetString("ollama.endpoint"),
			Model:    viper.GetString("ollama.model"),
		},
		Memory: MemoryConfigView{
			Enabled: r.memory != nil,
		},
		Cron: CronConfigView{
			Enabled: r.cronScheduler != nil,
		},
		MCP: MCPConfigView{
			ServerEnabled: r.mcpServer != nil,
			ClientEnabled: r.mcpClient != nil,
		},
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

// HandleUpdateConfig updates configuration settings.
func (r *Router) HandleUpdateConfig(w http.ResponseWriter, req *http.Request) {
	var body UpdateConfigRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	// Update provider configuration
	if body.Provider != nil {
		validProviders := []string{"copilot", "ollama"}

		// Validate and update default provider
		if body.Provider.Default != "" {
			valid := false
			for _, p := range validProviders {
				if body.Provider.Default == p {
					valid = true
					break
				}
			}
			if !valid {
				handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid provider type. Supported: copilot, ollama")
				return
			}
			viper.Set("provider.default", body.Provider.Default)
		}

		// Validate and update enabled providers
		if len(body.Provider.Enabled) > 0 {
			for _, p := range body.Provider.Enabled {
				valid := false
				for _, vp := range validProviders {
					if p == vp {
						valid = true
						break
					}
				}
				if !valid {
					handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid provider in enabled list: "+p)
					return
				}
			}
			viper.Set("provider.enabled", body.Provider.Enabled)
		}
	}

	// Update Ollama configuration
	if body.Ollama != nil {
		if body.Ollama.Endpoint != "" {
			viper.Set("ollama.endpoint", body.Ollama.Endpoint)
		}
		if body.Ollama.Model != "" {
			viper.Set("ollama.model", body.Ollama.Model)
		}
	}

	// Persist config changes to file
	if err := viper.WriteConfig(); err != nil {
		log.Warn().Err(err).Msg("Failed to persist config")
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to save configuration")
		return
	}

	// Hot reload providers if embedded server is available
	if r.embeddedServer != nil {
		if err := r.embeddedServer.ReloadProviders(); err != nil {
			log.Warn().Err(err).Msg("Failed to reload providers")
			// Don't fail the request, just log the warning
		} else {
			log.Info().Msg("Providers reloaded successfully")
		}
	}

	// Return success with updated config
	config := ConfigResponse{
		Gateway: GatewayConfigView{
			Host: viper.GetString("gateway.host"),
			Port: viper.GetInt("gateway.port"),
		},
		Provider: ProviderConfigView{
			Default: viper.GetString("provider.default"),
			Enabled: viper.GetStringSlice("provider.enabled"),
		},
		Ollama: OllamaConfigView{
			Endpoint: viper.GetString("ollama.endpoint"),
			Model:    viper.GetString("ollama.model"),
		},
		Memory: MemoryConfigView{
			Enabled: r.memory != nil,
		},
		Cron: CronConfigView{
			Enabled: r.cronScheduler != nil,
		},
		MCP: MCPConfigView{
			ServerEnabled: r.mcpServer != nil,
			ClientEnabled: r.mcpClient != nil,
		},
	}

	handlers.SendJSON(w, http.StatusOK, config)
}

// HandleReloadConfig reloads configuration from disk.
func (r *Router) HandleReloadConfig(w http.ResponseWriter, req *http.Request) {
	handlers.SendError(w, http.StatusMethodNotAllowed, "CONFIG_RELOAD_DISABLED", "Configuration reload is not enabled")
}

// ModelsResponse represents the response for GET /api/v1/models.
type ModelsResponse struct {
	Models    []ModelView      `json:"models"`
	Current   string           `json:"current"`
	Default   string           `json:"default"`
	Providers []ProviderStatus `json:"providers,omitempty"` // Provider status list
}

// ModelView represents a single model's information.
type ModelView struct {
	ID             string  `json:"id"`
	Provider       string  `json:"provider"` // Provider name: copilot, copilot-acp, ollama
	DisplayName    string  `json:"display_name"`
	Family         string  `json:"family"`
	IsFree         bool    `json:"is_free"`
	Multiplier     float64 `json:"multiplier"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	SupportsVision bool    `json:"supports_vision"`
	SupportsTools  bool    `json:"supports_tools"`
	Description    string  `json:"description"`
	Available      bool    `json:"available"` // Whether the model is currently available
}

// HandleListModels returns the list of supported models from all enabled providers.
func (r *Router) HandleListModels(w http.ResponseWriter, req *http.Request) {
	// Parse query parameters
	freeOnly := req.URL.Query().Get("free") == "true"
	family := req.URL.Query().Get("family")
	providerFilter := req.URL.Query().Get("provider")

	var models []ModelView
	var providerStatuses []ProviderStatus

	// If MultiProviderPool is available, use it; otherwise fallback to copilot only
	if r.multiPool != nil && r.multiPool.Count() > 0 {
		// Get all providers with health check
		for _, providerName := range r.multiPool.ListProviders() {
			modelCount := r.multiPool.ModelCountByProvider(providerName)

			// Perform health check to determine availability
			available := false
			var lastError string
			if prov := r.multiPool.GetAnyProvider(providerName); prov != nil {
				if hc, ok := prov.(provider.HealthCheckable); ok {
					ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
					if err := hc.Ping(ctx); err == nil {
						available = true
					} else {
						if pe, ok := err.(*provider.ProviderError); ok {
							lastError = pe.Message
						} else {
							lastError = err.Error()
						}
					}
					cancel()
				} else {
					// Provider doesn't support health check, assume available
					available = true
				}
			}

			providerStatuses = append(providerStatuses, ProviderStatus{
				Name:       providerName,
				Enabled:    true,
				Available:  available,
				ModelCount: modelCount,
				Error:      lastError,
			})
		}

		// Build model list from all providers
		if providerFilter == "" || providerFilter == "copilot" {
			// Add Copilot models
			for id, info := range copilot.SupportedModels {
				if freeOnly && !info.IsFree {
					continue
				}
				if family != "" && string(info.Family) != family {
					continue
				}
				models = append(models, ModelView{
					ID:             id,
					Provider:       "copilot",
					DisplayName:    info.DisplayName,
					Family:         string(info.Family),
					IsFree:         info.IsFree,
					Multiplier:     float64(info.Multiplier),
					ContextWindow:  info.ContextWindow,
					MaxOutput:      info.MaxOutput,
					SupportsVision: info.SupportsVision,
					SupportsTools:  info.SupportsTools,
					Description:    info.Description,
					Available:      r.multiPool.HasProvider("copilot"),
				})
			}
		}

		// Add Copilot ACP models (premium models via Copilot CLI)
		if providerFilter == "" || providerFilter == "copilot-acp" || providerFilter == "copilot" {
			for id, info := range copilot.ACPSupportedModels {
				if freeOnly {
					continue // All ACP models are premium
				}
				if family != "" && string(info.Family) != family {
					continue
				}
				models = append(models, ModelView{
					ID:             id,
					Provider:       "copilot-acp",
					DisplayName:    info.DisplayName,
					Family:         string(info.Family),
					IsFree:         false,
					Multiplier:     info.Multiplier,
					ContextWindow:  info.ContextWindow,
					MaxOutput:      info.MaxOutput,
					SupportsVision: info.SupportsVision,
					SupportsTools:  info.SupportsTools,
					Description:    info.Description,
					Available:      r.multiPool.HasProvider("copilot-acp"),
				})
			}
		}

		// Add Ollama models if provider is available and not filtered out
		if (providerFilter == "" || providerFilter == "ollama") && r.multiPool.HasProvider("ollama") {
			for _, modelInfo := range r.multiPool.ListAllModels() {
				if modelInfo.Provider != "ollama" {
					continue
				}
				// Ollama models don't have the same metadata as Copilot
				models = append(models, ModelView{
					ID:             modelInfo.ID,
					Provider:       "ollama",
					DisplayName:    modelInfo.OriginalID,
					Family:         "ollama",
					IsFree:         true, // Ollama models are free (local)
					Multiplier:     0,
					ContextWindow:  4096, // Default context window
					MaxOutput:      4096,
					SupportsVision: false, // TODO: Detect from Ollama model metadata
					SupportsTools:  false, // TODO: Detect from Ollama model metadata
					Description:    "Local Ollama model",
					Available:      modelInfo.Available,
				})
			}
		}
	} else {
		// Fallback: Only Copilot models (legacy behavior)
		for id, info := range copilot.SupportedModels {
			if freeOnly && !info.IsFree {
				continue
			}
			if family != "" && string(info.Family) != family {
				continue
			}
			models = append(models, ModelView{
				ID:             id,
				Provider:       "copilot",
				DisplayName:    info.DisplayName,
				Family:         string(info.Family),
				IsFree:         info.IsFree,
				Multiplier:     float64(info.Multiplier),
				ContextWindow:  info.ContextWindow,
				MaxOutput:      info.MaxOutput,
				SupportsVision: info.SupportsVision,
				SupportsTools:  info.SupportsTools,
				Description:    info.Description,
				Available:      true,
			})
		}
		providerStatuses = append(providerStatuses, ProviderStatus{
			Name:       "copilot",
			Enabled:    true,
			Available:  true,
			ModelCount: len(copilot.SupportedModels),
		})
	}

	// Sort by provider, then free status, then ID
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		if models[i].IsFree != models[j].IsFree {
			return models[i].IsFree
		}
		return models[i].ID < models[j].ID
	})

	// Get current model
	current := viper.GetString("copilot.model")
	if current == "" {
		current = copilot.DefaultModel
	}

	handlers.SendJSON(w, http.StatusOK, ModelsResponse{
		Models:    models,
		Current:   current,
		Default:   copilot.DefaultModel,
		Providers: providerStatuses,
	})
}

// SetCurrentModelRequest represents the request body for setting current model.
type SetCurrentModelRequest struct {
	Model string `json:"model"`
}

// HandleSetCurrentModel sets the current model.
func (r *Router) HandleSetCurrentModel(w http.ResponseWriter, req *http.Request) {
	var request SetCurrentModelRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if request.Model == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Model ID is required")
		return
	}

	// Validate model ID - support both Copilot and Ollama models
	isValidModel := false
	if _, ok := copilot.SupportedModels[request.Model]; ok {
		isValidModel = true
	} else if strings.HasPrefix(request.Model, "ollama:") {
		// Ollama models have "ollama:" prefix
		isValidModel = true
	} else if r.multiPool != nil {
		// Check if model exists in multi-provider pool
		_, _, err := r.multiPool.GetProvider(request.Model)
		isValidModel = err == nil
	}

	if !isValidModel {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid model ID")
		return
	}

	// Update viper config
	viper.Set("copilot.model", request.Model)

	// Persist to config file
	if err := viper.WriteConfig(); err != nil {
		// Log warning but don't fail - config is still updated in memory
		log.Warn().Err(err).Msg("Failed to write config file")
	}

	handlers.SendJSON(w, http.StatusOK, map[string]string{
		"model":   request.Model,
		"message": "Model updated successfully",
	})
}

// HandleUIComponents returns registered UI components.
func (r *Router) HandleUIComponents(w http.ResponseWriter, req *http.Request) {
	// Delegate to UI handler if available
	if r.uiHandler != nil {
		r.uiHandler.HandleComponents(w, req)
		return
	}

	handlers.SendJSON(w, http.StatusOK, UIComponentsResponse{Components: []UIComponent{}})
}

// HandleGetUIState returns the current UI state.
func (r *Router) HandleGetUIState(w http.ResponseWriter, req *http.Request) {
	if r.uiHandler != nil {
		r.uiHandler.HandleGetState(w, req)
		return
	}

	handlers.SendJSON(w, http.StatusOK, UIState{})
}

// HandleUpdateUIState updates the UI state.
func (r *Router) HandleUpdateUIState(w http.ResponseWriter, req *http.Request) {
	if r.uiHandler != nil {
		r.uiHandler.HandleUpdateState(w, req)
		return
	}

	handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "UI handler not available")
}

// generateSessionID generates a new unique session ID.
func generateSessionID() string {
	return "sess_" + randomString(16)
}

// randomString generates a random alphanumeric string of the given length.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	return string(b)
}

// HandleProviderStatus returns the status of all providers.
// GET /api/v1/providers/status
func (r *Router) HandleProviderStatus(w http.ResponseWriter, req *http.Request) {
	var states []provider.ProviderState

	if r.multiPool != nil {
		for _, providerName := range r.multiPool.ListProviders() {
			prov := r.multiPool.GetAnyProvider(providerName)
			if prov == nil {
				states = append(states, provider.ProviderState{
					Name:      providerName,
					Status:    provider.StatusDisconnected,
					LastError: "Provider not initialized",
				})
				continue
			}

			// Check if provider supports health check
			if hc, ok := prov.(provider.HealthCheckable); ok {
				state := hc.GetState()
				states = append(states, state)
			} else {
				// Provider doesn't support health check, assume connected
				states = append(states, provider.ProviderState{
					Name:   providerName,
					Status: provider.StatusConnected,
					Models: prov.Models(),
				})
			}
		}
	}

	handlers.SendJSON(w, http.StatusOK, provider.ProviderStatusResponse{
		Providers: states,
	})
}

// ProviderRecoverRequest is the request body for provider recovery.
type ProviderRecoverRequest struct {
	Action string `json:"action"` // reconnect, reauth
}

// ProviderRecoverResponse is the response for provider recovery.
type ProviderRecoverResponse struct {
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
	State   *provider.ProviderState `json:"state,omitempty"`
}

// HandleProviderRecover handles recovery actions for a specific provider.
// POST /api/v1/providers/{name}/recover
func (r *Router) HandleProviderRecover(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	providerName := vars["name"]

	if providerName == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Provider name is required")
		return
	}

	var request ProviderRecoverRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if request.Action == "" {
		request.Action = "reconnect"
	}

	if r.multiPool == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Provider pool not available")
		return
	}

	if !r.multiPool.HasProvider(providerName) {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Provider not found")
		return
	}

	prov := r.multiPool.GetAnyProvider(providerName)
	if prov == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Provider not initialized")
		return
	}

	// Perform recovery action
	switch request.Action {
	case "reconnect":
		// For now, just trigger a health check which may refresh connections
		if hc, ok := prov.(provider.HealthCheckable); ok {
			ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
			defer cancel()

			err := hc.Ping(ctx)
			state := hc.GetState()

			if err != nil {
				handlers.SendJSON(w, http.StatusOK, ProviderRecoverResponse{
					Success: false,
					Message: "重连失败: " + err.Error(),
					State:   &state,
				})
				return
			}

			handlers.SendJSON(w, http.StatusOK, ProviderRecoverResponse{
				Success: true,
				Message: "重连成功",
				State:   &state,
			})
			return
		}

		handlers.SendJSON(w, http.StatusOK, ProviderRecoverResponse{
			Success: true,
			Message: "Provider 不支持健康检查，假设已连接",
		})

	case "reauth":
		// For Copilot, this would involve re-authentication
		// For now, this is a placeholder - actual implementation depends on provider
		handlers.SendError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Reauth not yet implemented for this provider")

	default:
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Unknown action: "+request.Action)
	}
}
