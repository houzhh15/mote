// Package v1 provides API v1 data types and handlers.
package v1

import "time"

// =============================================================================
// Error Codes
// =============================================================================

// Error codes for API responses.
const (
	// Client errors (4xx)
	ErrCodeInvalidRequest    = "INVALID_REQUEST"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeMethodNotAllowed  = "METHOD_NOT_ALLOWED"
	ErrCodeRateLimitExceeded = "RATE_LIMIT_EXCEEDED"
	ErrCodeValidationFailed  = "VALIDATION_FAILED"

	// Server errors (5xx)
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeProviderError      = "PROVIDER_ERROR"
	ErrCodeToolExecutionError = "TOOL_EXECUTION_ERROR"
)

// =============================================================================
// Error Response
// =============================================================================

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

// =============================================================================
// Chat API Models
// =============================================================================

// ChatRequest represents a chat request.
type ChatRequest struct {
	SessionID string `json:"session_id,omitempty"` // Optional, auto-created if empty
	Message   string `json:"message"`              // Required
	Model     string `json:"model,omitempty"`      // Optional, model to use for this request
}

// ChatResponse represents a chat response.
type ChatResponse struct {
	SessionID string           `json:"session_id"`
	Message   string           `json:"message"`
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`
}

// ChatStreamEvent represents a streaming event.
type ChatStreamEvent struct {
	Type             string               `json:"type"`                         // "content", "tool_call", "tool_call_update", "tool_result", "thinking", "done", "error", "heartbeat", "truncated"
	Delta            string               `json:"delta,omitempty"`              // For content type
	Thinking         string               `json:"thinking,omitempty"`           // For thinking type (temporary display)
	ToolCall         *ToolCallResult      `json:"tool_call,omitempty"`          // For tool_call type
	ToolCallUpdate   *ToolCallUpdateEvent `json:"tool_call_update,omitempty"`   // For tool_call_update type
	ToolResult       *ToolResultEvent     `json:"tool_result,omitempty"`        // For tool_result type
	SessionID        string               `json:"session_id,omitempty"`         // For done type
	Error            string               `json:"error,omitempty"`              // For error type (legacy)
	ErrorDetail      *ErrorDetail         `json:"error_detail,omitempty"`       // For error type (detailed)
	TruncatedReason  string               `json:"truncated_reason,omitempty"`   // For truncated type (e.g., "length")
	PendingToolCalls int                  `json:"pending_tool_calls,omitempty"` // For truncated type
}

// ToolCallUpdateEvent represents a tool call progress update in streaming.
type ToolCallUpdateEvent struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Status     string `json:"status,omitempty"`    // "running", "completed"
	Arguments  string `json:"arguments,omitempty"` // May be partial during streaming
}

// ErrorDetail represents detailed error information for SSE events.
type ErrorDetail struct {
	Code       string `json:"code"`                  // Error code (e.g., AUTH_FAILED, RATE_LIMITED)
	Message    string `json:"message"`               // Human-readable error message
	Provider   string `json:"provider,omitempty"`    // Provider name if applicable
	Retryable  bool   `json:"retryable"`             // Whether the error can be retried
	RetryAfter int    `json:"retry_after,omitempty"` // Seconds until retry is allowed
}

// ToolCallResult represents a tool call result.
type ToolCallResult struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"` // Tool call arguments as JSON string
	Result    any    `json:"result"`
	Error     string `json:"error,omitempty"`
}

// ToolResultEvent represents a tool execution result in streaming.
type ToolResultEvent struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// =============================================================================
// Tool API Models
// =============================================================================

// ToolInfo represents tool information.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      any    `json:"schema"`
	Type        string `json:"type"` // builtin, js, mcp
}

// ToolsListResponse represents the response for listing tools.
type ToolsListResponse struct {
	Tools []ToolInfo `json:"tools"`
	Count int        `json:"count"`
}

// ToolExecuteRequest represents a tool execution request.
type ToolExecuteRequest struct {
	Params map[string]any `json:"params"`
}

