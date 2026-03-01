// ================================================================
// Shared Types for Mote UI
// ================================================================

export interface ServiceStatus {
  running: boolean;
  port: number;
  version: string;
  uptime: number;
  error?: string;
}

export interface HealthResponse {
  status: string;
  version: string;
  uptime: number;
}

// Authentication types
export interface AuthStatus {
  authenticated: boolean;
  token_masked?: string;
  copilot_verified?: boolean;
  error?: string;
}

export interface DeviceCodeResponse {
  device_code: string;
  user_code: string;
  verification_uri: string;
  expires_in: number;
  interval: number;
}

export interface AuthResult {
  success: boolean;
  error?: string;
  interval?: number; // Suggested polling interval in seconds when rate limited
}

// Model types
export interface Model {
  id: string;
  display_name: string;
  family: string;
  is_free: boolean;
  multiplier: number;
  context_window: number;
  max_output: number;
  supports_vision: boolean;
  supports_tools: boolean;
  description: string;
  provider?: string;    // Provider name: 'copilot' | 'ollama'
  available?: boolean;  // Whether the model is currently available
}

// Provider status for multi-provider support
export interface ProviderStatus {
  name: string;
  enabled: boolean;
  available: boolean;
  model_count: number;
  error?: string;
}

export interface ModelsResponse {
  models: Model[];
  current: string;
  default: string;
  providers?: ProviderStatus[];  // List of provider statuses
}

export interface Session {
  id: string;
  title?: string;  // May be derived from preview
  preview?: string;  // First 50 chars of first user message
  model?: string;  // Session-specific model (if set)
  scenario?: string;  // Session scenario (chat, cron, channel)
  source?: string;  // Source: chat/cron/delegate (derived from ID prefix)
  selected_skills?: string[];  // Selected skill IDs (undefined/null = all)
  has_pda_checkpoint?: boolean;  // True if session has a saved PDA checkpoint
  is_pda?: boolean;  // True if session was ever a PDA session (persists after completion)
  created_at: string;
  updated_at: string;
  message_count?: number;
}

export interface ToolCallResult {
  name: string;
  arguments?: string; // Tool call arguments as JSON string
  result?: any;
  error?: string;
}

export interface Message {
  id?: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  images?: ImageAttachment[];  // Images attached to user messages
  tool_calls?: ToolCallResult[];
  timestamp?: string;
}

export interface MessagesResponse {
  messages: Message[];
  estimated_tokens: number;  // Backend-computed token estimate
}

// Context analysis types (from GET /sessions/{id}/context)
export interface ContextSegment {
  type: 'compressed_summary' | 'kept_message' | 'history_message' | 'compressed_history';
  role?: string;
  index: number;
  char_count: number;
  estimated_tokens: number;
  content_preview?: string;
  has_tool_calls?: boolean;
  tool_call_count?: number;
  in_context: boolean;
  budgeted: boolean;
}

export interface CompressionInfo {
  has_compression: boolean;
  version?: number;
  summary?: string;
  kept_messages?: number;
  total_tokens?: number;
  original_tokens?: number;
}

export interface SessionContextResponse {
  session_id: string;
  model: string;
  context_window: number;
  segments: ContextSegment[];
  compression: CompressionInfo;
  total_messages: number;
  total_chars: number;
  total_tokens: number;
  effective_count: number;
  effective_chars: number;
  effective_tokens: number;
  budgeted_count: number;
  budgeted_chars: number;
  budgeted_tokens: number;
}

export interface Memory {
  id: string;
  content: string;
  category: string;
  created_at: string;
  relevance?: number;
  // Chunk metadata fields
  chunk_index?: number;
  chunk_total?: number;
  source_file?: string;
  source?: string;
  importance?: number;
  capture_method?: string;
}

export interface Tool {
  name: string;
  description: string;
  type: string;
  source?: string;
  parameters?: Record<string, unknown>;
}

export interface CronJob {
  name: string;  // name is the unique identifier
  schedule: string;
  prompt: string;
  model?: string;       // Model for this cron job
  agent_id?: string;    // Direct delegate to sub-agent
  session_id?: string;  // Associated session ID
  workspace_path?: string;   // Workspace directory path for this cron job
  workspace_alias?: string;  // Workspace alias name
  enabled: boolean;
  next_run?: string;
  last_run?: string;
}

