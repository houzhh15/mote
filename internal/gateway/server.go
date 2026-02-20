// Package gateway provides the HTTP gateway server.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	v1 "mote/api/v1"
	internalChannel "mote/internal/channel"
	"mote/internal/config"
	"mote/internal/cron"
	"mote/internal/gateway/handlers"
	"mote/internal/gateway/middleware"
	"mote/internal/gateway/websocket"
	"mote/internal/mcp/client"
	"mote/internal/mcp/server"
	"mote/internal/memory"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/prompts"
	"mote/internal/provider"
	"mote/internal/runner"
	"mote/internal/skills"
	"mote/internal/storage"
	"mote/internal/tools"
	"mote/internal/ui"
	"mote/internal/workspace"
	"mote/pkg/logger"
)

// Server represents the HTTP gateway server.
type Server struct {
	httpServer       *http.Server
	router           *mux.Router
	hub              *websocket.Hub
	watcher          *Watcher
	config           *config.Config
	db               *storage.DB
	uiHandler        *ui.Handler
	uiRegistry       *ui.Registry
	uiState          *ui.StateManager
	rateLimiter      *middleware.RateLimiter
	apiRouter        *v1.Router
	agentRunner      *runner.Runner
	toolRegistry     *tools.Registry
	memoryIndex      *memory.MemoryIndex
	memoryManager    *memory.MemoryManager
	mcpClient        *client.Manager
	mcpServer        *server.Server
	cronScheduler    *cron.Scheduler
	policyExecutor   *policy.PolicyExecutor
	approvalManager  *approval.Manager
	channelRegistry  *internalChannel.Registry
	workspaceManager *workspace.WorkspaceManager
	skillManager     *skills.Manager
	promptManager    *prompts.Manager
	multiPool        *provider.MultiProviderPool
	embeddedServer   v1.EmbeddedServerInterface
	versionChecker   interface{} // *skills.VersionChecker
	skillUpdater     interface{} // *skills.SkillUpdater
}

// NewServer creates a new gateway server.
func NewServer(cfg *config.Config, hub *websocket.Hub, db *storage.DB) *Server {
	router := mux.NewRouter()

	// Initialize rate limiter
	rlConfig := middleware.RateLimiterConfig{
		RequestsPerMinute: cfg.Gateway.RateLimit.RequestsPerMinute,
		Burst:             cfg.Gateway.RateLimit.Burst,
		Enabled:           cfg.Gateway.RateLimit.Enabled,
		CleanupInterval:   cfg.Gateway.RateLimit.CleanupInterval,
	}
	if rlConfig.RequestsPerMinute == 0 {
		rlConfig.RequestsPerMinute = 60
	}
	if rlConfig.Burst == 0 {
		rlConfig.Burst = 10
	}
	if rlConfig.CleanupInterval == 0 {
		rlConfig.CleanupInterval = 5 * time.Minute
	}
	rateLimiter := middleware.NewRateLimiter(rlConfig)

	// Initialize version middleware
	versionConfig := middleware.DefaultVersionConfig()

	// Apply middleware chain: Recovery -> Logging -> CORS -> RateLimit -> Version
	handler := middleware.Recovery(
		middleware.Logging(
			middleware.CORS(
				rateLimiter.RateLimit(
					middleware.Version(versionConfig)(router),
				),
			),
		),
	)

	// Initialize UI components
	uiRegistry := ui.NewRegistry(cfg.Gateway.UIDir)
	if err := uiRegistry.Scan(); err != nil {
		logger.Warn().Err(err).Msg("Failed to scan UI directory")
	}

	uiState := ui.NewStateManager(hub)
	staticServer := ui.NewStaticServer(cfg.Gateway.UIDir, ui.GetEmbedFS())
	uiHandler := ui.NewHandler(uiRegistry, uiState, staticServer, db)

	server := &Server{
		httpServer: &http.Server{
			Handler:      handler,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 0, // Disable write timeout for SSE streaming (handled by request context)
			IdleTimeout:  120 * time.Second,
		},
		router:      router,
		hub:         hub,
		config:      cfg,
		db:          db,
		uiHandler:   uiHandler,
		uiRegistry:  uiRegistry,
		uiState:     uiState,
		rateLimiter: rateLimiter,
	}

	// Note: setupRoutes() is called later via InitializeRoutes() after all dependencies are set

	return server
}

