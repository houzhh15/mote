// ================================================================
// API Adapter Interface - Abstract layer for different backends
// ================================================================

import type {
  ServiceStatus,
  Session,
  Message,
  Memory,
  Tool,
  CronJob,
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
} from '../types';

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
  
  // ============== Session Service ==============
  getSessions(): Promise<Session[]>;
  getSession?(sessionId: string): Promise<Session>;
  getSessionMessages(sessionId: string): Promise<Message[]>;
  createSession(title?: string, scenario?: string): Promise<Session>;
  updateSession?(sessionId: string, updates: { title?: string }): Promise<Session>;
  deleteSession(sessionId: string): Promise<void>;
  
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
  getMemories(options?: { limit?: number; offset?: number }): Promise<{ memories: Memory[]; total: number; limit: number; offset: number }>;
  searchMemories(query: string, limit?: number): Promise<Memory[]>;
  createMemory?(content: string, category?: string): Promise<Memory>;
  updateMemory?(id: string, content: string, category?: string): Promise<Memory>;
  deleteMemory(id: string): Promise<void>;
  
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
  getSessionMessages: async () => [],
  createSession: async () => ({ id: '', title: '', created_at: '', updated_at: '' }),
  deleteSession: async () => {},
  getMemories: async () => ({ memories: [], total: 0, limit: 100, offset: 0 }),
  searchMemories: async () => [],
  deleteMemory: async () => {},
  getTools: async () => [],
  getCronJobs: async () => [],
  createCronJob: async () => ({ id: '', name: '', schedule: '', prompt: '', enabled: false }),
  updateCronJob: async () => ({ id: '', name: '', schedule: '', prompt: '', enabled: false }),
  toggleCronJob: async () => {},
  deleteCronJob: async () => {},
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
  isGUIMode: () => false,
});
