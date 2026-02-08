// ================================================================
// Wails Adapter - Implements APIAdapter via unified CallAPI proxy
// ================================================================
// 
// This adapter uses a unified CallAPI method to proxy all HTTP requests
// through the Go backend, making the code structure identical to http.ts.
// This enables full code reuse between Wails GUI and Web UI.
//
// Reference: WAILS_GUI_WEB_UI_REUSE.md
// ================================================================

import type { APIAdapter } from './adapter';
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
  Prompt,
  MCPPrompt,
  MCPPromptContent,
  Skill,
  Workspace,
  WorkspaceFile,
} from '../types';

// Extend window with Wails runtime
declare global {
  interface Window {
    runtime?: {
      EventsOn(eventName: string, callback: (...data: unknown[]) => void): () => void;
      EventsOff(eventName: string, ...additionalEventNames: string[]): void;
    };
  }
}

// Memory search result type from backend
interface MemoryResult {
  id: string;
  content: string;
  score?: number;
  source?: string;
  created_at: string;
  category?: string;
  importance?: number;
  capture_method?: string;
}

// ================================================================
// Wails App Interface - Minimal bindings for GUI
// ================================================================

/**
 * WailsApp interface defines the Go methods exposed to frontend.
 * 
 * Key design principles:
 * 1. CallAPI is the unified entry point for all HTTP API calls
 * 2. ChatStream provides real-time streaming via Wails events
 * 3. GUI-specific methods (Auth, App control) are kept separate
 * 4. Service status includes local process info
 */
interface WailsApp {
  // Unified API Proxy - Single entry point for all backend calls
  // Returns []byte as base64 encoded string (Wails Go binding behavior)
  // bodyJSON: JSON string (empty string for no body), NOT object
  CallAPI(method: string, path: string, bodyJSON: string): Promise<string>;
  
  // Streaming Chat - Streams events via Wails event system
  // Events are emitted as "chat:stream" with JSON event data
  ChatStream(message: string, sessionID: string): Promise<void>;
  
  // GUI-specific: Service status (includes local process/IPC info)
  GetServiceStatus(): Promise<Record<string, unknown>>;
  
  // GUI-specific: Authentication (GitHub Copilot OAuth)
  GetAuthStatus(): Promise<{
    authenticated: boolean;
    token_masked?: string;
    copilot_verified?: boolean;
    error?: string;
  }>;
  StartDeviceLogin(): Promise<{
    device_code: string;
    user_code: string;
    verification_uri: string;
    expires_in: number;
    interval: number;
  }>;
  PollDeviceLogin(deviceCode: string): Promise<{
    success: boolean;
    error?: string;
    interval?: number;
  }>;
  Logout(): Promise<void>;
  
  // GUI-specific: App control
  RestartService(): Promise<void>;
  Quit(): Promise<void>;
}

// ================================================================
// Response Parser - Handle Wails Go binding response format
// ================================================================

/**
 * Parse JSON response from Wails CallAPI.
 * Wails returns Go []byte as base64 encoded string.
 * 
 * Note: atob() returns Latin-1 string, we need to convert to UTF-8 for CJK characters.
 */
function parseResponse<T>(data: string | number[]): T {
  // Handle empty response (e.g., 204 No Content)
  if (!data || (typeof data === 'string' && data === '') || (Array.isArray(data) && data.length === 0)) {
    return undefined as T;
  }

  let bytes: Uint8Array;
  if (typeof data === 'string') {
    // Base64 encoded string (Wails default behavior)
    // atob() returns Latin-1 string, convert each char to byte
    const binaryStr = atob(data);
    bytes = new Uint8Array(binaryStr.length);
    for (let i = 0; i < binaryStr.length; i++) {
      bytes[i] = binaryStr.charCodeAt(i);
    }
  } else {
    // Fallback: Uint8Array as number[]
    bytes = new Uint8Array(data);
  }

  // Handle empty bytes (after base64 decode)
  if (bytes.length === 0) {
    return undefined as T;
  }

  // Decode UTF-8 bytes to string
  const text = new TextDecoder('utf-8').decode(bytes);
  
  // Handle empty text
  if (!text || text.trim() === '') {
    return undefined as T;
  }

  return JSON.parse(text);
}

