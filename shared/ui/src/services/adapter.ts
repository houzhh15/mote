// ================================================================
// API Adapter Interface - Abstract layer for different backends
// ================================================================

import type {
  ServiceStatus,
  Session,
  MessagesResponse,
  Memory,
  Tool,
  CronJob,
  CronExecutingJob,
  MCPServer,
  Config,
  ChatRequest,
  StreamEvent,
  AuthStatus,
  DeviceCodeResponse,
  AuthResult,
  ChannelStatus,
  ChannelConfig,
  ModelsResponse,
  Skill,
  Workspace,
  WorkspaceFile,
  BrowseDirectoryResult,
  Prompt,
  MCPPrompt,
  MCPPromptContent,
  ReconfigureSessionRequest,
  ReconfigureSessionResponse,
  VersionCheckResult,
  UpdateOptions,
  UpdateResult,
  AgentConfig,
  DelegationRecord,
} from '../types';

import type {
  PolicyConfig,
  PolicyStatus,
  PolicyCheckRequest,
  PolicyCheckResponse,
  ApprovalListResponse,
} from '../types/policy';

// Re-export policy types for convenience
export type {
  PolicyConfig,
  PolicyStatus,
  PolicyCheckRequest,
  PolicyCheckResponse,
  ApprovalListResponse,
} from '../types/policy';

/**
 * Memory sync result
 */
export interface MemorySyncResult {
  synced?: number;
  created?: number;
  updated?: number;
  deleted?: number;
  errors?: number;
  duration?: string;
  status: string;
}

/**
 * Memory statistics
 */
export interface MemoryStats {
  total: number;
  by_category?: Record<string, number>;
  by_capture_method?: Record<string, number>;
  auto_capture_today?: number;
  auto_recall_today?: number;
  index_entries?: number;
  content_sections?: number;
  watch_enabled?: boolean;
  capture_enabled?: boolean;
  capture_mode?: string;
}

/**
 * Memory export result
 */
export interface MemoryExportResult {
  count: number;
  exported: string;
  memories: Array<{ id: string; content: string; source: string; created_at: string }>;
}

/**
 * API Adapter interface - implemented by Wails and HTTP adapters
 * This allows the same UI components to work with different backends
 */
export interface APIAdapter {
  // ============== Status Service ==============
  getStatus(): Promise<ServiceStatus>;
  
  // ============== Chat Service ==============
  /**
   * Send a chat message and receive streaming response
   * @param request Chat request with session_id and message
   * @param onEvent Callback for each stream event
   * @param signal Optional AbortSignal to cancel the request
   * @returns Promise that resolves when stream is complete
   */
  chat(
    request: ChatRequest,
    onEvent: (event: StreamEvent) => void,
    signal?: AbortSignal
  ): Promise<void>;

  /**
   * Cancel a running chat session on the backend.
   * This stops the runner execution for the given session.
   */
  cancelChat?(sessionId: string): Promise<void>;
  
  // ============== Session Service ==============
  getSessions(): Promise<Session[]>;
  getSession?(sessionId: string): Promise<Session>;
  getSessionMessages(sessionId: string): Promise<MessagesResponse>;
  createSession(title?: string, scenario?: string): Promise<Session>;
  updateSession?(sessionId: string, updates: { title?: string }): Promise<Session>;
  deleteSession(sessionId: string): Promise<void>;
  batchDeleteSessions?(ids: string[]): Promise<{ deleted: number; total: number }>;
  
  // ============== Pause Control Service ==============
  /**
   * Pause execution before next tool call
   */
  pauseSession?(sessionId: string): Promise<void>;
  /**
   * Resume execution (optionally with user input)
   */
  resumeSession?(sessionId: string, userInput?: string): Promise<void>;
  /**
   * Get pause status of a session
   */
  getPauseStatus?(sessionId: string): Promise<{ paused: boolean; paused_at?: string; timeout_remaining?: number; pending_tools?: string[] }>;
  
