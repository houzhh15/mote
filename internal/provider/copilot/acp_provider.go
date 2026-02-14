package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// sessionEntryCounter is an atomic counter used to generate unique IDs for
// sessionEventEntry instances. This ensures that when a Stream goroutine's defer
// cleanup runs, it only deletes its own entry and not a newer one registered by
// a subsequent Stream goroutine for the same conversationID.
var sessionEntryCounter int64

// PauseControllerInterface defines minimal pause control interface needed by ACPProvider.
// This avoids circular dependency between runner and provider packages.
type PauseControllerInterface interface {
	IsPaused(sessionID string) bool
	WaitForResume(ctx context.Context, sessionID string, toolName string, toolCallID string) (userInput string, wasPaused bool, timedOut bool)
}

// ACPProvider implements the Provider interface using ACP protocol.
// This provider communicates with Copilot CLI via JSON-RPC 2.0 over stdio.
//
// Key advantage: ACP uses per-prompt billing, so multiple tool calls
// within a single prompt only consume one premium request.
type ACPProvider struct {
	client       *ACPClient
	config       ACPConfig
	sessionID    string // Current ACP session ID
	sessionModel string // Model used for current session
	toolBridge   *ToolBridge
	// Session mapping: mote ConversationID → ACP sessionID
	sessionMap sync.Map
	// Reverse session mapping: ACP sessionID → mote ConversationID
	// Used by tool call handler to route events to correct conversation
	reverseSessionMap sync.Map
	// Session model mapping: conversationID → model used for that session
	sessionModelMap sync.Map
	// sessionEvents: conversationID → *sessionEventEntry
	// Per-conversation event channel isolation for concurrent sessions
	sessionEvents sync.Map

	mu          sync.Mutex
	initialized bool
	restarting  atomic.Bool // Flag to prevent concurrent initialization during restart

	// Pause controller for ACP mode (injected by Runner)
	pauseController PauseControllerInterface
	pauseMu         sync.RWMutex
}

// sessionEventEntry holds the event channel and metadata for a conversation session.
type sessionEventEntry struct {
	events    chan<- provider.ChatEvent
	ctx       context.Context
	createdAt time.Time
	id        int64 // Unique ID to distinguish entries for the same convID
}

// safeSendEvent safely sends an event to the specified conversation's events channel.
// Returns false if the channel is no longer active (stream has ended or not found).
func (p *ACPProvider) safeSendEvent(convID string, event provider.ChatEvent) bool {
	entry, ok := p.sessionEvents.Load(convID)
	if !ok {
		logger.Warn().Str("convID", convID).Msg("Session events channel not found")
		return false
	}

	se := entry.(*sessionEventEntry)

	// Check if context is cancelled
	select {
	case <-se.ctx.Done():
		return false
	default:
	}

	// Use select with default to avoid blocking on a potentially closed channel
	// This is a best-effort send - if the channel is full or closed, we skip
	defer func() {
		if r := recover(); r != nil {
			logger.Warn().Interface("panic", r).Str("convID", convID).Msg("Recovered from send on closed channel")
		}
	}()

	select {
	case se.events <- event:
		return true
	default:
		logger.Warn().Str("convID", convID).Msg("Events channel full, dropping event")
		return false
	}
}

// NewACPProvider creates a new ACP Provider.
func NewACPProvider(cfg ACPConfig) (*ACPProvider, error) {
	client, err := NewACPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACP client: %w", err)
	}

	p := &ACPProvider{
		client: client,
		config: cfg,
	}

	// Create ToolBridge if tool registry is available
	if cfg.ToolRegistry != nil {
		if len(cfg.ExcludeTools) > 0 {
			p.toolBridge = NewToolBridgeWithExcludes(cfg.ToolRegistry, cfg.ExcludeTools)
		} else {
			p.toolBridge = NewToolBridge(cfg.ToolRegistry)
		}
	}

	return p, nil
}

// Name returns the provider name.
func (p *ACPProvider) Name() string {
	return "copilot-acp"
}

// Models returns the list of supported ACP models.
// ACP has its own model registry, separate from the Copilot REST API models.
func (p *ACPProvider) Models() []string {
	return ACPListModels()
}

// ensureInitialized ensures the ACP connection is initialized.
func (p *ACPProvider) ensureInitialized(ctx context.Context) error {
	// Wait if CLI is currently restarting to avoid concurrent initialization
	maxWaitTime := 10 * time.Second
	waitStart := time.Now()
	for p.restarting.Load() {
		if time.Since(waitStart) > maxWaitTime {
			return fmt.Errorf("timeout waiting for CLI restart to complete")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Continue waiting
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// Check if client is closed
	if p.client.IsClosed() {
		logger.Info().Msg("ACP client is closed, recreating...")
		// Recreate the client
		client, err := NewACPClient(p.config)
		if err != nil {
			return fmt.Errorf("failed to recreate ACP client: %w", err)
		}
		p.client = client
	}

	// Initialize
	logger.Info().Msg("Initializing ACP client...")
	_, err := p.client.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("ACP initialize failed: %w", err)
	}

	// Register tool call handler if ToolBridge is available
	if p.toolBridge != nil {
		p.client.SetToolCallHandler(func(ctx context.Context, req ToolCallRequest) ToolCallResponse {
			// Find the conversationID from the ACP sessionID using reverse mapping
			convID := "default"
			if val, ok := p.reverseSessionMap.Load(req.SessionID); ok {
				convID = val.(string)
			}

			// Emit tool_call_start event for bridge tool
			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCall,
				ToolCall: &provider.ToolCall{
					ID:        req.ToolCallID,
					Name:      req.ToolName,
					Arguments: fmt.Sprintf("%v", req.Arguments),
				},
			})

			args, argErr := convertToolArgs(req.Arguments)
			if argErr != nil {
				logger.Warn().Err(argErr).Str("tool", req.ToolName).Str("convID", convID).Msg("Failed to convert tool args")
				return ToolCallResponse{Result: ToolResult{
					TextResultForLLM: "Error: " + argErr.Error(),
					ResultType:       "failure",
					Error:            argErr.Error(),
				}}
			}
			result, execErr := p.toolBridge.ExecuteTool(ctx, req.ToolName, args)

			// Emit tool_call_complete event
			p.safeSendEvent(convID, provider.ChatEvent{
				Type:  provider.EventTypeContent,
				Delta: fmt.Sprintf("\n[Tool %s completed]\n", req.ToolName),
			})

			if execErr != nil {
				logger.Warn().Err(execErr).Str("tool", req.ToolName).Str("convID", convID).Msg("Bridge tool execution failed")
			}
			return ToolCallResponse{Result: result}
		})
	}

	// Register hooks handler for preToolUse and other lifecycle hooks
	p.client.SetHooksHandler(func(ctx context.Context, req HooksInvokeRequest) (any, error) {
		return p.handleHooksInvoke(req)
	})

	// Register global notification handler that routes to correct session
	// This is set once during initialization and uses sessionId from notifications
	// to look up the correct convID via reverseSessionMap
	p.client.SetNotificationHandler(func(method string, params json.RawMessage) {
		p.handleNotification(method, params)
	})

	p.initialized = true
	return nil
}

