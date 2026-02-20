// Package server provides a shared server implementation for both CLI and GUI modes.
// Instead of duplicating initialization logic, both modes use this single implementation.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	v1 "mote/api/v1"
	"mote/internal/cli/defaults"
	"mote/internal/compaction"
	"mote/internal/config"
	internalContext "mote/internal/context"
	"mote/internal/cron"
	"mote/internal/gateway"
	"mote/internal/gateway/websocket"
	"mote/internal/hooks"
	hooksbuiltin "mote/internal/hooks/builtin"
	"mote/internal/jsvm"
	"mote/internal/mcp/client"
	"mote/internal/memory"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/prompt"
	"mote/internal/prompts"
	"mote/internal/provider"
	"mote/internal/provider/copilot"
	"mote/internal/provider/minimax"
	"mote/internal/provider/ollama"
	"mote/internal/runner"
	"mote/internal/scheduler"
	"mote/internal/skills"
	"mote/internal/storage"
	"mote/internal/tools"
	"mote/internal/tools/builtin"
	"mote/internal/workspace"

	"github.com/rs/zerolog"
)

// Server is the embedded mote server that runs in-process.
type Server struct {
	cfg              *config.Config
	logger           zerolog.Logger
	gatewayServer    *gateway.Server
	cronScheduler    *cron.Scheduler
	db               *storage.DB
	multiPool        *provider.MultiProviderPool // Provider pool for hot reload
	toolRegistry     *tools.Registry             // Tool registry for ACP bridge
	workspaceManager *workspace.WorkspaceManager // Workspace manager for session bindings
	skillManager     *skills.Manager             // Skill manager for skills prompt injection
	ctx              context.Context
	cancel           context.CancelFunc
	running          bool
	mu               sync.RWMutex
	startedAt        time.Time
	errChan          chan error
	onStateChange    func(bool)
}

// ServerConfig holds configuration for the embedded server.
type ServerConfig struct {
	ConfigPath    string
	StoragePath   string
	Logger        zerolog.Logger
	OnStateChange func(bool)
}

// NewServer creates a new embedded server instance.
func NewServer(cfg ServerConfig) (*Server, error) {
	// Load configuration
	moteCfg, err := config.Load(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Set defaults
	if moteCfg.Gateway.Port == 0 {
		moteCfg.Gateway.Port = 18788
	}
	if moteCfg.Gateway.Host == "" {
		moteCfg.Gateway.Host = "localhost"
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		cfg:           moteCfg,
		logger:        cfg.Logger,
		ctx:           ctx,
		cancel:        cancel,
		errChan:       make(chan error, 1),
		onStateChange: cfg.OnStateChange,
	}, nil
}

// ErrorChan returns the error channel for monitoring server errors.
func (s *Server) ErrorChan() <-chan error {
	return s.errChan
}

// Start starts the embedded server in a goroutine.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// Start server initialization in background
	go s.run()

	// Wait for server to be ready (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("server start timeout")
		case err := <-s.errChan:
			return fmt.Errorf("server start failed: %w", err)
		case <-ticker.C:
			if s.IsReady() {
				return nil
			}
		}
	}
}