// setupRoutes configures the server routes.
func (s *Server) setupRoutes() {
	// Initialize API v1 router with dependencies
	deps := &v1.RouterDeps{
		Runner:           s.agentRunner,
		Tools:            s.toolRegistry,
		Memory:           s.memoryIndex,
		MemoryManager:    s.memoryManager,
		MCPClient:        s.mcpClient,
		MCPServer:        s.mcpServer,
		CronScheduler:    s.cronScheduler,
		UIHandler:        s.uiHandler,
		DB:               s.db,
		PolicyExecutor:   s.policyExecutor,
		ApprovalManager:  s.approvalManager,
		ChannelRegistry:  s.channelRegistry,
		WorkspaceManager: s.workspaceManager,
		SkillManager:     s.skillManager,
		PromptManager:    s.promptManager,
		MultiPool:        s.multiPool,
		EmbeddedServer:   s.embeddedServer,
	}
	s.apiRouter = v1.NewRouter(deps)

	// Set additional dependencies that are not in RouterDeps
	if s.versionChecker != nil {
		s.apiRouter.SetVersionChecker(s.versionChecker)
	}
	if s.skillUpdater != nil {
		s.apiRouter.SetSkillUpdater(s.skillUpdater)
	}

	// Register API v1 routes
	s.apiRouter.RegisterRoutes(s.router)

	// Setup legacy API redirects (must be after v1 routes)
	v1.SetupLegacyRedirects(s.router)

	// WebSocket endpoint
	s.router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWs(s.hub, w, r)
	})

	// Register UI routes (includes static file serving)
	s.uiHandler.RegisterRoutes(s.router)
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	handlers.InitStartTime()

	addr := fmt.Sprintf("%s:%d", s.config.Gateway.Host, s.config.Gateway.Port)
	s.httpServer.Addr = addr

	// Start WebSocket hub
	go s.hub.Run()

	// Start channel registry if configured
	if s.channelRegistry != nil {
		ctx := context.Background()
		if err := s.channelRegistry.StartAll(ctx); err != nil {
			logger.Error().Err(err).Msg("Failed to start channel registry")
		} else {
			logger.Info().Int("count", s.channelRegistry.Count()).Msg("Started channel registry")
		}
	}

	logger.Info().
		Str("addr", addr).
		Msg("Starting gateway server")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	logger.Info().Msg("Shutting down gateway server")

	// Stop channel registry
	if s.channelRegistry != nil {
		if err := s.channelRegistry.StopAll(ctx); err != nil {
			logger.Warn().Err(err).Msg("Failed to stop channel registry")
		} else {
			logger.Info().Msg("Stopped channel registry")
		}
	}

	// Stop watcher if running
	if s.watcher != nil {
		s.watcher.Stop()
	}

	// Stop rate limiter
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	return nil
}

// IsReady returns true if the server is ready to accept requests.
func (s *Server) IsReady() bool {
	return s.httpServer != nil && s.httpServer.Addr != ""
}

// SetWatcher sets the file watcher for hot reload.
func (s *Server) SetWatcher(w *Watcher) {
	s.watcher = w
}

// Router returns the underlying router for testing.
func (s *Server) Router() *mux.Router {
	return s.router
}

// Hub returns the WebSocket hub.
func (s *Server) Hub() *websocket.Hub {
	return s.hub
}

// UIRegistry returns the UI component registry.
func (s *Server) UIRegistry() *ui.Registry {
	return s.uiRegistry
}

// UIState returns the UI state manager.
func (s *Server) UIState() *ui.StateManager {
	return s.uiState
}

// SetAgentRunner sets the agent runner dependency.
func (s *Server) SetAgentRunner(r *runner.Runner) {
	s.agentRunner = r
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetRunner(r)
	}

	// Set MCP client manager if already configured
	if s.mcpClient != nil && r != nil {
		r.SetMCPManager(s.mcpClient)
	}

	// Set up WebSocket chat handler
	if s.hub != nil && r != nil {
		s.hub.SetChatHandler(func(sessionID, message string) (<-chan []byte, error) {
			return s.handleWebSocketChat(sessionID, message)
		})
	}
}

