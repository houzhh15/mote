/**
 * ChatManager - 管理聊天请求的生命周期
 * 
 * 功能：
 * 1. 支持中止正在进行的聊天请求
 * 2. 页面切换后请求继续在后台运行
 * 3. 返回页面时可以查看进度
 */

import React, { createContext, useContext, useRef, useCallback } from 'react';
import type { ChatRequest, StreamEvent, Message, ToolCallResult, ErrorDetail, ApprovalRequestSSEEvent, PDAProgressSSEEvent } from '../types';

export interface ChatState {
  sessionId: string;
  streaming: boolean;
  currentContent: string;
  currentThinking: string;  // Temporary thinking content (cleared when other output arrives)
  thinkingDone: boolean;     // Whether thinking phase has ended (content/tool_call started)
  currentToolCalls: { [key: string]: { name: string; status?: string; arguments?: string; result?: unknown; error?: string } };
  messages: Message[];
  finalMessage?: Message;  // Set when streaming completes, contains the final assistant message
  error?: string;
  errorDetail?: ErrorDetail;  // Structured error info
  // Truncation info - when response is cut off due to max_tokens
  truncated?: boolean;
  truncatedReason?: string;
  pendingToolCalls?: number;
  // Pause info - when execution is paused
  paused?: boolean;
  pausedAt?: Date;
  pausePendingTools?: string[];  // Names of tools about to be executed
  // Multi-agent delegation
  activeAgentName?: string;   // Currently active sub-agent name
  activeAgentDepth?: number;  // Nesting depth (0 = main agent)
  // Approval request - tool call waiting for user approval
  approvalRequest?: ApprovalRequestSSEEvent;
  // PDA progress tracking
  pdaProgress?: PDAProgressSSEEvent;
  pdaSteps?: Array<{ label: string; type: string; phase: 'pending' | 'started' | 'completed' | 'failed' }>;
}

export interface ChatManagerContextType {
  /** 获取指定会话的聊天状态 */
  getChatState: (sessionId: string) => ChatState | undefined;
  /** 开始聊天请求 */
  startChat: (
    sessionId: string,
    request: ChatRequest,
    chatFn: (request: ChatRequest, onEvent: (event: StreamEvent) => void, signal?: AbortSignal) => Promise<void>,
    onComplete?: (assistantMessage: Message | null, error?: string) => void
  ) => void;
  /** 中止聊天请求 */
  abortChat: (sessionId: string) => void;
  /** 检查是否正在流式输出 */
  isStreaming: (sessionId: string) => boolean;
  /** 订阅状态变化 */
  subscribe: (sessionId: string, callback: (state: ChatState) => void) => () => void;
}

const ChatManagerContext = createContext<ChatManagerContextType | null>(null);

interface ActiveChat {
  abortController: AbortController;
  state: ChatState;
  subscribers: Set<(state: ChatState) => void>;
}