  // ============== Memory Service ==============
  getMemories(options?: { limit?: number; offset?: number; category?: string }): Promise<{ memories: Memory[]; total: number; limit: number; offset: number }>;
  searchMemories(query: string, limit?: number): Promise<Memory[]>;
  createMemory?(content: string, category?: string): Promise<Memory>;
  updateMemory?(id: string, content: string, category?: string): Promise<Memory>;
  deleteMemory(id: string): Promise<void>;
  
  /**
   * Sync memories from markdown files
   */
  syncMemory?(): Promise<MemorySyncResult>;
  
  /**
   * Get memory statistics
   */
  getMemoryStats?(): Promise<MemoryStats>;
  
  /**
   * Get daily log for a date (defaults to today)
   */
  getDailyLog?(date?: string): Promise<{ date: string; content: string }>;
  
  /**
   * Append content to today's daily log
   */
  appendDailyLog?(content: string, section?: string): Promise<void>;
  
  /**
   * Export all memories
   */
  exportMemories?(format?: string): Promise<MemoryExportResult>;
  
  /**
   * Import memories from data
   */
  importMemories?(memories: Array<{ content: string; source?: string }>): Promise<{ imported: number; total: number }>;
  
  /**
   * Batch delete memories by IDs
   */
  batchDeleteMemories?(ids: string[]): Promise<{ deleted: number; total: number }>;
  
  // ============== Tools Service ==============
  getTools(): Promise<Tool[]>;
  
  /**
   * Open tools directory in file manager
   */
  openToolsDir?(target: 'user' | 'workspace'): Promise<void>;
  
  /**
   * Create a new tool template
   */
  createTool?(name: string, runtime: string, target: 'user' | 'workspace'): Promise<{ path: string }>;
  
  // ============== Cron Service ==============
  getCronJobs(): Promise<CronJob[]>;
  createCronJob(job: Partial<CronJob>): Promise<CronJob>;
  updateCronJob(id: string, updates: Partial<CronJob>): Promise<CronJob>;
  toggleCronJob(id: string, enabled: boolean): Promise<void>;
  deleteCronJob(id: string): Promise<void>;
  /** Get currently executing cron jobs */
  getCronExecuting?(): Promise<CronExecutingJob[]>;
  
  // ============== MCP Service ==============
  getMCPServers(): Promise<MCPServer[]>;
  createMCPServer(server: Partial<MCPServer>): Promise<MCPServer>;
  updateMCPServer(name: string, updates: Partial<MCPServer>): Promise<MCPServer>;
  startMCPServer(name: string): Promise<void>;
  stopMCPServer(name: string): Promise<void>;
  deleteMCPServer(name: string): Promise<void>;
  /**
   * Import multiple MCP servers from JSON config
   * Format: { "server_name": { "type": "http", "url": "...", "headers": {...} }, ... }
   */
  importMCPServers(config: Record<string, unknown>): Promise<{ imported: string[]; errors: Record<string, string> }>;
  
  // ============== Config Service ==============
  getConfig(): Promise<Config>;
  updateConfig(config: Partial<Config>): Promise<void>;
  
  // ============== Models Service ==============
  getModels(): Promise<ModelsResponse>;
  setCurrentModel(modelId: string): Promise<void>;
  setSessionModel?(sessionId: string, modelId: string): Promise<void>;
  setSessionSkills?(sessionId: string, skillIds: string[]): Promise<void>;

  /**
   * Atomically reconfigure a session's model, workspace, and/or skills.
   * This is a major operation that cleans up ALL runtime resources (ACP sessions, caches, etc).
   * Use this when switching parameters during an active chat session.
   */
  reconfigureSession?(sessionId: string, config: ReconfigureSessionRequest): Promise<ReconfigureSessionResponse>;
  
  // ============== Channel Service ==============
  /**
   * Get all channel statuses
   */
  getChannels(): Promise<ChannelStatus[]>;
  
  /**
   * Get channel configuration
   */
  getChannelConfig(channelType: string): Promise<ChannelConfig>;
  
