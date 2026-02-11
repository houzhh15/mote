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

export interface ScenarioModels {
  chat: string;
  cron: string;
  channel: string;
}

export interface Session {
  id: string;
  title?: string;  // May be derived from preview
  preview?: string;  // First 50 chars of first user message
  model?: string;  // Session-specific model (if set)
  scenario?: string;  // Session scenario (chat, cron, channel)
  selected_skills?: string[];  // Selected skill IDs (undefined/null = all)
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
  tool_calls?: ToolCallResult[];
  timestamp?: string;
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
  session_id?: string;  // Associated session ID
  enabled: boolean;
  next_run?: string;
  last_run?: string;
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
    default?: string;  // 'copilot' | 'ollama'
    enabled?: string[];  // List of enabled providers: ['copilot', 'ollama']
  };
  ollama?: {
    endpoint?: string;  // Ollama API endpoint (default: http://localhost:11434)
    model?: string;     // Default Ollama model
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
    enabled?: string[];  // List of enabled providers: ['copilot', 'ollama']
  };
  ollama?: {
    endpoint?: string;
    model?: string;
  };
}

export interface APIError {
  code: string;
  message: string;
}

export interface ChatRequest {
  session_id: string;
  message: string;
  stream?: boolean;
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
export interface StreamEvent {
  type: 'content' | 'tool_call' | 'tool_call_update' | 'tool_result' | 'thinking' | 'error' | 'done' | 'truncated' | 'heartbeat';
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