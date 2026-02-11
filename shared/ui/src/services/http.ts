// ================================================================
// HTTP API Adapter - For Web mode (mote serve)
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
  ChannelStatus,
  ChannelConfig,
  ModelsResponse,
  ScenarioModels,
  Skill,
  Workspace,
  WorkspaceFile,
  BrowseDirectoryResult,
  Prompt,
  MCPPrompt,
  MCPPromptContent,
} from '../types';

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

export interface HttpAdapterOptions {
  baseUrl?: string;
}

export const createHttpAdapter = (options: HttpAdapterOptions = {}): APIAdapter => {
  const baseUrl = options.baseUrl || '';

  const fetchJSON = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const response = await fetch(`${baseUrl}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...init?.headers,
      },
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `HTTP ${response.status}`);
    }
    return response.json();
  };

  return {
    // ============== Status Service ==============
    getStatus: async (): Promise<ServiceStatus> => {
      try {
        const data = await fetchJSON<{ status: string; version: string; uptime: number; running?: boolean }>('/api/v1/health');
        return {
          running: data.running ?? (data.status === 'healthy'),
          port: parseInt(window.location.port) || 80,
          version: data.version || 'unknown',
          uptime: data.uptime || 0,
        };
      } catch {
        return { running: false, port: 0, version: 'unknown', uptime: 0 };
      }
    },

    // ============== Chat Service ==============
    chat: async (request: ChatRequest, onEvent: (event: StreamEvent) => void, signal?: AbortSignal): Promise<void> => {
      // Use /chat/stream endpoint for streaming
      const response = await fetch(`${baseUrl}/api/v1/chat/stream`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...request, stream: true }),
        signal,
      });

      if (!response.ok) {
        throw new Error(`Chat request failed: ${response.status}`);
      }

      const reader = response.body?.getReader();
      if (!reader) throw new Error('No reader available');

      const decoder = new TextDecoder();
      let buffer = '';

      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) {
            onEvent({ type: 'done' });
            break;
          }

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              try {
                const data = JSON.parse(line.slice(6));
                onEvent(data as StreamEvent);
              } catch {
                // Ignore parse errors
              }
            }
          }
        }
      } catch (err) {
        // 如果是取消请求，不要抛出错误
        if (signal?.aborted) {
          return;
        }
        throw err;
      }
    },

    // ============== Session Service ==============
    getSessions: async (): Promise<Session[]> => {
      const data = await fetchJSON<{ sessions: Session[] }>('/api/v1/sessions');
      return data.sessions || [];
    },

    getSession: async (sessionId: string): Promise<Session> => {
      return fetchJSON<Session>(`/api/v1/sessions/${sessionId}`);
    },

    getSessionMessages: async (sessionId: string): Promise<Message[]> => {
      const data = await fetchJSON<{ messages: Message[] }>(`/api/v1/sessions/${sessionId}/messages`);
      return data.messages || [];
    },

    createSession: async (title?: string, scenario?: string): Promise<Session> => {
      return fetchJSON<Session>('/api/v1/sessions', {
        method: 'POST',
        body: JSON.stringify({ title: title || '', scenario: scenario || 'chat' }),
      });
    },

    deleteSession: async (sessionId: string): Promise<void> => {
      await fetchJSON(`/api/v1/sessions/${sessionId}`, { method: 'DELETE' });
    },

    updateSession: async (sessionId: string, updates: { title?: string }): Promise<Session> => {
      return fetchJSON<Session>(`/api/v1/sessions/${sessionId}`, {
        method: 'PUT',
        body: JSON.stringify(updates),
      });
    },

    // ============== Memory Service ==============
    getMemories: async (options?: { limit?: number; offset?: number }): Promise<{ memories: Memory[]; total: number; limit: number; offset: number }> => {
      const params = new URLSearchParams();
      if (options?.limit) params.set('limit', String(options.limit));
      if (options?.offset) params.set('offset', String(options.offset));
      const queryString = params.toString();
      const data = await fetchJSON<{ memories: Memory[]; total: number; limit: number; offset: number }>(`/api/v1/memory${queryString ? '?' + queryString : ''}`);
      return {
        memories: data.memories || [],
        total: data.total || 0,
        limit: data.limit || 100,
        offset: data.offset || 0,
      };
    },

    searchMemories: async (query: string, limit?: number): Promise<Memory[]> => {
      const topK = limit || 10;
      const data = await fetchJSON<{ results: MemoryResult[] }>('/api/v1/memory/search', {
        method: 'POST',
        body: JSON.stringify({ query, top_k: topK }),
      });
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
      const data = await fetchJSON<{ id: string; category: string }>('/api/v1/memory', {
        method: 'POST',
        body: JSON.stringify({ content, category }),
      });
      return {
        id: data.id,
        content,
        category: data.category || category || 'other',
        created_at: new Date().toISOString(),
      };
    },

    updateMemory: async (id: string, content: string, category?: string): Promise<Memory> => {
      const response = await fetchJSON<{
        id: string;
        content: string;
        category: string;
        created_at: string;
        source?: string;
        importance?: number;
        capture_method?: string;
      }>(`/api/v1/memory/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ content, category }),
      });
      return {
        id: response.id,
        content: response.content,
        category: response.category,
        created_at: response.created_at,
      };
    },

    deleteMemory: async (id: string): Promise<void> => {
      const response = await fetch(`${baseUrl}/api/v1/memory/${id}`, { method: 'DELETE' });
      if (!response.ok) {
        const error = await response.json().catch(() => ({ message: response.statusText }));
        throw new Error(error.message || `HTTP ${response.status}`);
      }
      // 204 No Content - no need to parse JSON
    },

    // ============== Tools Service ==============
    getTools: async (): Promise<Tool[]> => {
      const data = await fetchJSON<{ tools: Tool[] }>('/api/v1/tools');
      return data.tools || [];
    },

    // ============== Cron Service ==============
    getCronJobs: async (): Promise<CronJob[]> => {
      const data = await fetchJSON<{ jobs: CronJob[] }>('/api/v1/cron/jobs');
      return data.jobs || [];
    },

    createCronJob: async (job: Partial<CronJob>): Promise<CronJob> => {
      const data = await fetchJSON<{ job: CronJob }>('/api/v1/cron/jobs', {
        method: 'POST',
        body: JSON.stringify(job),
      });
      return data.job;
    },

    updateCronJob: async (id: string, updates: Partial<CronJob>): Promise<CronJob> => {
      const data = await fetchJSON<{ job: CronJob }>(`/api/v1/cron/jobs/${id}`, {
        method: 'PUT',
        body: JSON.stringify(updates),
      });
      return data.job;
    },

    toggleCronJob: async (id: string, enabled: boolean): Promise<void> => {
      await fetchJSON(`/api/v1/cron/jobs/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled }),
      });
    },

    deleteCronJob: async (id: string): Promise<void> => {
      await fetchJSON(`/api/v1/cron/jobs/${id}`, { method: 'DELETE' });
    },

    // ============== MCP Service ==============
    getMCPServers: async (): Promise<MCPServer[]> => {
      const data = await fetchJSON<{ servers: MCPServer[] }>('/api/v1/mcp/servers');
      return data.servers || [];
    },

    createMCPServer: async (server: Partial<MCPServer>): Promise<MCPServer> => {
      const data = await fetchJSON<{ server: MCPServer }>('/api/v1/mcp/servers', {
        method: 'POST',
        body: JSON.stringify(server),
      });
      return data.server;
    },

    startMCPServer: async (name: string): Promise<void> => {
      await fetchJSON(`/api/v1/mcp/servers/${name}/restart`, { method: 'POST' });
    },

    stopMCPServer: async (name: string): Promise<void> => {
      await fetchJSON(`/api/v1/mcp/servers/${name}/stop`, { method: 'POST' });
    },

    deleteMCPServer: async (name: string): Promise<void> => {
      await fetchJSON(`/api/v1/mcp/servers/${name}`, { method: 'DELETE' });
    },

    updateMCPServer: async (name: string, updates: Partial<MCPServer>): Promise<MCPServer> => {
      const data = await fetchJSON<{ Name: string; Status: string; Transport: string; ToolCount?: number; Error?: string }>(`/api/v1/mcp/servers/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: JSON.stringify(updates),
      });
      return {
        name: data.Name,
        status: data.Status,
        transport: data.Transport,
        tool_count: data.ToolCount,
        error: data.Error,
      } as MCPServer;
    },

    importMCPServers: async (config: Record<string, unknown>): Promise<{ imported: string[]; errors: Record<string, string> }> => {
      return fetchJSON('/api/v1/mcp/servers/import', {
        method: 'POST',
        body: JSON.stringify(config),
      });
    },

    // ============== Config Service ==============
    getConfig: async (): Promise<Config> => {
      return fetchJSON<Config>('/api/v1/config');
    },

    updateConfig: async (config: Partial<Config>): Promise<void> => {
      await fetchJSON('/api/v1/config', {
        method: 'PUT',
        body: JSON.stringify(config),
      });
    },

    // ============== Models Service ==============
    getModels: async (): Promise<ModelsResponse> => {
      return fetchJSON<ModelsResponse>('/api/v1/models');
    },

    setCurrentModel: async (modelId: string): Promise<void> => {
      await fetchJSON('/api/v1/models/current', {
        method: 'PUT',
        body: JSON.stringify({ model: modelId }),
      });
    },

    setSessionModel: async (sessionId: string, modelId: string): Promise<void> => {
      await fetchJSON(`/api/v1/sessions/${sessionId}/model`, {
        method: 'PUT',
        body: JSON.stringify({ model: modelId }),
      });
    },

    setSessionSkills: async (sessionId: string, skillIds: string[]): Promise<void> => {
      await fetchJSON(`/api/v1/sessions/${sessionId}/skills`, {
        method: 'PUT',
        body: JSON.stringify({ selected_skills: skillIds }),
      });
    },

    getScenarioModels: async (): Promise<ScenarioModels> => {
      return fetchJSON<ScenarioModels>('/api/v1/settings/models');
    },

    setScenarioModel: async (scenario: string, modelId: string): Promise<void> => {
      const body: Record<string, string> = {};
      body[scenario] = modelId;
      await fetchJSON('/api/v1/settings/models', {
        method: 'PUT',
        body: JSON.stringify(body),
      });
    },

    // ============== Channel Service ==============
    getChannels: async (): Promise<ChannelStatus[]> => {
      const data = await fetchJSON<ChannelStatus[]>('/api/v1/channels');
      return data || [];
    },

    getChannelConfig: async (channelType: string): Promise<ChannelConfig> => {
      return fetchJSON<ChannelConfig>(`/api/v1/channels/${channelType}/config`);
    },

    updateChannelConfig: async (channelType: string, config: Partial<ChannelConfig>): Promise<void> => {
      await fetchJSON(`/api/v1/channels/${channelType}/config`, {
        method: 'PUT',
        body: JSON.stringify(config),
      });
    },

    startChannel: async (channelType: string): Promise<void> => {
      await fetchJSON(`/api/v1/channels/${channelType}/start`, { method: 'POST' });
    },

    stopChannel: async (channelType: string): Promise<void> => {
      await fetchJSON(`/api/v1/channels/${channelType}/stop`, { method: 'POST' });
    },

    // ============== Auth Service (Web mode) ==============
    getAuthStatus: async (): Promise<AuthStatus> => {
      const response = await fetchJSON<{
        authenticated: boolean;
        token_masked?: string;
        provider?: string;
        model?: string;
        message?: string;
      }>('/api/v1/auth/status');
      
      return {
        authenticated: response.authenticated,
        token_masked: response.token_masked,
        error: response.message,
      };
    },

    // ============== Skills Service ==============
    getSkills: async () => {
      const data = await fetchJSON<{ skills: Skill[] }>('/api/v1/skills');
      return data.skills || [];
    },

    activateSkill: async (skillId: string, config?: Record<string, unknown>) => {
      await fetchJSON(`/api/v1/skills/${skillId}/activate`, {
        method: 'POST',
        body: JSON.stringify(config || {}),
      });
    },

    deactivateSkill: async (skillId: string) => {
      await fetchJSON(`/api/v1/skills/${skillId}/deactivate`, { method: 'POST' });
    },

    reloadSkills: async () => {
      await fetchJSON('/api/v1/skills/reload', { method: 'POST' });
    },

    openSkillsDir: async (target: 'user' | 'workspace') => {
      await fetchJSON('/api/v1/skills/open', {
        method: 'POST',
        body: JSON.stringify({ target }),
      });
    },

    createSkill: async (name: string, target: 'user' | 'workspace') => {
      return fetchJSON<{ path: string }>('/api/v1/skills/create', {
        method: 'POST',
        body: JSON.stringify({ name, target }),
      });
    },

    // ============== Tools Service (Extended) ==============
    openToolsDir: async (target: 'user' | 'workspace') => {
      await fetchJSON('/api/v1/tools/open', {
        method: 'POST',
        body: JSON.stringify({ target }),
      });
    },

    createTool: async (name: string, runtime: string, target: 'user' | 'workspace') => {
      return fetchJSON<{ path: string }>('/api/v1/tools/create', {
        method: 'POST',
        body: JSON.stringify({ name, runtime, target }),
      });
    },

    // ============== Workspace Service ==============
    getWorkspaces: async () => {
      const data = await fetchJSON<{ workspaces: Workspace[] }>('/api/v1/workspaces');
      return data.workspaces || [];
    },

    bindWorkspace: async (sessionId: string, path: string, alias?: string, readOnly?: boolean) => {
      return fetchJSON<Workspace>('/api/v1/workspaces', {
        method: 'POST',
        body: JSON.stringify({ session_id: sessionId, path, alias, read_only: readOnly }),
      });
    },

    unbindWorkspace: async (sessionId: string) => {
      await fetchJSON(`/api/v1/workspaces/${sessionId}`, { method: 'DELETE' });
    },

    listWorkspaceFiles: async (sessionId: string, path?: string) => {
      const url = path 
        ? `/api/v1/workspaces/${sessionId}/files?path=${encodeURIComponent(path)}`
        : `/api/v1/workspaces/${sessionId}/files`;
      const data = await fetchJSON<{ files: WorkspaceFile[] }>(url);
      return data.files || [];
    },

    browseDirectory: async (path?: string) => {
      const url = path
        ? `/api/v1/browse-directory?path=${encodeURIComponent(path)}`
        : '/api/v1/browse-directory';
      return fetchJSON<BrowseDirectoryResult>(url);
    },

    // ============== Prompts Service ==============
    getPrompts: async () => {
      const data = await fetchJSON<{ prompts: Prompt[] }>('/api/v1/prompts');
      return data.prompts || [];
    },

    getMCPPrompts: async () => {
      const data = await fetchJSON<{ prompts: MCPPrompt[] }>('/api/v1/mcp/prompts');
      return data.prompts || [];
    },

    getMCPPromptContent: async (server: string, name: string, args?: Record<string, string>) => {
      const data = await fetchJSON<MCPPromptContent>(`/api/v1/mcp/prompts/${encodeURIComponent(server)}/${encodeURIComponent(name)}`, {
        method: 'POST',
        body: JSON.stringify({ arguments: args }),
      });
      return data;
    },

    createPrompt: async (prompt: Partial<Prompt>) => {
      return fetchJSON<Prompt>('/api/v1/prompts', {
        method: 'POST',
        body: JSON.stringify(prompt),
      });
    },

    updatePrompt: async (id: string, updates: Partial<Prompt>) => {
      return fetchJSON<Prompt>(`/api/v1/prompts/${id}`, {
        method: 'PUT',
        body: JSON.stringify(updates),
      });
    },

    deletePrompt: async (id: string) => {
      await fetchJSON(`/api/v1/prompts/${id}`, { method: 'DELETE' });
    },

    togglePrompt: async (id: string) => {
      await fetchJSON(`/api/v1/prompts/${id}/toggle`, { method: 'POST' });
    },

    openPromptsDir: async (target: 'user' | 'workspace') => {
      await fetchJSON('/api/v1/prompts/open', {
        method: 'POST',
        body: JSON.stringify({ target }),
      });
    },

    renderPrompt: async (id: string, variables: Record<string, string>) => {
      return fetchJSON<{ content: string }>(`/api/v1/prompts/${id}/render`, {
        method: 'POST',
        body: JSON.stringify({ variables }),
      });
    },

    // ============== GUI Mode ==============
    isGUIMode: (): boolean => false,
  };
};