// ensureSession ensures there's an active ACP session with the requested model.
// If conversationID is non-empty and a mapping exists, the ACP session is reused
// unless the model has changed (which requires a new session).
// Returns (isNewSession, error) where isNewSession indicates whether a new ACP
// session was created (as opposed to reusing an existing one). This is used to
// determine whether conversation history needs to be injected into the prompt.
func (p *ACPProvider) ensureSession(ctx context.Context, conversationID string, requestedModel string) (bool, error) {
	// Determine effective model: use requested model, or fall back to config default
	effectiveModel := requestedModel
	if effectiveModel == "" {
		effectiveModel = p.config.Model
	}

	// Validate model is ACP-compatible. API-only models (e.g., grok-code-fast-1)
	// cannot be used with the Copilot CLI and would cause startup/session errors.
	if effectiveModel != "" && !IsACPModel(effectiveModel) {
		slog.Warn("ensureSession: model not supported by ACP, falling back to default",
			"requestedModel", effectiveModel,
			"fallback", ACPDefaultModel)
		effectiveModel = ACPDefaultModel
	}

	// Check session mapping for conversation-aware reuse
	if conversationID != "" {
		if acpSID, ok := p.sessionMap.Load(conversationID); ok {
			// Check if model has changed
			if prevModel, ok := p.sessionModelMap.Load(conversationID); ok {
				if prevModel.(string) != effectiveModel {
					// Model changed, need to create new session
					slog.Info("ACP model changed, creating new session",
						"conversationID", conversationID,
						"oldModel", prevModel,
						"newModel", effectiveModel)
					p.sessionMap.Delete(conversationID)
					p.sessionModelMap.Delete(conversationID)
				} else {
					// Same model, reuse session
					p.sessionID = acpSID.(string)
					p.sessionModel = effectiveModel
					return false, nil
				}
			} else {
				// No model record, reuse session (backward compatibility)
				p.sessionID = acpSID.(string)
				p.sessionModel = effectiveModel
				return false, nil
			}
		}
	}

	// Fallback to current session if no conversation mapping
	if p.sessionID != "" && conversationID == "" {
		return false, nil
	}

	// Resolve working directory:
	// 1. Try WorkspaceResolver with conversationID (session bound workspace)
	// 2. Fall back to config.WorkingDirectory (static config)
	// 3. Fall back to os.Getwd() (process cwd)
	cwd := ""
	if p.config.WorkspaceResolver != nil && conversationID != "" {
		cwd = p.config.WorkspaceResolver(conversationID)
	}
	if cwd == "" {
		cwd = p.config.WorkingDirectory
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "/"
		}
	}

	// Load latest MCP configuration dynamically
	// This ensures new MCP servers added via mcp_add are visible to new sessions
	mcpServers := p.loadLatestMCPConfig()

	params := CreateSessionParams{
		Model:             effectiveModel,
		WorkingDirectory:  cwd,
		MCPServers:        mcpServers,
		SystemMessage:     p.config.SystemMessage,
		Streaming:         boolPtr(true),
		RequestPermission: boolPtr(false), // Disable permission requests - mote handles its own security
		RequestHooks:      boolPtr(true),
		SkillDirectories:  p.config.SkillDirectories,
	}

	// Add bridged tools if ToolBridge is available
	toolsCount := 0
	if p.toolBridge != nil {
		params.Tools = p.toolBridge.GetBridgeTools()
		toolsCount = len(params.Tools)
	}

	slog.Info("ACP ensureSession creating session",
		"conversationID", conversationID,
		"model", effectiveModel,
		"workingDirectory", cwd,
		"mcpServers", len(params.MCPServers),
		"toolsCount", toolsCount,
		"hasToolBridge", p.toolBridge != nil)

	result, err := p.client.CreateSession(ctx, params)
	if err != nil {
		return true, fmt.Errorf("ACP session create failed: %w", err)
	}

	p.sessionID = result.SessionID
	p.sessionModel = effectiveModel

	// Store conversation → ACP session mapping and model
	// Also store reverse mapping for tool call handler
	if conversationID != "" {
		p.sessionMap.Store(conversationID, result.SessionID)
		p.reverseSessionMap.Store(result.SessionID, conversationID)
		p.sessionModelMap.Store(conversationID, effectiveModel)
	}

	return true, nil
}

// ResetSession clears the session for a given conversation, forcing creation
// of a new ACP session on the next request. If conversationID is empty,
// resets the default session.
func (p *ACPProvider) ResetSession(conversationID string) {
	if conversationID != "" {
		p.sessionMap.Delete(conversationID)
	}
	p.mu.Lock()
	p.sessionID = ""
	p.mu.Unlock()
}