// ToolExecuteResponse represents a tool execution response.
type ToolExecuteResponse struct {
	Success bool   `json:"success"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// =============================================================================
// Memory API Models
// =============================================================================

// MemorySearchRequest represents a memory search request.
type MemorySearchRequest struct {
	Query         string   `json:"query"`                    // Required
	TopK          int      `json:"top_k,omitempty"`          // Default 10
	Categories    []string `json:"categories,omitempty"`     // P2: Filter by categories
	MinImportance float64  `json:"min_importance,omitempty"` // P2: Minimum importance threshold
}

// MemorySearchResponse represents a memory search response.
type MemorySearchResponse struct {
	Results []MemoryResult `json:"results"`
}

// MemoryListResponse represents a memory list response.
type MemoryListResponse struct {
	Memories []MemoryResult `json:"memories"`
	Total    int            `json:"total"`  // Total number of memories
	Limit    int            `json:"limit"`  // Items per page
	Offset   int            `json:"offset"` // Current offset
}

// MemoryResult represents a single memory search result.
type MemoryResult struct {
	ID            string  `json:"id"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
	Source        string  `json:"source"`
	CreatedAt     string  `json:"created_at"`               // RFC3339
	Category      string  `json:"category,omitempty"`       // P2: Memory category
	Importance    float64 `json:"importance,omitempty"`     // P2: Importance score
	CaptureMethod string  `json:"capture_method,omitempty"` // P2: How it was captured
	// P1: Chunk metadata fields
	ChunkIndex int    `json:"chunk_index,omitempty"` // Index of this chunk (0-based)
	ChunkTotal int    `json:"chunk_total,omitempty"` // Total number of chunks
	SourceFile string `json:"source_file,omitempty"` // Original source file path
}

// AddMemoryRequest represents a request to add memory.
type AddMemoryRequest struct {
	Content    string  `json:"content"`              // Required
	Source     string  `json:"source,omitempty"`     // Default "api"
	Category   string  `json:"category,omitempty"`   // P2: Auto-detected if empty
	Importance float64 `json:"importance,omitempty"` // P2: Default 0.7 if empty
}

// AddMemoryResponse represents a response after adding memory.
type AddMemoryResponse struct {
	ID       string `json:"id"`
	Category string `json:"category,omitempty"` // P2: Detected/assigned category
}

// MemoryEntryResponse represents a full memory entry response for GetByID.
type MemoryEntryResponse struct {
	ID            string         `json:"id"`
	Content       string         `json:"content"`
	Source        string         `json:"source"`
	CreatedAt     string         `json:"created_at"` // RFC3339
	Metadata      map[string]any `json:"metadata,omitempty"`
	Category      string         `json:"category,omitempty"`       // P2
	Importance    float64        `json:"importance,omitempty"`     // P2
	CaptureMethod string         `json:"capture_method,omitempty"` // P2
}

// MemoryStatsResponse represents the memory statistics response.
type MemoryStatsResponse struct {
	Total            int            `json:"total"`
	ByCategory       map[string]int `json:"by_category"`
	ByCaptureMethod  map[string]int `json:"by_capture_method"`
	AutoCaptureToday int            `json:"auto_capture_today"`
	AutoRecallToday  int            `json:"auto_recall_today"`
}

// =============================================================================
// Session API Models
// =============================================================================

// SessionSummary represents session summary info.
type SessionSummary struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	MessageCount   int       `json:"message_count"`
	Title          string    `json:"title,omitempty"`           // Session title/name
	Preview        string    `json:"preview,omitempty"`         // Message preview
	Model          string    `json:"model,omitempty"`           // Model used for this session
	Scenario       string    `json:"scenario,omitempty"`        // Scenario: chat/cron/channel
	SelectedSkills []string  `json:"selected_skills,omitempty"` // Selected skill IDs (nil=all)
}

// SessionDetail represents detailed session info.
type SessionDetail struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	MessageCount   int       `json:"message_count"`
	Title          string    `json:"title,omitempty"` // Session title/name
	Messages       []Message `json:"messages"`
	Model          string    `json:"model,omitempty"`           // Model used for this session
	Scenario       string    `json:"scenario,omitempty"`        // Scenario: chat/cron/channel
	SelectedSkills []string  `json:"selected_skills,omitempty"` // Selected skill IDs (nil=all)
}

// SessionsListResponse represents the response for listing sessions.
type SessionsListResponse struct {
	Sessions []SessionSummary `json:"sessions"`
}

// CreateSessionRequest represents a request to create a session.
type CreateSessionRequest struct {
	// No required fields, session ID is auto-generated
	Model    string `json:"model,omitempty"`    // Optional: model for the session
	Scenario string `json:"scenario,omitempty"` // Optional: scenario type
}

// UpdateSessionRequest represents a request to update session properties.
type UpdateSessionRequest struct {
	Title string `json:"title"` // Optional: new title for the session
}

// UpdateSessionModelRequest represents a request to update session model.
type UpdateSessionModelRequest struct {
	Model string `json:"model"` // Required: new model for the session
}

// UpdateSessionModelResponse represents the response for updating session model.
type UpdateSessionModelResponse struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

// UpdateSessionSkillsRequest represents a request to update session selected skills.
type UpdateSessionSkillsRequest struct {
	SelectedSkills []string `json:"selected_skills"` // Skill IDs to enable; empty array means all
}

