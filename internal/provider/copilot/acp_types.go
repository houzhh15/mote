package copilot

import (
	"context"
	"encoding/json"
)

// ========== ACP Protocol Constants ==========

// ACPProtocolVersion is the ACP protocol version.
const ACPProtocolVersion = 1

// Protocol method names — new dot-notation (SDK-aligned) and legacy slash-notation.
const (
	// New protocol method names (SDK-aligned, Copilot CLI >= v0.0.410)
	MethodSessionCreate     = "session.create"
	MethodSessionSend       = "session.send"
	MethodSessionEvent      = "session.event"
	MethodPermissionRequest = "permission.request"
	MethodPermissionRespond = "permission.response"

	// Legacy protocol method names (Copilot CLI < v0.0.410)
	LegacyMethodSessionNew         = "session/new"
	LegacyMethodSessionPrompt      = "session/prompt"
	LegacyMethodSessionUpdate      = "session/update"
	LegacyMethodRequestPermission  = "session/request_permission"
	LegacyMethodPermissionResponse = "session/permission_response"
)

// MinNewProtocolVersion is the minimum protocol version that supports
// the new dot-notation method names.
const MinNewProtocolVersion = 2

// SessionUpdate types from ACP protocol.
const (
	UpdateTypeAgentMessageChunk = "agent_message_chunk"
	UpdateTypeAgentMessageDone  = "agent_message_done"
	UpdateTypeAgentThoughtChunk = "agent_thought_chunk"
	UpdateTypeToolCallStart     = "tool_call_start"
	UpdateTypeToolCallComplete  = "tool_call_complete"
	UpdateTypeToolCall          = "tool_call"
	UpdateTypeToolCallUpdate    = "tool_call_update"
	UpdateTypeThinking          = "thinking"
	UpdateTypeThinkingDone      = "thinking_done"
)

// StopReason values from ACP protocol.
const (
	StopReasonEndTurn      = "end_turn"
	StopReasonToolUse      = "tool_use"
	StopReasonMaxTokens    = "max_tokens"
	StopReasonStopSequence = "stop_sequence"
)

// ========== JSON-RPC 2.0 Base Structures ==========

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"` // Requests have ID, notifications don't
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response or notification.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`     // Responses have ID
	Result  json.RawMessage `json:"result,omitempty"` // For successful responses
	Error   *JSONRPCError   `json:"error,omitempty"`  // For error responses
	Method  string          `json:"method,omitempty"` // For notifications
	Params  json.RawMessage `json:"params,omitempty"` // For notifications
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// IsNotification returns true if this response is actually a notification.
func (r *JSONRPCResponse) IsNotification() bool {
	return r.ID == nil && r.Method != ""
}

// ========== ACP Initialize ==========

// InitializeParams is the parameters for the "initialize" request.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
}

// ClientCapabilities describes the capabilities of the ACP client.
type ClientCapabilities struct {
	// Currently empty, reserved for future use
}

// InitializeResult is the result of the "initialize" request.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentInfo         AgentInfo         `json:"agentInfo"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
}

// AgentInfo provides information about the ACP agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentCapabilities describes the capabilities of the ACP agent.
type AgentCapabilities struct {
	LoadSession bool     `json:"loadSession,omitempty"`
	SetMode     bool     `json:"setMode,omitempty"`
	Modes       []string `json:"modes,omitempty"`
}

// ========== ACP Session ==========

// NewSessionParams is the parameters for the "session/new" request.
type NewSessionParams struct {
	Cwd        string      `json:"cwd"`
	McpServers []MCPServer `json:"mcpServers"` // Must be present, even if empty array
}

// MCPServer describes an MCP server configuration for ACP legacy protocol.
// NOTE: The CLI requires args and env to be arrays, not nil/object.
// NOTE: The field is "type" (not "transport") in the JSON payload.
// NOTE: tools 字段指定允许的工具列表，["*"] 表示全部工具
type MCPServer struct {
	Name    string      `json:"name,omitempty"`
	Type    string      `json:"type,omitempty"`    // "local", "stdio", "http", 或 "sse"
	Command string      `json:"command,omitempty"` // stdio/local 模式
	Args    []string    `json:"args"`              // Must be array, not null
	Env     []MCPEnvVar `json:"env"`               // Must be array of {name, value}
	URL     string      `json:"url,omitempty"`     // For HTTP/SSE transport
	Headers []MCPEnvVar `json:"headers,omitempty"` // For HTTP/SSE transport
	Tools   []string    `json:"tools,omitempty"`   // 允许的工具列表，["*"] 表示全部
}

// MCPEnvVar represents an environment variable or header as {name, value} pair.
// The legacy CLI requires env/headers as array of objects with "name" field (not "key").
type MCPEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewSessionResult is the result of the "session/new" request.
type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

// ========== ACP Prompt ==========

// PromptParams is the parameters for the "session/prompt" request.
type PromptParams struct {
	SessionID string          `json:"sessionId"`
	Prompt    []PromptContent `json:"prompt"`
}