// resetConversationSession removes the ACP session mapping for a specific conversation.
// This is called when a prompt is cancelled (e.g., user clicked Stop) so that the
// next request will create a fresh ACP session instead of reusing the potentially
// still-busy one.
func (p *ACPProvider) resetConversationSession(conversationID string) {
	if conversationID == "" {
		return
	}

	// Remove conversation → ACP session mapping
	if acpSID, ok := p.sessionMap.LoadAndDelete(conversationID); ok {
		// Also remove reverse mapping
		p.reverseSessionMap.Delete(acpSID)
		slog.Info("ACP session reset for conversation",
			"conversationID", conversationID,
			"acpSessionID", acpSID)
	}
	p.sessionModelMap.Delete(conversationID)

	// Clear the global sessionID if it matches this conversation's session
	p.mu.Lock()
	p.sessionID = ""
	p.mu.Unlock()
}

// restartClient terminates the current Copilot CLI process and resets all state
// so the next request will create a fresh client and session.
// This is the nuclear option for when the CLI is stuck processing an old prompt
// and cannot accept new requests (e.g., user clicked Stop during a long task).
func (p *ACPProvider) restartClient() {
	// Set restarting flag to block concurrent initialization attempts
	p.restarting.Store(true)
	defer p.restarting.Store(false)

	p.mu.Lock()
	defer p.mu.Unlock()

	slog.Warn("ACP: restarting CLI process to recover from cancelled prompt")

	// Close the old client (kills CLI process)
	if p.client != nil {
		p.client.Close()
	}

	// Clear ALL session mappings since they belong to the dead CLI process
	p.sessionMap.Range(func(key, _ any) bool {
		p.sessionMap.Delete(key)
		return true
	})
	p.reverseSessionMap.Range(func(key, _ any) bool {
		p.reverseSessionMap.Delete(key)
		return true
	})
	p.sessionModelMap.Range(func(key, _ any) bool {
		p.sessionModelMap.Delete(key)
		return true
	})

	p.sessionID = ""
	p.sessionModel = ""
	p.initialized = false

	slog.Info("ACP: CLI process terminated, will restart on next request")
}

// UpdateMCPServers updates the MCP servers configuration and resets all sessions
// so the next request creates sessions with the updated config.
func (p *ACPProvider) UpdateMCPServers(mcpServers map[string]MCPServerConfig) {
	p.mu.Lock()
	p.config.MCPServers = mcpServers
	p.sessionID = ""
	p.mu.Unlock()

	// Clear all conversation mappings — they need new sessions with updated MCP config
	p.sessionMap.Range(func(key, _ any) bool {
		p.sessionMap.Delete(key)
		return true
	})

	logger.Info().Int("count", len(mcpServers)).Msg("ACP: MCP servers config updated, sessions reset")
}

// Stream sends a streaming chat completion request.
// This is the main method for ACP - it handles the entire conversation turn,
// including multiple tool calls, within a single premium request.
func (p *ACPProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	events := make(chan provider.ChatEvent, 100)

	// Get conversation ID for session isolation
	convID := req.ConversationID
	if convID == "" {
		convID = "default" // Fallback for backward compatibility
	}

	go func() {
		defer close(events)

		// Register session events for this conversation
		entryID := atomic.AddInt64(&sessionEntryCounter, 1)
		entry := &sessionEventEntry{
			events:    events,
			ctx:       ctx,
			createdAt: time.Now(),
			id:        entryID,
		}
		p.sessionEvents.Store(convID, entry)
		slog.Info("Session events channel registered", "convID", convID, "entryID", entryID)

		// Ensure cleanup on exit — but ONLY delete if our entry is still the
		// current one. A newer Stream goroutine for the same convID may have
		// already replaced our entry, and we must not delete theirs.
		defer func() {
			if current, ok := p.sessionEvents.Load(convID); ok {
				if current.(*sessionEventEntry).id == entryID {
					p.sessionEvents.Delete(convID)
					slog.Info("Session events channel cleaned up", "convID", convID, "entryID", entryID)
				} else {
					slog.Info("Session events channel NOT cleaned up (replaced by newer entry)",
						"convID", convID, "myEntryID", entryID,
						"currentEntryID", current.(*sessionEventEntry).id)
				}
			}
		}()

		// Ensure initialized
		if err := p.ensureInitialized(ctx); err != nil {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
			return
		}

		// Ensure session with the requested model
		isNewSession, err := p.ensureSession(ctx, convID, req.Model)
		if err != nil {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
			return
		}

		// Build prompt from messages and attachments.
		// When isNewSession is true (e.g., after mote restart), conversation history
		// must be injected into the prompt since the ACP server has no memory of
		// previous turns. When reusing an existing session, ACP maintains its own
		// conversation state, so history injection is skipped to avoid duplication.
		prompt := p.buildPromptWithAttachments(req.Messages, req.Attachments, isNewSession)
		if len(prompt) == 0 {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: fmt.Errorf("no user message found in request"),
			}
			return
		}

		logger.Info().
			Str("convID", convID).
			Str("sessionId", p.sessionID).
			Int("promptContentCount", len(prompt)).
			Int("toolsCount", len(req.Tools)).
			Msg("Starting ACP prompt execution")

		// Execute prompt - this blocks until all tool calls are completed
		result, err := p.client.PromptWithContent(ctx, p.sessionID, prompt)
		if err != nil {
			// If the context was cancelled (user clicked Stop), the CLI is likely
			// still processing the old prompt. Since ACP protocol has no cancel
			// mechanism, we must kill the CLI process and restart it so the next
			// request can start fresh with a new CLI instance.
			if ctx.Err() != nil {
				slog.Warn("ACP prompt cancelled by user, restarting CLI to force stop",
					"convID", convID,
					"sessionId", p.sessionID,
					"ctxErr", ctx.Err())
				p.restartClient()
			}
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
			return
		}

		// Send done event with stop reason
		finishReason := convertStopReason(result.StopReason)
		events <- provider.ChatEvent{
			Type:         provider.EventTypeDone,
			FinishReason: finishReason,
		}

		// Log detailed completion info for debugging
		logger.Info().
			Str("convID", convID).
			Str("sessionId", p.sessionID).
			Str("stopReason", result.StopReason).
			Msg("ACP prompt completed")

		// Also log to slog for easier correlation with runner logs
		slog.Info("ACP prompt completed",
			"convID", convID,
			"sessionId", p.sessionID,
			"stopReason", result.StopReason)
	}()

	return events, nil
}