// run is the main server loop running in a goroutine.
func (s *Server) run() {
	s.logger.Info().Msg("Starting embedded mote server...")

	// Reload configuration to pick up any changes (e.g., new auth token)
	if err := s.reloadConfig(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to reload config, using existing config")
	}

	// Initialize database
	db, err := storage.Open(s.getStoragePath())
	if err != nil {
		s.errChan <- fmt.Errorf("failed to initialize database: %w", err)
		return
	}
	s.db = db

	// Initialize WebSocket hub
	hub := websocket.NewHub()

	// Initialize gateway server
	s.gatewayServer = gateway.NewServer(s.cfg, hub, db)

	// Initialize multi-provider pool for supporting multiple providers simultaneously
	multiPool := provider.NewMultiProviderPool()
	var chatModel string
	var maxTokens int = 4096

	// Get enabled providers from configuration
	enabledProviders := s.cfg.Provider.GetEnabledProviders()
	s.logger.Info().Strs("enabled_providers", enabledProviders).Msg("Initializing providers")

	// Initialize each enabled provider
	for _, providerName := range enabledProviders {
		switch providerName {
		case "ollama":
			// Initialize Ollama provider
			ollamaCfg := ollama.Config{
				Endpoint:  s.cfg.Ollama.Endpoint,
				Model:     s.cfg.Ollama.Model,
				KeepAlive: s.cfg.Ollama.KeepAlive,
			}
			if ollamaCfg.Endpoint == "" {
				ollamaCfg.Endpoint = ollama.DefaultEndpoint
			}
			if ollamaCfg.Model == "" {
				ollamaCfg.Model = ollama.DefaultModel
			}
			if s.cfg.Ollama.Timeout != "" {
				if d, err := time.ParseDuration(s.cfg.Ollama.Timeout); err == nil {
					ollamaCfg.Timeout = d
				}
			}
			if ollamaCfg.Timeout == 0 {
				ollamaCfg.Timeout = ollama.DefaultTimeout
			}
			if ollamaCfg.KeepAlive == "" {
				ollamaCfg.KeepAlive = ollama.DefaultKeepAlive
			}

			ollamaFactory := func(model string) (provider.Provider, error) {
				cfg := ollamaCfg
				if model != "" {
					cfg.Model = model
				}
				return ollama.NewOllamaProvider(cfg), nil
			}
			ollamaPool := provider.NewPool(ollamaFactory)

			// Get Ollama model list (may fail if Ollama is not running)
			ollamaModels := []string{ollamaCfg.Model} // At minimum, use configured default
			if ollamaProvider := ollama.NewOllamaProvider(ollamaCfg); ollamaProvider != nil {
				if modelList := ollamaProvider.Models(); len(modelList) > 0 {
					ollamaModels = modelList
				}
			}

			if err := multiPool.AddProvider("ollama", ollamaPool, ollamaModels); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to add Ollama provider")
			} else {
				s.logger.Info().
					Str("provider", "ollama").
					Str("endpoint", ollamaCfg.Endpoint).
					Int("models", len(ollamaModels)).
					Msg("Ollama provider initialized")
			}

		case "copilot":
			// Initialize Copilot API provider (free models via REST API)
			// Requires: GitHub Token
			githubToken := s.cfg.Copilot.Token
			if githubToken == "" {
				s.logger.Warn().Msg("GitHub token not configured, skipping Copilot API provider")
				continue
			}

			maxTokens = s.cfg.Copilot.MaxTokens
			if maxTokens <= 0 {
				maxTokens = 4096
			}

			copilotFactory := copilot.Factory(githubToken, maxTokens)
			copilotPool := provider.NewPool(copilotFactory)
			copilotModels := copilot.ListModels()

			if err := multiPool.AddProvider("copilot", copilotPool, copilotModels); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to add Copilot API provider")
			} else {
				s.logger.Info().
					Str("provider", "copilot").
					Int("models", len(copilotModels)).
					Msg("Copilot API provider initialized (REST API, free models)")
			}

		case "copilot-acp":
			// Initialize Copilot ACP provider (premium models via Copilot CLI)
			// Independent of copilot API — uses Copilot CLI for authentication
			// No GitHub Token required; CLI handles its own auth.
			acpFactory := copilot.ACPFactoryWithConfigFunc(s.buildACPConfig)
			acpPool := provider.NewPool(acpFactory)
			acpModels := copilot.ACPListModels()

			if err := multiPool.AddProvider("copilot-acp", acpPool, acpModels); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to add Copilot ACP provider")
			} else {
				s.logger.Info().
					Str("provider", "copilot-acp").
					Int("models", len(acpModels)).
					Msg("Copilot ACP provider initialized (Copilot CLI, premium models)")
			}

		case "minimax":
			// Initialize MiniMax provider (cloud API, OpenAI-compatible)
			apiKey := s.cfg.Minimax.APIKey
			if apiKey == "" {
				s.logger.Warn().Msg("MiniMax API key not configured, skipping MiniMax provider")
				continue
			}

			mmMaxTokens := s.cfg.Minimax.MaxTokens
			if mmMaxTokens <= 0 {
				mmMaxTokens = 4096
			}

			minimaxFactory := minimax.Factory(apiKey, mmMaxTokens)
			minimaxPool := provider.NewPool(minimaxFactory)
			minimaxModels := minimax.ListModels()

			if err := multiPool.AddProvider("minimax", minimaxPool, minimaxModels); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to add MiniMax provider")
			} else {
				s.logger.Info().
					Str("provider", "minimax").
					Int("models", len(minimaxModels)).
					Msg("MiniMax provider initialized")
			}
		}
	}

	// Set multiPool on gateway for /api/v1/models endpoint
	s.gatewayServer.SetMultiPool(multiPool)

	// Save multiPool reference for hot reload
	s.multiPool = multiPool

	// Set embedded server reference on gateway for hot reload support
	s.gatewayServer.SetEmbeddedServer(s)

	// Determine default provider — prefer explicit config, then auto-detect
	// from the enabled providers list. Never hardcode a specific provider
	// as fallback since the user may only have minimax/ollama enabled.
	defaultProviderName := s.cfg.Provider.Default
	if defaultProviderName == "copilot" {
		// copilot REST API is disabled; migrate to copilot-acp (CLI mode)
		defaultProviderName = "copilot-acp"
	}
	if defaultProviderName == "" {
		// Auto-detect: use the first actually-enabled provider
		if len(enabledProviders) > 0 {
			defaultProviderName = enabledProviders[0]
		} else {
			defaultProviderName = "copilot-acp" // last resort
		}
	}
	s.logger.Info().Str("default_provider", defaultProviderName).Msg("Default provider determined")

	// Determine default chat model based on the default provider.
	// Each provider has its own default model; we pick the right one
	// instead of always assuming copilot.
	chatModel = s.cfg.Copilot.Model // explicit config takes priority
	if chatModel == "" {
		switch defaultProviderName {
		case "copilot-acp":
			chatModel = copilot.ACPDefaultModel
		case "copilot":
			chatModel = copilot.DefaultModel
		case "minimax":
			chatModel = "minimax:" + minimax.DefaultModel
		case "ollama":
			if s.cfg.Ollama.Model != "" {
				chatModel = "ollama:" + s.cfg.Ollama.Model
			} else {
				chatModel = "ollama:" + ollama.DefaultModel
			}
		default:
			chatModel = copilot.ACPDefaultModel
		}
	} else {
		// Explicit copilot.model is set — cross-validate against the default provider.
		if defaultProviderName == "copilot-acp" && !copilot.IsACPModel(chatModel) {
			s.logger.Warn().Str("model", chatModel).Str("fallback", copilot.ACPDefaultModel).
				Msg("Default model is not compatible with copilot-acp provider, using ACP default")
			chatModel = copilot.ACPDefaultModel
		} else if defaultProviderName == "minimax" {
			// copilot.model is set but default provider is minimax — use minimax default
			s.logger.Warn().Str("copilot_model", chatModel).Str("provider", defaultProviderName).
				Msg("copilot.model ignored because default provider is minimax")
			chatModel = "minimax:" + minimax.DefaultModel
		} else if defaultProviderName == "ollama" {
			s.logger.Warn().Str("copilot_model", chatModel).Str("provider", defaultProviderName).
				Msg("copilot.model ignored because default provider is ollama")
			if s.cfg.Ollama.Model != "" {
				chatModel = "ollama:" + s.cfg.Ollama.Model
			} else {
				chatModel = "ollama:" + ollama.DefaultModel
			}
		}
	}

	// Get cron model
	cronModel := s.cfg.Cron.Model
	if cronModel == "" {
		cronModel = chatModel // Use chat model as default for cron
	}

	// Get default provider for runner
	var defaultProvider provider.Provider
	if pool, ok := multiPool.GetPool(defaultProviderName); ok {
		defaultProvider, err = pool.Get(chatModel)
		if err != nil {
			s.logger.Warn().Err(err).Str("model", chatModel).Msg("Failed to get provider for model")
		}
	}
	// Fallback to first available provider if default failed
	if defaultProvider == nil {
		for _, name := range multiPool.ListProviders() {
			if pool, ok := multiPool.GetPool(name); ok {
				defaultProvider, _ = pool.Get("")
				if defaultProvider != nil {
					break
				}
			}
		}
	}
	// Allow server to start without provider - provider supports hot reload
	// Users can configure provider later and it will be picked up
	if defaultProvider == nil {
		s.logger.Warn().Msg("No provider available at startup. Configure a provider (copilot/ollama) to enable chat functionality. Provider supports hot reload.")
	}

	// Initialize tools registry
	toolRegistry := tools.NewRegistry()
	if err := builtin.RegisterBuiltins(toolRegistry); err != nil {
		s.errChan <- fmt.Errorf("failed to register builtin tools: %w", err)
		return
	}
	s.toolRegistry = toolRegistry

	// Clear ACP provider cache so it will be recreated with the new toolRegistry.
	// ACP provider was initialized earlier (before toolRegistry was ready), so we
	// need to clear its cache to ensure new providers get the updated config.
	if acpPool, ok := multiPool.GetPool("copilot-acp"); ok {
		acpPool.Clear()
		s.logger.Debug().Msg("ACP provider cache cleared for toolRegistry injection")
	}

	// Initialize JSVM Runtime
	jsvmLogger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	jsvmRuntime := jsvm.NewRuntime(jsvm.DefaultRuntimeConfig(), db, jsvmLogger)

	// Initialize MCP client manager
	mcpManager := client.NewManager(nil)
	builtin.SetMCPManager(mcpManager)
	if err := builtin.RegisterMCPTools(toolRegistry); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to register MCP tools")
	}

	// Load saved MCP servers from config file
	if err := v1.LoadSavedMCPServers(s.ctx, mcpManager); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to load saved MCP servers")
	} else {
		s.logger.Info().Msg("Loaded saved MCP servers from config")
	}

	// Initialize session manager
	sessionManager := scheduler.NewSessionManager(db, 100)

	// Initialize hooks system
	hookManager := hooks.NewManager()

	// Initialize policy system
	policyConfig := policy.DefaultConfig()
	policyExecutor := policy.NewPolicyExecutor(&policyConfig.ToolPolicy)

	// Initialize approval manager
	hubAdapter := &hubBroadcaster{hub: hub}
	approvalNotifier := approval.NewNotifier(hubAdapter)
	approvalManager := approval.NewManager(&approval.ManagerConfig{
		Notifier:   approvalNotifier,
		Timeout:    300 * time.Second,
		MaxPending: policyConfig.Approval.MaxPending,
	})

	// Initialize runner
	runnerConfig := runner.Config{
		MaxIterations: 1000, // Reasonable limit to prevent quota exhaustion
		MaxTokens:     maxTokens,
		MaxMessages:   200, // Increased for complex tasks
		StreamOutput:  true,
		Timeout:       30 * time.Minute, // 30 minute timeout
	}
	agentRunner := runner.NewRunner(defaultProvider, toolRegistry, sessionManager, runnerConfig)
	agentRunner.SetMultiProviderPool(multiPool) // Enable multi-provider support
	agentRunner.SetHookManager(hookManager)
	agentRunner.SetPolicyExecutor(policyExecutor)
	agentRunner.SetApprovalManager(approvalManager)

	// Initialize system prompt builder with MCP support
	promptConfig := prompt.PromptConfig{
		AgentName: "Mote",
		Timezone:  "Local",
	}
	systemPromptBuilder := prompt.NewSystemPromptBuilder(promptConfig, toolRegistry).
		WithMCPManager(mcpManager)
	agentRunner.SetSystemPrompt(systemPromptBuilder)

	// Initialize compactor
	// The compactor's default provider is used as a fallback only. At runtime,
	// CompactWithFallback receives the session's active provider so that
	// compaction uses the same provider/model as the current conversation.
	compactorConfig := compaction.DefaultConfig()
	compactor := compaction.NewCompactor(compactorConfig, defaultProvider)
	agentRunner.SetCompactor(compactor)

	// Initialize Context Manager (enhanced M04 compression with persistence)
	// Memory will be wired later in initializeMemory
	contextConfig := internalContext.DefaultConfig()
	contextManager := internalContext.NewManager(s.db, compactor, nil, contextConfig)
	agentRunner.SetContextManager(contextManager)

	// Initialize skills system
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".mote", "skills")
	if err := skills.EnsureBuiltinSkills(skillsDir); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to install builtin skills")
	}

	// Initialize version checker for builtin skills
	embedFS := defaults.GetDefaultsFS()
	versionChecker := skills.NewVersionChecker(embedFS, s.logger)

	// Check for updates on startup
	result, err := versionChecker.CheckAllVersions(skillsDir)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to check builtin skill versions")
	} else if len(result.UpdatesAvailable) > 0 {
		s.logger.Info().
			Int("count", len(result.UpdatesAvailable)).
			Msg("Builtin skill updates available")
		for _, info := range result.UpdatesAvailable {
			s.logger.Info().
				Str("skill", info.SkillID).
				Str("current", info.LocalVersion).
				Str("latest", info.EmbedVersion).
				Msg("Update available for builtin skill")
		}
	}

	skillManager := skills.NewManager(skills.ManagerConfig{SkillsDir: skillsDir})
	skillManager.SetToolRegistry(toolRegistry)
	skillManager.SetJSRuntime(jsvmRuntime)
	skillManager.SetHookManager(hookManager)
	if err := skillManager.ScanDirectory(skillsDir); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to scan skills directory")
	}
	// Auto-activate all scanned skills
	allSkills := skillManager.ListSkills()
	for _, status := range allSkills {
		if status.Skill != nil {
			if err := skillManager.Activate(status.Skill.ID, nil); err != nil {
				s.logger.Debug().Str("skill", status.Skill.ID).Err(err).Msg("Failed to auto-activate skill")
			} else {
				s.logger.Info().Str("skill", status.Skill.ID).Msg("Auto-activated skill")
			}
		}
	}

	// Create skill updater
	skillUpdater := skills.NewSkillUpdater(embedFS, skillsDir, versionChecker, skillManager, s.logger)

	agentRunner.SetSkillManager(skillManager)
	s.skillManager = skillManager

	// Initialize channel system
	if err := agentRunner.InitChannels(s.cfg.Channels); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to initialize channels")
	} else {
		if registry := agentRunner.ChannelRegistry(); registry != nil {
			s.gatewayServer.SetChannelRegistry(registry)
			s.logger.Info().Int("channels", registry.Count()).Msg("Channel system initialized")
		}
	}

	// Initialize Memory system
	s.initializeMemory(db, agentRunner, s.gatewayServer, hookManager, contextManager)

	// Set dependencies on gateway
	s.gatewayServer.SetAgentRunner(agentRunner)
	s.gatewayServer.SetToolRegistry(toolRegistry)
	s.gatewayServer.SetMCPClient(mcpManager)
	s.gatewayServer.SetPolicyExecutor(policyExecutor)
	s.gatewayServer.SetApprovalManager(approvalManager)
	s.gatewayServer.SetSkillManager(skillManager)
	s.gatewayServer.SetVersionChecker(versionChecker)
	s.gatewayServer.SetSkillUpdater(skillUpdater)

	// Initialize Workspace Manager
	workspaceManager := workspace.NewWorkspaceManager()
	s.workspaceManager = workspaceManager
	s.gatewayServer.SetWorkspaceManager(workspaceManager)

	// Initialize Prompt Manager with file loading support
	promptsDirs := []string{}

	// Add user-level prompts directory
	if homeDir != "" {
		userPromptsDir := filepath.Join(homeDir, ".mote", "prompts")
		promptsDirs = append(promptsDirs, userPromptsDir)
	}

	// Add workspace-level prompts directory
	workspacePromptsDir := filepath.Join(".", ".mote", "prompts")
	promptsDirs = append(promptsDirs, workspacePromptsDir)

	promptManager := prompts.NewManagerWithConfig(prompts.ManagerConfig{
		PromptsDirs:    promptsDirs,
		EnableAutoSave: true,
	})
	s.gatewayServer.SetPromptManager(promptManager)

	// Initialize Cron system
	s.initializeCron(db, agentRunner, toolRegistry, jsvmRuntime, cronModel)

	// Initialize routes
	s.gatewayServer.InitializeRoutes()

	// Mark as running before starting
	s.mu.Lock()
	s.running = true
	s.startedAt = time.Now()
	s.mu.Unlock()

	if s.onStateChange != nil {
		s.onStateChange(true)
	}

	s.logger.Info().
		Str("address", fmt.Sprintf("http://%s:%d", s.cfg.Gateway.Host, s.cfg.Gateway.Port)).
		Msg("🚀 Embedded mote server started")

	// Start server (blocking)
	if err := s.gatewayServer.Start(); err != nil {
		s.logger.Error().Err(err).Msg("Server error")
		s.errChan <- err
	}

	// Server stopped
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.onStateChange != nil {
		s.onStateChange(false)
	}
}