// handleWebSocketChat handles a chat message received via WebSocket.
func (s *Server) handleWebSocketChat(sessionID, message string) (<-chan []byte, error) {
	if s.agentRunner == nil {
		return nil, nil
	}

	ctx := context.Background()

	// Run agent and convert events to WebSocket messages
	events, err := s.agentRunner.Run(ctx, sessionID, message)
	if err != nil {
		return nil, err
	}

	outChan := make(chan []byte, 100)

	go func() {
		defer close(outChan)

		for event := range events {
			var wsMsg websocket.WSMessage

			switch event.Type {
			case runner.EventTypeContent:
				wsMsg = websocket.WSMessage{
					Type:    websocket.TypeStream,
					Delta:   event.Content,
					Session: sessionID,
				}
			case runner.EventTypeToolCall:
				if event.ToolCall != nil {
					wsMsg = websocket.WSMessage{
						Type:    websocket.TypeToolCall,
						Tool:    event.ToolCall.GetName(),
						Params:  event.ToolCall.Function,
						Session: sessionID,
					}
				} else {
					continue
				}
			case runner.EventTypeToolResult:
				if event.ToolResult != nil {
					resultData, _ := json.Marshal(map[string]interface{}{
						"tool_call_id": event.ToolResult.ToolCallID,
						"tool_name":    event.ToolResult.ToolName,
						"output":       event.ToolResult.Output,
						"is_error":     event.ToolResult.IsError,
						"duration_ms":  event.ToolResult.DurationMs,
					})
					wsMsg = websocket.WSMessage{
						Type:    "tool_result",
						Session: sessionID,
						Params:  resultData,
					}
				} else {
					continue
				}
			case runner.EventTypeDone:
				wsMsg = websocket.WSMessage{
					Type:    "done",
					Session: sessionID,
				}
			case runner.EventTypeError:
				wsMsg = websocket.WSMessage{
					Type:    websocket.TypeError,
					Message: event.ErrorMsg,
					Session: sessionID,
				}
			default:
				continue
			}

			data, err := json.Marshal(wsMsg)
			if err != nil {
				continue
			}

			select {
			case outChan <- data:
			default:
				// Buffer full, skip event
			}
		}
	}()

	return outChan, nil
}

// SetToolRegistry sets the tool registry dependency.
func (s *Server) SetToolRegistry(r *tools.Registry) {
	s.toolRegistry = r
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetTools(r)
	}
}

// SetMemoryIndex sets the memory index dependency.
func (s *Server) SetMemoryIndex(m *memory.MemoryIndex) {
	s.memoryIndex = m
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetMemory(m)
	}
}

// SetMemoryManager sets the memory manager dependency.
func (s *Server) SetMemoryManager(m *memory.MemoryManager) {
	s.memoryManager = m
	if s.apiRouter != nil {
		s.apiRouter.SetMemoryManager(m)
	}
}

// SetMCPClient sets the MCP client manager dependency.
func (s *Server) SetMCPClient(c *client.Manager) {
	s.mcpClient = c
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetMCPClient(c)
	}
	// Also update the agent runner for dynamic tool injection in prompts
	if s.agentRunner != nil {
		s.agentRunner.SetMCPManager(c)
	}
}

// SetMCPServer sets the MCP server dependency.
func (s *Server) SetMCPServer(srv *server.Server) {
	s.mcpServer = srv
}

// SetCronScheduler sets the cron scheduler dependency.
func (s *Server) SetCronScheduler(c *cron.Scheduler) {
	s.cronScheduler = c
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetCronScheduler(c)
	}
}

// SetPolicyExecutor sets the policy executor dependency.
func (s *Server) SetPolicyExecutor(p *policy.PolicyExecutor) {
	s.policyExecutor = p
}

// SetApprovalManager sets the approval manager dependency.
func (s *Server) SetApprovalManager(m *approval.Manager) {
	s.approvalManager = m
}

// SetChannelRegistry sets the channel registry dependency.
func (s *Server) SetChannelRegistry(r *internalChannel.Registry) {
	s.channelRegistry = r
}

// SetWorkspaceManager sets the workspace manager dependency.
func (s *Server) SetWorkspaceManager(w *workspace.WorkspaceManager) {
	s.workspaceManager = w
}

// SetSkillManager sets the skill manager dependency.
func (s *Server) SetSkillManager(m *skills.Manager) {
	s.skillManager = m
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetSkillManager(m)
	}
}

// SetVersionChecker sets the version checker dependency.
func (s *Server) SetVersionChecker(vc interface{}) {
	s.versionChecker = vc
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetVersionChecker(vc)
	}
}

// SetSkillUpdater sets the skill updater dependency.
func (s *Server) SetSkillUpdater(su interface{}) {
	s.skillUpdater = su
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetSkillUpdater(su)
	}
}

// SetPromptManager sets the prompt manager dependency.
func (s *Server) SetPromptManager(m *prompts.Manager) {
	s.promptManager = m
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetPromptManager(m)
	}
}

// SetMultiPool sets the multi-provider pool dependency.
func (s *Server) SetMultiPool(pool *provider.MultiProviderPool) {
	s.multiPool = pool
	// Also update the API router
	if s.apiRouter != nil {
		s.apiRouter.SetMultiPool(pool)
	}
}

// SetEmbeddedServer sets the embedded server reference for hot reload support.
func (s *Server) SetEmbeddedServer(srv v1.EmbeddedServerInterface) {
	s.embeddedServer = srv
}

// InitializeRoutes initializes routes after all dependencies are set.
// This must be called before starting the server.
func (s *Server) InitializeRoutes() {
	s.setupRoutes()
}