// Chat sends a non-streaming chat completion request.
// Implemented by collecting all streaming events.
func (p *ACPProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	eventsChan, err := p.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	// Collect all events
	var content strings.Builder
	var toolCalls []provider.ToolCall
	var usage *provider.Usage
	var finishReason string

	for event := range eventsChan {
		switch event.Type {
		case provider.EventTypeContent:
			content.WriteString(event.Delta)
		case provider.EventTypeToolCall:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}
		case provider.EventTypeDone:
			finishReason = event.FinishReason
			usage = event.Usage
		case provider.EventTypeError:
			return nil, event.Error
		}
	}

	return &provider.ChatResponse{
		Content:      content.String(),
		ToolCalls:    toolCalls,
		Usage:        usage,
		FinishReason: finishReason,
	}, nil
}

// handleNotification processes ACP notifications and converts them to provider events.
// It extracts sessionId from the notification and uses reverseSessionMap to find the convID.
func (p *ACPProvider) handleNotification(method string, params json.RawMessage) {
	// Log all notifications for debugging
	logger.Debug().
		Str("method", method).
		Int("paramsLen", len(params)).
		Str("paramsPreview", string(params)[:min(300, len(params))]).
		Msg("ACP notification received")

	switch method {
	case MethodSessionEvent, LegacyMethodSessionUpdate:
		var updateParams SessionUpdateParams
		if err := json.Unmarshal(params, &updateParams); err != nil {
			logger.Warn().Err(err).Str("method", method).Msg("Failed to parse session update notification")
			return
		}

		// Look up convID from ACP sessionId using reverse mapping
		convID := "default"
		if val, ok := p.reverseSessionMap.Load(updateParams.SessionID); ok {
			convID = val.(string)
		} else {
			logger.Debug().
				Str("acpSessionId", updateParams.SessionID).
				Msg("No reverse mapping found for ACP session, using default")
		}

		p.handleSessionUpdate(convID, updateParams)

	case MethodPermissionRequest, LegacyMethodRequestPermission:
		// Always auto-approve permission requests in ACP mode.
		// Mote has its own security policy layer (internal/policy) that controls
		// tool access before messages reach the provider, so ACP-level permissions
		// can be safely auto-approved.
		var permParams RequestPermissionParams
		if err := json.Unmarshal(params, &permParams); err == nil {
			// Look up convID for logging
			convID := "default"
			if val, ok := p.reverseSessionMap.Load(permParams.SessionID); ok {
				convID = val.(string)
			}
			logger.Debug().
				Str("convID", convID).
				Str("sessionId", permParams.SessionID).
				Msg("Auto-approving ACP tool call permission")
			if err := p.client.RespondToPermission(permParams.SessionID, true); err != nil {
				logger.Warn().Err(err).Msg("Failed to respond to permission request")
			}
		} else {
			logger.Warn().Err(err).Msg("Failed to parse permission request")
		}

	default:
		logger.Info().
			Str("method", method).
			Str("params", string(params)[:min(500, len(params))]).
			Msg("Unhandled ACP notification - may contain important info")
	}
}

// handleHooksInvoke processes hooks.invoke requests from CLI.
// Handles preToolUse (policy checking) and userPromptSubmitted (prompt modification).
func (p *ACPProvider) handleHooksInvoke(req HooksInvokeRequest) (any, error) {
	switch req.HookType {
	case "preToolUse":
		var input PreToolUseInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			logger.Warn().Err(err).Msg("Failed to parse preToolUse input")
			return &PreToolUseOutput{PermissionDecision: "allow"}, nil
		}

		logger.Debug().Str("tool", input.ToolName).Msg("Hooks: preToolUse check")

		// Check for pause before tool execution
		p.pauseMu.RLock()
		pauseCtrl := p.pauseController
		p.pauseMu.RUnlock()

		if pauseCtrl != nil {
			// Extract conversationID from ACP session ID
			convID := p.getConversationID(req.SessionID)
			if convID == "" {
				logger.Warn().Str("acpSessionID", req.SessionID).Msg("No conversation ID mapping found for pause check")
			} else {
				// Check if this session is paused
				if pauseCtrl.IsPaused(convID) {
					logger.Info().Str("convID", convID).Str("tool", input.ToolName).Msg("Session paused, waiting for resume")

					// Block until resumed
					// Note: ToolCallID is not available in PreToolUseInput, pass empty string
					userInput, wasPaused, timedOut := pauseCtrl.WaitForResume(context.Background(), convID, input.ToolName, "")

					if timedOut {
						logger.Warn().Str("convID", convID).Msg("Pause timed out")
						// Allow tool execution to continue
						return &PreToolUseOutput{PermissionDecision: "allow"}, nil
					}

					if wasPaused && userInput != "" {
						// User provided input - deny tool execution
						// ACP doesn't support direct result injection via hooks, so we deny with explanation
						logger.Info().Str("convID", convID).Int("inputLen", len(userInput)).Msg("User input provided, denying tool execution")
						return &PreToolUseOutput{
							PermissionDecision: "deny",
							DenyReason:         fmt.Sprintf("User provided input: %s", userInput),
						}, nil
					}

					// Resumed without user input - continue normally
					logger.Info().Str("convID", convID).Msg("Pause resumed, continuing tool execution")
				}
			}
		}

		// Default: allow all tool calls.
		// When PolicyExecutor is integrated via ACPConfig, this will perform actual checks.
		return &PreToolUseOutput{PermissionDecision: "allow"}, nil

	case "userPromptSubmitted":
		var input UserPromptSubmittedInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			logger.Warn().Err(err).Msg("Failed to parse userPromptSubmitted input")
			return &UserPromptSubmittedOutput{ModifiedPrompt: ""}, nil
		}

		logger.Debug().Int("promptLen", len(input.Prompt)).Msg("Hooks: userPromptSubmitted")

		// Pass through unmodified for now
		return &UserPromptSubmittedOutput{ModifiedPrompt: input.Prompt}, nil

	default:
		logger.Debug().Str("hookType", req.HookType).Msg("Hooks: unhandled hook type")
		return map[string]any{}, nil
	}
}