// Stop stops the embedded server.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.logger.Info().Msg("Stopping embedded server...")
	s.cancel()

	if s.gatewayServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.gatewayServer.Shutdown(ctx); err != nil {
			s.logger.Error().Err(err).Msg("Error during server shutdown")
		}
	}

	if s.db != nil {
		s.db.Close()
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.onStateChange != nil {
		s.onStateChange(false)
	}

	s.logger.Info().Msg("Embedded server stopped")
	return nil
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// IsReady checks if the server is ready to accept connections.
func (s *Server) IsReady() bool {
	if !s.IsRunning() {
		return false
	}
	// Try health check
	return s.gatewayServer != nil && s.gatewayServer.IsReady()
}

// ReloadProviders reinitializes providers based on current configuration.
// This allows hot-reloading of provider settings without restarting the server.
func (s *Server) ReloadProviders() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server is not running")
	}

	s.logger.Info().Msg("Reloading providers...")

	// Reload configuration from disk
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	// Unmarshal updated config
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	s.cfg = &cfg

	// Get enabled providers from configuration
	enabledProviders := s.cfg.Provider.GetEnabledProviders()
	s.logger.Info().Strs("enabled_providers", enabledProviders).Msg("Reinitializing providers")

	// Track which providers should be enabled
	shouldEnable := make(map[string]bool)
	for _, p := range enabledProviders {
		shouldEnable[p] = true
	}

	// Remove providers that are no longer enabled
	for _, providerName := range s.multiPool.ListProviders() {
		if !shouldEnable[providerName] {
			if err := s.multiPool.RemoveProvider(providerName); err != nil {
				s.logger.Warn().Err(err).Str("provider", providerName).Msg("Failed to remove provider")
			} else {
				s.logger.Info().Str("provider", providerName).Msg("Provider removed")
			}
		}
	}

	// Initialize or update enabled providers
	for _, providerName := range enabledProviders {
		switch providerName {
		case "ollama":
			if err := s.reloadOllamaProvider(); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to reload Ollama provider")
			}
		case "copilot":
			if err := s.reloadCopilotProvider(); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to reload Copilot API provider")
			}
		case "copilot-acp":
			if err := s.reloadCopilotACPProvider(); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to reload Copilot ACP provider")
			}
		case "minimax":
			if err := s.reloadMinimaxProvider(); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to reload MiniMax provider")
			}
		}
	}

	s.logger.Info().Msg("Providers reloaded successfully")
	return nil
}