export interface CronExecutingJob {
  name: string;
  session_id: string;
  started_at: string;
  running_for: number; // seconds
}

export interface MCPServer {
  name: string;
  type?: 'stdio' | 'http';  // Used by backend API
  transport?: 'stdio' | 'http' | 'sse';  // Used by backend response
  command?: string;
  args?: string[];
  status: 'running' | 'stopped' | 'error' | 'connected' | 'disconnected';
  tools?: string[];
  tool_count?: number;
  prompt_count?: number;
  url?: string;
  headers?: Record<string, string>;
  error?: string;
}

// ================================================================
// Config Types
// ================================================================

export interface Config {
  gateway?: {
    host?: string;
    port?: number;
  };
  provider?: {
    default?: string;  // 'copilot' | 'ollama' | 'minimax' | 'glm'
    enabled?: string[];  // List of enabled providers: ['copilot', 'ollama', 'minimax', 'glm']
  };
  ollama?: {
    endpoint?: string;  // Ollama API endpoint (default: http://localhost:11434)
    model?: string;     // Default Ollama model
  };
  minimax?: {
    api_key?: string;    // MiniMax API key (masked)
    endpoint?: string;   // MiniMax API endpoint (default: https://api.minimaxi.com/v1)
    model?: string;      // Default MiniMax model
    max_tokens?: number; // Max output tokens
  };
  glm?: {
    api_key?: string;    // GLM API key (masked)
    endpoint?: string;   // GLM API endpoint (default: https://open.bigmodel.cn/api/coding/paas/v4)
    model?: string;      // Default GLM model
    max_tokens?: number; // Max output tokens
  };
  vllm?: {
    api_key?: string;    // Optional API key (if vLLM started with --api-key)
    endpoint?: string;   // vLLM API endpoint (default: http://localhost:8000)
    model?: string;      // Model name (auto-detected if empty)
    max_tokens?: number; // Max output tokens
  };
  memory?: {
    enabled?: boolean;
    auto_capture?: boolean;
    auto_recall?: boolean;
  };
  cron?: {
    enabled?: boolean;
  };
  mcp?: {
    server_enabled?: boolean;
    client_enabled?: boolean;
  };
}

export interface UpdateConfigRequest {
  provider?: {
    default?: string;
    enabled?: string[];  // List of enabled providers: ['copilot', 'ollama', 'minimax', 'glm']
  };
  ollama?: {
    endpoint?: string;
    model?: string;
  };
  minimax?: {
    api_key?: string;
    endpoint?: string;
    model?: string;
    max_tokens?: number;
  };
  glm?: {
    api_key?: string;
    endpoint?: string;
    model?: string;
    max_tokens?: number;
  };
  vllm?: {
    api_key?: string;
    endpoint?: string;
    model?: string;
    max_tokens?: number;
  };
}

export interface APIError {
  code: string;
  message: string;
}

export interface ImageAttachment {
  data: string;       // base64 encoded (without data: prefix)
  mime_type: string;  // e.g. "image/png", "image/jpeg"
  name?: string;      // optional filename
}

export interface ChatRequest {
  session_id: string;
  message: string;
  stream?: boolean;
  images?: ImageAttachment[];  // Pasted/uploaded images
  target_agent?: string;       // Direct delegate to specific sub-agent (skip main LLM)
}

export interface ChatResponse {
  session_id: string;
  message: Message;
}

// Error detail for stream events
export interface ErrorDetail {
  code: string;
  message: string;
  provider?: string;
  retryable: boolean;
  retry_after?: number;
}

// Streaming event types
export interface ApprovalRequestSSEEvent {
  id: string;
  tool_name: string;
  arguments: string;
  reason: string;
  session_id: string;
  expires_at: string;
}

export interface ApprovalResolvedSSEEvent {
  id: string;
  approved: boolean;
  decided_at: string;
}

// PDA checkpoint information from the backend
export interface PDACheckpointInfo {
  has_checkpoint: boolean;
  agent_name?: string;
  interrupt_step?: number;
  interrupt_agent?: string;
  interrupt_reason?: string;
  executed_steps?: string[];
  initial_prompt?: string;
  created_at?: string;
}