// PromptContent represents content in a prompt.
type PromptContent struct {
	Type string `json:"type"` // "text", "image", etc.
	Text string `json:"text,omitempty"`
	// For image content:
	// MimeType string `json:"mimeType,omitempty"`
	// Data     string `json:"data,omitempty"`
}

// PromptResult is the result of the "session/prompt" request.
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// ========== ACP Session Update Notification ==========

// SessionUpdateParams is the parameters for the "session/update" notification.
type SessionUpdateParams struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate describes an update to a session.
type SessionUpdate struct {
	SessionUpdate string         `json:"sessionUpdate"` // Update type
	Content       *UpdateContent `json:"content,omitempty"`
	ToolCall      *ToolCallInfo  `json:"toolCall,omitempty"`
	// Direct fields for tool_call events (when not nested in ToolCall)
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`   // read, execute, etc.
	Status     string `json:"status,omitempty"` // pending, completed, failed
}

// UpdateContent represents content in a session update.
// The CLI may send content as either a single object or an array of objects.
type UpdateContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text,omitempty"`
}

// UnmarshalJSON handles both single object and array formats for content.
func (u *UpdateContent) UnmarshalJSON(data []byte) error {
	// Try single object first
	type updateContentAlias UpdateContent
	var single updateContentAlias
	if err := json.Unmarshal(data, &single); err == nil {
		*u = UpdateContent(single)
		return nil
	}

	// Try array format - CLI sometimes sends content as array
	var arr []updateContentAlias
	if err := json.Unmarshal(data, &arr); err == nil {
		if len(arr) > 0 {
			// Take the first element, or combine text from all elements
			var combinedText string
			for _, item := range arr {
				if item.Type == "text" && item.Text != "" {
					combinedText += item.Text
				}
			}
			u.Type = "text"
			u.Text = combinedText
		}
		return nil
	}

	// If both fail, return original error
	return json.Unmarshal(data, (*updateContentAlias)(u))
}

// ToolCallInfo represents tool call information in a session update.
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Status    string `json:"status,omitempty"` // "running", "completed", "failed"
	Result    string `json:"result,omitempty"` // Tool execution result
}

// ========== ACP Permission Request ==========

// RequestPermissionParams is the parameters for the "session/request_permission" notification.
type RequestPermissionParams struct {
	SessionID   string             `json:"sessionId"`
	Permissions []Permission       `json:"permissions,omitempty"`
	ToolCall    *ToolCallInfo      `json:"toolCall,omitempty"`
	Options     []PermissionOption `json:"options,omitempty"` // Available response options
}

// PermissionOption represents an available response option for permission requests.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once
	Name     string `json:"name"`
}

// Permission represents a permission request.
type Permission struct {
	Tool        string `json:"tool"`
	Description string `json:"description,omitempty"`
}

// RequestPermissionResult is the result sent back for permission requests.
// Supports multiple formats to be compatible with different CLI versions.
type RequestPermissionResult struct {
	Outcome          *PermissionOutcome `json:"outcome,omitempty"`          // Legacy format
	OptionID         string             `json:"optionId,omitempty"`         // New format v1
	SelectedOptionID string             `json:"selectedOptionId,omitempty"` // New format v2
}

// PermissionOutcome describes the outcome of a permission request.
type PermissionOutcome struct {
	Outcome string `json:"outcome"` // "approved", "denied", "cancelled"
}

// ========== ACP Session (SDK-aligned) ==========

// CreateSessionParams 对齐 SDK 的 session.create 请求参数
type CreateSessionParams struct {
	// 基础配置
	Model            string `json:"model,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	ReasoningEffort  string `json:"reasoningEffort,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	Streaming        *bool  `json:"streaming,omitempty"`
	ConfigDir        string `json:"configDir,omitempty"`

	// 工具配置
	Tools          []ACPToolDef `json:"tools,omitempty"`          // 自定义工具
	AvailableTools []string     `json:"availableTools,omitempty"` // 可用工具白名单
	ExcludedTools  []string     `json:"excludedTools,omitempty"`  // 排除工具黑名单

	// MCP 配置
	MCPServers map[string]MCPServerConfig `json:"mcpServers,omitempty"` // MCP 服务器配置

	// Prompt 配置
	SystemMessage *SystemMessageConfig `json:"systemMessage,omitempty"`
	CustomAgents  []CustomAgentConfig  `json:"customAgents,omitempty"`

	// Skills 配置
	SkillDirectories []string `json:"skillDirectories,omitempty"`
	DisabledSkills   []string `json:"disabledSkills,omitempty"`

	// 能力声明
	RequestPermission *bool `json:"requestPermission,omitempty"`
	RequestUserInput  *bool `json:"requestUserInput,omitempty"`
	RequestHooks      *bool `json:"requestHooks,omitempty"`

	// Infinite sessions
	InfiniteSessions *InfiniteSessionConfig `json:"infiniteSessions,omitempty"`
}

// ACPToolDef 对齐 SDK 的 Tool 定义
type ACPToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
}