// reloadOllamaProvider reinitializes the Ollama provider.
func (s *Server) reloadOllamaProvider() error {
	ollamaCfg := ollama.Config{
		Endpoint:  s.cfg.Ollama.Endpoint,
		Model:     s.cfg.Ollama.Model,
		KeepAlive: s.cfg.Ollama.KeepAlive,
	}
	if ollamaCfg.Endpoint == "" {
		ollamaCfg.Endpoint = ollama.DefaultEndpoint
	}
	if ollamaCfg.Model == "" {
		ollamaCfg.Model = ollama.DefaultModel
	}
	if s.cfg.Ollama.Timeout != "" {
		if d, err := time.ParseDuration(s.cfg.Ollama.Timeout); err == nil {
			ollamaCfg.Timeout = d
		}
	}
	if ollamaCfg.Timeout == 0 {
		ollamaCfg.Timeout = ollama.DefaultTimeout
	}
	if ollamaCfg.KeepAlive == "" {
		ollamaCfg.KeepAlive = ollama.DefaultKeepAlive
	}

	ollamaFactory := func(model string) (provider.Provider, error) {
		cfg := ollamaCfg
		if model != "" {
			cfg.Model = model
		}
		return ollama.NewOllamaProvider(cfg), nil
	}
	ollamaPool := provider.NewPool(ollamaFactory)

	// Get Ollama model list
	ollamaModels := []string{ollamaCfg.Model}
	if ollamaProvider := ollama.NewOllamaProvider(ollamaCfg); ollamaProvider != nil {
		if modelList := ollamaProvider.Models(); len(modelList) > 0 {
			ollamaModels = modelList
		}
	}

	if err := s.multiPool.UpdateProvider("ollama", ollamaPool, ollamaModels); err != nil {
		return fmt.Errorf("failed to update Ollama provider: %w", err)
	}

	s.logger.Info().
		Str("provider", "ollama").
		Str("endpoint", ollamaCfg.Endpoint).
		Int("models", len(ollamaModels)).
		Msg("Ollama provider reloaded")

	return nil
}

