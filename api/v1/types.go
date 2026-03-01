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

// ImageData represents an image attachment in a chat request.
type ImageData struct {
	Data     string `json:"data"`           // Base64 encoded image data
	MimeType string `json:"mime_type"`      // MIME type (e.g. "image/png")
	Name     string `json:"name,omitempty"` // Optional filename
}

// ChatRequest represents a chat request.
type ChatRequest struct {
	SessionID   string      `json:"session_id,omitempty"`   // Optional, auto-created if empty
	Message     string      `json:"message"`                // Required (can be empty if images provided)
	Model       string      `json:"model,omitempty"`        // Optional, model to use for this request
	Images      []ImageData `json:"images,omitempty"`       // Optional, pasted/uploaded images
	TargetAgent string      `json:"target_agent,omitempty"` // Optional, directly route to this sub-agent (skip main LLM)
}

// ChatResponse represents a chat response.
type ChatResponse struct {
	SessionID string           `json:"session_id"`
	Message   string           `json:"message"`
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`
}

// ChatStreamEvent represents a streaming event.
type ChatStreamEvent struct {
	Type             string                    `json:"type"`                         // "content", "tool_call", "tool_call_update", "tool_result", "thinking", "done", "error", "heartbeat", "truncated", "approval_request", "approval_resolved"
	Delta            string                    `json:"delta,omitempty"`              // For content type
	Thinking         string                    `json:"thinking,omitempty"`           // For thinking type (temporary display)
	ToolCall         *ToolCallResult           `json:"tool_call,omitempty"`          // For tool_call type
	ToolCallUpdate   *ToolCallUpdateEvent      `json:"tool_call_update,omitempty"`   // For tool_call_update type
	ToolResult       *ToolResultEvent          `json:"tool_result,omitempty"`        // For tool_result type
	SessionID        string                    `json:"session_id,omitempty"`         // For done type
	Error            string                    `json:"error,omitempty"`              // For error type (legacy)
	ErrorDetail      *ErrorDetail              `json:"error_detail,omitempty"`       // For error type (detailed)
	TruncatedReason  string                    `json:"truncated_reason,omitempty"`   // For truncated type (e.g., "length")
	PendingToolCalls int                       `json:"pending_tool_calls,omitempty"` // For truncated type
	ApprovalRequest  *ApprovalRequestSSEEvent  `json:"approval_request,omitempty"`   // For approval_request type
	ApprovalResolved *ApprovalResolvedSSEEvent `json:"approval_resolved,omitempty"`  // For approval_resolved type
	PDAProgress      *PDAProgressSSEEvent      `json:"pda_progress,omitempty"`       // For pda_progress type

	// Multi-agent delegate identity (set when event comes from a sub-agent)
	AgentName  string `json:"agent_name,omitempty"`
	AgentDepth int    `json:"agent_depth,omitempty"`
}

// ApprovalRequestSSEEvent represents an approval request sent via SSE.
type ApprovalRequestSSEEvent struct {
	ID        string `json:"id"`
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	Reason    string `json:"reason"`
	SessionID string `json:"session_id"`
	ExpiresAt string `json:"expires_at"`
}

// ApprovalResolvedSSEEvent represents an approval resolution sent via SSE.
type ApprovalResolvedSSEEvent struct {
	ID        string `json:"id"`
	Approved  bool   `json:"approved"`
	DecidedAt string `json:"decided_at"`
}

// PDAProgressSSEEvent represents PDA step progress sent via SSE.
type PDAProgressSSEEvent struct {
	AgentName     string             `json:"agent_name"`
	StepIndex     int                `json:"step_index"`
	TotalSteps    int                `json:"total_steps"`
	StepLabel     string             `json:"step_label"`
	StepType      string             `json:"step_type"`
	Phase         string             `json:"phase"`
	StackDepth    int                `json:"stack_depth"`
	ExecutedSteps []string           `json:"executed_steps,omitempty"`
	TotalTokens   int                `json:"total_tokens,omitempty"`
	Model         string             `json:"model,omitempty"`
	ParentSteps   []PDAParentStepSSE `json:"parent_steps,omitempty"`
}

// PDAParentStepSSE describes a parent frame's step progress.
type PDAParentStepSSE struct {
	AgentName  string `json:"agent_name"`
	StepIndex  int    `json:"step_index"`
	TotalSteps int    `json:"total_steps"`
	StepLabel  string `json:"step_label"`
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
	Content       string  `json:"content"`                  // Required
	Source        string  `json:"source,omitempty"`         // Default "api"
	Category      string  `json:"category,omitempty"`       // P2: Auto-detected if empty
	Importance    float64 `json:"importance,omitempty"`     // P2: Default 0.7 if empty
	CaptureMethod string  `json:"capture_method,omitempty"` // manual|auto|import; default "manual"
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
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	MessageCount     int       `json:"message_count"`
	Title            string    `json:"title,omitempty"`              // Session title/name
	Preview          string    `json:"preview,omitempty"`            // Message preview
	Model            string    `json:"model,omitempty"`              // Model used for this session
	Scenario         string    `json:"scenario,omitempty"`           // Scenario: chat/cron/channel
	Source           string    `json:"source,omitempty"`             // Source: chat/cron/delegate (derived from ID prefix)
	SelectedSkills   []string  `json:"selected_skills,omitempty"`    // Selected skill IDs (nil=all)
	HasPDACheckpoint bool      `json:"has_pda_checkpoint,omitempty"` // True if session has a saved PDA checkpoint
	IsPDA            bool      `json:"is_pda,omitempty"`             // True if session was ever a PDA session
}

// BatchDeleteSessionsRequest is the request body for batch session deletion.
type BatchDeleteSessionsRequest struct {
	IDs []string `json:"ids"`
}

// BatchDeleteDelegationsRequest is the request body for batch delegation deletion.
type BatchDeleteDelegationsRequest struct {
	IDs []string `json:"ids"`
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
	Messages        []Message `json:"messages"`
	EstimatedTokens int       `json:"estimated_tokens"` // Backend-computed token estimate (same heuristic as compaction)
}

// ContextSegment represents a single segment of the LLM context window.
type ContextSegment struct {
	Type            string `json:"type"`                      // "compressed_summary", "kept_message", "history_message", "compressed_history"
	Role            string `json:"role,omitempty"`            // provider role (system, user, assistant, tool)
	Index           int    `json:"index"`                     // position in the final message array
	CharCount       int    `json:"char_count"`                // byte length of content
	EstimatedTokens int    `json:"estimated_tokens"`          // heuristic token count
	ContentPreview  string `json:"content_preview,omitempty"` // first N chars preview
	HasToolCalls    bool   `json:"has_tool_calls,omitempty"`  // whether this message contains tool calls
	ToolCallCount   int    `json:"tool_call_count,omitempty"` // number of tool calls
	InContext       bool   `json:"in_context"`                // true if part of effective context (BuildContext output)
	Budgeted        bool   `json:"budgeted"`                  // true if this segment survives BudgetMessages
}

// CompressionInfo provides information about the compressed context state.
type CompressionInfo struct {
	HasCompression bool   `json:"has_compression"`           // true if a compressed context exists
	Version        int    `json:"version,omitempty"`         // compression version number
	Summary        string `json:"summary,omitempty"`         // the compression summary text
	KeptMessages   int    `json:"kept_messages,omitempty"`   // number of kept messages
	TotalTokens    int    `json:"total_tokens,omitempty"`    // tokens in compressed context
	OriginalTokens int    `json:"original_tokens,omitempty"` // tokens before compression
}

// SessionContextResponse represents the full context analysis for a session.
// Three tiers of counting:
//   - Total: all DB messages (full history)
//   - Effective: what BuildContext assembles (summary + kept + new messages)
//   - Budgeted: after BudgetMessages truncation (what LLM actually receives)
type SessionContextResponse struct {
	SessionID       string           `json:"session_id"`
	Model           string           `json:"model"`
	ContextWindow   int              `json:"context_window"` // model's context window (tokens), 0 if unknown
	Segments        []ContextSegment `json:"segments"`
	Compression     CompressionInfo  `json:"compression"`
	TotalMessages   int              `json:"total_messages"`   // count of all DB messages
	TotalChars      int              `json:"total_chars"`      // chars of all segments (history + effective)
	TotalTokens     int              `json:"total_tokens"`     // tokens of all segments
	EffectiveCount  int              `json:"effective_count"`  // segments in effective context (BuildContext)
	EffectiveChars  int              `json:"effective_chars"`  // chars in effective context
	EffectiveTokens int              `json:"effective_tokens"` // tokens in effective context
	BudgetedCount   int              `json:"budgeted_count"`   // segments after BudgetMessages
	BudgetedChars   int              `json:"budgeted_chars"`   // chars after BudgetMessages
	BudgetedTokens  int              `json:"budgeted_tokens"`  // tokens after BudgetMessages (headline number)
}

// =============================================================================
// Config API Models
// =============================================================================

// ConfigResponse represents configuration (safe view).
type ConfigResponse struct {
	Gateway  GatewayConfigView  `json:"gateway"`
	Provider ProviderConfigView `json:"provider"`
	Ollama   OllamaConfigView   `json:"ollama"`
	Minimax  MinimaxConfigView  `json:"minimax"`
	GLM      GLMConfigView      `json:"glm"`
	VLLM     VLLMConfigView     `json:"vllm"`
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

// MinimaxConfigView represents MiniMax provider configuration for API.
type MinimaxConfigView struct {
	APIKey    string `json:"api_key,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// GLMConfigView represents GLM (智谱AI) provider configuration for API.
type GLMConfigView struct {
	APIKey    string `json:"api_key,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// VLLMConfigView represents vLLM provider configuration for API.
type VLLMConfigView struct {
	APIKey    string `json:"api_key,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
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
	Minimax  *MinimaxConfigView  `json:"minimax,omitempty"`
	GLM      *GLMConfigView      `json:"glm,omitempty"`
	VLLM     *VLLMConfigView     `json:"vllm,omitempty"`
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
	Name           string     `json:"name"`
	Schedule       string     `json:"schedule"`
	Type           string     `json:"type"`
	Prompt         string     `json:"prompt"`
	Enabled        bool       `json:"enabled"`
	Model          string     `json:"model,omitempty"`           // Model for this cron job
	AgentID        string     `json:"agent_id,omitempty"`        // Direct delegate to sub-agent
	SessionID      string     `json:"session_id,omitempty"`      // Associated session ID
	WorkspacePath  string     `json:"workspace_path,omitempty"`  // Workspace directory path
	WorkspaceAlias string     `json:"workspace_alias,omitempty"` // Workspace display alias
	LastRun        *time.Time `json:"last_run,omitempty"`
	NextRun        *time.Time `json:"next_run,omitempty"`
	RunCount       int        `json:"run_count"`
	Description    string     `json:"description,omitempty"`
}

// CronJobsListResponse represents the response for listing cron jobs.
type CronJobsListResponse struct {
	Jobs []CronJob `json:"jobs"`
}

// CreateCronJobRequest represents a request to create a cron job.
type CreateCronJobRequest struct {
	Name           string `json:"name"`               // Required
	Schedule       string `json:"schedule"`           // Required, cron expression
	Type           string `json:"type,omitempty"`     // Optional: prompt (default), tool, script
	Prompt         string `json:"prompt,omitempty"`   // For prompt type
	Payload        string `json:"payload,omitempty"`  // Generic payload (JSON), alternative to prompt
	Model          string `json:"model,omitempty"`    // Model for this cron job
	AgentID        string `json:"agent_id,omitempty"` // Direct delegate to sub-agent
	Enabled        bool   `json:"enabled"`            // Default true
	Description    string `json:"description,omitempty"`
	WorkspacePath  string `json:"workspace_path,omitempty"`  // Workspace directory path
	WorkspaceAlias string `json:"workspace_alias,omitempty"` // Workspace display alias
}

// UpdateCronJobRequest represents a request to update a cron job.
type UpdateCronJobRequest struct {
	Schedule       *string `json:"schedule,omitempty"`
	Prompt         *string `json:"prompt,omitempty"`
	Model          *string `json:"model,omitempty"`
	AgentID        *string `json:"agent_id,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	Description    *string `json:"description,omitempty"`
	WorkspacePath  *string `json:"workspace_path,omitempty"`
	WorkspaceAlias *string `json:"workspace_alias,omitempty"`
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

// CronExecutingJob represents a currently executing cron job.
type CronExecutingJob struct {
	Name       string `json:"name"`
	SessionID  string `json:"session_id"`
	StartedAt  string `json:"started_at"`
	RunningFor int    `json:"running_for"` // seconds
}

// CronExecutingResponse represents the response for currently executing cron jobs.
type CronExecutingResponse struct {
	Jobs []CronExecutingJob `json:"jobs"`
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