export const ChatManagerProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const activeChatsRef = useRef<Map<string, ActiveChat>>(new Map());
  // 预注册的订阅者（在 startChat 之前订阅的）
  const pendingSubscribersRef = useRef<Map<string, Set<(state: ChatState) => void>>>(new Map());
  
  // RAF 节流相关：每个 session 的待更新标记和 RAF id
  const pendingUpdatesRef = useRef<Map<string, boolean>>(new Map());
  const rafIdsRef = useRef<Map<string, number>>(new Map());

  // 立即通知订阅者（用于关键事件如 done, error）
  const notifySubscribersImmediate = useCallback((sessionId: string) => {
    // 取消该 session 的待处理 RAF
    const rafId = rafIdsRef.current.get(sessionId);
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafIdsRef.current.delete(sessionId);
    }
    pendingUpdatesRef.current.delete(sessionId);
    
    const chat = activeChatsRef.current.get(sessionId);
    if (chat) {
      chat.subscribers.forEach(cb => cb({ ...chat.state }));
    }
    // 不再调用 forceUpdate，订阅者已经被通知
  }, []);

  // RAF 节流版本的通知（用于高频内容更新）
  const notifySubscribers = useCallback((sessionId: string, immediate = false) => {
    if (immediate) {
      notifySubscribersImmediate(sessionId);
      return;
    }
    
    // 标记有待更新
    pendingUpdatesRef.current.set(sessionId, true);
    
    // 如果已经有 RAF 在排队，不重复请求
    if (rafIdsRef.current.has(sessionId)) {
      return;
    }
    
    // 请求下一帧更新
    const rafId = requestAnimationFrame(() => {
      rafIdsRef.current.delete(sessionId);
      
      if (pendingUpdatesRef.current.get(sessionId)) {
        pendingUpdatesRef.current.delete(sessionId);
        const chat = activeChatsRef.current.get(sessionId);
        if (chat) {
          chat.subscribers.forEach(cb => cb({ ...chat.state }));
        }
        // 不再调用 forceUpdate，订阅者已经被通知
      }
    });
    
    rafIdsRef.current.set(sessionId, rafId);
  }, [notifySubscribersImmediate]);

  const getChatState = useCallback((sessionId: string): ChatState | undefined => {
    return activeChatsRef.current.get(sessionId)?.state;
  }, []);

  const isStreaming = useCallback((sessionId: string): boolean => {
    return activeChatsRef.current.get(sessionId)?.state.streaming ?? false;
  }, []);

  const subscribe = useCallback((sessionId: string, callback: (state: ChatState) => void): () => void => {
    const chat = activeChatsRef.current.get(sessionId);
    if (chat) {
      chat.subscribers.add(callback);
      // 立即通知当前状态
      callback({ ...chat.state });
    } else {
      // 如果还没有 activeChat，添加到待处理订阅者
      let pending = pendingSubscribersRef.current.get(sessionId);
      if (!pending) {
        pending = new Set();
        pendingSubscribersRef.current.set(sessionId, pending);
      }
      pending.add(callback);
    }
    
    // unsubscribe 时动态查找，避免闭包捕获旧引用导致无法清理
    // （startChat 会将回调从 pending 迁移到新的 ActiveChat.subscribers，
    //   如果 unsubscribe 只在旧引用上操作就会泄漏）
    return () => {
      // 从 activeChat 中移除
      const currentChat = activeChatsRef.current.get(sessionId);
      if (currentChat) {
        currentChat.subscribers.delete(callback);
      }
      // 从 pending 中移除
      const p = pendingSubscribersRef.current.get(sessionId);
      if (p) {
        p.delete(callback);
      }
    };
  }, []);

  const abortChat = useCallback((sessionId: string) => {
    const chat = activeChatsRef.current.get(sessionId);
    if (chat) {
      chat.abortController.abort();
      chat.state.streaming = false;
      chat.state.error = '已停止';
      notifySubscribers(sessionId);
    }
  }, [notifySubscribers]);

  const startChat = useCallback((
    sessionId: string,
    request: ChatRequest,
    chatFn: (request: ChatRequest, onEvent: (event: StreamEvent) => void, signal?: AbortSignal) => Promise<void>,
    onComplete?: (assistantMessage: Message | null, error?: string) => void
  ) => {
    // 如果已有请求，先中止
    const existing = activeChatsRef.current.get(sessionId);
    if (existing) {
      existing.abortController.abort();
    }

    const abortController = new AbortController();
    const state: ChatState = {
      sessionId,
      streaming: true,
      currentContent: '',
      currentThinking: '',
      thinkingDone: false,
      currentToolCalls: {},
      messages: [],
    };

    // 收集已有订阅者和待处理订阅者
    const existingSubscribers = existing?.subscribers ?? new Set<(state: ChatState) => void>();
    const pendingSubscribers = pendingSubscribersRef.current.get(sessionId);
    const allSubscribers = new Set([...existingSubscribers, ...(pendingSubscribers ?? [])]);
    
    // 清除待处理订阅者
    pendingSubscribersRef.current.delete(sessionId);

    const chat: ActiveChat = {
      abortController,
      state,
      subscribers: allSubscribers,
    };
    activeChatsRef.current.set(sessionId, chat);
    notifySubscribers(sessionId);

    let accumulatedContent = '';
    let accumulatedThinking = '';  // 累积 thinking 内容
    let accumulatedToolCalls: { [key: string]: { name: string; status?: string; arguments?: string; result?: unknown; error?: string } } = {};
    let isFinalized = false;
    let lastContentAgentName: string | undefined = undefined; // Track agent for inline tags

    const handleEvent = (event: StreamEvent) => {
      if (abortController.signal.aborted) return;

      // Track active sub-agent from multi-agent delegation events
      if (event.agent_name) {
        state.activeAgentName = event.agent_name;
        state.activeAgentDepth = event.agent_depth || 0;
      }

      const content = event.delta || event.content;
      if (event.type === 'content' && content) {
        // Insert agent transition marker when content source changes
        const currentAgent = event.agent_name || undefined;
        if (currentAgent !== lastContentAgentName) {
          if (currentAgent) {
            // Entering or switching sub-agent output
            const prefix = accumulatedContent.length > 0 ? '\n' : '';
            accumulatedContent += `${prefix}<<AGENT:${currentAgent}:${event.agent_depth || 0}>>`;
          } else if (lastContentAgentName) {
            // Returning to main agent output
            accumulatedContent += '\n<<AGENT_END>>';
          }
          lastContentAgentName = currentAgent;
        }
        accumulatedContent += content;
        state.currentContent = accumulatedContent;
        // Mark thinking as done (don't clear text — let UI animate out)
        if (accumulatedThinking) {
          state.thinkingDone = true;
        }
        // 使用 RAF 节流，减少高频更新
        notifySubscribers(sessionId);
      } else if (event.type === 'thinking' && event.thinking) {
        // Accumulate thinking chunks within a round (providers send deltas, not full text).
        // When a new round starts (after thinkingDone was set, e.g. after content arrived
        // between tool call iterations), reset and start fresh.
        if (state.thinkingDone) {
          accumulatedThinking = '';
        }
        accumulatedThinking += event.thinking;
        state.currentThinking = accumulatedThinking;
        // Reset thinkingDone so UI re-shows the panel for new thinking rounds
        // (e.g. MiniMax produces thinking before each tool call iteration)
        state.thinkingDone = false;
        // thinking 使用立即通知，避免 RAF 节流被后续 tool_call/done 的
        // immediate 通知取消导致 thinking 状态永远不可见
        notifySubscribers(sessionId, true);
      } else if (event.type === 'tool_call' && event.tool_call) {
        const toolName = event.tool_call.name;
        const toolArgs = event.tool_call.arguments;
        if (toolName) {
          accumulatedToolCalls[toolName] = { name: toolName, status: 'running', arguments: toolArgs };
          state.currentToolCalls = { ...accumulatedToolCalls };
          // Don't mark thinking as done on tool_call — keep panel visible
          // during tool call loops. Only hide when actual content arrives.
          // 工具调用开始时立即通知
          notifySubscribers(sessionId, true);
        }
      } else if (event.type === 'tool_call_update' && event.tool_call_update) {
        const toolName = event.tool_call_update.tool_name;
        const status = event.tool_call_update.status;
        const args = event.tool_call_update.arguments;
        if (toolName) {
          const existing = accumulatedToolCalls[toolName];
          accumulatedToolCalls[toolName] = {
            ...existing,
            name: toolName,
            status: status || existing?.status,
            arguments: args || existing?.arguments,
          };
          state.currentToolCalls = { ...accumulatedToolCalls };
          // Don't mark thinking as done on tool_call_update — keep panel visible
          // 工具更新使用节流
          notifySubscribers(sessionId);
        }
      } else if (event.type === 'tool_result' && event.tool_result) {
        const toolName = event.tool_result.tool_name || event.tool_result.ToolName;
        const output = event.tool_result.output || event.tool_result.Output;
        const isError = event.tool_result.is_error || event.tool_result.IsError;
        
        if (toolName) {
          // Preserve existing arguments when adding result
          const existing = accumulatedToolCalls[toolName];
          accumulatedToolCalls[toolName] = {
            name: toolName,
            arguments: existing?.arguments,
            result: output,
            error: isError ? output : undefined,
          };
          state.currentToolCalls = { ...accumulatedToolCalls };
          notifySubscribers(sessionId);
        }
      } else if (event.type === 'done' && !isFinalized) {
        isFinalized = true;
        state.streaming = false;
        
        // Keep the final content visible (don't clear it immediately)
        // ChatPage will handle clearing after adding to messages list
        state.currentContent = accumulatedContent;
        accumulatedThinking = '';
        state.currentThinking = '';
        state.thinkingDone = false;
        state.activeAgentName = undefined;
        state.activeAgentDepth = undefined;
        state.pdaProgress = undefined;
        state.pdaSteps = undefined;
        // Keep tool calls visible until ChatPage processes them
        // state.currentToolCalls = {}; // Don't clear - let ChatPage handle it

        const toolCallsArray: ToolCallResult[] = Object.values(accumulatedToolCalls).filter(tc => tc.name);
        
        // Keep agent transition markers in content — they are now persisted to the
        // session and rendered with styled agent tags in the history view.
        
        const assistantMessage: Message = {
          role: 'assistant',
          content: accumulatedContent,
          timestamp: new Date().toISOString(),
        };
        
        if (toolCallsArray.length > 0) {
          assistantMessage.tool_calls = toolCallsArray;
        }

        state.messages = [assistantMessage];
        state.finalMessage = assistantMessage; // Mark completion with final message
        // done 事件立即通知
        notifySubscribers(sessionId, true);
        onComplete?.(assistantMessage);
      } else if (event.type === 'truncated') {
        // Response hit max_tokens limit but execution continues
        // Just update the warning counter, don't stop streaming
        state.truncated = true;
        state.truncatedReason = event.truncated_reason || 'length';
        // Accumulate the count of truncation events
        state.pendingToolCalls = (state.pendingToolCalls || 0) + 1;
        // truncated 立即通知
        notifySubscribers(sessionId, true);
        // Don't stop streaming - execution continues automatically
      } else if (event.type === 'error') {
        state.streaming = false;
        state.error = event.error;
        // Store structured error detail if available
        if (event.error_detail) {
          state.errorDetail = event.error_detail;
        }
        // error 立即通知
        notifySubscribers(sessionId, true);
        onComplete?.(null, event.error);
      } else if (event.type === 'pause') {
        // Execution paused before tool call
        state.paused = true;
        state.pausedAt = new Date();
        if (event.pause_data?.pending_tools) {
          state.pausePendingTools = event.pause_data.pending_tools.map(t => t.name);
        }
        // Clear thinking when paused
        accumulatedThinking = '';
        state.currentThinking = '';
        state.thinkingDone = false;
        // pause 立即通知
        notifySubscribers(sessionId, true);
      } else if (event.type === 'pause_timeout') {
        // Pause timed out, execution will continue
        state.paused = false;
        state.pausedAt = undefined;
        state.pausePendingTools = undefined;
        // pause_timeout 立即通知
        notifySubscribers(sessionId, true);
      } else if (event.type === 'pause_resumed') {
        // Execution resumed after pause
        state.paused = false;
        state.pausedAt = undefined;
        state.pausePendingTools = undefined;
        // pause_resumed 立即通知
        notifySubscribers(sessionId, true);
      } else if (event.type === 'approval_request' && event.approval_request) {
        // Tool call waiting for user approval
        state.approvalRequest = event.approval_request;
        // approval_request 立即通知
        notifySubscribers(sessionId, true);
      } else if (event.type === 'approval_resolved') {
        // Approval resolved (approved or rejected) — clear the popup
        state.approvalRequest = undefined;
        // approval_resolved 立即通知
        notifySubscribers(sessionId, true);
      } else if (event.type === 'pda_progress' && event.pda_progress) {
        const p = event.pda_progress;
        state.pdaProgress = p;
        // Build / update step list on first progress event or when total_steps changes
        if (!state.pdaSteps || state.pdaSteps.length !== p.total_steps) {
          state.pdaSteps = Array.from({ length: p.total_steps }, (_, i) => ({
            label: i === p.step_index ? p.step_label : `步骤 ${i + 1}`,
            type: i === p.step_index ? p.step_type : '',
            phase: i < p.step_index ? 'completed' as const : 'pending' as const,
          }));
        }
        // Update current step
        if (p.step_index < state.pdaSteps.length) {
          state.pdaSteps[p.step_index] = {
            label: p.step_label,
            type: p.step_type,
            phase: p.phase === 'started' ? 'started' : p.phase === 'completed' ? 'completed' : p.phase === 'failed' ? 'failed' : 'started',
          };
          // Mark all previous steps as completed
          for (let i = 0; i < p.step_index; i++) {
            if (state.pdaSteps[i].phase !== 'completed') {
              state.pdaSteps[i].phase = 'completed';
            }
          }
        }
        // Force new array reference for React change detection
        state.pdaSteps = [...state.pdaSteps];
        // pda_progress 立即通知
        notifySubscribers(sessionId, true);
      }
    };

    // 启动聊天请求
    chatFn(request, handleEvent, abortController.signal).catch(err => {
      if (!abortController.signal.aborted) {
        state.streaming = false;
        state.error = err.message;
        notifySubscribers(sessionId);
        onComplete?.(null, err.message);
      }
    });
  }, [notifySubscribers]);

  return (
    <ChatManagerContext.Provider value={{ getChatState, startChat, abortChat, isStreaming, subscribe }}>
      {children}
    </ChatManagerContext.Provider>
  );
};

export const useChatManager = (): ChatManagerContextType => {
  const context = useContext(ChatManagerContext);
  if (!context) {
    throw new Error('useChatManager must be used within a ChatManagerProvider');
  }
  return context;
};