// reloadCopilotProvider reinitializes the Copilot API provider (free models).
func (s *Server) reloadCopilotProvider() error {
	githubToken := s.cfg.Copilot.Token
	if githubToken == "" {
		return fmt.Errorf("GitHub token not configured")
	}

	maxTokens := s.cfg.Copilot.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	copilotFactory := copilot.Factory(githubToken, maxTokens)
	copilotPool := provider.NewPool(copilotFactory)
	copilotModels := copilot.ListModels()

	if err := s.multiPool.UpdateProvider("copilot", copilotPool, copilotModels); err != nil {
		return fmt.Errorf("failed to update Copilot API provider: %w", err)
	}

	s.logger.Info().
		Str("provider", "copilot").
		Int("models", len(copilotModels)).
		Msg("Copilot API provider reloaded")

	return nil
}

// reloadCopilotACPProvider reinitializes the Copilot ACP provider (premium models).
func (s *Server) reloadCopilotACPProvider() error {
	acpCfg := s.buildACPConfig()
	acpFactory := copilot.ACPFactory(acpCfg)
	acpPool := provider.NewPool(acpFactory)
	acpModels := copilot.ACPListModels()

	if err := s.multiPool.UpdateProvider("copilot-acp", acpPool, acpModels); err != nil {
		// If copilot-acp doesn't exist yet, add it
		if addErr := s.multiPool.AddProvider("copilot-acp", acpPool, acpModels); addErr != nil {
			return fmt.Errorf("failed to add Copilot ACP provider: %w", addErr)
		}
	}

	s.logger.Info().
		Str("provider", "copilot-acp").
		Int("models", len(acpModels)).
		Msg("Copilot ACP provider reloaded")

	return nil
}