  /**
   * Update channel configuration
   */
  updateChannelConfig(channelType: string, config: Partial<ChannelConfig>): Promise<void>;
  
  /**
   * Start a channel
   */
  startChannel(channelType: string): Promise<void>;
  
  /**
   * Stop a channel
   */
  stopChannel(channelType: string): Promise<void>;
  
  // ============== Auth Service (GUI only) ==============
  /**
   * Get current authentication status
   * Only available in GUI mode (Wails)
   */
  getAuthStatus?(): Promise<AuthStatus>;
  
  /**
   * Start device login flow (GitHub Device OAuth)
   */
  startDeviceLogin?(): Promise<DeviceCodeResponse>;
  
  /**
   * Poll for device login completion
   */
  pollDeviceLogin?(deviceCode: string): Promise<AuthResult>;
  
  /**
   * Logout from current session
   */
  logout?(): Promise<void>;
  
  // ============== App Control (GUI only) ==============
  /**
   * Restart the mote service
   */
  restartService?(): Promise<void>;
  
  /**
   * Quit the application
   */
  quit?(): Promise<void>;
  
  // ============== Skills Service ==============
  /**
   * Get all skills
   */
  getSkills?(): Promise<Skill[]>;
  
  /**
   * Activate a skill
   */
  activateSkill?(skillId: string, config?: Record<string, unknown>): Promise<void>;
  
  /**
   * Deactivate a skill
   */
  deactivateSkill?(skillId: string): Promise<void>;
  
  /**
   * Reload all skills from disk
   */
  reloadSkills?(): Promise<void>;
  
  /**
   * Check for skill updates
   */
  checkSkillUpdates?(): Promise<VersionCheckResult>;
  
  /**
   * Update a skill to latest version
   */
  updateSkill?(skillId: string, options?: UpdateOptions): Promise<UpdateResult>;
  
  /**
   * Open skills directory in file manager
   */
  openSkillsDir?(target: 'user' | 'workspace'): Promise<void>;
  
  /**
   * Create a new skill template
   */
  createSkill?(name: string, target: 'user' | 'workspace'): Promise<{ path: string }>;
  
  /**
   * Check for available updates for builtin skills
   */
  checkSkillUpdates?(): Promise<VersionCheckResult>;
  
  /**
   * Update a builtin skill to the latest embedded version
   */
  updateSkill?(skillId: string, options?: UpdateOptions): Promise<UpdateResult>;
  
  // ============== Workspace Service ==============
  /**
   * Get all workspace bindings
   */
  getWorkspaces?(): Promise<Workspace[]>;
  
  /**
   * Bind a workspace to a session
   */
  bindWorkspace?(sessionId: string, path: string, alias?: string, readOnly?: boolean): Promise<Workspace>;
  
  /**
   * Unbind a workspace from a session
   */
  unbindWorkspace?(sessionId: string): Promise<void>;
  
  /**
   * List files in a workspace
   */
  listWorkspaceFiles?(sessionId: string, path?: string): Promise<WorkspaceFile[]>;

  /**
   * Browse system directories for the directory picker.
   * If path is empty, returns home directory contents (or drive list on Windows).
   */
  browseDirectory?(path?: string): Promise<BrowseDirectoryResult>;
  
  // ============== Prompts Service ==============
  /**
   * Get all user prompts
   */
  getPrompts?(): Promise<Prompt[]>;
  
  /**
   * Get all MCP prompts from connected servers
   */
  getMCPPrompts?(): Promise<MCPPrompt[]>;
  
  /**
   * Get MCP prompt content by calling the MCP server
   */
  getMCPPromptContent?(server: string, name: string, args?: Record<string, string>): Promise<MCPPromptContent>;
  
  /**
   * Create a new prompt
   */
  createPrompt?(prompt: Partial<Prompt>): Promise<Prompt>;
  
  /**
   * Update a prompt
   */
  updatePrompt?(id: string, updates: Partial<Prompt>): Promise<Prompt>;
  
  /**
   * Delete a prompt
   */
  deletePrompt?(id: string): Promise<void>;
  
