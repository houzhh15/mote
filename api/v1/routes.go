package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	internalChannel "mote/internal/channel"
	"mote/internal/compaction"
	"mote/internal/config"
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
	"mote/internal/provider/glm"
	"mote/internal/provider/minimax"
	"mote/internal/runner"
	"mote/internal/runner/delegate"
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
	MemoryManager    *memory.MemoryManager // New: MemoryManager replaces MemoryIndex
	RecallEngine     *memory.RecallEngine  // For recall stats
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
	memoryManager    *memory.MemoryManager // New: MemoryManager replaces MemoryIndex
	recallEngine     *memory.RecallEngine  // For recall stats
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
	delegateTracker  *delegate.DelegationTracker
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
		memoryManager:    deps.MemoryManager,
		recallEngine:     deps.RecallEngine,
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

// SetMemoryManager updates the memory manager dependency.
// When set, the new MemoryManager is used for search/add operations.
func (r *Router) SetMemoryManager(m *memory.MemoryManager) {
	r.memoryManager = m
}

// SetRecallEngine updates the recall engine dependency.
func (r *Router) SetRecallEngine(re *memory.RecallEngine) {
	r.recallEngine = re
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

// SetDelegateTracker sets the delegation tracker for audit queries.
func (r *Router) SetDelegateTracker(t *delegate.DelegationTracker) {
	r.delegateTracker = t
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

	// Session cancel (must be before /sessions/{id} routes)
	v1.HandleFunc("/sessions/{id}/cancel", r.HandleCancelSession).Methods(http.MethodPost)

	// Sessions
	v1.HandleFunc("/sessions", r.HandleListSessions).Methods(http.MethodGet)
	v1.HandleFunc("/sessions", r.HandleCreateSession).Methods(http.MethodPost)
	v1.HandleFunc("/sessions/batch-delete", r.HandleBatchDeleteSessions).Methods(http.MethodPost)
	v1.HandleFunc("/sessions/{id}", r.HandleGetSession).Methods(http.MethodGet)
	v1.HandleFunc("/sessions/{id}", r.HandleUpdateSession).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}", r.HandleDeleteSession).Methods(http.MethodDelete)
	v1.HandleFunc("/sessions/{id}/messages", r.HandleGetMessages).Methods(http.MethodGet)
	v1.HandleFunc("/sessions/{id}/context", r.HandleGetSessionContext).Methods(http.MethodGet)
	v1.HandleFunc("/sessions/{id}/model", r.HandleUpdateSessionModel).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}/skills", r.HandleUpdateSessionSkills).Methods(http.MethodPut)
	v1.HandleFunc("/sessions/{id}/reconfigure", r.HandleReconfigureSession).Methods(http.MethodPost)
	v1.HandleFunc("/sessions/{id}/pda", r.HandlePDAControl).Methods(http.MethodPost)

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
	v1.HandleFunc("/cron/executing", r.HandleGetExecutingJobs).Methods(http.MethodGet)
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
	v1.HandleFunc("/policy/config", r.HandleGetPolicyConfig).Methods(http.MethodGet)
	v1.HandleFunc("/policy/config", r.HandleUpdatePolicyConfig).Methods(http.MethodPut)

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
	v1.HandleFunc("/workspaces/open", r.HandleOpenWorkspaceDir).Methods(http.MethodPost)
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
	v1.HandleFunc("/skills/{id}", r.HandleDeleteSkill).Methods(http.MethodDelete)
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

	// Delegations (multi-agent)
	v1.HandleFunc("/delegations", r.HandleListRecentDelegations).Methods(http.MethodGet)
	v1.HandleFunc("/delegations/batch-delete", r.HandleBatchDeleteDelegations).Methods(http.MethodPost)
	v1.HandleFunc("/sessions/{id}/delegations", r.HandleListDelegations).Methods(http.MethodGet)
	v1.HandleFunc("/delegations/{id}", r.HandleGetDelegation).Methods(http.MethodGet)

	// Agents (multi-agent CRUD)
	v1.HandleFunc("/agents", r.HandleListAgents).Methods(http.MethodGet)
	v1.HandleFunc("/agents", r.HandleAddAgent).Methods(http.MethodPost)
	v1.HandleFunc("/agents/reload", r.HandleReloadAgents).Methods(http.MethodPost)
	v1.HandleFunc("/agents/validate-dir", r.HandleValidateAgentsDir).Methods(http.MethodGet)
	v1.HandleFunc("/agents/{name}/validate-cfg", r.HandleValidateAgentCFG).Methods(http.MethodPost)
	v1.HandleFunc("/agents/{name}/draft", r.HandleSaveAgentDraft).Methods(http.MethodPost)
	v1.HandleFunc("/agents/{name}/draft", r.HandleDiscardAgentDraft).Methods(http.MethodDelete)
	v1.HandleFunc("/agents/{name}", r.HandleGetAgent).Methods(http.MethodGet)
	v1.HandleFunc("/agents/{name}", r.HandleUpdateAgent).Methods(http.MethodPut)
	v1.HandleFunc("/agents/{name}", r.HandleDeleteAgent).Methods(http.MethodDelete)
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

	// Determine the default provider
	defaultProvider := viper.GetString("provider.default")
	if defaultProvider == "" {
		defaultProvider = "copilot-acp"
	}

	response := AuthStatusResponse{
		Provider: defaultProvider,
		Model:    viper.GetString("provider.model"),
	}

	// For ACP mode, authentication is managed by the Copilot CLI itself
	// (via `copilot login`), not by the mote config token.
	if defaultProvider == "copilot-acp" {
		if token != "" {
			// Has legacy API token — show as authenticated
			response.Authenticated = true
			if len(token) > 8 {
				response.TokenMasked = token[:4] + "..." + token[len(token)-4:]
			} else {
				response.TokenMasked = "***"
			}
		} else {
			// No legacy token, but ACP uses CLI auth — check if copilot CLI is available
			response.Authenticated = true
			response.Message = "使用 Copilot CLI 认证 (copilot login)"
			response.TokenMasked = "cli-auth"
		}
	} else if token == "" {
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
		       ) as preview,
		       CASE WHEN s.metadata LIKE '%"pda_checkpoint"%' THEN 1 ELSE 0 END as has_pda,
		       CASE WHEN s.metadata LIKE '%"pda_session"%' OR s.metadata LIKE '%"pda_checkpoint"%' THEN 1 ELSE 0 END as is_pda
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
		var hasPDA, isPDA int
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.Title, &s.Model, &s.Scenario, &selectedSkillsStr, &s.MessageCount, &preview, &hasPDA, &isPDA); err != nil {
			continue
		}
		if preview != "" {
			s.Preview = preview
		}
		s.HasPDACheckpoint = hasPDA == 1
		s.IsPDA = isPDA == 1
		s.SelectedSkills = parseSkillsJSON(selectedSkillsStr)
		s.Source = deriveSessionSource(s.ID)
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

	// Determine scenario
	scenario := body.Scenario
	if scenario == "" {
		scenario = "chat"
	}

	// Use explicitly provided model (empty means session has no default model yet)
	model := body.Model

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
	var hasPDA, isPDA int
	err := r.db.QueryRow(`
		SELECT id, created_at, updated_at, title, model, scenario, selected_skills,
		       (SELECT COUNT(*) FROM messages WHERE session_id = sessions.id) as message_count,
		       CASE WHEN metadata LIKE '%"pda_checkpoint"%' THEN 1 ELSE 0 END as has_pda,
		       CASE WHEN metadata LIKE '%"pda_session"%' OR metadata LIKE '%"pda_checkpoint"%' THEN 1 ELSE 0 END as is_pda
		FROM sessions 
		WHERE id = ?
	`, id).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &title, &model, &scenario, &selectedSkills, &s.MessageCount, &hasPDA, &isPDA)

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
	s.HasPDACheckpoint = hasPDA == 1
	s.IsPDA = isPDA == 1

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