// reloadMinimaxProvider reinitializes the MiniMax provider.
func (s *Server) reloadMinimaxProvider() error {
	apiKey := s.cfg.Minimax.APIKey
	if apiKey == "" {
		return fmt.Errorf("MiniMax API key not configured")
	}

	mmMaxTokens := s.cfg.Minimax.MaxTokens
	if mmMaxTokens <= 0 {
		mmMaxTokens = 4096
	}

	minimaxFactory := minimax.Factory(apiKey, mmMaxTokens)
	minimaxPool := provider.NewPool(minimaxFactory)
	minimaxModels := minimax.ListModels()

	if err := s.multiPool.UpdateProvider("minimax", minimaxPool, minimaxModels); err != nil {
		// If minimax doesn't exist yet, add it
		if addErr := s.multiPool.AddProvider("minimax", minimaxPool, minimaxModels); addErr != nil {
			return fmt.Errorf("failed to add MiniMax provider: %w", addErr)
		}
	}

	s.logger.Info().
		Str("provider", "minimax").
		Int("models", len(minimaxModels)).
		Msg("MiniMax provider reloaded")

	return nil
}

// GetStartedAt returns when the server started.
func (s *Server) GetStartedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt
}

// getStoragePath returns the storage path.
func (s *Server) getStoragePath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".mote", "data.db")
}

// getConfigPath returns the config path.
func (s *Server) getConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".mote", "config.yaml")
}

// buildACPConfig constructs ACPConfig with MCP servers, system message, and working directory.
func (s *Server) buildACPConfig() copilot.ACPConfig {
	cfg := copilot.ACPConfig{
		// Always allow all tools in ACP mode. Mote has its own security
		// policy layer (internal/policy) that controls tool access before
		// messages reach the provider. Without this flag, the CLI blocks
		// waiting for interactive permission confirmation.
		AllowAllTools: true,
	}

	// Load MCP servers config (non-blocking: failure uses empty map)
	var mcpServerNames []string
	if mcpServers, err := v1.LoadMCPServersConfigPublic(); err == nil && len(mcpServers) > 0 {
		persistInfos := make([]copilot.MCPServerPersistInfo, len(mcpServers))
		for i, srv := range mcpServers {
			persistInfos[i] = copilot.MCPServerPersistInfo{
				Name:    srv.Name,
				Type:    srv.Type,
				URL:     srv.URL,
				Headers: srv.Headers,
				Command: srv.Command,
				Args:    srv.Args,
			}
			mcpServerNames = append(mcpServerNames, srv.Name)
		}
		cfg.MCPServers = copilot.ConvertMCPServers(persistInfos)
		s.logger.Info().Int("count", len(cfg.MCPServers)).Msg("ACP: loaded MCP servers")
	}

	// Build system message from config, including MCP server info and skills prompts
	extraPrompt := viper.GetString("prompt.extra")
	workspaceRules := viper.GetString("prompt.workspace_rules")

	// Build skills prompts from skill manager
	var skillsPrompt string
	if s.skillManager != nil {
		skillsSection := skills.NewPromptSection(s.skillManager)
		if section := skillsSection.Build(); section != "" {
			skillsPrompt += section
		}
		if activePrompts := skillsSection.BuildActivePrompts(); activePrompts != "" {
			skillsPrompt += "\n" + activePrompts
		}
	}

	cfg.SystemMessage = copilot.BuildACPSystemMessage(extraPrompt, workspaceRules, mcpServerNames, skillsPrompt)

	// Inject tool registry for ACP ToolBridge
	if s.toolRegistry != nil {
		cfg.ToolRegistry = &acpToolRegistryAdapter{registry: s.toolRegistry}
	}

	// Set workspace resolver to get session bound workspace path
	// This is a closure that captures s, so it can access s.workspaceManager
	// at call time (when ACP session is created), not at config build time.
	cfg.WorkspaceResolver = func(sessionID string) string {
		if s.workspaceManager == nil {
			return ""
		}
		if binding, ok := s.workspaceManager.Get(sessionID); ok && binding != nil {
			return binding.Path
		}
		return ""
	}

	// Set skills directories for CLI skill loading
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		skillsDir := filepath.Join(homeDir, ".mote", "skills")
		cfg.SkillDirectories = []string{skillsDir}
	}

	// Pass GitHub token for authentication (only if copilot REST API provider is enabled).
	// NOTE: When only copilot-acp is enabled, do NOT inject the old REST API token
	// as GITHUB_TOKEN, because it would override the CLI's own OAuth authentication
	// (from `copilot login`) and cause 403 errors if the token is stale/invalid.
	enabledProviders := s.cfg.Provider.GetEnabledProviders()
	copilotAPIEnabled := false
	for _, p := range enabledProviders {
		if p == "copilot" {
			copilotAPIEnabled = true
			break
		}
	}
	if copilotAPIEnabled && s.cfg.Copilot.Token != "" {
		cfg.GithubToken = s.cfg.Copilot.Token
	}

	return cfg
}