// handleSessionUpdate processes session/update notifications.
// convID is used to route events to the correct session's channel.
func (p *ACPProvider) handleSessionUpdate(convID string, params SessionUpdateParams) {
	update := params.Update

	switch update.SessionUpdate {
	case UpdateTypeAgentMessageChunk:
		// Text content chunk
		if update.Content != nil && update.Content.Type == "text" {
			p.safeSendEvent(convID, provider.ChatEvent{
				Type:  provider.EventTypeContent,
				Delta: update.Content.Text,
			})
		}

	case UpdateTypeAgentThoughtChunk:
		// Agent thinking/reasoning chunk - send as thinking event for temporary display
		if update.Content != nil && update.Content.Type == "text" {
			logger.Debug().
				Str("convID", convID).
				Str("thought", update.Content.Text).
				Msg("Agent thinking chunk")

			p.safeSendEvent(convID, provider.ChatEvent{
				Type:     provider.EventTypeThinking,
				Thinking: update.Content.Text,
			})
		}

	case UpdateTypeToolCallStart:
		// Tool call started - send both tool_call and tool_call_update events
		// Support both nested and flat formats
		toolCallID := ""
		toolName := ""
		arguments := ""

		if update.ToolCall != nil {
			toolCallID = update.ToolCall.ID
			toolName = update.ToolCall.Name
			arguments = update.ToolCall.Arguments
		} else if update.ToolCallID != "" {
			toolCallID = update.ToolCallID
			toolName = update.Title
		}

		if toolCallID != "" {
			logger.Info().
				Str("convID", convID).
				Str("toolId", toolCallID).
				Str("toolName", toolName).
				Msg("Tool call started")

			// Check for pause before emitting tool call events
			p.pauseMu.RLock()
			pauseCtrl := p.pauseController
			p.pauseMu.RUnlock()

			if pauseCtrl != nil && pauseCtrl.IsPaused(convID) {
				logger.Info().Str("convID", convID).Str("tool", toolName).Msg("Session paused before tool call, waiting for resume")

				// Block until resumed
				userInput, wasPaused, timedOut := pauseCtrl.WaitForResume(context.Background(), convID, toolName, toolCallID)

				if timedOut {
					logger.Warn().Str("convID", convID).Msg("Pause timed out before tool call")
					// Continue with tool call emission after timeout
				} else if wasPaused && userInput != "" {
					logger.Info().Str("convID", convID).Int("inputLen", len(userInput)).Msg("User input provided during pause, skipping tool call emission")
					// User provided input - skip emitting tool call events
					// The input will be handled by the hook layer
					return
				} else {
					logger.Info().Str("convID", convID).Msg("Pause resumed, continuing with tool call")
				}
			}

			// Send tool_call event for the initial call
			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCall,
				ToolCall: &provider.ToolCall{
					ID:        toolCallID,
					Name:      toolName,
					Arguments: arguments,
				},
			})

			// Also send tool_call_update with "running" status
			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCallUpdate,
				ToolCallUpdate: &provider.ToolCallUpdate{
					ID:        toolCallID,
					Name:      toolName,
					Status:    "running",
					Arguments: arguments,
				},
			})
		}

	case UpdateTypeToolCallUpdate:
		// Tool call progress update - send as tool_call_update event
		// Support both nested and flat formats
		toolCallID := ""
		toolName := ""
		status := ""
		arguments := ""

		if update.ToolCall != nil {
			toolCallID = update.ToolCall.ID
			toolName = update.ToolCall.Name
			status = update.ToolCall.Status
			arguments = update.ToolCall.Arguments
		} else if update.ToolCallID != "" {
			toolCallID = update.ToolCallID
			toolName = update.Title
			status = update.Status
		}

		if toolCallID != "" {
			logger.Info().
				Str("convID", convID).
				Str("toolId", toolCallID).
				Str("toolName", toolName).
				Str("status", status).
				Msg("Tool call update")

			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCallUpdate,
				ToolCallUpdate: &provider.ToolCallUpdate{
					ID:        toolCallID,
					Name:      toolName,
					Status:    status,
					Arguments: arguments,
				},
			})
		}

	case UpdateTypeToolCallComplete:
		// Tool call completed - send tool_call_update with completed status
		// Support both nested and flat formats
		toolCallID := ""
		toolName := ""

		if update.ToolCall != nil {
			toolCallID = update.ToolCall.ID
			toolName = update.ToolCall.Name
		} else if update.ToolCallID != "" {
			toolCallID = update.ToolCallID
			toolName = update.Title
		}

		if toolCallID != "" {
			logger.Info().
				Str("convID", convID).
				Str("toolId", toolCallID).
				Str("toolName", toolName).
				Msg("Tool call completed")

			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCallUpdate,
				ToolCallUpdate: &provider.ToolCallUpdate{
					ID:     toolCallID,
					Name:   toolName,
					Status: "completed",
				},
			})
		}

	case UpdateTypeToolCall:
		// Generic tool_call event - check status to determine action
		// Support both nested (update.ToolCall) and flat (update.ToolCallID) formats
		toolCallID := ""
		toolName := ""
		status := ""
		arguments := ""

		if update.ToolCall != nil {
			// Nested format
			toolCallID = update.ToolCall.ID
			toolName = update.ToolCall.Name
			status = update.ToolCall.Status
			arguments = update.ToolCall.Arguments
		} else if update.ToolCallID != "" {
			// Flat format - fields directly on update object
			toolCallID = update.ToolCallID
			toolName = update.Title // Use title as tool name in flat format
			status = update.Status
		}

		if toolCallID == "" {
			logger.Warn().Str("convID", convID).Msg("Tool call event with no toolCallId")
			return
		}

		logger.Info().
			Str("convID", convID).
			Str("toolId", toolCallID).
			Str("toolName", toolName).
			Str("status", status).
			Str("title", update.Title).
			Msg("Tool call event")

		if status == "pending" || status == "" {
			// Tool call started - send both tool_call and tool_call_update events
			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCall,
				ToolCall: &provider.ToolCall{
					ID:        toolCallID,
					Name:      toolName,
					Arguments: arguments,
				},
			})

			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCallUpdate,
				ToolCallUpdate: &provider.ToolCallUpdate{
					ID:        toolCallID,
					Name:      toolName,
					Status:    "running",
					Arguments: arguments,
				},
			})
		} else {
			// Tool call completed or other status
			p.safeSendEvent(convID, provider.ChatEvent{
				Type: provider.EventTypeToolCallUpdate,
				ToolCallUpdate: &provider.ToolCallUpdate{
					ID:     toolCallID,
					Name:   toolName,
					Status: status,
				},
			})
		}

	case UpdateTypeThinking:
		// Agent is thinking - send thinking event
		logger.Debug().Str("convID", convID).Msg("Agent thinking...")
		p.safeSendEvent(convID, provider.ChatEvent{
			Type:     provider.EventTypeThinking,
			Thinking: "思考中...",
		})

	case UpdateTypeThinkingDone:
		logger.Debug().Str("convID", convID).Msg("Agent thinking done")

	case UpdateTypeAgentMessageDone:
		// Agent message completed - the done event will be sent when prompt() returns
		logger.Info().
			Str("convID", convID).
			Str("updateType", update.SessionUpdate).
			Msg("Agent message done - awaiting prompt return")

	default:
		logger.Debug().
			Str("convID", convID).
			Str("updateType", update.SessionUpdate).
			Msg("Unhandled session update type")
	}
}

