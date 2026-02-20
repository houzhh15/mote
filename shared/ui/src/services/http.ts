// ================================================================
// HTTP API Adapter - For Web mode (mote serve)
// ================================================================

import type { APIAdapter, MemorySyncResult, MemoryStats, MemoryExportResult } from './adapter';
import type {
  ServiceStatus,
  Session,
  Message,
  Memory,
  Tool,
  CronJob,
  CronExecutingJob,
  MCPServer,
  Config,
  ChatRequest,
  StreamEvent,
  AuthStatus,
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

    // ============== Pause Control Service ==============
    pauseSession: async (sessionId: string): Promise<void> => {
      await fetchJSON('/api/v1/pause', {
        method: 'POST',
        body: JSON.stringify({ session_id: sessionId }),
      });
    },

    resumeSession: async (sessionId: string, userInput?: string): Promise<void> => {
      await fetchJSON('/api/v1/resume', {
        method: 'POST',
        body: JSON.stringify({ session_id: sessionId, user_input: userInput }),
      });
    },

    getPauseStatus: async (sessionId: string): Promise<{ paused: boolean; paused_at?: string; timeout_remaining?: number; pending_tools?: string[] }> => {
      return fetchJSON<{ paused: boolean; paused_at?: string; timeout_remaining?: number; pending_tools?: string[] }>(
        `/api/v1/pause/status?session_id=${encodeURIComponent(sessionId)}`
      );
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

    syncMemory: async (): Promise<MemorySyncResult> => {
      return fetchJSON<MemorySyncResult>('/api/v1/memory/sync', {
        method: 'POST',
        body: JSON.stringify({}),
      });
    },

    getMemoryStats: async (): Promise<MemoryStats> => {
      return fetchJSON<MemoryStats>('/api/v1/memory/stats');
    },

    getDailyLog: async (date?: string): Promise<{ date: string; content: string }> => {
      const params = date ? `?date=${encodeURIComponent(date)}` : '';
      return fetchJSON<{ date: string; content: string }>(`/api/v1/memory/daily${params}`);
    },

    appendDailyLog: async (content: string, section?: string): Promise<void> => {
      await fetchJSON('/api/v1/memory/daily', {
        method: 'POST',
        body: JSON.stringify({ content, section }),
      });
    },

    exportMemories: async (format?: string): Promise<MemoryExportResult> => {
      const params = format ? `?format=${encodeURIComponent(format)}` : '';
      return fetchJSON<MemoryExportResult>(`/api/v1/memory/export${params}`);
    },

    importMemories: async (memories: Array<{ content: string; source?: string }>): Promise<{ imported: number; total: number }> => {
      return fetchJSON<{ imported: number; total: number }>('/api/v1/memory/import', {
        method: 'POST',
        body: JSON.stringify({ memories }),
      });
    },

    batchDeleteMemories: async (ids: string[]): Promise<{ deleted: number; total: number }> => {
      return fetchJSON<{ deleted: number; total: number }>('/api/v1/memory/batch', {
        method: 'DELETE',
        body: JSON.stringify({ ids }),
      });
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

    getCronExecuting: async (): Promise<CronExecutingJob[]> => {
      const data = await fetchJSON<{ jobs: CronExecutingJob[] }>('/api/v1/cron/executing');
      return data.jobs || [];
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

    reconfigureSession: async (sessionId: string, config: ReconfigureSessionRequest): Promise<ReconfigureSessionResponse> => {
      return fetchJSON<ReconfigureSessionResponse>(`/api/v1/sessions/${sessionId}/reconfigure`, {
        method: 'POST',
        body: JSON.stringify(config),
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

    checkSkillUpdates: async () => {
      const data = await fetchJSON<{ success: boolean; data: { total_checked: number; updates_available: Array<{ skill_id: string; local_version: string; embed_version: string; update_available: boolean; local_modified: boolean; description?: string; checked_at: string }> } }>('/api/v1/skills/check-updates', {
        method: 'POST',
      });
      // Map backend response to frontend expected format
      return {
        updates: data.data.updates_available || [],
        total: data.data.total_checked || 0,
        updated_at: data.data.updates_available?.[0]?.checked_at || new Date().toISOString(),
      };
    },

    updateSkill: async (skillId: string, options?: { force?: boolean; backup?: boolean }) => {
      const data = await fetchJSON<{ success: boolean; data: { skill_id: string; old_version: string; new_version: string; backup_path?: string; reloaded: boolean; duration: string } }>(`/api/v1/skills/${skillId}/update`, {
        method: 'POST',
        body: JSON.stringify(options || {}),
      });
      // Map backend response to frontend expected format
      return {
        success: data.success,
        skill_id: data.data.skill_id,
        old_version: data.data.old_version,
        new_version: data.data.new_version,
        backup_path: data.data.backup_path,
      };
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

    reloadPrompts: async () => {
      await fetchJSON('/api/v1/prompts/reload', { method: 'POST' });
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