// reloadConfig reloads the configuration from disk.
func (s *Server) reloadConfig() error {
	configPath := s.getConfigPath()

	// Reset viper to force re-reading the config file
	config.Reset()

	newCfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Preserve defaults
	if newCfg.Gateway.Port == 0 {
		newCfg.Gateway.Port = 18788
	}
	if newCfg.Gateway.Host == "" {
		newCfg.Gateway.Host = "localhost"
	}

	s.cfg = newCfg
	s.logger.Info().
		Str("copilot_token_set", fmt.Sprintf("%v", newCfg.Copilot.Token != "")).
		Msg("Configuration reloaded")
	return nil
}

// initializeMemory initializes the memory subsystem.
func (s *Server) initializeMemory(db *storage.DB, agentRunner *runner.Runner, gatewayServer *gateway.Server, hookManager *hooks.Manager, contextManager *internalContext.Manager) {
	memoryEmbedder := memory.NewSimpleEmbedder(384)

	homeDir, _ := os.UserHomeDir()
	moteDir := filepath.Join(homeDir, ".mote")

	// Create MemoryManager (the new unified entry point)
	memoryManager, err := memory.NewMemoryManager(
		db.DB,
		memoryEmbedder,
		memory.ManagerConfig{
			BaseDir:       moteDir,
			EnableWatch:   true,
			EnableCapture: s.cfg.Memory.AutoCapture.Enabled,
			CaptureMode:   memory.CaptureModeLLMSummary,
			IndexConfig: memory.IndexConfig{
				Dimensions:     384,
				EnableFTS:      true,
				EnableVec:      true,
				ChunkThreshold: 2000,
			},
			HybridConfig: memory.DefaultHybridConfig(),
			BM25Config:   memory.DefaultBM25Config(),
			BatchConfig:  memory.DefaultBatchConfig(),
		},
		s.logger,
	)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to initialize memory manager")
		return
	}

	// Run initial sync (non-blocking)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if initErr := memoryManager.Init(ctx); initErr != nil {
			s.logger.Warn().Err(initErr).Msg("Memory manager init failed")
		}
	}()

	// Get the legacy MemoryIndex from IndexManager for backward compatibility
	memoryIndex := memoryManager.GetIndexManager().GetLegacyIndex()

	// Inject dependencies: legacy MemoryIndex for backward compat, MemoryManager for new features
	agentRunner.SetMemory(memoryIndex)
	gatewayServer.SetMemoryIndex(memoryIndex)
	gatewayServer.SetMemoryManager(memoryManager)
	if contextManager != nil {
		contextManager.SetMemory(memoryIndex)
	}

	// Initialize auto-capture and auto-recall if enabled
	if s.cfg.Memory.AutoCapture.Enabled || s.cfg.Memory.AutoRecall.Enabled {
		categoryDetector, err := memory.NewCategoryDetector()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to create category detector")
			return
		}

		var captureEngine *memory.CaptureEngine
		if s.cfg.Memory.AutoCapture.Enabled && categoryDetector != nil {
			captureConfig := memory.CaptureConfig{
				Enabled:       true,
				MinLength:     s.cfg.Memory.AutoCapture.MinLength,
				MaxLength:     s.cfg.Memory.AutoCapture.MaxLength,
				DupThreshold:  s.cfg.Memory.AutoCapture.DupThreshold,
				MaxPerSession: s.cfg.Memory.AutoCapture.MaxPerSession,
			}
			if captureConfig.MinLength <= 0 {
				captureConfig.MinLength = 10
			}
			if captureConfig.MaxLength <= 0 {
				captureConfig.MaxLength = 500
			}
			if captureConfig.DupThreshold <= 0 {
				captureConfig.DupThreshold = 0.95
			}
			if captureConfig.MaxPerSession <= 0 {
				captureConfig.MaxPerSession = 3
			}
			captureEngine, _ = memory.NewCaptureEngine(memory.CaptureEngineOptions{
				Memory:   memoryIndex,
				Detector: categoryDetector,
				Config:   captureConfig,
				Logger:   s.logger,
			})
		}

		var recallEngine *memory.RecallEngine
		if s.cfg.Memory.AutoRecall.Enabled {
			recallConfig := memory.RecallConfig{
				Enabled:      true,
				Limit:        s.cfg.Memory.AutoRecall.Limit,
				Threshold:    s.cfg.Memory.AutoRecall.Threshold,
				MinPromptLen: s.cfg.Memory.AutoRecall.MinPromptLen,
			}
			if recallConfig.Limit <= 0 {
				recallConfig.Limit = 3
			}
			if recallConfig.Threshold <= 0 {
				recallConfig.Threshold = 0.3
			}
			if recallConfig.MinPromptLen <= 0 {
				recallConfig.MinPromptLen = 5
			}
			recallEngine = memory.NewRecallEngine(memory.RecallEngineOptions{
				Memory: memoryIndex,
				Config: recallConfig,
				Logger: s.logger,
			})
		}

		if captureEngine != nil || recallEngine != nil {
			memBridge := hooksbuiltin.NewMemoryHookBridge(hooksbuiltin.MemoryHookConfig{
				CaptureEngine: captureEngine,
				RecallEngine:  recallEngine,
				Logger:        s.logger,
			})
			if recallEngine != nil {
				_ = hookManager.Register(hooks.HookBeforeMessage, memBridge.BeforeMessageHandler("memory-recall"))
			}
			if captureEngine != nil {
				_ = hookManager.Register(hooks.HookAfterMessage, memBridge.AfterMessageHandler("memory-capture"))
				_ = hookManager.Register(hooks.HookSessionCreate, memBridge.SessionCreateHandler("memory-session"))
			}
		}
	}
}

