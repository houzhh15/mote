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
	"mote/internal/compaction"
	"mote/internal/config"
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
	cfg           *config.Config
	logger        zerolog.Logger
	gatewayServer *gateway.Server
	cronScheduler *cron.Scheduler
	db            *storage.DB
	multiPool     *provider.MultiProviderPool // Provider pool for hot reload
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
	mu            sync.RWMutex
	startedAt     time.Time
	errChan       chan error
	onStateChange func(bool)
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
			// Initialize Copilot provider pool
			githubToken := s.cfg.Copilot.Token
			if githubToken == "" {
				s.logger.Warn().Msg("GitHub token not configured, skipping Copilot provider")
				continue
			}

			maxTokens = s.cfg.Copilot.MaxTokens
			if maxTokens <= 0 {
				maxTokens = 4096
			}

			// Create provider pool with factory
			copilotFactory := copilot.Factory(githubToken, maxTokens)
			copilotPool := provider.NewPool(copilotFactory)

			// Get Copilot model list
			copilotModels := copilot.ListModels()

			if err := multiPool.AddProvider("copilot", copilotPool, copilotModels); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to add Copilot provider")
			} else {
				s.logger.Info().
					Str("provider", "copilot").
					Int("models", len(copilotModels)).
					Msg("Copilot provider initialized")
			}
		}
	}

	// Set multiPool on gateway for /api/v1/models endpoint
	s.gatewayServer.SetMultiPool(multiPool)

	// Save multiPool reference for hot reload
	s.multiPool = multiPool

	// Set embedded server reference on gateway for hot reload support
	s.gatewayServer.SetEmbeddedServer(s)

	// Determine default provider and model
	defaultProviderName := s.cfg.Provider.Default
	if defaultProviderName == "" {
		defaultProviderName = "copilot"
	}

	// Get default chat model
	chatModel = s.cfg.Copilot.Model
	if chatModel == "" {
		chatModel = copilot.DefaultModel
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
		MaxIterations: 10000, // Increased for complex tasks
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
	compactorConfig := compaction.DefaultConfig()
	compactor := compaction.NewCompactor(compactorConfig, defaultProvider)
	agentRunner.SetCompactor(compactor)

	// Initialize skills system
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".mote", "skills")
	if err := skills.EnsureBuiltinSkills(skillsDir); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to install builtin skills")
	}
	skillManager := skills.NewManager(skills.ManagerConfig{SkillsDir: skillsDir})
	skillManager.SetToolRegistry(toolRegistry)
	skillManager.SetJSRuntime(jsvmRuntime)
	skillManager.SetHookManager(hookManager)
	if err := skillManager.ScanDirectory(skillsDir); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to scan skills directory")
	}
	for _, skillID := range []string{"mote-mcp-config", "mote-self", "mote-memory", "mote-cron"} {
		if err := skillManager.Activate(skillID, nil); err != nil {
			s.logger.Debug().Str("skill", skillID).Err(err).Msg("Failed to auto-activate skill")
		}
	}
	agentRunner.SetSkillManager(skillManager)

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
	s.initializeMemory(db, agentRunner, s.gatewayServer, hookManager)

	// Set dependencies on gateway
	s.gatewayServer.SetAgentRunner(agentRunner)
	s.gatewayServer.SetToolRegistry(toolRegistry)
	s.gatewayServer.SetMCPClient(mcpManager)
	s.gatewayServer.SetPolicyExecutor(policyExecutor)
	s.gatewayServer.SetApprovalManager(approvalManager)
	s.gatewayServer.SetSkillManager(skillManager)

	// Initialize Workspace Manager
	workspaceManager := workspace.NewWorkspaceManager()
	s.gatewayServer.SetWorkspaceManager(workspaceManager)

	// Initialize Prompt Manager
	promptManager := prompts.NewManager()
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
				s.logger.Warn().Err(err).Msg("Failed to reload Copilot provider")
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

// reloadCopilotProvider reinitializes the Copilot provider.
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
		return fmt.Errorf("failed to update Copilot provider: %w", err)
	}

	s.logger.Info().
		Str("provider", "copilot").
		Int("models", len(copilotModels)).
		Msg("Copilot provider reloaded")

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
func (s *Server) initializeMemory(db *storage.DB, agentRunner *runner.Runner, gatewayServer *gateway.Server, hookManager *hooks.Manager) {
	memoryEmbedder := memory.NewSimpleEmbedder(384)
	memoryIndex, err := memory.NewMemoryIndex(db.DB, memoryEmbedder, memory.IndexConfig{
		Dimensions:     384,
		EnableFTS:      true,
		EnableVec:      true, // Enable vector search
		ChunkThreshold: 2000, // Auto-chunk content > 2000 chars
	})
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to initialize memory index")
		return
	}

	// Initialize MarkdownStore
	homeDir, _ := os.UserHomeDir()
	moteDir := filepath.Join(homeDir, ".mote")
	markdownStore, err := memory.NewMarkdownStore(memory.MarkdownStoreOptions{
		BaseDir: moteDir,
		Logger:  s.logger,
	})
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to initialize markdown store")
	} else {
		memoryIndex.SetMarkdownStore(markdownStore)
	}

	agentRunner.SetMemory(memoryIndex)
	gatewayServer.SetMemoryIndex(memoryIndex)

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
			memHooks := memory.NewMemoryHooks(memory.MemoryHooksOptions{
				Capture: captureEngine,
				Recall:  recallEngine,
				Logger:  s.logger,
			})
			memBridge := hooksbuiltin.NewMemoryHookBridge(hooksbuiltin.MemoryHookConfig{
				MemoryHooks: memHooks,
				Logger:      s.logger,
			})
			if recallEngine != nil {
				hookManager.Register(hooks.HookBeforeMessage, memBridge.BeforeMessageHandler("memory-recall"))
			}
			if captureEngine != nil {
				hookManager.Register(hooks.HookAfterMessage, memBridge.AfterMessageHandler("memory-capture"))
				hookManager.Register(hooks.HookSessionCreate, memBridge.SessionCreateHandler("memory-session"))
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
	// Extract job name from opts for session sharing
	var jobName string
	if len(opts) > 0 {
		if name, ok := opts[0].(string); ok {
			jobName = name
		}
	}
	if jobName == "" {
		jobName = "unknown"
	}

	// Use job name as session ID - same job shares context across executions
	sessionID := fmt.Sprintf("cron-job:%s", jobName)

	// Run the prompt with cron-specific model
	events, err := a.runner.RunWithModel(ctx, sessionID, prompt, a.cronModel, "cron")
	if err != nil {
		return "", err
	}

	// Collect the response
	var result strings.Builder
	for event := range events {
		switch event.Type {
		case runner.EventTypeContent:
			result.WriteString(event.Content)
		case runner.EventTypeError:
			if event.Error != nil {
				return "", fmt.Errorf("agent error: %v", event.Error)
			}
			return "", fmt.Errorf("agent error: %s", event.ErrorMsg)
		}
	}

	return result.String(), nil
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