// buildPrompt extracts the user message from the message list.
// In ACP mode, we primarily pass the latest user message as the prompt.
// The session context is maintained by ACP internally.
func (p *ACPProvider) buildPrompt(messages []provider.Message) string {
	// Find the last user message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			return messages[i].Content
		}
	}

	// If no user message, try to find any content
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Content != "" && messages[i].Role != provider.RoleSystem {
			return messages[i].Content
		}
	}

	return ""
}

// buildPromptWithAttachments builds ACP prompt content array from messages and attachments.
// It includes system messages (e.g., skills) as a prefix to the user message.
//
// When isNewSession is true (e.g., after mote restart creates a fresh ACP session),
// conversation history is serialized and injected into the prompt so the AI can
// see previous turns. When false, ACP maintains its own session state and history
// injection is skipped to avoid duplication.
func (p *ACPProvider) buildPromptWithAttachments(messages []provider.Message, attachments []provider.Attachment, isNewSession bool) []PromptContent {
	var promptContent []PromptContent

	// 1. Find system message and last user message index
	var systemMessage string
	var userMessage string
	lastUserIdx := -1
	for _, msg := range messages {
		if msg.Role == provider.RoleSystem && msg.Content != "" {
			systemMessage = msg.Content
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			userMessage = messages[i].Content
			lastUserIdx = i
			break
		}
	}

	// 2. Collect conversation history (only for new sessions with existing history)
	// When the ACP session is freshly created but the mote conversation has prior
	// turns (e.g., after a restart), we need to inject history so the AI knows
	// about previous interactions.
	var historySection string
	if isNewSession {
		var historyParts []string
		for i, msg := range messages {
			// Skip system messages (handled separately) and the last user message (current turn)
			if msg.Role == provider.RoleSystem {
				continue
			}
			if i == lastUserIdx {
				continue
			}
			// Include user, assistant, and tool messages as history
			if msg.Content != "" {
				var roleLabel string
				switch msg.Role {
				case provider.RoleUser:
					roleLabel = "User"
				case provider.RoleAssistant:
					roleLabel = "Assistant"
				case provider.RoleTool:
					roleLabel = "Tool"
				default:
					roleLabel = msg.Role
				}
				// Truncate very long messages to avoid exceeding context limits
				content := msg.Content
				if len(content) > 4000 {
					content = content[:4000] + "\n...[truncated]"
				}
				historyParts = append(historyParts, fmt.Sprintf("[%s]: %s", roleLabel, content))
			}
		}
		if len(historyParts) > 0 {
			historySection = "<conversation_history>\n" +
				"The following is the conversation history from previous turns in this session.\n" +
				"Use this context to understand the ongoing conversation:\n\n" +
				strings.Join(historyParts, "\n\n") +
				"\n</conversation_history>"
			slog.Info("ACP buildPromptWithAttachments: injected conversation history",
				"historyTurns", len(historyParts),
				"historySectionLen", len(historySection))
		}
	}

	// 3. Build combined text content: system message + history (if new session) + user message
	var combinedText string
	if systemMessage != "" {
		combinedText = systemMessage
		if historySection != "" {
			combinedText += "\n\n---\n\n" + historySection
		}
		combinedText += "\n\n---\n\n" + userMessage
		slog.Info("ACP buildPromptWithAttachments: injected system message",
			"systemMsgLen", len(systemMessage),
			"historyLen", len(historySection),
			"userMsgLen", len(userMessage))
	} else {
		if historySection != "" {
			combinedText = historySection + "\n\n---\n\n" + userMessage
		} else {
			combinedText = userMessage
		}
	}

	if combinedText != "" {
		promptContent = append(promptContent, PromptContent{
			Type: "text",
			Text: combinedText,
		})
	}

	// 4. Add attachments
	for _, att := range attachments {
		switch att.Type {
		case "image_url":
			if att.ImageURL != nil {
				// Parse data URI to extract mimeType and pure base64 data
				// Format: "data:image/png;base64,iVBORw0KGgo..."
				dataURI := att.ImageURL.URL
				mimeType := att.MimeType
				base64Data := dataURI

				if strings.HasPrefix(dataURI, "data:") {
					// Parse data URI: data:<mimeType>;base64,<data>
					parts := strings.SplitN(dataURI, ",", 2)
					if len(parts) == 2 {
						base64Data = parts[1]
						// Extract mimeType from "data:image/png;base64"
						header := parts[0] // "data:image/png;base64"
						header = strings.TrimPrefix(header, "data:")
						if idx := strings.Index(header, ";"); idx != -1 {
							mimeType = header[:idx]
						}
					}
				}

				if mimeType == "" {
					mimeType = "image/png" // fallback
				}

				promptContent = append(promptContent, PromptContent{
					Type:     "image",
					MimeType: mimeType,
					Data:     base64Data,
				})
				slog.Info("ACP buildPromptWithAttachments: added image",
					"mimeType", mimeType,
					"dataLen", len(base64Data))
			}
		case "text":
			// Append text content with formatting
			contentText := fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```",
				att.Filename,
				att.Metadata["language"],
				att.Text)
			promptContent = append(promptContent, PromptContent{
				Type: "text",
				Text: contentText,
			})
		}
	}

	return promptContent
}

// convertStopReason converts ACP stop reason to provider finish reason.
func convertStopReason(acpReason string) string {
	switch acpReason {
	case StopReasonEndTurn:
		return provider.FinishReasonStop
	case StopReasonToolUse:
		return provider.FinishReasonToolCalls
	case StopReasonMaxTokens:
		return provider.FinishReasonLength
	case StopReasonStopSequence:
		return provider.FinishReasonStop
	default:
		return provider.FinishReasonStop
	}
}

// Ping checks if the ACP provider is available.
// Implements provider.HealthCheckable interface.
//
// Unlike the API provider, Ping does NOT try to start the Copilot CLI process.
// The CLI is lazily started on first actual use (Stream/Chat call).
// For health checks, we only verify that the CLI binary can be found.
func (p *ACPProvider) Ping(ctx context.Context) error {
	// If already initialized and running, just check if alive
	if p.initialized {
		if p.client.IsClosed() {
			return ErrACPClosed
		}
		return nil
	}

	// Not yet initialized — only check if the CLI binary exists.
	// Don't start the process; it will be started lazily on first request.
	cliPath := p.config.CopilotPath
	if cliPath == "" {
		var err error
		cliPath, err = findCopilotCLI()
		if err != nil {
			return fmt.Errorf("Copilot CLI 未安装，请运行: npm install -g @github/copilot")
		}
	}

	return nil
}

// GetState returns the current state of the ACP provider.
// Implements provider.HealthCheckable interface.
func (p *ACPProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "copilot-acp",
		LastCheck: time.Now(),
		Models:    ACPListModels(),
	}

	if p.initialized {
		if p.client.IsClosed() {
			state.Status = provider.StatusDisconnected
			state.LastError = "ACP client closed"
			return state
		}
		state.Status = provider.StatusConnected
		return state
	}

	// Not initialized yet — check if CLI is available
	if _, err := findCopilotCLI(); err != nil {
		state.Status = provider.StatusDisconnected
		state.LastError = "Copilot CLI 未安装，请运行: npm install -g @github/copilot"
		return state
	}

	// CLI found but not yet started — it will be lazily initialized
	state.Status = provider.StatusConnected
	return state
}

// Close closes the ACP provider and its underlying client.
func (p *ACPProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// GetSessionID returns the current session ID.
func (p *ACPProvider) GetSessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessionID
}

// IsACPProvider returns true - used for type checking in runner.
func (p *ACPProvider) IsACPProvider() bool {
	return true
}

// MCPServerPersistInfo is a portable representation of a persisted MCP server config.
// Defined here to avoid circular imports with api/v1.
type MCPServerPersistInfo struct {
	Name    string
	Type    string // "stdio", "http", "sse"
	URL     string
	Headers map[string]string
	Command string
	Args    []string
}

// ConvertMCPServers 将 mote 的 MCP 配置转换为 ACP 格式。
// 根据 GitHub 文档，tools 字段是必需的，这里默认使用 ["*"] 表示启用所有工具。
func ConvertMCPServers(persisted []MCPServerPersistInfo) map[string]MCPServerConfig {
	result := make(map[string]MCPServerConfig)
	for _, s := range persisted {
		cfg := MCPServerConfig{
			Type:  s.Type,
			Tools: []string{"*"}, // 默认启用所有工具
		}
		switch s.Type {
		case "stdio", "local":
			cfg.Command = s.Command
			cfg.Args = s.Args
		case "http", "sse":
			// 保持原始类型，不强制转换
			cfg.URL = s.URL
			cfg.Headers = s.Headers
		}
		result[s.Name] = cfg
	}
	return result
}

// loadLatestMCPConfig dynamically loads the latest MCP configuration from mcp_servers.json.
// This allows new sessions to see MCP servers added after the provider was initialized.
// Falls back to the initial config if loading fails.
func (p *ACPProvider) loadLatestMCPConfig() map[string]MCPServerConfig {
	// Import is here to avoid import cycle
	// We'll need to access v1.LoadMCPServersConfigPublic
	// But we can't import api/v1 from provider package
	// So we'll use a different approach: read the file directly

	// Try to load from the same location as api/v1 does
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get home dir, using fallback MCP config")
		return p.config.MCPServers
	}

	configPath := homeDir + "/.mote/mcp_servers.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn().Err(err).Str("path", configPath).Msg("Failed to read MCP config, using fallback")
		}
		return p.config.MCPServers
	}

	// Parse the JSON
	var servers []struct {
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		URL     string            `json:"url,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
		Command string            `json:"command,omitempty"`
		Args    []string          `json:"args,omitempty"`
	}

	if err := json.Unmarshal(data, &servers); err != nil {
		logger.Warn().Err(err).Msg("Failed to parse MCP config, using fallback")
		return p.config.MCPServers
	}

	if len(servers) == 0 {
		return p.config.MCPServers
	}

	// Convert to our format
	infos := make([]MCPServerPersistInfo, len(servers))
	for i, s := range servers {
		infos[i] = MCPServerPersistInfo{
			Name:    s.Name,
			Type:    s.Type,
			URL:     s.URL,
			Headers: s.Headers,
			Command: s.Command,
			Args:    s.Args,
		}
	}

	result := ConvertMCPServers(infos)

	// Log the difference if configuration changed
	if len(result) != len(p.config.MCPServers) {
		logger.Info().
			Int("previousCount", len(p.config.MCPServers)).
			Int("currentCount", len(result)).
			Msg("MCP configuration updated dynamically")
	}

	return result
}

// BuildACPSystemMessage 构建 ACP 的 System Message。
// 包含自定义提示词、工作区规则、MCP 服务器信息和技能提示词。
func BuildACPSystemMessage(customPrompt, workspaceRules string, mcpServerNames []string, skillsPrompt string) *SystemMessageConfig {
	var parts []string

	if customPrompt != "" {
		parts = append(parts, customPrompt)
	}

	if workspaceRules != "" {
		parts = append(parts, "## Workspace Rules\n"+workspaceRules)
	}

	// Add skills prompts to help the AI understand available skills
	if skillsPrompt != "" {
		parts = append(parts, skillsPrompt)
	}

	// Add MCP server information to help the AI understand available tools
	if len(mcpServerNames) > 0 {
		mcpInfo := "## Available MCP Servers\n"
		mcpInfo += "The following MCP (Model Context Protocol) servers are configured and available for use:\n"
		for _, name := range mcpServerNames {
			mcpInfo += fmt.Sprintf("- **%s**\n", name)
		}
		mcpInfo += "\nYou can use tools provided by these MCP servers to accomplish tasks. "
		mcpInfo += "Each server provides specific capabilities - use mcp_list_tools to discover available tools from each server."
		parts = append(parts, mcpInfo)
	}

	if len(parts) == 0 {
		return nil
	}

	return &SystemMessageConfig{
		Content: strings.Join(parts, "\n\n---\n\n"),
	}
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(v bool) *bool {
	return &v
}

// convertToolArgs converts tool arguments from any type to map[string]any.
func convertToolArgs(args any) (map[string]any, error) {
	if args == nil {
		return nil, nil
	}
	if v, ok := args.(map[string]any); ok {
		return v, nil
	}
	// Try JSON round-trip for other types
	data, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool args: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool args: %w", err)
	}
	return result, nil
}

// ACPFactory creates a ProviderFactory function that produces ACPProviders.
// This can be used with provider.Pool for consistent provider creation.
// NOTE: The config is captured at factory creation time. For dynamic config
// (e.g., toolRegistry injection after init), use ACPFactoryWithConfigFunc instead.
func ACPFactory(cfg ACPConfig) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		// Override model in config
		cfg := cfg // Copy
		if model != "" {
			// Validate model is ACP-compatible before setting on config.
			// Invalid models (e.g., API-only grok-code-fast-1) would cause
			// CLI startup failure with --model argument error.
			if IsACPModel(model) {
				cfg.Model = model
			} else {
				slog.Warn("ACPFactory: model not supported by ACP, using default",
					"requestedModel", model,
					"fallback", ACPDefaultModel)
				cfg.Model = ACPDefaultModel
			}
		}
		return NewACPProvider(cfg)
	}
}