// PDA progress event for step execution tracking
export interface PDAProgressSSEEvent {
  agent_name: string;
  step_index: number;
  total_steps: number;
  step_label: string;
  step_type: string;
  phase: 'started' | 'completed' | 'failed';
  stack_depth: number;
  executed_steps?: string[];
  total_tokens?: number;
  model?: string;
  parent_steps?: PDAParentStepInfo[];
}

// Parent frame step info in PDA stack
export interface PDAParentStepInfo {
  agent_name: string;
  step_index: number;
  total_steps: number;
  step_label: string;
}

export interface StreamEvent {
  type: 'content' | 'tool_call' | 'tool_call_update' | 'tool_result' | 'thinking' | 'error' | 'done' | 'truncated' | 'heartbeat' | 'pause' | 'pause_timeout' | 'pause_resumed' | 'approval_request' | 'approval_resolved' | 'pda_progress';
  delta?: string;  // Content delta from backend
  content?: string;  // Also support content for compatibility
  thinking?: string;  // Thinking/reasoning content (temporary display)
  session_id?: string;  // Session ID (for done event)
  tool_call?: {
    name: string;
    arguments?: string;
  };
  tool_call_update?: {
    tool_call_id: string;
    tool_name: string;
    status?: string;  // "running", "completed"
    arguments?: string;
  };
  tool_result?: {
    tool_call_id?: string;
    tool_name?: string;
    ToolName?: string;  // Backend may use PascalCase
    output?: string;
    Output?: string;  // Backend may use PascalCase
    is_error?: boolean;
    IsError?: boolean;  // Backend may use PascalCase
    duration_ms?: number;
  };
  error?: string;
  error_detail?: ErrorDetail;  // Detailed error info for recovery
  // For truncated events
  truncated_reason?: string;  // e.g., "length" for max_tokens limit
  pending_tool_calls?: number;  // Number of pending tool calls when truncated
  // For pause events
  pause_data?: {
    session_id: string;
    pending_tools?: Array<{ id: string; name: string; arguments?: any }>;
    has_user_input?: boolean;
  };
  // Approval events
  approval_request?: ApprovalRequestSSEEvent;
  approval_resolved?: ApprovalResolvedSSEEvent;
  // Multi-agent delegation
  agent_name?: string;   // Name of the sub-agent producing this event
  agent_depth?: number;  // Nesting depth (0 = main agent)
  // PDA progress
  pda_progress?: PDAProgressSSEEvent;
}

// ================================================================
// Channel Types
// ================================================================

export interface ChannelStatus {
  type: string;
  name: string;
  enabled: boolean;
  status: 'running' | 'stopped' | 'error';
  error?: string;
}

export interface IMessageChannelConfig {
  enabled: boolean;
  model?: string;
  selfId?: string;
  trigger: {
    prefix: string;
    caseSensitive: boolean;
    selfTrigger: boolean;
  };
  reply: {
    prefix: string;
    separator: string;
  };
  allowFrom: string[];
}

export interface AppleNotesChannelConfig {
  enabled: boolean;
  model?: string;
  trigger: {
    prefix: string;
    caseSensitive: boolean;
  };
  reply: {
    prefix: string;
    separator: string;
  };
  watchFolder: string;
  archiveFolder?: string;
  pollInterval: string;
}

export interface AppleRemindersChannelConfig {
  enabled: boolean;
  model?: string;
  trigger: {
    prefix: string;
    caseSensitive: boolean;
  };
  reply: {
    prefix: string;
    separator: string;
  };
  watchList: string;
  pollInterval: string;
}

// Union type for all channel configs (extensible)
export type ChannelConfig = IMessageChannelConfig | AppleNotesChannelConfig | AppleRemindersChannelConfig;

// ================================================================
// Skill Types
// ================================================================

export interface Skill {
  id: string;
  name: string;
  description?: string;
  version?: string;
  state: 'registered' | 'active' | 'error';
  source: string;
  tools?: string[];
  hooks?: string[];
  prompts?: string[];
  author?: string;
  path?: string;
  error?: string;
}

export interface SkillListResponse {
  skills: Skill[];
  count: number;
}

