/**
 * ChatManager - 管理聊天请求的生命周期
 * 
 * 功能：
 * 1. 支持中止正在进行的聊天请求
 * 2. 页面切换后请求继续在后台运行
 * 3. 返回页面时可以查看进度
 */

import React, { createContext, useContext, useRef, useCallback, useState } from 'react';
import type { ChatRequest, StreamEvent, Message, ToolCallResult, ErrorDetail } from '../types';

export interface ChatState {
  sessionId: string;
  streaming: boolean;
  currentContent: string;
  currentToolCalls: { [key: string]: { name: string; result?: unknown; error?: string } };
  messages: Message[];
  error?: string;
  errorDetail?: ErrorDetail;  // Structured error info
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
  // 强制更新用的状态
  const [, forceUpdate] = useState(0);

  const notifySubscribers = useCallback((sessionId: string) => {
    const chat = activeChatsRef.current.get(sessionId);
    if (chat) {
      chat.subscribers.forEach(cb => cb({ ...chat.state }));
    }
    forceUpdate(n => n + 1);
  }, []);

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
      return () => {
        chat.subscribers.delete(callback);
      };
    }
    
    // 如果还没有 activeChat，添加到待处理订阅者
    let pending = pendingSubscribersRef.current.get(sessionId);
    if (!pending) {
      pending = new Set();
      pendingSubscribersRef.current.set(sessionId, pending);
    }
    pending.add(callback);
    
    return () => {
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
    let accumulatedToolCalls: { [key: string]: { name: string; arguments?: string; result?: unknown; error?: string } } = {};
    let isFinalized = false;

    const handleEvent = (event: StreamEvent) => {
      if (abortController.signal.aborted) return;

      const content = event.delta || event.content;
      if (event.type === 'content' && content) {
        accumulatedContent += content;
        state.currentContent = accumulatedContent;
        notifySubscribers(sessionId);
      } else if (event.type === 'tool_call' && event.tool_call) {
        const toolName = event.tool_call.name;
        const toolArgs = event.tool_call.arguments;
        if (toolName) {
          accumulatedToolCalls[toolName] = { name: toolName, arguments: toolArgs };
          state.currentToolCalls = { ...accumulatedToolCalls };
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
        state.currentContent = '';
        state.currentToolCalls = {};

        const toolCallsArray: ToolCallResult[] = Object.values(accumulatedToolCalls).filter(tc => tc.name);
        
        const assistantMessage: Message = {
          role: 'assistant',
          content: accumulatedContent,
          timestamp: new Date().toISOString(),
        };
        
        if (toolCallsArray.length > 0) {
          assistantMessage.tool_calls = toolCallsArray;
        }

        state.messages = [assistantMessage];
        notifySubscribers(sessionId);
        onComplete?.(assistantMessage);
      } else if (event.type === 'error') {
        state.streaming = false;
        state.error = event.error;
        // Store structured error detail if available
        if (event.error_detail) {
          state.errorDetail = event.error_detail;
        }
        notifySubscribers(sessionId);
        onComplete?.(null, event.error);
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