  /**
   * Toggle prompt enabled state
   */
  togglePrompt?(id: string): Promise<void>;
  
  /**
   * Open prompts directory in file manager
   */
  openPromptsDir?(target: 'user' | 'workspace'): Promise<void>;
  
  /**
   * Reload prompts from configured directories
   */
  reloadPrompts?(): Promise<void>;
  
  /**
   * Render a prompt with variables
   */
  renderPrompt?(id: string, variables: Record<string, string>): Promise<{ content: string }>;
  
  // ============== Agents Service (Multi-Agent Delegate) ==============
  /**
   * Get all configured delegate agents
   */
  getAgents?(): Promise<Record<string, AgentConfig>>;
  
  /**
   * Get a single agent configuration by name
   */
  getAgent?(name: string): Promise<{ name: string; config: AgentConfig }>;
  
  /**
   * Add a new delegate agent
   */
  addAgent?(name: string, agent: AgentConfig): Promise<{ name: string; agent: AgentConfig }>;
  
  /**
   * Update an existing delegate agent
   */
  updateAgent?(name: string, agent: AgentConfig): Promise<{ name: string; agent: AgentConfig }>;
  
  /**
   * Delete a delegate agent
   */
  deleteAgent?(name: string): Promise<void>;
  
  /**
   * Get delegation records for a session
   */
  getSessionDelegations?(sessionId: string): Promise<DelegationRecord[]>;

  /**
   * Get recent delegation records across all sessions
   */
  getDelegations?(limit?: number): Promise<DelegationRecord[]>;
  
  /**
   * Get a single delegation record by ID
   */
  getDelegation?(id: string): Promise<DelegationRecord>;

  /**
   * Batch delete delegation records
   */
  batchDeleteDelegations?(ids: string[]): Promise<{ deleted: number; total: number }>;
  
  // ============== Security Policy Service (M08B) ==============
  /**
   * Get the full policy configuration
   */
  getPolicyConfig?(): Promise<PolicyConfig>;
  
  /**
   * Update the policy configuration
   */
  updatePolicyConfig?(config: PolicyConfig): Promise<{ success: boolean }>;
  
  /**
   * Get policy status summary
   */
  getPolicyStatus?(): Promise<PolicyStatus>;
  
  /**
   * Check if a tool call would be allowed
   */
  checkPolicy?(request: PolicyCheckRequest): Promise<PolicyCheckResponse>;
  
  /**
   * Get pending approval requests
   */
  getApprovals?(): Promise<ApprovalListResponse>;
  
  /**
   * Respond to a pending approval request
   */
  respondApproval?(id: string, approved: boolean, reason?: string, modifiedArguments?: string): Promise<{ success: boolean }>;
  
  /**
   * Check if running in GUI mode (Wails)
   */
  isGUIMode(): boolean;
}

/**
 * No-op adapter for initialization
 */