// initializeCron initializes the cron scheduler.
func (s *Server) initializeCron(db *storage.DB, agentRunner *runner.Runner, toolRegistry *tools.Registry, jsvmRuntime *jsvm.Runtime, cronModel string) {
	cronJobStore := cron.NewJobStore(db.DB)
	cronHistoryStore := cron.NewHistoryStore(db.DB)

	jsExecutor := &jsvmAdapter{runtime: jsvmRuntime}
	cronExecutor := cron.NewExecutor(
		&cronRunnerAdapter{runner: agentRunner, cronModel: cronModel},
		&cronToolRegistryAdapter{registry: toolRegistry},
		jsExecutor,
		cronHistoryStore,
		s.workspaceManager,
		cron.DefaultExecutorConfig(),
		s.logger,
	)

	s.cronScheduler = cron.NewScheduler(
		cronJobStore,
		cronHistoryStore,
		cronExecutor,
		nil,
		nil,
	)

	if err := s.cronScheduler.Start(s.ctx); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to start cron scheduler")
	} else {
		s.gatewayServer.SetCronScheduler(s.cronScheduler)
	}
}

// hubBroadcaster adapts websocket.Hub to approval.Broadcaster interface.
type hubBroadcaster struct {
	hub *websocket.Hub
}

func (b *hubBroadcaster) BroadcastAll(messageType string, data any) error {
	return b.hub.BroadcastTyped(messageType, data)
}

// jsvmAdapter adapts jsvm.Runtime to the cron.JSExecutor interface.
type jsvmAdapter struct {
	runtime *jsvm.Runtime
}

func (a *jsvmAdapter) Execute(ctx context.Context, script, scriptName, executionID string) (interface{}, error) {
	result, err := a.runtime.Execute(ctx, script, scriptName, executionID)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func (a *jsvmAdapter) ExecuteFile(ctx context.Context, filePath, executionID string) (interface{}, error) {
	result, err := a.runtime.ExecuteFile(ctx, filePath, executionID)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// cronRunnerAdapter adapts runner.Runner to cron.Runner interface.
type cronRunnerAdapter struct {
	runner    *runner.Runner
	cronModel string // Model to use for cron jobs
}

func (a *cronRunnerAdapter) Run(ctx context.Context, prompt string, opts ...interface{}) (string, error) {
	// Extract session ID and optional per-job model from opts.
	// opts[0] = sessionID (string), opts[1] = model (string, optional)
	var sessionID string
	var jobModel string
	if len(opts) > 0 {
		if id, ok := opts[0].(string); ok {
			sessionID = id
		}
	}
	if len(opts) > 1 {
		if m, ok := opts[1].(string); ok {
			jobModel = m
		}
	}
	if sessionID == "" {
		sessionID = "cron-job:unknown"
	}

	// Use per-job model if specified, otherwise fall back to global cron model
	effectiveModel := a.cronModel
	if jobModel != "" {
		effectiveModel = jobModel
	}

	// Run the prompt with the effective model
	events, err := a.runner.RunWithModel(ctx, sessionID, prompt, effectiveModel, "cron")
	if err != nil {
		return "", err
	}

	// Collect the response, but also watch for context cancellation.
	// Without this select, a stuck tool execution would block forever
	// even after the cron timeout fires.
	var result strings.Builder
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return result.String(), nil
			}
			switch event.Type {
			case runner.EventTypeContent:
				result.WriteString(event.Content)
			case runner.EventTypeError:
				if event.Error != nil {
					return "", fmt.Errorf("agent error: %v", event.Error)
				}
				return "", fmt.Errorf("agent error: %s", event.ErrorMsg)
			}
		case <-ctx.Done():
			// Context cancelled (cron timeout). Cancel the session's RunQueue
			// to abort stuck tool execution and free the worker for next run.
			a.runner.CancelSession(sessionID)
			return "", ctx.Err()
		}
	}
}

// CancelSession implements cron.CancellableRunner.
func (a *cronRunnerAdapter) CancelSession(sessionID string) {
	a.runner.CancelSession(sessionID)
}

// acpToolRegistryAdapter adapts tools.Registry to copilot.ToolRegistryInterface.
type acpToolRegistryAdapter struct {
	registry *tools.Registry
}

func (a *acpToolRegistryAdapter) ListToolInfo() []copilot.ToolInfo {
	registered := a.registry.List()
	infos := make([]copilot.ToolInfo, len(registered))
	for i, t := range registered {
		infos[i] = copilot.ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		}
	}
	return infos
}

func (a *acpToolRegistryAdapter) ExecuteTool(ctx context.Context, name string, args map[string]any) (copilot.ToolExecResult, error) {
	result, err := a.registry.Execute(ctx, name, args)
	if err != nil {
		return copilot.ToolExecResult{}, err
	}
	return copilot.ToolExecResult{
		Content: result.Content,
		IsError: result.IsError,
	}, nil
}

// cronToolRegistryAdapter adapts tools.Registry to cron.ToolRegistry interface.
type cronToolRegistryAdapter struct {
	registry *tools.Registry
}

func (a *cronToolRegistryAdapter) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	result, err := a.registry.Execute(ctx, name, args)
	if err != nil {
		return nil, err
	}
	return result.Content, nil
}