// UpdateSessionSkillsResponse represents the response for updating session skills.
type UpdateSessionSkillsResponse struct {
	ID             string   `json:"id"`
	SelectedSkills []string `json:"selected_skills"`
}

// ReconfigureSessionRequest represents a request to reconfigure a session's model,
// workspace, and/or skills. This is a major operation that:
// 1. Aborts any running task for the session
// 2. Updates the session parameters in DB
// 3. Cleans up all runtime resources (ACP sessions, cache, token tracking)
// 4. Forces fresh resource creation on the next request
type ReconfigureSessionRequest struct {
	Model          *string   `json:"model,omitempty"`           // New model (nil = no change)
	WorkspacePath  *string   `json:"workspace_path,omitempty"`  // New workspace path (nil = no change, "" = unbind)
	WorkspaceAlias *string   `json:"workspace_alias,omitempty"` // Workspace alias
	SelectedSkills *[]string `json:"selected_skills,omitempty"` // New skills (nil = no change)
}

// ReconfigureSessionResponse represents the response for session reconfiguration.
type ReconfigureSessionResponse struct {
	ID             string   `json:"id"`
	Model          string   `json:"model"`
	WorkspacePath  string   `json:"workspace_path,omitempty"`
	SelectedSkills []string `json:"selected_skills,omitempty"`
	Message        string   `json:"message"`
}

// ScenarioModelsResponse represents scenario default models.
type ScenarioModelsResponse struct {
	Chat    string `json:"chat"`    // Default model for chat scenario
	Cron    string `json:"cron"`    // Default model for cron scenario
	Channel string `json:"channel"` // Default model for channel scenario
}

// UpdateScenarioModelsRequest represents a request to update scenario models.
type UpdateScenarioModelsRequest struct {
	Chat    string `json:"chat,omitempty"`    // New default model for chat
	Cron    string `json:"cron,omitempty"`    // New default model for cron
	Channel string `json:"channel,omitempty"` // New default model for channel
}

// CreateSessionResponse represents a response after creating a session.
type CreateSessionResponse struct {
	ID        string    `json:"id"`
	Model     string    `json:"model,omitempty"`
	Scenario  string    `json:"scenario,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Message represents a chat message.
type Message struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // user, assistant, system, tool
	Content   string           `json:"content"`
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// MessagesListResponse represents the response for listing messages.
type MessagesListResponse struct {
	Messages []Message `json:"messages"`
}

// =============================================================================
// Config API Models
// =============================================================================

// ConfigResponse represents configuration (safe view).
type ConfigResponse struct {
	Gateway  GatewayConfigView  `json:"gateway"`
	Provider ProviderConfigView `json:"provider"`
	Ollama   OllamaConfigView   `json:"ollama"`
	Memory   MemoryConfigView   `json:"memory"`
	Cron     CronConfigView     `json:"cron"`
	MCP      MCPConfigView      `json:"mcp"`
}

// GatewayConfigView represents gateway configuration for API.
type GatewayConfigView struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ProviderConfigView represents provider configuration for API.
type ProviderConfigView struct {
	Default string   `json:"default"`           // copilot, ollama
	Enabled []string `json:"enabled,omitempty"` // List of enabled providers
}

// ProviderStatus represents the status of a provider.
type ProviderStatus struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Available  bool   `json:"available"`
	ModelCount int    `json:"model_count"`
	Error      string `json:"error,omitempty"` // Last error message if not available
}

// OllamaConfigView represents Ollama provider configuration for API.
type OllamaConfigView struct {
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
}

// MemoryConfigView represents memory configuration for API.
type MemoryConfigView struct {
	Enabled bool `json:"enabled"`
}

// CronConfigView represents cron configuration for API.
type CronConfigView struct {
	Enabled bool `json:"enabled"`
}

// MCPConfigView represents MCP configuration for API.
type MCPConfigView struct {
	ServerEnabled bool `json:"server_enabled"`
	ClientEnabled bool `json:"client_enabled"`
}

// UpdateConfigRequest represents a request to update configuration.
type UpdateConfigRequest struct {
	Gateway  *GatewayConfigView  `json:"gateway,omitempty"`
	Provider *ProviderConfigView `json:"provider,omitempty"`
	Ollama   *OllamaConfigView   `json:"ollama,omitempty"`
	Memory   *MemoryConfigView   `json:"memory,omitempty"`
	Cron     *CronConfigView     `json:"cron,omitempty"`
}

// =============================================================================
// MCP API Models
// =============================================================================

// MCPServerInfo represents MCP server connection info.
type MCPServerInfo struct {
	Name        string            `json:"name"`
	Status      string            `json:"status"`    // connected, disconnected, error
	Transport   string            `json:"transport"` // stdio, http
	Tools       []string          `json:"tools,omitempty"`
	ToolCount   int               `json:"tool_count,omitempty"`
	PromptCount int               `json:"prompt_count,omitempty"`
	Error       string            `json:"error,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
}