// Skill Update Types
export interface SkillVersionInfo {
  skill_id: string;
  local_version: string;
  embed_version: string;
  update_available: boolean;
  local_modified: boolean;
  description?: string;
}

export interface VersionCheckResult {
  updates: SkillVersionInfo[];
  total: number;
  updated_at: string;
}

export interface UpdateOptions {
  force?: boolean;
  backup?: boolean;
}

export interface UpdateResult {
  success: boolean;
  skill_id: string;
  old_version: string;
  new_version: string;
  backup_path?: string;
  error?: string;
}

// ================================================================
// Workspace Types
// ================================================================

export interface Workspace {
  id?: string;
  name?: string;
  session_id?: string;
  path: string;
  alias?: string;
  read_only?: boolean;
  is_default?: boolean;
  description?: string;
  bound_at?: string;
  last_access?: string;
  created_at?: string;
}

export interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  children?: WorkspaceFile[];
}

export interface DirectoryEntry {
  name: string;
  path: string;
  is_dir: boolean;
}

export interface BrowseDirectoryResult {
  path: string;
  parent: string;
  entries: DirectoryEntry[];
}

// ================================================================
// Prompt Types
// ================================================================

export interface Prompt {
  id: string;
  name: string;
  type?: 'system' | 'user' | 'assistant';
  content: string;
  priority?: number;
  enabled: boolean;
  description?: string;
  arguments?: MCPPromptArgument[]; // Support for prompt parameters
  category?: string;
  tags?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface PromptListResponse {
  prompts: Prompt[];
  count: number;
}

// ================================================================
// MCP Prompt Types
// ================================================================

export interface MCPPromptArgument {
  name: string;
  description?: string;
  required?: boolean;
}

export interface MCPPrompt {
  name: string;
  description?: string;
  server: string;
  arguments?: MCPPromptArgument[];
}
export interface MCPPromptMessage {
  role: string;
  content: string;
}

export interface MCPPromptContent {
  description?: string;
  messages: MCPPromptMessage[];
}

// ================================================================
// Session Reconfigure Types
// ================================================================

export interface ReconfigureSessionRequest {
  model?: string;
  workspace_path?: string;
  workspace_alias?: string;
  selected_skills?: string[];
}

export interface ReconfigureSessionResponse {
  id: string;
  model: string;
  workspace_path: string;
  selected_skills: string[] | null;
  message: string;
}

// ================================================================
// Multi-Agent Delegate Types
// ================================================================

// PDA (Pushdown-Automaton) Orchestration Step Types
export type StepType = 'prompt' | 'agent_ref' | 'route';

export interface Step {
  type: StepType;
  label?: string;
  content?: string;   // for 'prompt' steps; also task description for 'agent_ref' and 'route' steps
  agent?: string;     // for 'agent_ref' steps
  prompt?: string;    // for 'route' steps (LLM routing prompt)
  branches?: Record<string, string>; // for 'route' steps: match → target_agent
}

export interface ValidationResult {
  level: 'error' | 'warning';
  code: string;
  message: string;
  agent_name: string;
  step_index: number;
}

/** Lightweight agent descriptor for dropdowns / selectors */
export interface AgentOption {
  name: string;
  tags?: string[];
}

export interface AgentConfig {
  enabled?: boolean;
  stealth?: boolean;      // 隐身：不注入到系统提示词
  entry_point?: boolean;  // 入口：在 @ 引用中优先展示
  description?: string;
  provider?: string;
  model?: string;
  system_prompt?: string;
  tools?: string[];
  tags?: string[];
  max_depth?: number;
  timeout?: string;
  max_iterations?: number;
  temperature?: number;
  // PDA structured orchestration
  steps?: Step[];
  max_recursion?: number;
  draft?: { steps?: Step[]; saved_at: string };
}

export interface DelegationRecord {
  id: string;
  parent_session_id: string;
  child_session_id: string;
  agent_name: string;
  depth: number;
  chain: string; // JSON array string: '["main","researcher"]'
  prompt: string;
  status: 'running' | 'completed' | 'failed' | 'timeout';
  started_at: string;
  completed_at?: string;
  result_length: number;
  tokens_used: number;
  error_message?: string;
  // PDA tracking
  mode?: 'legacy' | 'structured';
  executed_steps?: string[];
  pda_stack_depth?: number;
}