// ACPFactoryWithConfigFunc creates a ProviderFactory that calls configFunc
// each time a provider is created. This allows for dynamic configuration
// updates (e.g., toolRegistry becomes available after initial setup).
func ACPFactoryWithConfigFunc(configFunc func() ACPConfig) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		cfg := configFunc()
		if model != "" {
			// Validate model is ACP-compatible before setting on config.
			// Invalid models (e.g., API-only grok-code-fast-1) would cause
			// CLI startup failure with --model argument error.
			if IsACPModel(model) {
				cfg.Model = model
			} else {
				slog.Warn("ACPFactoryWithConfigFunc: model not supported by ACP, using default",
					"requestedModel", model,
					"fallback", ACPDefaultModel)
				cfg.Model = ACPDefaultModel
			}
		}
		return NewACPProvider(cfg)
	}
}

// SetPauseController sets the pause controller for ACP mode.
// This is called by Runner during initialization to enable pause functionality.
func (p *ACPProvider) SetPauseController(ctrl PauseControllerInterface) {
	p.pauseMu.Lock()
	defer p.pauseMu.Unlock()
	p.pauseController = ctrl
	slog.Info("ACP pause controller set")
}

// getConversationID retrieves the mote conversation ID from ACP session ID.
func (p *ACPProvider) getConversationID(acpSessionID string) string {
	if val, ok := p.reverseSessionMap.Load(acpSessionID); ok {
		return val.(string)
	}
	return ""
}