export const createNoopAdapter = (): APIAdapter => ({
  getStatus: async () => ({ running: false, port: 0, version: 'unknown', uptime: 0 }),
  chat: async () => {},
  getSessions: async () => [],
  getSessionMessages: async () => ({ messages: [], estimated_tokens: 0 }),
  createSession: async () => ({ id: '', title: '', created_at: '', updated_at: '' }),
  deleteSession: async () => {},
  getMemories: async () => ({ memories: [], total: 0, limit: 100, offset: 0 }),
  searchMemories: async () => [],
  deleteMemory: async () => {},
  syncMemory: async () => ({ status: 'ok', synced: 0 }),
  getMemoryStats: async () => ({ total: 0 }),
  getDailyLog: async () => ({ date: new Date().toISOString().slice(0, 10), content: '' }),
  appendDailyLog: async () => {},
  exportMemories: async () => ({ count: 0, exported: new Date().toISOString(), memories: [] }),
  importMemories: async () => ({ imported: 0, total: 0 }),
  batchDeleteMemories: async () => ({ deleted: 0, total: 0 }),
  getTools: async () => [],
  getCronJobs: async () => [],
  createCronJob: async () => ({ id: '', name: '', schedule: '', prompt: '', enabled: false }),
  updateCronJob: async () => ({ id: '', name: '', schedule: '', prompt: '', enabled: false }),
  toggleCronJob: async () => {},
  deleteCronJob: async () => {},
  getCronExecuting: async () => [],
  getMCPServers: async () => [],
  createMCPServer: async () => ({ name: '', transport: 'stdio', command: '', status: 'stopped' }),
  updateMCPServer: async () => ({ name: '', transport: 'stdio', command: '', status: 'stopped' }),
  startMCPServer: async () => {},
  stopMCPServer: async () => {},
  deleteMCPServer: async () => {},
  importMCPServers: async () => ({ imported: [], errors: {} }),
  getConfig: async () => ({}),
  updateConfig: async () => {},
  getModels: async () => ({ models: [], current: '', default: '', providers: [] }),
  setCurrentModel: async () => {},
  getChannels: async () => [],
  getChannelConfig: async () => ({ enabled: false, trigger: { prefix: '', caseSensitive: false, selfTrigger: false }, reply: { prefix: '', separator: '' }, allowFrom: [] }),
  updateChannelConfig: async () => {},
  startChannel: async () => {},
  stopChannel: async () => {},
  // Skills
  getSkills: async () => [],
  activateSkill: async () => {},
  deactivateSkill: async () => {},
  reloadSkills: async () => {},
  checkSkillUpdates: async () => ({ updates: [], total: 0, updated_at: new Date().toISOString() }),
  updateSkill: async () => ({ success: false, skill_id: '', old_version: '', new_version: '', error: 'Not implemented' }),
  openSkillsDir: async () => {},
  createSkill: async () => ({ path: '' }),
  // Tools
  openToolsDir: async () => {},
  createTool: async () => ({ path: '' }),
  // Workspaces
  getWorkspaces: async () => [],
  bindWorkspace: async () => ({ path: '' }),
  unbindWorkspace: async () => {},
  listWorkspaceFiles: async () => [],
  // Prompts
  getPrompts: async () => [],
  getMCPPrompts: async () => [],
  getMCPPromptContent: async () => ({ messages: [] }),
  createPrompt: async () => ({ id: '', name: '', content: '', enabled: true }),
  updatePrompt: async () => ({ id: '', name: '', content: '', enabled: true }),
  deletePrompt: async () => {},
  togglePrompt: async () => {},
  openPromptsDir: async () => {},
  reloadPrompts: async () => {},
  renderPrompt: async () => ({ content: '' }),
  // Agents
  getAgents: async () => ({}),
  getAgent: async () => ({ name: '', config: {} }),
  addAgent: async () => ({ name: '', agent: {} }),
  updateAgent: async () => ({ name: '', agent: {} }),
  deleteAgent: async () => {},
  getSessionDelegations: async () => [],
  getDelegations: async () => [],
  getDelegation: async () => ({ id: '', parent_session_id: '', child_session_id: '', agent_name: '', depth: 0, chain: '[]', prompt: '', status: 'completed' as const, started_at: '', result_length: 0, tokens_used: 0 }),
  batchDeleteDelegations: async () => ({ deleted: 0, total: 0 }),
  // Security Policy (M08B)
  getPolicyConfig: async () => ({ default_allow: true, require_approval: false, allowlist: [], blocklist: [], dangerous_ops: [], param_rules: {}, scrub_rules: [], block_message_template: '', circuit_breaker_threshold: 3 }),
  updatePolicyConfig: async () => ({ success: true }),
  getPolicyStatus: async () => ({ default_allow: true, require_approval: false, blocklist_count: 0, allowlist_count: 0, dangerous_rules_count: 0, param_rules_count: 0 }),
  checkPolicy: async () => ({ tool: '', allowed: true, require_approval: false, blocked: false }),
  getApprovals: async () => ({ pending: [], count: 0 }),
  respondApproval: async () => ({ success: true }),
  cancelChat: async () => {},
  isGUIMode: () => false,
});