// HandleBatchDeleteSessions deletes multiple sessions at once.
func (r *Router) HandleBatchDeleteSessions(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	var body BatchDeleteSessionsRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if len(body.IDs) == 0 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "No session IDs provided")
		return
	}

	// Limit batch size to prevent abuse
	if len(body.IDs) > 200 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Too many session IDs (max 200)")
		return
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(body.IDs))
	args := make([]interface{}, len(body.IDs))
	for i, id := range body.IDs {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	// Delete messages first
	_, _ = r.db.Exec("DELETE FROM messages WHERE session_id IN ("+inClause+")", args...)

	// Delete sessions
	result, err := r.db.Exec("DELETE FROM sessions WHERE id IN ("+inClause+")", args...)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to delete sessions")
		return
	}

	deleted, _ := result.RowsAffected()
	handlers.SendJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": deleted,
		"total":   len(body.IDs),
	})
}

// deriveSessionSource determines the session source from its ID prefix.
// Returns "delegate" for sub-agent sessions, "cron" for cron sessions, "chat" for normal sessions.
func deriveSessionSource(sessionID string) string {
	if strings.HasPrefix(sessionID, "delegate:") {
		return "delegate"
	}
	if strings.HasPrefix(sessionID, "cron-") {
		return "cron"
	}
	return "chat"
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

	// --- Context-usage estimation: simulate what the model actually sees ---
	//
	// The runtime's BuildContext assembles:
	//   - When compressed context exists: summary + kept messages + new messages
	//   - Otherwise: all DB messages
	// Then BudgetMessages truncates/drops to fit the context window.
	// We replicate this flow to produce an accurate token estimate.

	// 1. Check for compressed context
	compressedCtx, _ := r.db.GetLatestContext(sessionID)

	// 2. Build effective message set (same logic as BuildContext)
	toProvMsg := func(rm rawMessage) provider.Message {
		pm := provider.Message{Role: rm.Role, Content: rm.Content}
		if rm.ToolCallID != nil && *rm.ToolCallID != "" {
			pm.ToolCallID = *rm.ToolCallID
		}
		if rm.ToolCallsJSON != nil && *rm.ToolCallsJSON != "" {
			var storageTCs []storage.ToolCall
			if err := json.Unmarshal([]byte(*rm.ToolCallsJSON), &storageTCs); err == nil {
				for _, stc := range storageTCs {
					pm.ToolCalls = append(pm.ToolCalls, provider.ToolCall{
						ID:        stc.ID,
						Type:      stc.Type,
						Arguments: stc.GetArguments(),
						Function: &struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      stc.GetName(),
							Arguments: stc.GetArguments(),
						},
					})
				}
			}
		}
		return pm
	}

	var effectivePMs []provider.Message
	if compressedCtx != nil {
		// Compressed path: summary + kept messages + new messages
		// (mirrors context.Manager.BuildContext)
		if compressedCtx.Summary != "" {
			effectivePMs = append(effectivePMs, provider.Message{
				Role:    "assistant",
				Content: "[Previous conversation summary]\n" + compressedCtx.Summary,
			})
		}
		// Kept messages by ID, skip leading orphan tool results
		msgByID := make(map[string]rawMessage)
		for _, rm := range rawMessages {
			msgByID[rm.ID] = rm
		}
		skippingLeadingTools := true
		for _, kid := range compressedCtx.KeptMessageIDs {
			if rm, ok := msgByID[kid]; ok {
				if skippingLeadingTools && rm.Role == string(provider.RoleTool) {
					continue
				}
				skippingLeadingTools = false
				effectivePMs = append(effectivePMs, toProvMsg(rm))
			}
		}
		// New messages after compression point
		skippingLeadingTools = true
		for _, rm := range rawMessages {
			if rm.CreatedAt.After(compressedCtx.CreatedAt) {
				if skippingLeadingTools && rm.Role == string(provider.RoleTool) {
					continue
				}
				skippingLeadingTools = false
				effectivePMs = append(effectivePMs, toProvMsg(rm))
			}
		}
	} else {
		// No compression: all messages
		for _, rm := range rawMessages {
			effectivePMs = append(effectivePMs, toProvMsg(rm))
		}
	}

	// 3. Get session model and context window
	contextWindow := 0
	if session, err := r.db.GetSession(sessionID); err == nil && session.Model != "" {
		if r.multiPool != nil {
			if prov, _, err := r.multiPool.GetProvider(session.Model); err == nil {
				if cwp, ok := prov.(provider.ContextWindowProvider); ok {
					contextWindow = cwp.ContextWindow(session.Model)
				}
			}
		}
	}

	// 4. Apply BudgetMessages on the effective set
	budgeted := effectivePMs
	if contextWindow > 0 {
		cfg := compaction.DefaultConfig()
		cfg.AdaptForModel(contextWindow)
		tmpCompactor := compaction.NewCompactor(cfg, nil)
		budgeted = tmpCompactor.BudgetMessages(effectivePMs, 0)
	}

	// 5. Count tokens only on the budgeted messages
	tokenCounter := compaction.NewTokenCounter()
	estimatedTokens := tokenCounter.EstimateMessages(budgeted)

	handlers.SendJSON(w, http.StatusOK, MessagesListResponse{
		Messages:        messages,
		EstimatedTokens: estimatedTokens,
	})
}