// ================================================================
// Wails Adapter Factory
// ================================================================

export function createWailsAdapter(app: WailsApp): APIAdapter {
  /**
   * Unified API call helper - mirrors fetchJSON in http.ts
   * All API calls go through this single method.
   * Note: body is serialized to JSON string for Wails compatibility
   */
  const callAPI = async <T>(
    method: string, 
    path: string, 
    body?: unknown
  ): Promise<T> => {
    // Serialize body to JSON string (empty string if no body)
    const bodyJSON = body !== undefined ? JSON.stringify(body) : '';
    const data = await app.CallAPI(method, path, bodyJSON);
    return parseResponse<T>(data);
  };

  return {
    // ============== Status Service ==============
    getStatus: async (): Promise<ServiceStatus> => {
      // Use GUI-specific method for local process info
      const status = await app.GetServiceStatus();
      return status as unknown as ServiceStatus;
    },

    // ============== Chat Service ==============
    // Uses ChatStream for real-time streaming via Wails events
    chat: async (
      request: ChatRequest,
      onEvent: (event: StreamEvent) => void,
      signal?: AbortSignal
    ): Promise<void> => {
      // Set up event listener for stream events
      const cleanup = window.runtime?.EventsOn('chat:stream', (...data: unknown[]) => {
        // 如果已取消，忽略后续事件
        if (signal?.aborted) return;
        try {
          const eventData = data[0] as string;
          const event = JSON.parse(eventData) as StreamEvent;
          onEvent(event);
        } catch {
          // Ignore parse errors
        }
      });
      
      try {
        // 如果已取消，直接返回
        if (signal?.aborted) return;
        // Start streaming chat - this returns when stream is complete
        await app.ChatStream(request.message, request.session_id || '');
      } finally {
        // Clean up event listener
        if (cleanup) {
          cleanup();
        } else if (window.runtime?.EventsOff) {
          window.runtime.EventsOff('chat:stream');
        }
      }
    },

    // ============== Session Service ==============
    getSessions: async (): Promise<Session[]> => {
      const data = await callAPI<{ sessions: Session[] }>('GET', '/api/v1/sessions');
      return data.sessions || [];
    },

    getSessionMessages: async (sessionId: string): Promise<Message[]> => {
      const data = await callAPI<{ messages: Message[] }>(
        'GET', 
        `/api/v1/sessions/${sessionId}/messages`
      );
      return data.messages || [];
    },

    createSession: async (title?: string, scenario?: string): Promise<Session> => {
      return callAPI<Session>('POST', '/api/v1/sessions', { title: title || '', scenario: scenario || 'chat' });
    },

    getSession: async (sessionId: string): Promise<Session> => {
      return callAPI<Session>('GET', `/api/v1/sessions/${sessionId}`);
    },

    deleteSession: async (sessionId: string): Promise<void> => {
      await callAPI('DELETE', `/api/v1/sessions/${sessionId}`);
    },

    updateSession: async (sessionId: string, updates: { title?: string }): Promise<Session> => {
      return callAPI<Session>('PUT', `/api/v1/sessions/${sessionId}`, updates);
    },

    // ============== Memory Service ==============
    getMemories: async (query?: string): Promise<Memory[]> => {
      const params = new URLSearchParams();
      if (query) params.set('q', query);
      const queryString = params.toString();
      const path = `/api/v1/memory${queryString ? '?' + queryString : ''}`;
      const data = await callAPI<{ memories: Memory[] }>('GET', path);
      return data.memories || [];
    },

    searchMemories: async (query: string, limit?: number): Promise<Memory[]> => {
      const topK = limit || 10;
      const data = await callAPI<{ results: MemoryResult[] }>(
        'POST', 
        '/api/v1/memory/search',
        { query, top_k: topK }
      );
      // Convert MemoryResult to Memory format
      return (data.results || []).map(r => ({
        id: r.id,
        content: r.content,
        category: r.category || 'other',
        created_at: r.created_at,
        importance: r.importance,
      }));
    },

    createMemory: async (content: string, category?: string): Promise<Memory> => {
      const data = await callAPI<{ id: string; category: string }>('POST', '/api/v1/memory', {
        content,
        category,
      });
      return {
        id: data.id,
        content,
        category: data.category || category || 'other',
        created_at: new Date().toISOString(),
      };
    },

    updateMemory: async (id: string, content: string, category?: string): Promise<Memory> => {
      const response = await callAPI<{
        id: string;
        content: string;
        category: string;
        created_at: string;
        source?: string;
        importance?: number;
        capture_method?: string;
      }>('PUT', `/api/v1/memory/${id}`, { content, category });
      return {
        id: response.id,
        content: response.content,
        category: response.category,
        created_at: response.created_at,
      };
    },

    deleteMemory: async (id: string): Promise<void> => {
      // callAPI handles 204 No Content response
      await callAPI('DELETE', `/api/v1/memory/${id}`);
    },

    // ============== Tools Service ==============
    getTools: async (): Promise<Tool[]> => {
      const data = await callAPI<{ tools: Tool[] }>('GET', '/api/v1/tools');
      return data.tools || [];
    },

    openToolsDir: async (target: 'user' | 'workspace'): Promise<void> => {
      await callAPI('POST', '/api/v1/tools/open', { target });
    },

    createTool: async (name: string, runtime: string, target: 'user' | 'workspace'): Promise<{ path: string }> => {
      return callAPI('POST', '/api/v1/tools/create', { name, runtime, target });
    },

    // ============== Skills Service ==============
    getSkills: async (): Promise<Skill[]> => {
      const data = await callAPI<{ skills: Skill[] }>('GET', '/api/v1/skills');
      return data.skills || [];
    },

    activateSkill: async (skillId: string, config?: Record<string, unknown>): Promise<void> => {
      await callAPI('POST', `/api/v1/skills/${skillId}/activate`, config || {});
    },

    deactivateSkill: async (skillId: string): Promise<void> => {
      await callAPI('POST', `/api/v1/skills/${skillId}/deactivate`);
    },

    reloadSkills: async (): Promise<void> => {
      await callAPI('POST', '/api/v1/skills/reload');
    },

    openSkillsDir: async (target: 'user' | 'workspace'): Promise<void> => {
      await callAPI('POST', '/api/v1/skills/open', { target });
    },

    createSkill: async (name: string, target: 'user' | 'workspace'): Promise<{ path: string }> => {
      return callAPI('POST', '/api/v1/skills/create', { name, target });
    },

    // ============== Cron Service ==============
    getCronJobs: async (): Promise<CronJob[]> => {
      const data = await callAPI<{ jobs: CronJob[] }>('GET', '/api/v1/cron/jobs');
      return data.jobs || [];
    },

    createCronJob: async (job: Partial<CronJob>): Promise<CronJob> => {
      const data = await callAPI<{ job: CronJob }>('POST', '/api/v1/cron/jobs', job);
      return data.job;
    },

    updateCronJob: async (id: string, updates: Partial<CronJob>): Promise<CronJob> => {
      const data = await callAPI<{ job: CronJob }>('PUT', `/api/v1/cron/jobs/${id}`, updates);
      return data.job;
    },

    toggleCronJob: async (id: string, enabled: boolean): Promise<void> => {
      await callAPI('PUT', `/api/v1/cron/jobs/${id}`, { enabled });
    },

    deleteCronJob: async (id: string): Promise<void> => {
      await callAPI('DELETE', `/api/v1/cron/jobs/${id}`);
    },

    // ============== MCP Service ==============
    getMCPServers: async (): Promise<MCPServer[]> => {
      const data = await callAPI<{ servers: MCPServer[] }>('GET', '/api/v1/mcp/servers');
      return data.servers || [];
    },

    createMCPServer: async (server: Partial<MCPServer>): Promise<MCPServer> => {
      const data = await callAPI<{ server: MCPServer }>(
        'POST', 
        '/api/v1/mcp/servers', 
        server
      );
      return data.server;
    },

    startMCPServer: async (name: string): Promise<void> => {
      await callAPI('POST', `/api/v1/mcp/servers/${name}/restart`);
    },

    stopMCPServer: async (name: string): Promise<void> => {
      await callAPI('POST', `/api/v1/mcp/servers/${name}/stop`);
    },

    deleteMCPServer: async (name: string): Promise<void> => {
      await callAPI('DELETE', `/api/v1/mcp/servers/${name}`);
    },

    updateMCPServer: async (name: string, updates: Partial<MCPServer>): Promise<MCPServer> => {
      const data = await callAPI<{ Name: string; Status: string; Transport: string; ToolCount?: number; Error?: string }>(
        'PUT',
        `/api/v1/mcp/servers/${encodeURIComponent(name)}`,
        updates
      );
      return {
        name: data.Name,
        status: data.Status,
        transport: data.Transport,
        tool_count: data.ToolCount,
        error: data.Error,
      } as MCPServer;
    },

    importMCPServers: async (config: Record<string, unknown>): Promise<{ imported: string[]; errors: Record<string, string> }> => {
      return callAPI('POST', '/api/v1/mcp/servers/import', config);
    },

    // ============== Prompts Service ==============
    getPrompts: async (): Promise<Prompt[]> => {
      const data = await callAPI<{ prompts: Prompt[] }>('GET', '/api/v1/prompts');
      return data.prompts || [];
    },

    getMCPPrompts: async (): Promise<MCPPrompt[]> => {
      const data = await callAPI<{ prompts: MCPPrompt[] }>('GET', '/api/v1/mcp/prompts');
      return data.prompts || [];
    },

    getMCPPromptContent: async (server: string, name: string, args?: Record<string, string>): Promise<MCPPromptContent> => {
      return callAPI<MCPPromptContent>(
        'POST',
        `/api/v1/mcp/prompts/${encodeURIComponent(server)}/${encodeURIComponent(name)}`,
        { arguments: args }
      );
    },

    createPrompt: async (prompt: Partial<Prompt>): Promise<Prompt> => {
      return callAPI<Prompt>('POST', '/api/v1/prompts', prompt);
    },

    updatePrompt: async (id: string, updates: Partial<Prompt>): Promise<Prompt> => {
      return callAPI<Prompt>('PUT', `/api/v1/prompts/${id}`, updates);
    },

    deletePrompt: async (id: string): Promise<void> => {
      await callAPI('DELETE', `/api/v1/prompts/${id}`);
    },

    togglePrompt: async (id: string): Promise<void> => {
      await callAPI('POST', `/api/v1/prompts/${id}/toggle`);
    },

    openPromptsDir: async (target: 'user' | 'workspace'): Promise<void> => {
      await callAPI('POST', '/api/v1/prompts/open', { target });
    },

    // ============== Workspace Service ==============
    getWorkspaces: async (): Promise<Workspace[]> => {
      const data = await callAPI<{ workspaces: Workspace[] }>('GET', '/api/v1/workspaces');
      return data.workspaces || [];
    },

    bindWorkspace: async (sessionId: string, path: string, alias?: string, readOnly?: boolean): Promise<Workspace> => {
      return callAPI<Workspace>('POST', '/api/v1/workspaces', {
        session_id: sessionId,
        path,
        alias,
        read_only: readOnly,
      });
    },

    unbindWorkspace: async (sessionId: string): Promise<void> => {
      await callAPI('DELETE', `/api/v1/workspaces/${sessionId}`);
    },

    listWorkspaceFiles: async (sessionId: string, subPath?: string): Promise<WorkspaceFile[]> => {
      const url = subPath
        ? `/api/v1/workspaces/${sessionId}/files?path=${encodeURIComponent(subPath)}`
        : `/api/v1/workspaces/${sessionId}/files`;
      const data = await callAPI<{ files: WorkspaceFile[] }>('GET', url);
      return data.files || [];
    },

    // ============== Config Service ==============
    getConfig: async (): Promise<Config> => {
      return callAPI<Config>('GET', '/api/v1/config');
    },

    updateConfig: async (config: Partial<Config>): Promise<void> => {
      await callAPI('PUT', '/api/v1/config', config);
    },

    // ============== Models Service ==============
    getModels: async (): Promise<ModelsResponse> => {
      return callAPI<ModelsResponse>('GET', '/api/v1/models');
    },

    setCurrentModel: async (modelId: string): Promise<void> => {
      await callAPI('PUT', '/api/v1/models/current', { model: modelId });
    },

    setSessionModel: async (sessionId: string, modelId: string): Promise<void> => {
      await callAPI('PUT', `/api/v1/sessions/${sessionId}/model`, { model: modelId });
    },

    getScenarioModels: async (): Promise<{ chat: string; cron: string; channel: string }> => {
      return callAPI('GET', '/api/v1/settings/models');
    },

    setScenarioModel: async (scenario: string, modelId: string): Promise<void> => {
      const body: Record<string, string> = {};
      body[scenario] = modelId;
      await callAPI('PUT', '/api/v1/settings/models', body);
    },

    // ============== Auth Service (GUI only) ==============
    getAuthStatus: async (): Promise<AuthStatus> => {
      const status = await app.GetAuthStatus();
      return {
        authenticated: status.authenticated,
        token_masked: status.token_masked,
        copilot_verified: status.copilot_verified,
        error: status.error,
      };
    },

    startDeviceLogin: async (): Promise<DeviceCodeResponse> => {
      const resp = await app.StartDeviceLogin();
      return {
        device_code: resp.device_code,
        user_code: resp.user_code,
        verification_uri: resp.verification_uri,
        expires_in: resp.expires_in,
        interval: resp.interval,
      };
    },

    pollDeviceLogin: async (deviceCode: string): Promise<AuthResult> => {
      const result = await app.PollDeviceLogin(deviceCode);
      return {
        success: result.success,
        error: result.error,
        interval: result.interval,
      };
    },

    logout: async (): Promise<void> => {
      await app.Logout();
    },

    // ============== App Control (GUI only) ==============
    restartService: async (): Promise<void> => {
      await app.RestartService();
    },

    quit: async (): Promise<void> => {
      await app.Quit();
    },

    // ============== Channel Service ==============
    getChannels: async (): Promise<ChannelStatus[]> => {
      const data = await callAPI<ChannelStatus[]>('GET', '/api/v1/channels');
      return data || [];
    },

    getChannelConfig: async (channelType: string): Promise<ChannelConfig> => {
      return callAPI<ChannelConfig>('GET', `/api/v1/channels/${channelType}/config`);
    },

    updateChannelConfig: async (channelType: string, config: ChannelConfig): Promise<void> => {
      await callAPI('PUT', `/api/v1/channels/${channelType}/config`, config);
    },

    startChannel: async (channelType: string): Promise<void> => {
      await callAPI('POST', `/api/v1/channels/${channelType}/start`);
    },

    stopChannel: async (channelType: string): Promise<void> => {
      await callAPI('POST', `/api/v1/channels/${channelType}/stop`);
    },

    // ============== Mode Detection ==============
    isGUIMode: (): boolean => true,
  };
}