// MCPServerConfig 对齐 SDK 的 MCP 服务器配置（命名映射格式）
// 根据 GitHub 文档: type 可以是 "local", "stdio", "http", 或 "sse"
// tools 字段是必需的，指定允许的工具列表（如 ["*"] 表示所有工具）
type MCPServerConfig struct {
	Type    string            `json:"type"`              // "local", "stdio", "http", 或 "sse"
	Command string            `json:"command,omitempty"` // stdio/local 模式
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`     // http/sse 模式
	Headers map[string]string `json:"headers,omitempty"` // http/sse 模式
	Tools   []string          `json:"tools,omitempty"`   // 允许的工具列表，如 ["*"] 表示全部
}

// SystemMessageConfig 自定义系统消息
type SystemMessageConfig struct {
	Content string `json:"content"`
}

// CustomAgentConfig 自定义 Agent 角色配置
type CustomAgentConfig struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

// InfiniteSessionConfig 无限会话配置
type InfiniteSessionConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// ========== ACP Tool Call (SDK-aligned) ==========

// ToolCallRequest CLI → mote 的工具调用请求
type ToolCallRequest struct {
	SessionID  string `json:"sessionId"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Arguments  any    `json:"arguments"`
}

// ToolCallResponse mote → CLI 的工具调用响应
type ToolCallResponse struct {
	Result ToolResult `json:"result"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	TextResultForLLM string         `json:"textResultForLlm"` // 返回给 LLM 的文本
	ResultType       string         `json:"resultType"`       // "success" | "failure"
	Error            string         `json:"error,omitempty"`
	ToolTelemetry    map[string]any `json:"toolTelemetry,omitempty"`
}

// ========== ACP Config ==========

// ToolRegistryInterface is the interface for tool registry used by ACP.
// Defined here to avoid circular imports with the tools package.
type ToolRegistryInterface interface {
	// List returns all registered tools as a slice of ToolInfo.
	ListToolInfo() []ToolInfo
	// ExecuteTool runs a tool by name with the given arguments.
	ExecuteTool(ctx context.Context, name string, args map[string]any) (ToolExecResult, error)
}

// ToolInfo provides tool metadata for bridge registration.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolExecResult is a simplified tool execution result for ACP bridging.
type ToolExecResult struct {
	Content string
	IsError bool
}

// ACPConfig holds configuration for the ACP Provider.
type ACPConfig struct {
	// Model to use (e.g., "claude-sonnet-4.5", "gpt-4o")
	Model string

	// CopilotPath is the optional path to copilot CLI.
	// If empty, will try to find it automatically.
	CopilotPath string

	// AllowAllTools if true, automatically approves all tool calls.
	// Useful for testing, but use with caution in production.
	AllowAllTools bool

	// Deprecated: McpServers is the old-style array MCP config.
	// Use MCPServers (map format) instead.
	McpServers []MCPServer

	// Timeout for requests. Default is 2 minutes.
	Timeout int64 // in seconds

	// WorkingDirectory is the working directory for the session.
	// If empty, uses the current working directory.
	WorkingDirectory string

	// WorkspaceResolver is a callback to resolve workspace path by sessionID.
	// This allows ACPProvider to get the bound workspace path when creating sessions.
	// If nil or returns empty string, falls back to WorkingDirectory or os.Getwd().
	WorkspaceResolver func(sessionID string) string

	// MCPServers is the MCP servers config in SDK map format.
	MCPServers map[string]MCPServerConfig

	// SystemMessage is the custom system prompt for ACP session.
	SystemMessage *SystemMessageConfig

	// ToolRegistry is the mote tool registry reference for bridging.
	ToolRegistry ToolRegistryInterface

	// ExcludeTools lists tool names not to bridge (CLI already has them).
	ExcludeTools []string

	// SkillDirectories is the list of skill directory paths.
	SkillDirectories []string

	// GithubToken is an optional GitHub token for authentication.
	GithubToken string
}

// ========== Hooks Types ==========

// HooksInvokeRequest represents a hooks.invoke request from CLI.
type HooksInvokeRequest struct {
	SessionID string          `json:"sessionId"`
	HookType  string          `json:"hookType"` // "preToolUse", "userPromptSubmitted", etc.
	Input     json.RawMessage `json:"input"`
}

// PreToolUseInput is the input for preToolUse hook.
type PreToolUseInput struct {
	ToolName string         `json:"toolName"`
	ToolArgs map[string]any `json:"toolArgs,omitempty"`
}

// PreToolUseOutput is the output for preToolUse hook.
type PreToolUseOutput struct {
	// PermissionDecision: "allow", "deny", or "ask"
	PermissionDecision string `json:"permissionDecision"`
	// DenyReason is the reason for denial (when PermissionDecision is "deny")
	DenyReason string `json:"denyReason,omitempty"`
}

// UserPromptSubmittedInput is the input for userPromptSubmitted hook.
type UserPromptSubmittedInput struct {
	Prompt string `json:"prompt"`
}

// UserPromptSubmittedOutput is the output for userPromptSubmitted hook.
type UserPromptSubmittedOutput struct {
	ModifiedPrompt string `json:"modifiedPrompt"`
}