// HandleGetSessionContext returns a detailed context-window analysis for a session.
// It shows ALL DB messages as segments, each classified and marked with:
//   - type: compressed_summary, kept_message, history_message, compressed_history
//   - in_context: true if part of effective context (what BuildContext assembles)
//   - budgeted: true if surviving BudgetMessages (what LLM actually receives)
//
// Three tiers of statistics:
//
//	Total     = all DB messages (full history including compressed-away ones)
//	Effective = summary + kept + new-after-compression (BuildContext output)
//	Budgeted  = after BudgetMessages truncation
//
// GET /sessions/{id}/context
func (r *Router) HandleGetSessionContext(w http.ResponseWriter, req *http.Request) {
	if r.db == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "Database not available")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["id"]

	// 1. Load session metadata
	session, err := r.db.GetSession(sessionID)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, handlers.ErrCodeNotFound, "Session not found")
		return
	}

	// 2. Determine context window from model
	contextWindow := 0
	if session.Model != "" && r.multiPool != nil {
		if prov, _, err := r.multiPool.GetProvider(session.Model); err == nil {
			if cwp, ok := prov.(provider.ContextWindowProvider); ok {
				contextWindow = cwp.ContextWindow(session.Model)
			}
		}
	}

	// 3. Load compressed context info
	compInfo := CompressionInfo{}
	compressedCtx, _ := r.db.GetLatestContext(sessionID)
	if compressedCtx != nil {
		compInfo = CompressionInfo{
			HasCompression: true,
			Version:        compressedCtx.Version,
			Summary:        compressedCtx.Summary,
			KeptMessages:   len(compressedCtx.KeptMessageIDs),
			TotalTokens:    compressedCtx.TotalTokens,
			OriginalTokens: compressedCtx.OriginalTokens,
		}
	}

	// 4. Load all raw messages from DB
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

	type rawMsg struct {
		ID            string
		Role          string
		Content       string
		ToolCallsJSON *string
		ToolCallID    *string
		CreatedAt     time.Time
	}
	var allRawMessages []rawMsg
	for rows.Next() {
		var rm rawMsg
		if err := rows.Scan(&rm.ID, &rm.Role, &rm.Content, &rm.ToolCallsJSON, &rm.ToolCallID, &rm.CreatedAt); err != nil {
			continue
		}
		allRawMessages = append(allRawMessages, rm)
	}
	totalDBMessages := len(allRawMessages)

	// Helper: convert rawMsg → provider.Message
	toProviderMsg := func(rm rawMsg) provider.Message {
		pm := provider.Message{Role: rm.Role, Content: rm.Content}
		if rm.ToolCallID != nil && *rm.ToolCallID != "" {
			pm.ToolCallID = *rm.ToolCallID
		}
		if rm.ToolCallsJSON != nil && *rm.ToolCallsJSON != "" {
			var storageTCs []storage.ToolCall
			if err := json.Unmarshal([]byte(*rm.ToolCallsJSON), &storageTCs); err == nil {
				for _, stc := range storageTCs {
					pm.ToolCalls = append(pm.ToolCalls, provider.ToolCall{
						ID:        stc.ID,
						Type:      stc.Type,
						Arguments: stc.GetArguments(),
						Function: &struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      stc.GetName(),
							Arguments: stc.GetArguments(),
						},
					})
				}
			}
		}
		return pm
	}

	tc := compaction.NewTokenCounter()
	previewLen := 120
	truncate := func(s string, maxLen int) string {
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen] + "..."
	}

	// Helper: compute char count and token count for a rawMsg + provider.Message
	computeStats := func(rm rawMsg, pm provider.Message) (int, int) {
		charCount := len(rm.Content)
		tokens := tc.EstimateText(rm.Content) + 4 // +4 for role overhead
		for _, toolCall := range pm.ToolCalls {
			tokens += tc.EstimateText(toolCall.Arguments)
			if toolCall.Function != nil {
				tokens += tc.EstimateText(toolCall.Function.Name)
				tokens += tc.EstimateText(toolCall.Function.Arguments)
			}
			charCount += len(toolCall.Arguments)
			if toolCall.Function != nil {
				charCount += len(toolCall.Function.Name) + len(toolCall.Function.Arguments)
			}
		}
		return charCount, tokens
	}

	// 5. Classify every message and build effective set.
	//
	// Effective set mirrors context.Manager.BuildContext:
	//   - When compressed context exists: kept messages (skip leading orphan tools)
	//     + new messages after compression (skip leading orphan tools)
	//   - Otherwise: all messages

	keptSet := make(map[string]bool)
	if compressedCtx != nil {
		for _, kid := range compressedCtx.KeptMessageIDs {
			keptSet[kid] = true
		}
	}

	// Determine which allRawMessages indices are in the effective set.
	// This exactly mirrors BuildContext's assembly order.
	effectiveSet := make(map[int]bool)
	var effectivePMs []provider.Message
	var effectiveOrigIndices []int // map from effectivePMs index → allRawMessages index

	if compressedCtx != nil {
		// a) Kept messages in KeptMessageIDs order, skip leading orphan tool results
		msgIdxByID := make(map[string]int)
		for i, rm := range allRawMessages {
			msgIdxByID[rm.ID] = i
		}
		skippingLeadingTools := true
		for _, kid := range compressedCtx.KeptMessageIDs {
			if idx, ok := msgIdxByID[kid]; ok {
				rm := allRawMessages[idx]
				if skippingLeadingTools && rm.Role == string(provider.RoleTool) {
					continue
				}
				skippingLeadingTools = false
				pm := toProviderMsg(rm)
				effectiveSet[idx] = true
				effectivePMs = append(effectivePMs, pm)
				effectiveOrigIndices = append(effectiveOrigIndices, idx)
			}
		}

		// b) New messages after compression point, skip leading orphan tool results
		skippingLeadingTools = true
		for i, rm := range allRawMessages {
			if rm.CreatedAt.After(compressedCtx.CreatedAt) {
				if skippingLeadingTools && rm.Role == string(provider.RoleTool) {
					continue
				}
				skippingLeadingTools = false
				pm := toProviderMsg(rm)
				effectiveSet[i] = true
				effectivePMs = append(effectivePMs, pm)
				effectiveOrigIndices = append(effectiveOrigIndices, i)
			}
		}
	} else {
		// No compression — all messages are effective
		for i, rm := range allRawMessages {
			pm := toProviderMsg(rm)
			effectiveSet[i] = true
			effectivePMs = append(effectivePMs, pm)
			effectiveOrigIndices = append(effectiveOrigIndices, i)
		}
	}

	// 6. Apply BudgetMessages on the effective set only
	budgeted := effectivePMs
	if contextWindow > 0 {
		cfg := compaction.DefaultConfig()
		cfg.AdaptForModel(contextWindow)
		tmpCompactor := compaction.NewCompactor(cfg, nil)
		budgeted = tmpCompactor.BudgetMessages(effectivePMs, 0)
	}

	// Mark which original DB indices survived BudgetMessages.
	//
	// BudgetMessages may:
	//   - truncate Content (phases 1-2-4)
	//   - drop messages entirely (phase 3)
	//   - INSERT a synthetic notice message ("[Earlier context dropped...]")
	//   - reorder system messages to the front (dropOldestToBudget)
	//
	// Because of inserted notices & content truncation, simple two-pointer
	// matching on Content fails. Instead we use a fingerprint approach:
	// build fingerprints for all effective messages, then for each budgeted
	// message (that isn't a synthetic notice), find the best match.
	budgetedOrigSet := make(map[int]bool)
	{
		// Build fingerprint index: effectivePM index → (role, toolCallID, contentPrefix)
		type fp struct {
			role       string
			toolCallID string
			prefix     string
		}
		mkfp := func(pm provider.Message) fp {
			prefix := pm.Content
			if len(prefix) > 80 {
				prefix = prefix[:80]
			}
			return fp{role: pm.Role, toolCallID: pm.ToolCallID, prefix: prefix}
		}

		// Build reverse map: fingerprint → list of effective indices
		fpMap := make(map[fp][]int)
		for ei, pm := range effectivePMs {
			f := mkfp(pm)
			fpMap[f] = append(fpMap[f], ei)
		}
		fpUsed := make(map[fp]int) // tracks how many times each fp has been consumed

		for _, bpm := range budgeted {
			// Skip synthetic notice messages injected by BudgetMessages Phase 3
			if strings.Contains(bpm.Content, "[Earlier context dropped") {
				continue
			}

			f := mkfp(bpm)
			candidates := fpMap[f]
			usedCount := fpUsed[f]
			if usedCount < len(candidates) {
				ei := candidates[usedCount]
				budgetedOrigSet[effectiveOrigIndices[ei]] = true
				fpUsed[f] = usedCount + 1
			} else {
				// Fingerprint didn't match (content was truncated).
				// Fall back: match by role + toolCallID + content prefix
				// allowing for the truncation suffix.
				bPrefix := bpm.Content
				if len(bPrefix) > 50 {
					bPrefix = bPrefix[:50]
				}
				for ei, pm := range effectivePMs {
					origIdx := effectiveOrigIndices[ei]
					if budgetedOrigSet[origIdx] {
						continue // already matched
					}
					if pm.Role != bpm.Role || pm.ToolCallID != bpm.ToolCallID {
						continue
					}
					ePrefix := pm.Content
					if len(ePrefix) > 50 {
						ePrefix = ePrefix[:50]
					}
					if ePrefix == bPrefix {
						budgetedOrigSet[origIdx] = true
						break
					}
				}
			}
		}
	}

	// 7. Build segments — ALL messages, each fully classified
	var segments []ContextSegment
	var totalChars, totalTokens int
	var effectiveChars, effectiveTokens int

	// a) Compressed summary as a virtual segment (always first, always in_context & budgeted)
	if compressedCtx != nil && compressedCtx.Summary != "" {
		summaryChars := len(compressedCtx.Summary)
		summaryTokens := tc.EstimateText(compressedCtx.Summary) + 4
		segments = append(segments, ContextSegment{
			Type:            "compressed_summary",
			Role:            "assistant",
			Index:           -1,
			CharCount:       summaryChars,
			EstimatedTokens: summaryTokens,
			ContentPreview:  truncate(compressedCtx.Summary, previewLen),
			InContext:       true,
			Budgeted:        true,
		})
		totalChars += summaryChars
		totalTokens += summaryTokens
		effectiveChars += summaryChars
		effectiveTokens += summaryTokens
	}

	// b) All DB messages
	for i, rm := range allRawMessages {
		pm := toProviderMsg(rm)
		charCount, tokens := computeStats(rm, pm)

		// Classify segment type
		segType := "history_message"
		if compressedCtx != nil {
			if keptSet[rm.ID] {
				segType = "kept_message"
			} else if rm.CreatedAt.After(compressedCtx.CreatedAt) {
				segType = "history_message"
			} else {
				segType = "compressed_history"
			}
		}

		inContext := effectiveSet[i]
		isBudgeted := budgetedOrigSet[i]

		segments = append(segments, ContextSegment{
			Type:            segType,
			Role:            rm.Role,
			Index:           i,
			CharCount:       charCount,
			EstimatedTokens: tokens,
			ContentPreview:  truncate(rm.Content, previewLen),
			HasToolCalls:    len(pm.ToolCalls) > 0,
			ToolCallCount:   len(pm.ToolCalls),
			InContext:       inContext,
			Budgeted:        isBudgeted,
		})

		totalChars += charCount
		totalTokens += tokens
		if inContext {
			effectiveChars += charCount
			effectiveTokens += tokens
		}
	}

	// 8. Compute budgeted totals directly from the budgeted slice.
	// Do NOT derive BudgetedCount from segment matching — compute it
	// directly from len(budgeted) which is 100% accurate.
	budgetedTokens := tc.EstimateMessages(budgeted)
	budgetedChars := 0
	for _, pm := range budgeted {
		budgetedChars += len(pm.Content)
		for _, toolCall := range pm.ToolCalls {
			budgetedChars += len(toolCall.Arguments)
			if toolCall.Function != nil {
				budgetedChars += len(toolCall.Function.Name) + len(toolCall.Function.Arguments)
			}
		}
	}
	// BudgetMessages Phase 3 may inject a notice message — don't count it
	budgetedCount := 0
	for _, pm := range budgeted {
		if !strings.Contains(pm.Content, "[Earlier context dropped") {
			budgetedCount++
		}
	}
	// Summary is always sent to LLM — include in budgeted totals
	if compressedCtx != nil && compressedCtx.Summary != "" {
		budgetedTokens += tc.EstimateText(compressedCtx.Summary) + 4
		budgetedChars += len(compressedCtx.Summary)
		budgetedCount++
	}

	effectiveCount := 0
	for _, s := range segments {
		if s.InContext {
			effectiveCount++
		}
	}

	resp := SessionContextResponse{
		SessionID:       sessionID,
		Model:           session.Model,
		ContextWindow:   contextWindow,
		Segments:        segments,
		Compression:     compInfo,
		TotalMessages:   totalDBMessages,
		TotalChars:      totalChars,
		TotalTokens:     totalTokens,
		EffectiveCount:  effectiveCount,
		EffectiveChars:  effectiveChars,
		EffectiveTokens: effectiveTokens,
		BudgetedCount:   budgetedCount,
		BudgetedChars:   budgetedChars,
		BudgetedTokens:  budgetedTokens,
	}

	if resp.Segments == nil {
		resp.Segments = []ContextSegment{}
	}

	handlers.SendJSON(w, http.StatusOK, resp)
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
	// Check MiniMax models (with "minimax:" prefix)
	if !modelValid && strings.HasPrefix(body.Model, "minimax:") {
		modelValid = true
	}
	// Check GLM models (with "glm:" prefix)
	if !modelValid && strings.HasPrefix(body.Model, "glm:") {
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

	// Persist as the provider's default model in config.yaml
	persistProviderDefaultModel(body.Model)
	if err := viper.WriteConfig(); err != nil {
		log.Warn().Err(err).Msg("Failed to write config file after model change")
	}

	// Propagate to runner + delegate factory so PDA sub-agents use the new model
	if r.runner != nil {
		r.runner.UpdateDefaultModel(body.Model)
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
		if !modelValid && strings.HasPrefix(*body.Model, "minimax:") {
			modelValid = true
		}
		if !modelValid && strings.HasPrefix(*body.Model, "glm:") {
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

		// Persist as the provider's default model in config.yaml
		persistProviderDefaultModel(*body.Model)
		if err := viper.WriteConfig(); err != nil {
			log.Warn().Err(err).Msg("Failed to write config file after model change")
		}

		// Propagate to runner + delegate factory so PDA sub-agents use the new model
		if r.runner != nil {
			r.runner.UpdateDefaultModel(*body.Model)
		}
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
		Minimax: MinimaxConfigView{
			APIKey:    maskAPIKey(viper.GetString("minimax.api_key")),
			Endpoint:  viper.GetString("minimax.endpoint"),
			Model:     viper.GetString("minimax.model"),
			MaxTokens: viper.GetInt("minimax.max_tokens"),
		},
		GLM: GLMConfigView{
			APIKey:    maskAPIKey(viper.GetString("glm.api_key")),
			Endpoint:  viper.GetString("glm.endpoint"),
			Model:     viper.GetString("glm.model"),
			MaxTokens: viper.GetInt("glm.max_tokens"),
		},
		VLLM: VLLMConfigView{
			APIKey:    maskAPIKey(viper.GetString("vllm.api_key")),
			Endpoint:  viper.GetString("vllm.endpoint"),
			Model:     viper.GetString("vllm.model"),
			MaxTokens: viper.GetInt("vllm.max_tokens"),
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
		validProviders := []string{"copilot", "copilot-acp", "ollama", "minimax", "glm", "vllm"}

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
				handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "Invalid provider type. Supported: copilot, copilot-acp, ollama, minimax, glm, vllm")
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

	// Update MiniMax configuration
	if body.Minimax != nil {
		if body.Minimax.APIKey != "" {
			viper.Set("minimax.api_key", body.Minimax.APIKey)
		}
		if body.Minimax.Endpoint != "" {
			viper.Set("minimax.endpoint", body.Minimax.Endpoint)
		}
		if body.Minimax.Model != "" {
			viper.Set("minimax.model", body.Minimax.Model)
		}
		if body.Minimax.MaxTokens > 0 {
			viper.Set("minimax.max_tokens", body.Minimax.MaxTokens)
		}
	}

	// Update GLM configuration
	if body.GLM != nil {
		if body.GLM.APIKey != "" {
			viper.Set("glm.api_key", body.GLM.APIKey)
		}
		if body.GLM.Endpoint != "" {
			viper.Set("glm.endpoint", body.GLM.Endpoint)
		}
		if body.GLM.Model != "" {
			viper.Set("glm.model", body.GLM.Model)
		}
		if body.GLM.MaxTokens > 0 {
			viper.Set("glm.max_tokens", body.GLM.MaxTokens)
		}
	}

	// Update vLLM configuration
	if body.VLLM != nil {
		if body.VLLM.APIKey != "" {
			viper.Set("vllm.api_key", body.VLLM.APIKey)
		}
		if body.VLLM.Endpoint != "" {
			viper.Set("vllm.endpoint", body.VLLM.Endpoint)
		}
		if body.VLLM.Model != "" {
			viper.Set("vllm.model", body.VLLM.Model)
		}
		if body.VLLM.MaxTokens > 0 {
			viper.Set("vllm.max_tokens", body.VLLM.MaxTokens)
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
		Minimax: MinimaxConfigView{
			APIKey:    maskAPIKey(viper.GetString("minimax.api_key")),
			Endpoint:  viper.GetString("minimax.endpoint"),
			Model:     viper.GetString("minimax.model"),
			MaxTokens: viper.GetInt("minimax.max_tokens"),
		},
		GLM: GLMConfigView{
			APIKey:    maskAPIKey(viper.GetString("glm.api_key")),
			Endpoint:  viper.GetString("glm.endpoint"),
			Model:     viper.GetString("glm.model"),
			MaxTokens: viper.GetInt("glm.max_tokens"),
		},
		VLLM: VLLMConfigView{
			APIKey:    maskAPIKey(viper.GetString("vllm.api_key")),
			Endpoint:  viper.GetString("vllm.endpoint"),
			Model:     viper.GetString("vllm.model"),
			MaxTokens: viper.GetInt("vllm.max_tokens"),
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

// maskAPIKey masks an API key for safe display, showing only the last 4 chars.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
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
		// Build provider status without Ping — providers are assumed available
		// if they are registered in the pool. Users can manually check status
		// via POST /api/v1/providers/{name}/recover.
		for _, providerName := range r.multiPool.ListProviders() {
			modelCount := r.multiPool.ModelCountByProvider(providerName)

			providerStatuses = append(providerStatuses, ProviderStatus{
				Name:       providerName,
				Enabled:    true,
				Available:  true,
				ModelCount: modelCount,
			})
		}

		// Build model list from all providers
		if (providerFilter == "" || providerFilter == "copilot") && r.multiPool.HasProvider("copilot") {
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
		if (providerFilter == "" || providerFilter == "copilot-acp" || providerFilter == "copilot") && r.multiPool.HasProvider("copilot-acp") {
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

		// Add MiniMax models if provider is available and not filtered out
		if (providerFilter == "" || providerFilter == "minimax") && r.multiPool.HasProvider("minimax") {
			for _, modelInfo := range r.multiPool.ListAllModels() {
				if modelInfo.Provider != "minimax" {
					continue
				}
				// Use metadata if available, otherwise use defaults
				meta, hasMeta := minimax.ModelMetadata[modelInfo.OriginalID]
				displayName := modelInfo.OriginalID
				contextWindow := 204800
				maxOutput := 16384
				supportsTools := true
				description := "MiniMax cloud model"
				if hasMeta {
					displayName = meta.DisplayName
					contextWindow = meta.ContextWindow
					maxOutput = meta.MaxOutput
					supportsTools = meta.SupportsTools
					description = meta.Description
				}
				models = append(models, ModelView{
					ID:             modelInfo.ID,
					Provider:       "minimax",
					DisplayName:    displayName,
					Family:         "minimax",
					IsFree:         false,
					Multiplier:     0,
					ContextWindow:  contextWindow,
					MaxOutput:      maxOutput,
					SupportsVision: false,
					SupportsTools:  supportsTools,
					Description:    description,
					Available:      modelInfo.Available,
				})
			}
		}

		// Add GLM models if provider is available and not filtered out
		if (providerFilter == "" || providerFilter == "glm") && r.multiPool.HasProvider("glm") {
			for _, modelInfo := range r.multiPool.ListAllModels() {
				if modelInfo.Provider != "glm" {
					continue
				}
				// Use metadata if available, otherwise use defaults
				meta, hasMeta := glm.ModelMetadata[modelInfo.OriginalID]
				displayName := modelInfo.OriginalID
				contextWindow := 128000
				maxOutput := 16384
				supportsTools := true
				description := "GLM (智谱AI) cloud model"
				if hasMeta {
					displayName = meta.DisplayName
					contextWindow = meta.ContextWindow
					maxOutput = meta.MaxOutput
					supportsTools = meta.SupportsTools
					description = meta.Description
				}
				models = append(models, ModelView{
					ID:             modelInfo.ID,
					Provider:       "glm",
					DisplayName:    displayName,
					Family:         "glm",
					IsFree:         false,
					Multiplier:     0,
					ContextWindow:  contextWindow,
					MaxOutput:      maxOutput,
					SupportsVision: false,
					SupportsTools:  supportsTools,
					Description:    description,
					Available:      modelInfo.Available,
				})
			}
		}

		// Add vLLM models if provider is available and not filtered out
		if (providerFilter == "" || providerFilter == "vllm") && r.multiPool.HasProvider("vllm") {
			for _, modelInfo := range r.multiPool.ListAllModels() {
				if modelInfo.Provider != "vllm" {
					continue
				}
				models = append(models, ModelView{
					ID:             modelInfo.ID,
					Provider:       "vllm",
					DisplayName:    modelInfo.OriginalID,
					Family:         "vllm",
					IsFree:         true, // vLLM models are free (local/self-hosted)
					Multiplier:     0,
					ContextWindow:  4096, // Default; actual depends on model
					MaxOutput:      4096,
					SupportsVision: false, // TODO: Detect from model metadata
					SupportsTools:  true,  // vLLM supports tool calling
					Description:    "vLLM local model",
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

	// Get current model from config.
	// copilot.model stores the full model ID (with provider prefix) of the
	// last-used model, regardless of provider.
	current := viper.GetString("copilot.model")
	if current == "" {
		current = copilot.DefaultModel
	}

	// Determine provider-aware default model.
	// The default must be a model from an actually enabled provider.
	defaultModel := copilot.DefaultModel
	if r.multiPool != nil {
		hasAPI := r.multiPool.HasProvider("copilot")
		hasACP := r.multiPool.HasProvider("copilot-acp")
		hasMinimax := r.multiPool.HasProvider("minimax")
		hasOllama := r.multiPool.HasProvider("ollama")

		if hasAPI {
			defaultModel = copilot.DefaultModel
		} else if hasACP {
			defaultModel = copilot.ACPDefaultModel
		} else if hasMinimax {
			defaultModel = "minimax:" + minimax.DefaultModel
		} else if hasOllama {
			// Use first available ollama model
			for _, m := range models {
				if m.Provider == "ollama" {
					defaultModel = m.ID
					break
				}
			}
		}

		// Correct current if it refers to a model from an unavailable provider
		currentValid := false
		for _, m := range models {
			if m.ID == current {
				currentValid = true
				break
			}
		}
		if !currentValid {
			current = defaultModel
		}
	}

	handlers.SendJSON(w, http.StatusOK, ModelsResponse{
		Models:    models,
		Current:   current,
		Default:   defaultModel,
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

	// Update viper config — save to the correct provider's model field
	persistProviderDefaultModel(request.Model)

	// Propagate to runner + delegate factory so PDA sub-agents use the new model
	if r.runner != nil {
		r.runner.UpdateDefaultModel(request.Model)
	}

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
//
// This endpoint does NOT trigger Ping or network requests. It returns cached
// state for providers that have it (copilot, copilot-acp) and assumes
// "connected" for providers without cached state (minimax, ollama).
// Use POST /api/v1/providers/{name}/recover to manually check connectivity.
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

			// For copilot/copilot-acp: GetState() only reads cached state (no network).
			// For minimax/ollama: GetState() would trigger Ping, so we skip it
			// and return a default "connected" state.
			if hc, ok := prov.(provider.HealthCheckable); ok {
				switch providerName {
				case "copilot", "copilot-acp":
					// Safe — no network calls, just reads in-memory token/process state
					state := hc.GetState()
					states = append(states, state)
				default:
					// minimax, ollama: don't trigger Ping, assume connected
					states = append(states, provider.ProviderState{
						Name:   providerName,
						Status: provider.StatusConnected,
						Models: prov.Models(),
					})
				}
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

// HandleListDelegations returns all delegation records for a session.
// HandleListRecentDelegations returns the most recent delegation records
// across all sessions.  This is the primary data source for the Agents page
// delegation history tab.
func (r *Router) HandleListRecentDelegations(w http.ResponseWriter, req *http.Request) {
	if r.delegateTracker == nil {
		handlers.SendJSON(w, http.StatusOK, []any{})
		return
	}

	limit := 50
	if v := req.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	records, err := r.delegateTracker.GetRecent(limit)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list recent delegations")
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to list delegations")
		return
	}

	if records == nil {
		records = []delegate.DelegationRecord{}
	}
	handlers.SendJSON(w, http.StatusOK, records)
}

func (r *Router) HandleListDelegations(w http.ResponseWriter, req *http.Request) {
	if r.delegateTracker == nil {
		handlers.SendJSON(w, http.StatusOK, []any{})
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["id"]
	if sessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "session ID required")
		return
	}

	records, err := r.delegateTracker.GetByParentSession(sessionID)
	if err != nil {
		log.Error().Err(err).Str("sessionID", sessionID).Msg("Failed to list delegations")
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to list delegations")
		return
	}

	if records == nil {
		records = []delegate.DelegationRecord{}
	}
	handlers.SendJSON(w, http.StatusOK, records)
}

// HandleGetDelegation returns a single delegation record by ID.
func (r *Router) HandleGetDelegation(w http.ResponseWriter, req *http.Request) {
	if r.delegateTracker == nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "delegation tracking not enabled")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	if id == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "delegation ID required")
		return
	}

	record, err := r.delegateTracker.GetByID(id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get delegation")
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "failed to get delegation")
		return
	}

	if record == nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "delegation not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, record)
}

// HandleBatchDeleteDelegations deletes multiple delegation records at once.
func (r *Router) HandleBatchDeleteDelegations(w http.ResponseWriter, req *http.Request) {
	if r.delegateTracker == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, handlers.ErrCodeServiceUnavailable, "delegation tracking not enabled")
		return
	}

	var body BatchDeleteDelegationsRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if len(body.IDs) == 0 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "No delegation IDs provided")
		return
	}

	if len(body.IDs) > 200 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Too many delegation IDs (max 200)")
		return
	}

	deleted, err := r.delegateTracker.DeleteByIDs(body.IDs)
	if err != nil {
		log.Error().Err(err).Int("count", len(body.IDs)).Msg("Failed to batch delete delegations")
		handlers.SendError(w, http.StatusInternalServerError, handlers.ErrCodeInternalError, "Failed to delete delegations")
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": deleted,
		"total":   len(body.IDs),
	})
}

// HandleListAgents returns all configured delegate agents.
func (r *Router) HandleListAgents(w http.ResponseWriter, req *http.Request) {
	cfg := config.GetConfig()
	if cfg == nil || len(cfg.Agents) == 0 {
		handlers.SendJSON(w, http.StatusOK, map[string]any{"agents": map[string]any{}})
		return
	}
	handlers.SendJSON(w, http.StatusOK, map[string]any{"agents": cfg.Agents})
}

// HandleGetAgent returns a single agent configuration by name.
func (r *Router) HandleGetAgent(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]
	cfg := config.GetConfig()
	if cfg == nil || cfg.Agents == nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}
	agent, ok := cfg.Agents[name]
	if !ok {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}
	handlers.SendJSON(w, http.StatusOK, map[string]any{"name": name, "config": agent})
}

// HandleAddAgent adds a new agent configuration.
func (r *Router) HandleAddAgent(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Name  string             `json:"name"`
		Agent config.AgentConfig `json:"agent"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid JSON body")
		return
	}
	if body.Name == "" {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "agent name required")
		return
	}
	if err := config.AddAgent(body.Name, body.Agent); err != nil {
		handlers.SendError(w, http.StatusConflict, "CONFLICT", err.Error())
		return
	}
	handlers.SendJSON(w, http.StatusCreated, map[string]any{"name": body.Name, "agent": body.Agent})
}

// HandleUpdateAgent updates an existing agent configuration.
func (r *Router) HandleUpdateAgent(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]
	var agent config.AgentConfig
	if err := json.NewDecoder(req.Body).Decode(&agent); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid JSON body")
		return
	}
	if err := config.UpdateAgent(name, agent); err != nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	handlers.SendJSON(w, http.StatusOK, map[string]any{"name": name, "agent": agent})
}

// HandleDeleteAgent removes an agent configuration.
func (r *Router) HandleDeleteAgent(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]
	if err := config.RemoveAgent(name); err != nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	handlers.SendJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

// HandleReloadAgents reloads agents from agents.yaml and agents/ directory.
// POST /api/v1/agents/reload
func (r *Router) HandleReloadAgents(w http.ResponseWriter, req *http.Request) {
	count, err := config.ReloadAgents()
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, "RELOAD_FAILED", err.Error())
		return
	}
	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  count,
	})
}

// HandleValidateAgentsDir validates all YAML files in the agents/ directory.
// GET /api/v1/agents/validate-dir
func (r *Router) HandleValidateAgentsDir(w http.ResponseWriter, req *http.Request) {
	summary := config.ValidateAgentsDir()
	handlers.SendJSON(w, http.StatusOK, summary)
}

// persistProviderDefaultModel saves the model as the default for its provider
// in viper config. For example, "ollama:gpt-oss:20b" sets ollama.model to
// "gpt-oss:20b". Also updates copilot.model with the full model ID for
// backward compatibility.
func persistProviderDefaultModel(modelID string) {
	// Always update copilot.model as the "current global model" for backward compat
	viper.Set("copilot.model", modelID)

	// Parse provider prefix and save to provider-specific config key
	switch {
	case strings.HasPrefix(modelID, "ollama:"):
		viper.Set("ollama.model", strings.TrimPrefix(modelID, "ollama:"))
	case strings.HasPrefix(modelID, "minimax:"):
		viper.Set("minimax.model", strings.TrimPrefix(modelID, "minimax:"))
	case strings.HasPrefix(modelID, "glm:"):
		viper.Set("glm.model", strings.TrimPrefix(modelID, "glm:"))
	case strings.HasPrefix(modelID, "vllm:"):
		viper.Set("vllm.model", strings.TrimPrefix(modelID, "vllm:"))
	}
	// For copilot models (no prefix), copilot.model is already set above
}
