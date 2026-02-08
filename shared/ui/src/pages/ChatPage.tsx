// ================================================================
// ChatPage - Shared chat page component
// ================================================================

import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Input, Button, List, Typography, Space, Spin, message, Select, Tooltip, Collapse, Modal, Tag, theme } from 'antd';
import { SendOutlined, ClearOutlined, PlusOutlined, ToolOutlined, FolderOutlined, FolderOpenOutlined, LinkOutlined, DisconnectOutlined, GithubOutlined, StopOutlined, CopyOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import { useAPI } from '../context/APIContext';
import { useTheme } from '../context/ThemeContext';
import { useChatManager } from '../context/ChatManager';
import { useInputHistory } from '../hooks';
import { PromptSelector } from '../components/PromptSelector';
import { OllamaIcon } from '../components/OllamaIcon';
import { StatusIndicator, ErrorBanner } from '../components';
import { useConnectionStatus, useHasConnectionIssues } from '../context/ConnectionStatusContext';
import moteLogo from '../assets/mote_logo.png';
import userAvatar from '../assets/user.png';
import type { Message, Model, Workspace, ErrorDetail } from '../types';

const { TextArea } = Input;
const { Text } = Typography;

export interface ChatPageProps {
  sessionId?: string;
  onSessionCreated?: (sessionId: string) => void;
}

export const ChatPage: React.FC<ChatPageProps> = ({ sessionId: initialSessionId, onSessionCreated }) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const { effectiveTheme } = useTheme();
  const chatManager = useChatManager();
  const { addHistory, navigatePrev, navigateNext, resetNavigation } = useInputHistory();
  
  // Connection status (may not be available if ConnectionStatusProvider is not present)
  let connectionStatus: ReturnType<typeof useConnectionStatus> | null = null;
  let hasConnectionIssues = false;
  try {
    connectionStatus = useConnectionStatus();
    hasConnectionIssues = useHasConnectionIssues();
  } catch {
    // ConnectionStatusProvider not available, ignore
  }
  
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [currentResponse, setCurrentResponse] = useState('');
  // 跟踪是否正在浏览历史记录
  const isNavigatingHistoryRef = useRef(false);
  const [sessionId, setSessionId] = useState<string | undefined>(initialSessionId);
  const [sessionLoading, setSessionLoading] = useState(true);
  const [models, setModels] = useState<Model[]>([]);
  const [currentModel, setCurrentModel] = useState<string>('');
  const [modelLoading, setModelLoading] = useState(false);
  const [currentToolCalls, setCurrentToolCalls] = useState<{ [key: string]: { name: string; arguments?: string; result?: any; error?: string } }>({});
  const [promptSelectorVisible, setPromptSelectorVisible] = useState(false);
  const [promptSearchQuery, setPromptSearchQuery] = useState('');
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [workspaceModalVisible, setWorkspaceModalVisible] = useState(false);
  const [workspacePath, setWorkspacePath] = useState('');
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingMessage, setEditingMessage] = useState<{ index: number; content: string } | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  // 用于保存当前累积的响应内容（停止时使用）
  const currentResponseRef = useRef('');
  // Stream error state
  const [streamError, setStreamError] = useState<ErrorDetail | null>(null);

  const scrollToBottom = (behavior: ScrollBehavior = 'smooth') => {
    messagesEndRef.current?.scrollIntoView({ behavior });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, currentResponse]);

  // Load models on mount
  useEffect(() => {
    const loadModels = async () => {
      try {
        const response = await api.getModels();
        setModels(response.models || []);
        setCurrentModel(response.current || response.default || '');
      } catch (error) {
        console.error('Failed to load models:', error);
      }
    };
    loadModels();
  }, [api]);

  // Initialize session - NEVER auto-create, only load existing or show empty state
  const initializeSession = useCallback(async () => {
    setSessionLoading(true);
    try {
      if (initialSessionId) {
        // Load existing session
        setSessionId(initialSessionId);
        try {
          const history = await api.getSessionMessages(initialSessionId);
          setMessages(history || []);
        } catch (e) {
          console.error('Failed to load session messages:', e);
          setMessages([]);
        }
        // Load session model if available
        if (api.getSession) {
          try {
            const session = await api.getSession(initialSessionId);
            if (session.model) {
              setCurrentModel(session.model);
            }
          } catch (e) {
            console.error('Failed to load session model:', e);
          }
        }
      } else {
        // No session ID provided - try to find the most recent chat session
        try {
          const sessions = await api.getSessions();
          // Find the most recent session with 'chat' scenario, or fallback to any session
          const chatSession = sessions.find(s => s.scenario === 'chat') || sessions[0];
          
          if (chatSession) {
            // Use existing session
            setSessionId(chatSession.id);
            onSessionCreated?.(chatSession.id);
            try {
              const history = await api.getSessionMessages(chatSession.id);
              setMessages(history || []);
            } catch (e) {
              console.error('Failed to load session messages:', e);
              setMessages([]);
            }
            if (api.getSession) {
              try {
                const session = await api.getSession(chatSession.id);
                if (session.model) {
                  setCurrentModel(session.model);
                }
              } catch (e) {
                console.error('Failed to load session model:', e);
              }
            }
          } else {
            // No sessions exist - show empty state, user must click "New" to create
            setSessionId(undefined);
            setMessages([]);
          }
        } catch (e) {
          console.error('Failed to fetch sessions:', e);
          // On error, show empty state
          setSessionId(undefined);
          setMessages([]);
        }
      }
    } catch (error) {
      console.error('Failed to initialize session:', error);
      setSessionId(undefined);
      setMessages([]);
    } finally {
      setSessionLoading(false);
    }
  }, [api, initialSessionId, onSessionCreated]);

  // 处理来自 NewChatPage 的 pending message
  const processPendingMessage = useCallback(async (sid: string) => {
    const storageKey = `mote_pending_message_${sid}`;
    const pendingMessage = sessionStorage.getItem(storageKey);
    if (pendingMessage) {
      sessionStorage.removeItem(storageKey);
      // 等待一小段时间确保页面已完全渲染
      await new Promise(resolve => setTimeout(resolve, 100));
      
      const userMessage: Message = {
        role: 'user',
        content: pendingMessage.trim(),
        timestamp: new Date().toISOString(),
      };
      
      addHistory(pendingMessage.trim());
      setMessages((prev: Message[]) => [...prev, userMessage]);
      setLoading(true);
      setStreaming(true);
      setCurrentResponse('');
      setCurrentToolCalls({});
      currentResponseRef.current = '';

      // 使用 ChatManager 发起请求
      chatManager.startChat(
        sid,
        {
          message: userMessage.content,
          session_id: sid,
          stream: true,
        },
        api.chat,
        (_assistantMessage, error) => {
          if (error) {
            message.error(`发送失败: ${error}`);
          }
          // 消息添加由 ChatManager 订阅回调处理
        }
      );
    }
  }, [api, addHistory, chatManager]);

  useEffect(() => {
    initializeSession().then(() => {
      // 初始加载后滚动到底部（使用 instant 避免动画延迟）
      setTimeout(() => scrollToBottom('instant'), 100);
      
      // 检查是否有来自 NewChatPage 的 pending message
      if (initialSessionId) {
        processPendingMessage(initialSessionId);
      }
    });
  }, [initializeSession, initialSessionId, processPendingMessage]);

  // Subscribe to ChatManager state for background task persistence
  useEffect(() => {
    if (!sessionId) return;

    // Restore state if there's an active chat for this session
    const existingState = chatManager.getChatState(sessionId);
    if (existingState) {
      setStreaming(existingState.streaming);
      setCurrentResponse(existingState.currentContent || '');
      currentResponseRef.current = existingState.currentContent || '';
      setCurrentToolCalls(existingState.currentToolCalls || {});
      if (existingState.streaming) {
        setLoading(true);
      }
    }

    // Subscribe to state changes
    const unsubscribe = chatManager.subscribe(sessionId, (state) => {
      setStreaming(state.streaming);
      setCurrentResponse(state.currentContent || '');
      currentResponseRef.current = state.currentContent || '';
      setCurrentToolCalls(state.currentToolCalls || {});
      
      // Handle errors with structured error detail
      if (state.errorDetail) {
        setStreamError(state.errorDetail);
      } else if (state.error && !state.streaming) {
        // Fallback for unstructured errors
        setStreamError({
          code: 'UNKNOWN',
          message: state.error,
          retryable: false,
        });
      }
      
      if (!state.streaming) {
        setLoading(false);
        // When streaming ends, add the final message to the list
        if (state.currentContent && state.currentContent.trim()) {
          setMessages((prev: Message[]) => {
            // Avoid duplicating if already added
            const lastMsg = prev[prev.length - 1];
            if (lastMsg && lastMsg.role === 'assistant' && lastMsg.content === state.currentContent) {
              return prev;
            }
            return [...prev, {
              role: 'assistant' as const,
              content: state.currentContent,
              tool_calls: state.currentToolCalls ? Object.values(state.currentToolCalls) : undefined,
              timestamp: new Date().toISOString(),
            }];
          });
          setCurrentResponse('');
          setCurrentToolCalls({});
        }
      }
    });

    return unsubscribe;
  }, [sessionId, chatManager]);

  // Load workspace binding for current session
  useEffect(() => {
    const loadWorkspace = async () => {
      if (!sessionId || !api.getWorkspaces) return;
      try {
        const workspaces = await api.getWorkspaces();
        const currentWorkspace = workspaces.find((w: Workspace) => w.session_id === sessionId);
        setWorkspace(currentWorkspace || null);
      } catch (e) {
        console.error('Failed to load workspace:', e);
        setWorkspace(null);
      }
    };
    loadWorkspace();
  }, [api, sessionId]);

  // Handle workspace binding
  const handleBindWorkspace = async () => {
    if (!sessionId || !workspacePath.trim()) {
      message.warning('请输入工作区路径');
      return;
    }
    try {
      const result = await api.bindWorkspace?.(sessionId, workspacePath.trim());
      if (result) {
        setWorkspace(result);
        message.success('工作区已绑定');
      }
      setWorkspaceModalVisible(false);
      setWorkspacePath('');
    } catch (error: any) {
      message.error(error.message || '绑定工作区失败');
    }
  };

  // Handle workspace unbinding
  const handleUnbindWorkspace = async () => {
    if (!sessionId) return;
    try {
      await api.unbindWorkspace?.(sessionId);
      setWorkspace(null);
      message.success('工作区已解绑');
    } catch (error: any) {
      message.error(error.message || '解绑工作区失败');
    }
  };

  // Handle new session creation
  const handleNewSession = async () => {
    try {
      const session = await api.createSession('New Chat', 'chat');
      setSessionId(session.id);
      setMessages([]);
      // Update current model to session's model
      if (session.model) {
        setCurrentModel(session.model);
      }
      onSessionCreated?.(session.id);
      message.success('新会话已创建');
    } catch (error) {
      message.error('创建会话失败');
    }
  };

  // Handle model change (per session only)
  const handleModelChange = async (modelId: string) => {
    if (!sessionId) {
      message.error('无会话可设置模型');
      return;
    }
    if (!api.setSessionModel) {
      message.error('当前版本不支持会话级模型切换');
      return;
    }
    setModelLoading(true);
    try {
      await api.setSessionModel(sessionId, modelId);
      setCurrentModel(modelId);
      message.success('会话模型已切换');
    } catch (error) {
      message.error('切换模型失败');
    } finally {
      setModelLoading(false);
    }
  };

  const handleSend = async () => {
    if (!inputValue.trim() || loading || !sessionId) return;

    const userMessage: Message = {
      role: 'user',
      content: inputValue.trim(),
      timestamp: new Date().toISOString(),
    };

    // 添加到输入历史
    addHistory(inputValue.trim());

    setMessages((prev: Message[]) => [...prev, userMessage]);
    setInputValue('');
    setLoading(true);
    setStreaming(true);
    setCurrentResponse('');
    setCurrentToolCalls({});
    currentResponseRef.current = '';

    // 使用 ChatManager 发起请求，它会在后台持续运行
    chatManager.startChat(
      sessionId,
      {
        message: userMessage.content,
        session_id: sessionId,
        stream: true,
      },
      api.chat,
      (_assistantMessage, error) => {
        if (error) {
          message.error(`发送失败: ${error}`);
        }
        // 消息添加由 ChatManager 订阅回调处理
      }
    );
  };

  const handleStop = () => {
    if (streaming && sessionId) {
      chatManager.abortChat(sessionId);
      
      // 如果有累积的响应内容，保存为一条中断的消息
      if (currentResponseRef.current.trim()) {
        const interruptedMessage: Message = {
          role: 'assistant',
          content: currentResponseRef.current + '\n\n*[已停止]*',
          timestamp: new Date().toISOString(),
        };
        setMessages((prev: Message[]) => [...prev, interruptedMessage]);
      }
      
      setLoading(false);
      setStreaming(false);
      setCurrentResponse('');
      setCurrentToolCalls({});
      currentResponseRef.current = '';
      message.info('已停止生成');
    }
  };

  const handleClear = async () => {
    if (!sessionId) {
      message.warning('没有活动会话');
      return;
    }
    // Clear messages in current session (keep using same session)
    setMessages([]);
    // Optionally clear server-side messages if API supports it
    // For now, just clear the UI state
    message.success('聊天已清空');
  };

  const handleKeyPress = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      // 发送时重置历史导航状态
      isNavigatingHistoryRef.current = false;
      handleSend();
    }
    // 上键导航到上一条历史：输入为空或者正在浏览历史时都可以继续导航
    if (e.key === 'ArrowUp' && !e.shiftKey && (inputValue === '' || isNavigatingHistoryRef.current)) {
      e.preventDefault();
      const prev = navigatePrev();
      if (prev !== null) {
        isNavigatingHistoryRef.current = true;
        setInputValue(prev);
      }
    }
    // 下键导航到下一条历史：只有在浏览历史时才响应
    if (e.key === 'ArrowDown' && !e.shiftKey && isNavigatingHistoryRef.current) {
      e.preventDefault();
      const next = navigateNext();
      if (next !== null) {
        setInputValue(next);
      } else {
        // 返回到最新位置，清空输入，结束历史浏览
        isNavigatingHistoryRef.current = false;
        setInputValue('');
      }
    }
    // Close prompt selector on Escape
    if (e.key === 'Escape' && promptSelectorVisible) {
      setPromptSelectorVisible(false);
      setPromptSearchQuery('');
    }
  };

  // Handle input change with prompt detection
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    setInputValue(value);
    // 用户手动输入时退出历史浏览模式
    isNavigatingHistoryRef.current = false;

    // 用户手动输入时重置历史导航
    resetNavigation();

    // Detect / command at the start of input
    const match = value.match(/^\/(\S*)$/);
    if (match) {
      setPromptSelectorVisible(true);
      setPromptSearchQuery(match[1] || '');
    } else if (!value.startsWith('/')) {
      setPromptSelectorVisible(false);
      setPromptSearchQuery('');
    }
  };

  // Handle prompt selection
  const handlePromptSelect = (content: string) => {
    setInputValue(content);
    setPromptSelectorVisible(false);
    setPromptSearchQuery('');
  };

  // Copy message content to clipboard
  const handleCopyMessage = (content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      message.success('已复制到剪贴板');
    }).catch(() => {
      message.error('复制失败');
    });
  };

  // Open edit modal for a message
  const handleEditMessage = (index: number, content: string) => {
    setEditingMessage({ index, content });
    setEditModalVisible(true);
  };

  // Save edited message
  const handleSaveEdit = () => {
    if (editingMessage) {
      setMessages(prev => prev.map((msg, idx) => 
        idx === editingMessage.index 
          ? { ...msg, content: editingMessage.content }
          : msg
      ));
      setEditModalVisible(false);
      setEditingMessage(null);
      message.success('消息已更新');
    }
  };

  // Delete a message
  const handleDeleteMessage = (index: number) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除这条消息吗？',
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: () => {
        setMessages(prev => prev.filter((_, idx) => idx !== index));
        message.success('消息已删除');
      },
    });
  };

  const renderMessage = (msg: Message, index: number) => {
    const isUser = msg.role === 'user';
    const hasToolCalls = msg.tool_calls && msg.tool_calls.length > 0;
    
    // Check if content looks like JSON (tool result)
    const isJsonContent = !isUser && msg.content && msg.content.trim().startsWith('{') && msg.content.trim().endsWith('}');
    
    // Hide JSON-only assistant messages (they're likely tool results shown separately)
    if (!isUser && isJsonContent && !hasToolCalls) {
      return null;
    }
    
    // Don't render main content bubble if it's empty or only whitespace
    const hasContent = msg.content && msg.content.trim().length > 0 && !isJsonContent;
    
    return (
      <List.Item
        key={index}
        style={{
          justifyContent: isUser ? 'flex-end' : 'flex-start',
          padding: '12px 16px',
          border: 'none',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            flexDirection: isUser ? 'row-reverse' : 'row',
            gap: 12,
            maxWidth: '80%',
          }}
        >
          {isUser ? (
            <div
              style={{
                width: 36,
                height: 36,
                backgroundColor: effectiveTheme === 'dark' ? '#1f1f1f' : '#F5F5F5',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 6,
                flexShrink: 0,
                marginTop: 4,
              }}
            >
              <img src={userAvatar} alt="User" style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
            </div>
          ) : (
            <div
              style={{
                width: 36,
                height: 36,
                backgroundColor: effectiveTheme === 'dark' ? '#1f1f1f' : '#F5F5F5',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 6,
                flexShrink: 0,
                marginTop: 4,
              }}
            >
              <img src={moteLogo} alt="AI" style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
            </div>
          )}
          <div style={{ flex: 1 }}>
            {/* Tool Calls - Collapsible - Show before content for consistency */}
            {hasToolCalls && (
              <div style={{ marginBottom: hasContent ? 8 : 0 }}>
                <Collapse
                  ghost
                  size="small"
                  items={[
                    {
                      key: '1',
                      label: (
                        <span style={{ fontSize: 12, color: token.colorTextSecondary }}>
                          <ToolOutlined style={{ marginRight: 4 }} />
                          工具调用 ({msg.tool_calls?.length})
                        </span>
                      ),
                      children: (
                        <div style={{ fontSize: 12, color: token.colorTextSecondary }}>
                          {msg.tool_calls?.map((tool, idx) => (
                            <div key={idx} style={{ marginBottom: idx < msg.tool_calls!.length - 1 ? 12 : 0 }}>
                              <div style={{ fontWeight: 500, marginBottom: 4 }}>
                                {tool.name}
                              </div>
                              {tool.arguments && (
                                <div style={{ marginBottom: 4 }}>
                                  <div style={{ color: token.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>参数:</div>
                                  <pre style={{
                                    background: token.colorBgLayout,
                                    padding: 8,
                                    borderRadius: 4,
                                    overflow: 'auto',
                                    margin: 0,
                                    fontSize: 11,
                                    border: `1px solid ${token.colorBorderSecondary}`,
                                    whiteSpace: 'pre-wrap',
                                    wordBreak: 'break-word',
                                    maxWidth: '100%',
                                  }}>
                                    {(() => {
                                      try {
                                        return JSON.stringify(JSON.parse(tool.arguments!), null, 2);
                                      } catch {
                                        return tool.arguments;
                                      }
                                    })()}
                                  </pre>
                                </div>
                              )}
                              {tool.error && (
                                <div style={{ color: '#ff4d4f', marginBottom: 4 }}>
                                  错误: {tool.error}
                                </div>
                              )}
                              {tool.result && (
                                <div>
                                  <div style={{ color: token.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>结果:</div>
                                  <pre style={{
                                    background: token.colorBgLayout,
                                    padding: 8,
                                    borderRadius: 4,
                                    overflow: 'auto',
                                    margin: 0,
                                    fontSize: 11,
                                    border: `1px solid ${token.colorBorderSecondary}`,
                                    whiteSpace: 'pre-wrap',
                                    wordBreak: 'break-word',
                                    maxWidth: '100%',
                                  }}>
                                    {typeof tool.result === 'string' 
                                      ? tool.result 
                                      : JSON.stringify(tool.result, null, 2)}
                                  </pre>
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      ),
                    },
                  ]}
                />
              </div>
            )}            
            {/* Main message content - only show if there's actual content - Display after tool calls */}
            {hasContent && (
              <div
                style={{
                  background: isUser ? token.colorPrimary : token.colorBgLayout,
                  color: isUser ? '#fff' : token.colorText,
                  padding: '12px 16px',
                  borderRadius: 12,
                  maxWidth: '100%',
                  wordBreak: 'break-word',
                  marginTop: hasToolCalls ? 8 : 0,
                  fontSize: 13,
                }}
              >
                {isUser ? (
                  <Text style={{ color: '#fff', whiteSpace: 'pre-wrap', fontSize: 13 }}>{msg.content}</Text>
                ) : (
                  <div className="markdown-content">
                    <ReactMarkdown>{msg.content}</ReactMarkdown>
                  </div>
                )}
              </div>
            )}
            {/* Action buttons for assistant messages */}
            {!isUser && hasContent && (
              <div style={{ 
                display: 'flex', 
                gap: 4, 
                marginTop: 4,
                opacity: 0.6,
              }}
              className="message-actions"
              >
                <Tooltip title="复制">
                  <Button
                    type="text"
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => handleCopyMessage(msg.content)}
                    style={{ fontSize: 12, color: token.colorTextSecondary }}
                  />
                </Tooltip>
                <Tooltip title="编辑">
                  <Button
                    type="text"
                    size="small"
                    icon={<EditOutlined />}
                    onClick={() => handleEditMessage(index, msg.content)}
                    style={{ fontSize: 12, color: token.colorTextSecondary }}
                  />
                </Tooltip>
                <Tooltip title="删除">
                  <Button
                    type="text"
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => handleDeleteMessage(index)}
                    style={{ fontSize: 12, color: token.colorTextSecondary }}
                  />
                </Tooltip>
              </div>
            )}
          </div>
        </div>
      </List.Item>
    );
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Error Banner - shown when there's a connection or stream error */}
      {(connectionStatus?.status.activeError || streamError) && (
        <div style={{ padding: '8px 24px 0', background: token.colorBgContainer }}>
          <ErrorBanner
            source={connectionStatus?.status.activeError?.source || 'stream'}
            name={connectionStatus?.status.activeError?.name || 'provider'}
            detail={connectionStatus?.status.activeError?.detail || streamError!}
            onDismiss={() => {
              connectionStatus?.dismissError();
              setStreamError(null);
            }}
            onRetry={async () => {
              const activeError = connectionStatus?.status.activeError;
              if (activeError?.source === 'provider') {
                await connectionStatus?.recoverProvider(activeError.name);
              } else if (activeError?.source === 'mcp') {
                await connectionStatus?.reconnectMCP(activeError.name);
              }
              setStreamError(null);
            }}
          />
        </div>
      )}
      
      {/* Header - 无标题 */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          {/* Left side: Connection status indicator */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {connectionStatus && (
              <Tooltip title={hasConnectionIssues ? '部分连接异常，点击查看' : '连接正常'}>
                <div>
                  <StatusIndicator
                    status={connectionStatus.status.overallHealth}
                    showLabel={hasConnectionIssues}
                    size="sm"
                    onClick={() => connectionStatus.refreshStatus()}
                  />
                </div>
              </Tooltip>
            )}
          </div>
          
          {/* Right side: Controls */}
          <Space>
            <Select
              value={currentModel}
              onChange={handleModelChange}
              loading={modelLoading}
              style={{ width: 240 }}
              placeholder="选择模型"
              optionLabelProp="label"
            >
              {/* Group models by provider if multiple providers exist */}
              {(() => {
                const providers = [...new Set(models.map(m => m.provider || 'copilot'))];
                if (providers.length <= 1) {
                  // Single provider: flat list
                  return models.map((model) => (
                    <Select.Option 
                      key={model.id} 
                      value={model.id} 
                      label={model.display_name}
                      disabled={model.available === false}
                    >
                      <Tooltip title={model.description} placement="left">
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                          <span>{model.display_name}</span>
                          <Space size={4}>
                            {model.is_free && <Tag color="green" style={{ marginRight: 0 }}>免费</Tag>}
                            {model.available === false && <Tag color="red" style={{ marginRight: 0 }}>不可用</Tag>}
                          </Space>
                        </div>
                      </Tooltip>
                    </Select.Option>
                  ));
                }
                // Multiple providers: use OptGroup
                const grouped: Record<string, Model[]> = {};
                models.forEach(m => {
                  const p = m.provider || 'copilot';
                  if (!grouped[p]) grouped[p] = [];
                  grouped[p].push(m);
                });
                return Object.keys(grouped).map(provider => (
                  <Select.OptGroup 
                    key={provider}
                    label={
                      <Space>
                        {provider === 'copilot' ? <GithubOutlined /> : <OllamaIcon />}
                        {provider === 'copilot' ? 'GitHub Copilot' : 'Ollama'}
                      </Space>
                    }
                  >
                    {grouped[provider].map(model => (
                      <Select.Option 
                        key={model.id} 
                        value={model.id} 
                        label={model.display_name}
                        disabled={model.available === false}
                      >
                        <Tooltip title={model.description} placement="left">
                          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <span>{model.display_name}</span>
                            <Space size={4}>
                              {model.is_free && <Tag color="green" style={{ marginRight: 0 }}>免费</Tag>}
                              {model.available === false && <Tag color="red" style={{ marginRight: 0 }}>不可用</Tag>}
                            </Space>
                          </div>
                        </Tooltip>
                      </Select.Option>
                    ))}
                  </Select.OptGroup>
                ));
              })()}
            </Select>
            {/* Workspace button */}
            {workspace ? (
              <Tooltip title={`工作区: ${workspace.path}`}>
                <Button 
                  icon={<LinkOutlined />} 
                  onClick={() => setWorkspaceModalVisible(true)}
                  style={{ color: '#52c41a' }}
                  className="page-header-btn"
                >
                  {workspace.alias || workspace.path.split('/').pop()}
                </Button>
              </Tooltip>
            ) : (
              <Tooltip title="设置工作区">
                <Button 
                  icon={<FolderOutlined />} 
                  onClick={() => setWorkspaceModalVisible(true)}
                  disabled={!sessionId}
                  className="page-header-btn"
                >
                  工作区
                </Button>
              </Tooltip>
            )}
            <Tooltip title="新建对话">
              <Button icon={<PlusOutlined />} onClick={handleNewSession} className="page-header-btn">
                新建
              </Button>
            </Tooltip>
            <Button icon={<ClearOutlined />} onClick={handleClear} className="page-header-btn">
              清空
            </Button>
          </Space>
        </div>
      </div>

      {/* Workspace Modal */}
      <Modal
        title={workspace ? '工作区设置' : '绑定工作区'}
        open={workspaceModalVisible}
        onCancel={() => {
          setWorkspaceModalVisible(false);
          setWorkspacePath('');
        }}
        footer={workspace ? [
          <Button key="open" type="primary" icon={<FolderOpenOutlined />} onClick={() => {
            // Copy workspace path to clipboard
            navigator.clipboard.writeText(workspace.path).then(() => {
              message.success('工作区路径已复制到剪贴板');
            }).catch(() => {
              message.info(`工作区路径: ${workspace.path}`);
            });
          }}>
            复制路径
          </Button>,
          <Button key="unbind" danger icon={<DisconnectOutlined />} onClick={handleUnbindWorkspace}>
            解除绑定
          </Button>,
          <Button key="cancel" onClick={() => setWorkspaceModalVisible(false)}>
            关闭
          </Button>,
        ] : [
          <Button key="cancel" onClick={() => setWorkspaceModalVisible(false)}>
            取消
          </Button>,
          <Button key="bind" type="primary" onClick={handleBindWorkspace}>
            绑定
          </Button>,
        ]}
      >
        {workspace ? (
          <div>
            <Typography.Paragraph>
              <strong>路径:</strong> {workspace.path}
            </Typography.Paragraph>
            {workspace.alias && (
              <Typography.Paragraph>
                <strong>别名:</strong> {workspace.alias}
              </Typography.Paragraph>
            )}
            <Typography.Paragraph>
              <strong>模式:</strong> {workspace.read_only ? '只读' : '读写'}
            </Typography.Paragraph>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
              工作区绑定后，对话中可以访问该目录下的文件
            </Typography.Paragraph>
          </div>
        ) : (
          <div>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
              绑定工作区后，对话中可以访问该目录下的文件。工作区与当前会话关联。
            </Typography.Paragraph>
            <Input
              placeholder="输入工作区路径，如 /path/to/project"
              value={workspacePath}
              onChange={(e) => setWorkspacePath(e.target.value)}
              onPressEnter={handleBindWorkspace}
            />
          </div>
        )}
      </Modal>

      {/* Edit Message Modal */}
      <Modal
        title="编辑消息"
        open={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false);
          setEditingMessage(null);
        }}
        footer={[
          <Button key="cancel" onClick={() => {
            setEditModalVisible(false);
            setEditingMessage(null);
          }}>
            取消
          </Button>,
          <Button key="save" type="primary" onClick={handleSaveEdit}>
            保存
          </Button>,
        ]}
        width={600}
      >
        <Input.TextArea
          value={editingMessage?.content || ''}
          onChange={(e) => setEditingMessage(prev => prev ? { ...prev, content: e.target.value } : null)}
          autoSize={{ minRows: 6, maxRows: 20 }}
          style={{ fontFamily: 'monospace', fontSize: 13 }}
          placeholder="输入消息内容 (Markdown 格式)"
        />
      </Modal>

      {/* Messages List */}
      <div style={{ flex: 1, overflow: 'auto', padding: '0 8px' }}>
        {sessionLoading ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <Spin tip="加载中..." />
          </div>
        ) : messages.length === 0 && !streaming ? (
          <div style={{ textAlign: 'center', padding: 48, color: '#999' }}>
            <img src={moteLogo} alt="Mote" style={{ width: 48, height: 48, marginBottom: 16 }} />
            <div>开始对话吧！</div>
          </div>
        ) : (
          <List dataSource={messages} renderItem={renderMessage} split={false} />
        )}

        {/* Streaming Response */}
        {streaming && (currentResponse || Object.keys(currentToolCalls).length > 0) && (
          <List.Item style={{ justifyContent: 'flex-start', padding: '16px 16px', border: 'none' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                flexDirection: 'row',
                gap: 12,
                maxWidth: '80%',
              }}
            >
              <div
                style={{
                  width: 36,
                  height: 36,
                  backgroundColor: effectiveTheme === 'dark' ? '#1f1f1f' : '#F5F5F5',
                  borderRadius: '50%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  padding: 6,
                  flexShrink: 0,
                  marginTop: 4,
                }}
              >
                <img src={moteLogo} alt="AI" style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
              </div>
              <div style={{ flex: 1 }}>
                {/* Show current tool calls first */}
                {Object.keys(currentToolCalls).length > 0 && (
                  <div style={{ marginBottom: currentResponse ? 8 : 0 }}>
                    <Collapse
                      ghost
                      size="small"
                      defaultActiveKey={['1']}
                      items={[
                        {
                          key: '1',
                          label: (
                            <span style={{ fontSize: 12, color: token.colorTextSecondary }}>
                              <ToolOutlined style={{ marginRight: 4 }} />
                              工具调用 ({Object.keys(currentToolCalls).length})
                            </span>
                          ),
                          children: (
                            <div style={{ fontSize: 12, color: token.colorTextSecondary }}>
                              {Object.values(currentToolCalls).map((tool, idx) => (
                                <div key={idx} style={{ marginBottom: idx < Object.keys(currentToolCalls).length - 1 ? 12 : 0 }}>
                                  <div style={{ fontWeight: 500, marginBottom: 4 }}>
                                    {tool.name}
                                    {!tool.result && !tool.error && (
                                      <Spin size="small" style={{ marginLeft: 8 }} />
                                    )}
                                  </div>
                                  {tool.arguments && (
                                    <div style={{ marginBottom: 4 }}>
                                      <div style={{ color: token.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>参数:</div>
                                      <pre style={{
                                        background: token.colorBgLayout,
                                        padding: 8,
                                        borderRadius: 4,
                                        overflow: 'auto',
                                        margin: 0,
                                        fontSize: 11,
                                        border: `1px solid ${token.colorBorderSecondary}`,
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        maxWidth: '100%',
                                      }}>
                                        {(() => {
                                          try {
                                            return JSON.stringify(JSON.parse(tool.arguments!), null, 2);
                                          } catch {
                                            return tool.arguments;
                                          }
                                        })()}
                                      </pre>
                                    </div>
                                  )}
                                  {tool.error && (
                                    <div style={{ color: '#ff4d4f', marginBottom: 4 }}>
                                      错误: {tool.error}
                                    </div>
                                  )}
                                  {tool.result && !tool.error && (
                                    <div>
                                      <div style={{ color: token.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>结果:</div>
                                      <pre style={{
                                        background: token.colorBgLayout,
                                        padding: 8,
                                        borderRadius: 4,
                                        overflow: 'auto',
                                        margin: 0,
                                        fontSize: 11,
                                        border: `1px solid ${token.colorBorderSecondary}`,
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        maxWidth: '100%',
                                      }}>
                                        {typeof tool.result === 'string' 
                                          ? tool.result 
                                          : JSON.stringify(tool.result, null, 2)}
                                      </pre>
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          ),
                        },
                      ]}
                    />
                  </div>
                )}
                
                {/* Show streaming content */}
                {currentResponse && (
                  <div
                    style={{
                      background: token.colorBgLayout,
                      color: token.colorText,
                      padding: '12px 16px',
                      borderRadius: 12,
                      maxWidth: '100%',
                      fontSize: 13,
                    }}
                  >
                    <div className="markdown-content">
                      <ReactMarkdown>{currentResponse}</ReactMarkdown>
                    </div>
                    <Spin size="small" style={{ marginLeft: 8 }} />
                  </div>
                )}
              </div>
            </div>
          </List.Item>
        )}

        {loading && !currentResponse && (
          <div style={{ textAlign: 'center', padding: 16 }}>
            <Spin tip="思考中..." />
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Area */}
      <div style={{ padding: 16, borderTop: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, position: 'relative' }}>
        {/* Prompt Selector */}
        <PromptSelector
          visible={promptSelectorVisible}
          searchQuery={promptSearchQuery}
          onSelect={handlePromptSelect}
          onCancel={() => {
            setPromptSelectorVisible(false);
            setPromptSearchQuery('');
          }}
        />

        <Space.Compact style={{ width: '100%' }}>
          <TextArea
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyPress}
            placeholder="输入消息... (/ 引用提示词, Shift+Enter 换行)"
            autoSize={{ minRows: 1, maxRows: 10 }}
            disabled={loading}
            style={{ resize: 'none' }}
            className="mote-input"
          />
          {streaming ? (
            <Button
              type="primary"
              danger
              icon={<StopOutlined />}
              onClick={handleStop}
            >
              停止
            </Button>
          ) : (
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={handleSend}
              loading={loading}
              disabled={!inputValue.trim()}
            >
              发送
            </Button>
          )}
        </Space.Compact>
      </div>
    </div>
  );
};