// MCPServersResponse represents the response for listing MCP servers.
type MCPServersResponse struct {
	Servers []MCPServerInfo `json:"servers"`
}

// MCPToolInfo represents an MCP tool.
type MCPToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Server      string `json:"server"`
	Schema      any    `json:"schema"`
}

// MCPToolsResponse represents the response for listing MCP tools.
type MCPToolsResponse struct {
	Tools []MCPToolInfo `json:"tools"`
}

// MCPPromptInfo represents an MCP prompt.
type MCPPromptInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Server      string              `json:"server,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

// MCPPromptArgument represents a prompt argument.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPPromptsResponse represents the response for listing MCP prompts.
type MCPPromptsResponse struct {
	Prompts []MCPPromptInfo `json:"prompts"`
}

// MCPServerDetail represents detailed info about an MCP server.
type MCPServerDetail struct {
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Transport   string          `json:"transport"`
	URL         string          `json:"url,omitempty"`
	ToolCount   int             `json:"tool_count"`
	PromptCount int             `json:"prompt_count"`
	Tools       []MCPToolInfo   `json:"tools"`
	Prompts     []MCPPromptInfo `json:"prompts"`
}

// =============================================================================
// Health API Models
// =============================================================================

// HealthResponse represents health check response.
type HealthResponse struct {
	Status     string                     `json:"status"` // healthy, degraded, unhealthy
	Version    string                     `json:"version"`
	Uptime     string                     `json:"uptime"`
	Timestamp  string                     `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

// ComponentHealth represents component health status.
type ComponentHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// =============================================================================
// Cron API Models
// =============================================================================

// CronJob represents a cron job.
type CronJob struct {
	Name        string     `json:"name"`
	Schedule    string     `json:"schedule"`
	Type        string     `json:"type"`
	Prompt      string     `json:"prompt"`
	Enabled     bool       `json:"enabled"`
	Model       string     `json:"model,omitempty"`      // Model for this cron job
	SessionID   string     `json:"session_id,omitempty"` // Associated session ID
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	RunCount    int        `json:"run_count"`
	Description string     `json:"description,omitempty"`
}

// CronJobsListResponse represents the response for listing cron jobs.
type CronJobsListResponse struct {
	Jobs []CronJob `json:"jobs"`
}

// CreateCronJobRequest represents a request to create a cron job.
type CreateCronJobRequest struct {
	Name        string `json:"name"`              // Required
	Schedule    string `json:"schedule"`          // Required, cron expression
	Type        string `json:"type,omitempty"`    // Optional: prompt (default), tool, script
	Prompt      string `json:"prompt,omitempty"`  // For prompt type
	Payload     string `json:"payload,omitempty"` // Generic payload (JSON), alternative to prompt
	Model       string `json:"model,omitempty"`   // Model for this cron job
	Enabled     bool   `json:"enabled"`           // Default true
	Description string `json:"description,omitempty"`
}

// UpdateCronJobRequest represents a request to update a cron job.
type UpdateCronJobRequest struct {
	Schedule    *string `json:"schedule,omitempty"`
	Prompt      *string `json:"prompt,omitempty"`
	Model       *string `json:"model,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CronHistoryEntry represents a cron execution history entry.
type CronHistoryEntry struct {
	ID        string    `json:"id"`
	JobName   string    `json:"job_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Status    string    `json:"status"` // success, error
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// CronHistoryResponse represents the response for cron history.
type CronHistoryResponse struct {
	History []CronHistoryEntry `json:"history"`
}

// =============================================================================
// UI API Models
// =============================================================================

// UIComponent represents a UI component's metadata.
type UIComponent struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	File        string         `json:"file"`
	Version     string         `json:"version,omitempty"`
	Props       map[string]any `json:"props,omitempty"`
}

// UIComponentsResponse represents the response for listing UI components.
type UIComponentsResponse struct {
	Components []UIComponent `json:"components"`
}

// UIState represents the current UI state synchronized across clients.
type UIState struct {
	ActiveSession string         `json:"active_session,omitempty"`
	Theme         string         `json:"theme,omitempty"`
	SidebarOpen   bool           `json:"sidebar_open"`
	CurrentPage   string         `json:"current_page,omitempty"`
	Custom        map[string]any `json:"custom,omitempty"`
}

// =============================================================================
// Common Response Models
// =============================================================================

// SuccessResponse represents a generic success response.
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}
