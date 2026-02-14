// ================================================================
// ChatPage - Shared chat page component
// ================================================================

import React, { useState, useRef, useEffect, useCallback, useMemo, memo } from 'react';
import { Input, Button, Typography, Space, Spin, message, Select, Tooltip, Collapse, Modal, Tag, theme, Dropdown } from 'antd';
import { SendOutlined, ClearOutlined, PlusOutlined, ToolOutlined, FolderOutlined, FolderOpenOutlined, LinkOutlined, DisconnectOutlined, GithubOutlined, StopOutlined, CopyOutlined, EditOutlined, DeleteOutlined, ThunderboltOutlined, PauseOutlined, PlayCircleOutlined, CloseCircleFilled } from '@ant-design/icons';
import { MinimaxIcon } from '../components/MinimaxIcon';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useAPI } from '../context/APIContext';
import { useTheme } from '../context/ThemeContext';
import { useChatManager } from '../context/ChatManager';
import { useInputHistory } from '../hooks';
import { PromptSelector } from '../components/PromptSelector';
import { DirectoryPicker } from '../components/DirectoryPicker';
import { FileSelector } from '../components/FileSelector';
import type { FileSelectorMode } from '../components/FileSelector';
import { OllamaIcon } from '../components/OllamaIcon';
import { StatusIndicator, ErrorBanner } from '../components';
import { useConnectionStatus, useHasConnectionIssues } from '../context/ConnectionStatusContext';
import moteLogo from '../assets/mote_logo.png';
import userAvatar from '../assets/user.png';
import type { Message, Model, Workspace, ErrorDetail, Skill, ReconfigureSessionResponse, ImageAttachment } from '../types';

const { TextArea } = Input;
const { Text } = Typography;

// ================================================================
// Memoized Message Item Component - Prevents unnecessary re-renders
// ================================================================
interface TokenColors {
  colorBgLayout: string;
  colorBgContainer: string;
  colorText: string;
  colorTextSecondary: string;
  colorBorderSecondary: string;
  colorPrimary: string;
  colorWarning: string;
  colorWarningBg: string;
  colorWarningBorder: string;
  colorWarningText: string;
}

interface MessageItemProps {
  msg: Message;
  index: number;
  isUser: boolean;
  effectiveTheme: string;
  tokenColors: TokenColors;
  onCopy: (content: string) => void;
  onEdit: (index: number, content: string) => void;
  onDelete: (index: number) => void;
}

const MessageItem = memo<MessageItemProps>(({ 
  msg, 
  index, 
  isUser, 
  effectiveTheme, 
  tokenColors, 
  onCopy, 
  onEdit, 
  onDelete 
}) => {
  const hasToolCalls = msg.tool_calls && msg.tool_calls.length > 0;
  const hasContent = msg.content && msg.content.trim().length > 0;

  return (
    <div style={{ padding: '12px 0' }}>
      <div style={{ display: 'flex', gap: 12, width: '100%', alignItems: 'flex-start' }}>
        {isUser ? (
          <div
            style={{
              width: 36,
              height: 36,
              backgroundColor: '#8B5CF6',
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
        <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
          {/* Tool Calls - Collapsible */}
          {hasToolCalls && (
            <div style={{ marginBottom: hasContent ? 8 : 0 }}>
              <Collapse
                ghost
                size="small"
                items={[
                  {
                    key: '1',
                    label: (
                      <span style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}>
                        <ToolOutlined style={{ marginRight: 4 }} />
                        工具调用 ({msg.tool_calls?.length})
                      </span>
                    ),
                    children: (
                      <div style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}>
                        {msg.tool_calls?.map((tool, idx) => (
                          <div key={idx} style={{ marginBottom: idx < msg.tool_calls!.length - 1 ? 12 : 0 }}>
                            <div style={{ fontWeight: 500, marginBottom: 4 }}>
                              {tool.name}
                            </div>
                            {tool.arguments && (
                              <div style={{ marginBottom: 4 }}>
                                <div style={{ color: tokenColors.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>参数:</div>
                                <pre style={{
                                  background: tokenColors.colorBgLayout,
                                  padding: 8,
                                  borderRadius: 4,
                                  overflow: 'auto',
                                  margin: 0,
                                  fontSize: 11,
                                  border: `1px solid ${tokenColors.colorBorderSecondary}`,
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
                                <div style={{ color: tokenColors.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>结果:</div>
                                <pre style={{
                                  background: tokenColors.colorBgLayout,
                                  padding: 8,
                                  borderRadius: 4,
                                  overflow: 'auto',
                                  margin: 0,
                                  fontSize: 11,
                                  border: `1px solid ${tokenColors.colorBorderSecondary}`,
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word',
                                  maxWidth: '100%',
                                }}>
                                  {typeof tool.result === 'string' 
                                    ? tool.result 
                                    : JSON.stringify(tool.result as object, null, 2)}
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
          
          {/* Message Content */}
          {(hasContent || (isUser && msg.images && msg.images.length > 0)) && (
            <div
              style={{
                background: isUser ? '#8B5CF6' : (effectiveTheme === 'light' ? '#ffffff' : tokenColors.colorBgLayout),
                color: isUser ? 'white' : tokenColors.colorText,
                padding: '12px 16px',
                borderRadius: 12,
                maxWidth: '100%',
                fontSize: 13,
                overflowX: 'auto',
              }}
            >
              {isUser ? (
                <>
                  {/* Display attached images */}
                  {msg.images && msg.images.length > 0 && (
                    <div style={{ display: 'flex', gap: 8, marginBottom: hasContent ? 8 : 0, flexWrap: 'wrap' }}>
                      {msg.images.map((img, i) => (
                        <img 
                          key={i} 
                          src={`data:${img.mime_type};base64,${img.data}`}
                          alt={img.name || 'image'}
                          style={{ maxHeight: 200, maxWidth: '100%', borderRadius: 8, cursor: 'pointer' }}
                          onClick={() => window.open(`data:${img.mime_type};base64,${img.data}`, '_blank')}
                        />
                      ))}
                    </div>
                  )}
                  {hasContent && (
                    <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{msg.content}</div>
                  )}
                </>
              ) : (
                <div className="markdown-content">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
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
                  onClick={() => onCopy(msg.content)}
                  style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}
                />
              </Tooltip>
              <Tooltip title="编辑">
                <Button
                  type="text"
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => onEdit(index, msg.content)}
                  style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}
                />
              </Tooltip>
              <Tooltip title="删除">
                <Button
                  type="text"
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => onDelete(index)}
                  style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}
                />
              </Tooltip>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}, (prevProps, nextProps) => {
  // Custom comparison - only re-render if message content or theme changes
  return prevProps.msg === nextProps.msg && 
         prevProps.index === nextProps.index &&
         prevProps.effectiveTheme === nextProps.effectiveTheme &&
         prevProps.tokenColors === nextProps.tokenColors;
});

MessageItem.displayName = 'MessageItem';

// HTML 转义函数，防止 XSS
const escapeHtml = (str: string): string => {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
};

// ================================================================
// Streaming Content Component - Self-managed state for isolation
// Uses internal state + direct ChatManager subscription to prevent
// parent component re-renders from causing flicker
// ================================================================
interface StreamingContentProps {
  sessionId: string;
  tokenColors: TokenColors;
  effectiveTheme: string;
  onScrollRequest?: () => void;
}

/**
 * StreamingContent - 使用 DOM 直接更新避免 React 重渲染闪烁
 * 
 * 关键优化：
 * 1. 完全使用 useRef 和 DOM 操作，避免任何 setState
 * 2. 使用 CSS display 切换代替条件渲染
 * 3. 只在组件挂载时订阅一次
 */
const StreamingContent: React.FC<StreamingContentProps> = ({ sessionId, tokenColors, effectiveTheme, onScrollRequest }) => {
  const chatManager = useChatManager();
  
  // 所有 DOM 元素引用
  const contentRef = useRef<HTMLPreElement>(null);
  const contentWrapperRef = useRef<HTMLDivElement>(null);
  const thinkingRef = useRef<HTMLDivElement>(null);
  const toolCallsRef = useRef<HTMLDivElement>(null);
  const toolCallsContentRef = useRef<HTMLDivElement>(null);
  
  // 状态存储在 ref 中
  const lastContentLengthRef = useRef(0);
  const lastToolCallsJsonRef = useRef('');
  const thinkingFadeTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const thinkingDoneRef = useRef(false);

  // 使用 useLayoutEffect 确保 DOM 已准备好
  useEffect(() => {
    if (!sessionId) return;

    const unsubscribe = chatManager.subscribe(sessionId, (state) => {
      // === 1. 更新内容文本（直接操作 DOM）===
      const newContent = state.currentContent || '';
      // 压缩连续空行（3个以上换行 → 2个），并去掉开头空行
      const displayContent = newContent.replace(/^\n+/, '').replace(/\n{3,}/g, '\n\n');
      if (contentRef.current && contentRef.current.textContent !== displayContent) {
        contentRef.current.textContent = displayContent;
        
        // 内容增加时请求滚动
        if (newContent.length > lastContentLengthRef.current) {
          lastContentLengthRef.current = newContent.length;
          onScrollRequest?.();
        }
      }
      
      // 切换内容区域显示/隐藏
      if (contentWrapperRef.current) {
        const hasContent = !!(newContent && newContent.trim());
        contentWrapperRef.current.style.display = hasContent ? 'block' : 'none';
      }
      
      // === 2. 更新 thinking 文本 ===
      if (thinkingRef.current) {
        const thinkingText = state.currentThinking || '';
        const thinkingSpan = thinkingRef.current.querySelector('.thinking-text');
        if (thinkingSpan) {
          thinkingSpan.textContent = thinkingText;
        }
        
        const hasThinking = !!(thinkingText && thinkingText.trim());
        
        if (hasThinking && !state.thinkingDone) {
          // Thinking 进行中：显示并重置状态
          if (thinkingFadeTimerRef.current) {
            clearTimeout(thinkingFadeTimerRef.current);
            thinkingFadeTimerRef.current = undefined;
          }
          thinkingDoneRef.current = false;
          thinkingRef.current.style.display = 'block';
          thinkingRef.current.style.opacity = '1';
          thinkingRef.current.style.transition = '';
          // thinking 增长时也请求滚动
          onScrollRequest?.();
        } else if (hasThinking && state.thinkingDone && !thinkingDoneRef.current) {
          // Thinking 刚结束：启动淡出动画
          thinkingDoneRef.current = true;
          thinkingRef.current.style.transition = 'opacity 0.6s ease-out';
          thinkingRef.current.style.opacity = '0';
          thinkingFadeTimerRef.current = setTimeout(() => {
            if (thinkingRef.current) {
              thinkingRef.current.style.display = 'none';
            }
          }, 600);
        } else if (!hasThinking) {
          // 没有 thinking 内容：直接隐藏
          thinkingRef.current.style.display = 'none';
        }
      }
      
      // === 3. 更新 Tool Calls（只有结构变化时才更新 DOM）===
      const newToolCalls = state.currentToolCalls || {};
      const newToolCallsJson = JSON.stringify(newToolCalls);
      
      if (newToolCallsJson !== lastToolCallsJsonRef.current) {
        lastToolCallsJsonRef.current = newToolCallsJson;
        
        if (toolCallsRef.current && toolCallsContentRef.current) {
          const hasToolCalls = Object.keys(newToolCalls).length > 0;
          toolCallsRef.current.style.display = hasToolCalls ? 'block' : 'none';
          
          if (hasToolCalls) {
            // 直接更新工具调用内容
            const toolCallsHtml = Object.entries(newToolCalls).map(([_name, tool]) => {
              let argsHtml = '';
              if (tool.arguments) {
                let argsStr = tool.arguments;
                try {
                  argsStr = JSON.stringify(JSON.parse(tool.arguments), null, 2);
                } catch {}
                argsHtml = `
                  <div style="margin-bottom: 4px">
                    <div style="color: ${tokenColors.colorTextSecondary}; margin-bottom: 2px; font-size: 11px">参数:</div>
                    <pre style="background: ${tokenColors.colorBgLayout}; padding: 8px; border-radius: 4px; overflow: auto; margin: 0; font-size: 11px; border: 1px solid ${tokenColors.colorBorderSecondary}; white-space: pre-wrap; word-break: break-word; max-width: 100%">${escapeHtml(argsStr)}</pre>
                  </div>
                `;
              }
              
              let resultHtml = '';
              if (tool.result) {
                const resultStr = typeof tool.result === 'string' ? tool.result : JSON.stringify(tool.result, null, 2);
                resultHtml = `
                  <div>
                    <div style="color: ${tokenColors.colorTextSecondary}; margin-bottom: 2px; font-size: 11px">结果:</div>
                    <pre style="background: ${tokenColors.colorBgLayout}; padding: 8px; border-radius: 4px; overflow: auto; margin: 0; font-size: 11px; border: 1px solid ${tokenColors.colorBorderSecondary}; white-space: pre-wrap; word-break: break-word; max-width: 100%">${escapeHtml(resultStr)}</pre>
                  </div>
                `;
              }
              
              let errorHtml = '';
              if (tool.error) {
                errorHtml = `<div style="color: #ff4d4f; margin-bottom: 4px">错误: ${escapeHtml(tool.error)}</div>`;
              }
              
              const statusIcon = (tool.status === 'running' || (!tool.status && !tool.result && !tool.error))
                ? '<span class="ant-spin ant-spin-sm" style="margin-left: 8px"><span class="ant-spin-dot ant-spin-dot-spin"><i></i><i></i><i></i><i></i></span></span>'
                : tool.status === 'completed'
                ? '<span style="margin-left: 8px; background: #52c41a; color: white; padding: 0 7px; border-radius: 4px; font-size: 10px">完成</span>'
                : '';
              
              return `
                <div style="margin-bottom: 12px">
                  <div style="font-weight: 500; margin-bottom: 4px; display: flex; align-items: center">
                    ${escapeHtml(tool.name)}${statusIcon}
                  </div>
                  ${argsHtml}${errorHtml}${resultHtml}
                </div>
              `;
            }).join('');
            
            toolCallsContentRef.current.innerHTML = toolCallsHtml;
          }
        }
      }
    });

    return () => {
      unsubscribe();
      if (thinkingFadeTimerRef.current) {
        clearTimeout(thinkingFadeTimerRef.current);
      }
    };
  }, [sessionId, chatManager, onScrollRequest, tokenColors]);

  return (
    <div style={{ padding: '12px 0', contain: 'layout style' }}>
      <div style={{ display: 'flex', gap: 12, width: '100%', alignItems: 'flex-start' }}>
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
        <div style={{ flex: 1, minWidth: 0 }}>
          {/* Thinking content - 初始隐藏，由 JS 控制显示和淡出 */}
          <div
            ref={thinkingRef}
            style={{
              display: 'none',
              background: effectiveTheme === 'dark' ? '#2a2a2a' : '#fafafa',
              color: tokenColors.colorTextSecondary,
              padding: '8px 12px',
              borderRadius: 8,
              fontSize: 12,
              fontStyle: 'italic',
              marginBottom: 8,
              borderLeft: `3px solid ${tokenColors.colorPrimary}`,
              maxHeight: 200,
              overflowY: 'auto',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4, fontWeight: 500, fontStyle: 'normal', fontSize: 11, opacity: 0.7 }}>
              <Spin size="small" style={{ marginRight: 6 }} />
              思考中...
            </div>
            <span className="thinking-text" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}></span>
          </div>

          {/* Tool Calls - 初始隐藏，由 JS 控制显示和内容 */}
          <div 
            ref={toolCallsRef}
            style={{ 
              display: 'none',
              marginBottom: 8,
              fontSize: 12,
              color: tokenColors.colorTextSecondary,
              background: tokenColors.colorBgLayout,
              borderRadius: 8,
              padding: '8px 12px',
            }}
          >
            <div style={{ fontWeight: 500, marginBottom: 8, display: 'flex', alignItems: 'center' }}>
              <ToolOutlined style={{ marginRight: 4 }} />
              工具调用中
            </div>
            <div ref={toolCallsContentRef}></div>
          </div>
          
          {/* Streaming text content - 初始隐藏，由 JS 控制显示 */}
          <div
            ref={contentWrapperRef}
            style={{
              display: 'none',
              background: tokenColors.colorBgLayout,
              color: tokenColors.colorText,
              padding: '12px 16px',
              borderRadius: 12,
              maxWidth: '100%',
              fontSize: 13,
              contain: 'layout paint',
            }}
          >
            <pre 
              ref={contentRef}
              style={{ 
                whiteSpace: 'pre-wrap', 
                wordBreak: 'break-word',
                margin: 0,
                fontFamily: 'inherit',
                fontSize: 'inherit',
              }}
            />
            <Spin size="small" style={{ marginLeft: 8 }} />
          </div>
        </div>
      </div>
    </div>
  );
};

// 旧版 LegacyStreamingContent 保留作为参考
interface LegacyStreamingContentProps {
  content: string;
  thinking: string;
  toolCalls: { [key: string]: { name: string; status?: string; arguments?: string; result?: string | object; error?: string } };
  tokenColors: TokenColors;
  effectiveTheme: string;
}

const LegacyStreamingContent = memo<LegacyStreamingContentProps>(({ content, thinking, toolCalls, tokenColors, effectiveTheme }) => {
  const hasToolCalls = Object.keys(toolCalls).length > 0;
  const hasThinking = thinking && thinking.trim().length > 0;
  
  return (
    <div style={{ 
      padding: '12px 0',
      // CSS containment: 限制重绘范围，防止内容变化影响页面其他部分
      contain: 'content',
    }}>
      <div style={{ display: 'flex', gap: 12, width: '100%', alignItems: 'flex-start' }}>
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
          {/* Thinking content - temporary display, shown when no other content */}
          {hasThinking && !content && !hasToolCalls && (
            <div
              style={{
                background: effectiveTheme === 'dark' ? '#2a2a2a' : '#fafafa',
                color: tokenColors.colorTextSecondary,
                padding: '8px 12px',
                borderRadius: 8,
                fontSize: 12,
                fontStyle: 'italic',
                marginBottom: 8,
                borderLeft: `3px solid ${tokenColors.colorPrimary}`,
              }}
            >
              <Spin size="small" style={{ marginRight: 8 }} />
              {thinking}
            </div>
          )}

          {/* Tool Calls during streaming */}
          {hasToolCalls && (
            <div style={{ marginBottom: content ? 8 : 0 }}>
              <Collapse
                ghost
                size="small"
                defaultActiveKey={['1']}
                items={[
                  {
                    key: '1',
                    label: (
                      <span style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}>
                        <ToolOutlined style={{ marginRight: 4 }} />
                        工具调用中 ({Object.keys(toolCalls).length})
                      </span>
                    ),
                    children: (
                      <div style={{ fontSize: 12, color: tokenColors.colorTextSecondary }}>
                        {Object.entries(toolCalls).map(([name, tool], idx) => (
                          <div key={name} style={{ marginBottom: idx < Object.keys(toolCalls).length - 1 ? 12 : 0 }}>
                            <div style={{ fontWeight: 500, marginBottom: 4, display: 'flex', alignItems: 'center' }}>
                              {tool.name}
                              {tool.status === 'running' && <Spin size="small" style={{ marginLeft: 8 }} />}
                              {tool.status === 'completed' && (
                                <Tag color="success" style={{ marginLeft: 8, fontSize: 10 }}>完成</Tag>
                              )}
                              {!tool.status && !tool.result && !tool.error && <Spin size="small" style={{ marginLeft: 8 }} />}
                            </div>
                            {tool.arguments && (
                              <div style={{ marginBottom: 4 }}>
                                <div style={{ color: tokenColors.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>参数:</div>
                                <pre style={{
                                  background: tokenColors.colorBgLayout,
                                  padding: 8,
                                  borderRadius: 4,
                                  overflow: 'auto',
                                  margin: 0,
                                  fontSize: 11,
                                  border: `1px solid ${tokenColors.colorBorderSecondary}`,
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
                                <div style={{ color: tokenColors.colorTextSecondary, marginBottom: 2, fontSize: 11 }}>结果:</div>
                                <pre style={{
                                  background: tokenColors.colorBgLayout,
                                  padding: 8,
                                  borderRadius: 4,
                                  overflow: 'auto',
                                  margin: 0,
                                  fontSize: 11,
                                  border: `1px solid ${tokenColors.colorBorderSecondary}`,
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word',
                                  maxWidth: '100%',
                                }}>
                                  {typeof tool.result === 'string' 
                                    ? tool.result 
                                    : JSON.stringify(tool.result as object, null, 2)}
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
          
          {/* Streaming text content - Use plain text during streaming for performance */}
          {content && (
            <div
              style={{
                background: tokenColors.colorBgLayout,
                color: tokenColors.colorText,
                padding: '12px 16px',
                borderRadius: 12,
                maxWidth: '100%',
                fontSize: 13,
                // 性能优化：提示浏览器此元素会频繁变化
                willChange: 'contents',
                // 创建独立的层叠上下文，减少重绘影响
                isolation: 'isolate',
              }}
            >
              {/* During streaming, render as plain text with basic formatting */}
              <pre style={{ 
                whiteSpace: 'pre-wrap', 
                wordBreak: 'break-word',
                margin: 0,
                fontFamily: 'inherit',
                fontSize: 'inherit',
              }}>
                {content}
              </pre>
              <Spin size="small" style={{ marginLeft: 8 }} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
});

LegacyStreamingContent.displayName = 'LegacyStreamingContent';

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
  
  // Extract stable token values to prevent re-renders
  // These values rarely change and can be memoized
  const tokenColors = useMemo(() => ({
    colorBgLayout: token.colorBgLayout,
    colorBgContainer: token.colorBgContainer,
    colorText: token.colorText,
    colorTextSecondary: token.colorTextSecondary,
    colorBorderSecondary: token.colorBorderSecondary,
    colorPrimary: token.colorPrimary,
    colorWarning: token.colorWarning,
    colorWarningBg: token.colorWarningBg,
    colorWarningBorder: token.colorWarningBorder,
    colorWarningText: token.colorWarningText,
  }), [effectiveTheme]); // Only recalculate when theme changes
  
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
  const [isInitializing, setIsInitializing] = useState(false); // Track ACP initialization state
  // 跟踪是否正在浏览历史记录
  const isNavigatingHistoryRef = useRef(false);
  const [sessionId, setSessionId] = useState<string | undefined>(initialSessionId);
  const [sessionLoading, setSessionLoading] = useState(true);
  const [models, setModels] = useState<Model[]>([]);
  const [currentModel, setCurrentModel] = useState<string>('');
  const [modelLoading, setModelLoading] = useState(false);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]); // empty = all
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [promptSelectorVisible, setPromptSelectorVisible] = useState(false);
  const [promptSearchQuery, setPromptSearchQuery] = useState('');
  
  // File selector state
  const [fileSelectorVisible, setFileSelectorVisible] = useState(false);
  const [fileSearchQuery, setFileSearchQuery] = useState('');
  const [fileSelectorMode, setFileSelectorMode] = useState<FileSelectorMode>('context');
  const [_filesToContext, _setFilesToContext] = useState<Set<string>>(new Set());
  const [tabTriggerPos, setTabTriggerPos] = useState<number | null>(null);
  
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [workspaceModalVisible, setWorkspaceModalVisible] = useState(false);
  const [workspacePath, setWorkspacePath] = useState('');
  const [directoryPickerVisible, setDirectoryPickerVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingMessage, setEditingMessage] = useState<{ index: number; content: string } | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<any>(null);
  // 用于保存当前累积的响应内容（停止时使用）
  const currentResponseRef = useRef('');
  // Stream error state
  const [streamError, setStreamError] = useState<ErrorDetail | null>(null);
  // Pause control state
  const [pauseInputModalVisible, setPauseInputModalVisible] = useState(false);
  const [pauseInputValue, setPauseInputValue] = useState('');
  // Pause state from ChatManager
  const [paused, setPaused] = useState(false);
  const [pausePendingTools, setPausePendingTools] = useState<string[]>([]);
  // Pasted images state
  const [pastedImages, setPastedImages] = useState<ImageAttachment[]>([]);
  const lastScrollHeightRef = useRef(0);
  // Auto-scroll state: true = 自动滚动到底部, false = 用户手动上滚，暂停自动滚动
  const shouldAutoScrollRef = useRef(true);
  // 标记程序化滚动，避免 scroll 事件反向影响 shouldAutoScrollRef
  const programmaticScrollRef = useRef(false);
  // Truncation state - when response is cut off due to max_tokens
  const [truncated, setTruncated] = useState(false);
  const [truncatedInfo, setTruncatedInfo] = useState<{ reason: string; pendingToolCalls: number } | null>(null);

  // 检测用户是否在消息列表底部附近
  const isNearBottom = useCallback(() => {
    const container = messagesContainerRef.current;
    if (!container) return true;
    
    const threshold = 150; // 150px 阈值
    const { scrollTop, scrollHeight, clientHeight } = container;
    return scrollHeight - scrollTop - clientHeight < threshold;
  }, []);

  // 监听用户滚动，更新 shouldAutoScrollRef
  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container) return;

    let scrollTimer: ReturnType<typeof setTimeout>;

    const handleScroll = () => {
      // 跳过程序化滚动触发的 scroll 事件
      if (programmaticScrollRef.current) return;
      // 用户滚动后，延迟检查位置来更新自动滚动状态
      clearTimeout(scrollTimer);
      scrollTimer = setTimeout(() => {
        shouldAutoScrollRef.current = isNearBottom();
      }, 50);
    };

    // 鼠标滚轮滚动 → 可能是用户主动操作
    const handleWheel = (e: WheelEvent) => {
      if (e.deltaY < 0) {
        // 向上滚动 → 用户想看历史
        shouldAutoScrollRef.current = false;
      }
    };

    container.addEventListener('scroll', handleScroll, { passive: true });
    container.addEventListener('wheel', handleWheel, { passive: true });
    return () => {
      container.removeEventListener('scroll', handleScroll);
      container.removeEventListener('wheel', handleWheel);
      clearTimeout(scrollTimer);
    };
  }, [isNearBottom]);

  // 智能滚动：基于 shouldAutoScrollRef 状态决定是否滚动
  const scrollToBottom = useCallback((_behavior: ScrollBehavior = 'auto', force = false) => {
    if (!force && !shouldAutoScrollRef.current) {
      // 用户正在查看历史消息，不要自动滚动
      return;
    }
    const container = messagesContainerRef.current;
    if (container) {
      // 标记程序化滚动，避免 handleScroll 中的 isNearBottom 检查干扰
      programmaticScrollRef.current = true;
      container.scrollTop = container.scrollHeight;
      // 延迟重置标志，确保 scroll 事件已处理完
      requestAnimationFrame(() => {
        programmaticScrollRef.current = false;
      });
    }
    shouldAutoScrollRef.current = true;
  }, []);

  // 稳定的 onScrollRequest 回调引用，避免 StreamingContent useEffect 反复重执行
  const handleScrollRequest = useCallback(() => {
    scrollToBottom('auto', false);
  }, [scrollToBottom]);

  // 消息变化时的智能滚动
  useEffect(() => {
    const container = messagesContainerRef.current;
    if (!container) return;

    // 使用 requestAnimationFrame 确保 DOM 已更新
    requestAnimationFrame(() => {
      if (shouldAutoScrollRef.current) {
        scrollToBottom('auto', true);
      }
      lastScrollHeightRef.current = container.scrollHeight;
    });
  }, [messages, scrollToBottom]);

  // 流式内容滚动由 StreamingContent 组件的 onScrollRequest 回调处理

  // Load models on mount - only set currentModel if no session will be loaded
  // This avoids race condition where models API response overwrites session's model
  useEffect(() => {
    const loadModels = async () => {
      try {
        const response = await api.getModels();
        // Filter out unavailable models so they don't appear in selector
        const availableModels = (response.models || []).filter(m => m.available !== false);
        setModels(availableModels);
        // Only set currentModel from models API if there's no initial session
        // Session loading will set the correct model from session data
        if (!initialSessionId) {
          setCurrentModel(response.current || response.default || '');
        }
      } catch (error) {
        console.error('Failed to load models:', error);
      }
    };
    loadModels();
  }, [api, initialSessionId]);

  // Load skills on mount
  useEffect(() => {
    const loadSkills = async () => {
      if (!api.getSkills) return;
      try {
        const skillsList = await api.getSkills();
        setSkills(skillsList || []);
      } catch (error) {
        console.error('Failed to load skills:', error);
      }
    };
    loadSkills();
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
        // Load session model and skills if available
        if (api.getSession) {
          try {
            const session = await api.getSession(initialSessionId);
            if (session.model) {
              setCurrentModel(session.model);
            }
            // Load session skills: undefined/null/empty means all
            setSelectedSkills(session.selected_skills || []);
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
                setSelectedSkills(session.selected_skills || []);
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
    const imagesKey = `mote_pending_images_${sid}`;
    const pendingMessage = sessionStorage.getItem(storageKey);
    if (pendingMessage) {
      sessionStorage.removeItem(storageKey);
      
      // Check for pending images
      let pendingImages: ImageAttachment[] | undefined;
      const imagesJSON = sessionStorage.getItem(imagesKey);
      if (imagesJSON) {
        sessionStorage.removeItem(imagesKey);
        try {
          pendingImages = JSON.parse(imagesJSON);
        } catch {
          // Ignore parse errors
        }
      }
      
      // 等待一小段时间确保页面已完全渲染
      await new Promise(resolve => setTimeout(resolve, 100));
      
      const userMessage: Message = {
        role: 'user',
        content: pendingMessage.trim(),
        images: pendingImages,
        timestamp: new Date().toISOString(),
      };
      
      addHistory(pendingMessage.trim());
      setMessages((prev: Message[]) => [...prev, userMessage]);
      setLoading(true);
      setStreaming(true);
      shouldAutoScrollRef.current = true;
      currentResponseRef.current = '';

      // 使用 ChatManager 发起请求
      chatManager.startChat(
        sid,
        {
          message: userMessage.content,
          session_id: sid,
          stream: true,
          images: pendingImages,
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
      // 初始加载后强制滚动到底部
      setTimeout(() => {
        const container = messagesContainerRef.current;
        if (container) {
          container.scrollTop = container.scrollHeight;
        }
        lastScrollHeightRef.current = container?.scrollHeight || 0;
      }, 100);
      
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
      currentResponseRef.current = existingState.currentContent || '';
      if (existingState.streaming) {
        setLoading(true);
      }
      // Restore truncation state
      if (existingState.truncated) {
        setTruncated(true);
        setTruncatedInfo({
          reason: existingState.truncatedReason || 'length',
          pendingToolCalls: existingState.pendingToolCalls || 0,
        });
      }
    } else {
      // No active chat for this session - reset streaming state
      // This ensures clean state when switching between sessions
      setStreaming(false);
      setLoading(false);
      currentResponseRef.current = '';
      setTruncated(false);
      setTruncatedInfo(null);
      setStreamError(null);
    }

    // Subscribe to state changes
    // 注意：内容更新由 StreamingContent 组件内部订阅处理，这里只处理状态和最终消息
    const unsubscribe = chatManager.subscribe(sessionId, (state) => {
      // 只在 streaming 状态变化时更新（避免频繁重渲染）
      setStreaming(prev => prev !== state.streaming ? state.streaming : prev);
      
      // 保存到 ref（用于停止时），不触发重渲染
      currentResponseRef.current = state.currentContent || '';
      
      // Handle truncation state
      if (state.truncated) {
        setTruncated(true);
        setTruncatedInfo({
          reason: state.truncatedReason || 'length',
          pendingToolCalls: state.pendingToolCalls || 0,
        });
      }
      
      // Handle pause state
      setPaused(state.paused || false);
      setPausePendingTools(state.pausePendingTools || []);
      
      // Handle initialization state - check for specific error messages
      if (state.error) {
        const errorLower = state.error.toLowerCase();
        if (errorLower.includes('initialize') || 
            errorLower.includes('initializing') || 
            errorLower.includes('starting') ||
            errorLower.includes('restarting') ||
            errorLower.includes('timeout waiting')) {
          setIsInitializing(true);
        } else {
          setIsInitializing(false);
        }
      } else {
        setIsInitializing(false);
      }
      
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
        // Clear truncation state when streaming ends normally
        setTruncated(false);
        setTruncatedInfo(null);
        // When streaming ends, add the final message to the list
        // Use finalMessage from ChatManager if available (set on 'done' event)
        const finalContent = state.finalMessage?.content || state.currentContent;
        const finalToolCalls = state.finalMessage?.tool_calls || 
          (state.currentToolCalls ? Object.values(state.currentToolCalls) : undefined);
        
        if (finalContent && finalContent.trim()) {
          setMessages((prev: Message[]) => {
            // Avoid duplicating if already added
            const lastMsg = prev[prev.length - 1];
            if (lastMsg && lastMsg.role === 'assistant' && lastMsg.content === finalContent) {
              return prev;
            }
            return [...prev, {
              role: 'assistant' as const,
              content: finalContent,
              tool_calls: finalToolCalls,
              timestamp: new Date().toISOString(),
            }];
          });
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
    const execute = async () => {
      try {
        if (api.reconfigureSession) {
          await doReconfigure({ workspace_path: workspacePath.trim() }, '工作区已绑定，运行时资源已重置');
        } else {
          const result = await api.bindWorkspace?.(sessionId, workspacePath.trim());
          if (result) {
            setWorkspace(result);
            message.success('工作区已绑定');
          }
        }
        setWorkspaceModalVisible(false);
        setWorkspacePath('');
      } catch (error: any) {
        message.error(error.message || '绑定工作区失败');
      }
    };

    if (streaming) {
      Modal.confirm({
        title: '绑定工作区会中止当前任务',
        content: '当前正在生成回复，绑定工作区将中止进行中的任务并重置会话运行时资源。是否继续？',
        okText: '继续绑定',
        cancelText: '取消',
        okType: 'danger',
        onOk: execute,
      });
    } else {
      execute();
    }
  };

  // Handle workspace unbinding
  const handleUnbindWorkspace = async () => {
    if (!sessionId) return;
    const execute = async () => {
      try {
        if (api.reconfigureSession) {
          await doReconfigure({ workspace_path: '' }, '工作区已解绑，运行时资源已重置');
        } else {
          await api.unbindWorkspace?.(sessionId);
          setWorkspace(null);
          message.success('工作区已解绑');
        }
      } catch (error: any) {
        message.error(error.message || '解绑工作区失败');
      }
    };

    if (streaming) {
      Modal.confirm({
        title: '解绑工作区会中止当前任务',
        content: '当前正在生成回复，解绑工作区将中止进行中的任务并重置会话运行时资源。是否继续？',
        okText: '继续解绑',
        cancelText: '取消',
        okType: 'danger',
        onOk: execute,
      });
    } else {
      execute();
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
      // New session defaults to all skills
      setSelectedSkills([]);
      onSessionCreated?.(session.id);
      message.success('新会话已创建');
    } catch (error) {
      message.error('创建会话失败');
    }
  };

  // ============== Unified Reconfigure Logic ==============

  // Helper: perform reconfigure and update local state
  const doReconfigure = useCallback(async (
    config: { model?: string; workspace_path?: string; workspace_alias?: string; selected_skills?: string[] },
    successMsg: string,
  ) => {
    if (!sessionId) return;

    // Abort streaming if active
    if (streaming) {
      chatManager.abortChat(sessionId);
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
      currentResponseRef.current = '';
    }

    if (api.reconfigureSession) {
      const resp: ReconfigureSessionResponse = await api.reconfigureSession(sessionId, config);
      // Sync local state from response
      if (config.model !== undefined) {
        setCurrentModel(resp.model);
      }
      if (config.selected_skills !== undefined) {
        setSelectedSkills(resp.selected_skills || []);
      }
      if (config.workspace_path !== undefined) {
        if (resp.workspace_path) {
          setWorkspace({ session_id: sessionId, path: resp.workspace_path } as Workspace);
        } else {
          setWorkspace(null);
        }
      }
      message.success(successMsg);
    } else {
      // Fallback to legacy per-field API
      if (config.model !== undefined && api.setSessionModel) {
        await api.setSessionModel(sessionId, config.model);
        setCurrentModel(config.model);
      }
      if (config.selected_skills !== undefined && api.setSessionSkills) {
        await api.setSessionSkills(sessionId, config.selected_skills);
        setSelectedSkills(config.selected_skills);
      }
      message.success(successMsg);
    }
  }, [sessionId, streaming, chatManager, api, message]);

  // Helper: show confirmation if streaming, then reconfigure
  const confirmAndReconfigure = useCallback((
    config: { model?: string; workspace_path?: string; workspace_alias?: string; selected_skills?: string[] },
    successMsg: string,
    setLoadingFn?: (v: boolean) => void,
  ) => {
    const execute = async () => {
      setLoadingFn?.(true);
      try {
        await doReconfigure(config, successMsg);
      } catch (error: any) {
        message.error(error.message || '重新配置会话失败');
      } finally {
        setLoadingFn?.(false);
      }
    };

    if (streaming) {
      Modal.confirm({
        title: '切换会中止当前任务',
        content: '当前正在生成回复，切换配置将中止进行中的任务并重置会话运行时资源。是否继续？',
        okText: '继续切换',
        cancelText: '取消',
        okType: 'danger',
        onOk: execute,
      });
    } else {
      execute();
    }
  }, [streaming, doReconfigure, message]);

  // Handle model change (per session only)
  const handleModelChange = (modelId: string) => {
    if (!sessionId) {
      message.error('无会话可设置模型');
      return;
    }
    confirmAndReconfigure({ model: modelId }, '会话模型已切换，运行时资源已重置', setModelLoading);
  };

  // Handle skills selection change (per session)
  const handleSkillsChange = (skillIds: string[]) => {
    if (!sessionId) {
      message.error('无会话可设置技能');
      return;
    }
    confirmAndReconfigure({ selected_skills: skillIds }, '技能选择已更新', setSkillsLoading);
  };

  const handleSendInternal = async (retryCount = 0) => {
    const hasText = inputValue.trim().length > 0;
    const hasImages = pastedImages.length > 0;
    if ((!hasText && !hasImages) || loading || !sessionId || isInitializing) return;

    // Warn if current model doesn't support vision but images are attached
    if (hasImages) {
      const currentModelObj = models.find(m => m.id === currentModel);
      if (currentModelObj && !currentModelObj.supports_vision) {
        message.warning(`当前模型 ${currentModelObj.display_name || currentModel} 不支持图片输入，图片可能被忽略`);
      }
    }

    // Capture current images before clearing
    const currentImages = hasImages ? [...pastedImages] : undefined;

    const userMessage: Message = {
      role: 'user',
      content: inputValue.trim() || (hasImages ? '请描述这张图片' : ''),
      images: currentImages,
      timestamp: new Date().toISOString(),
    };

    // 添加到输入历史
    addHistory(inputValue.trim());

    setMessages((prev: Message[]) => [...prev, userMessage]);
    setInputValue('');
    setPastedImages([]);
    setLoading(true);
    setStreaming(true);
    shouldAutoScrollRef.current = true;
    currentResponseRef.current = '';

    // 使用 ChatManager 发起请求，它会在后台持续运行
    chatManager.startChat(
      sessionId,
      {
        message: userMessage.content,
        session_id: sessionId,
        stream: true,
        images: currentImages,
      },
      api.chat,
      (_assistantMessage, error) => {
        if (error) {
          // Check if error is due to initialization delay and retry
          const errorLower = error.toLowerCase();
          const isInitError = errorLower.includes('initialize') || 
                             errorLower.includes('initializing') || 
                             errorLower.includes('starting') ||
                             errorLower.includes('restarting') ||
                             errorLower.includes('timeout waiting') ||
                             errorLower.includes('not initialized');
          
          if (isInitError && retryCount < 3) {
            // Retry with exponential backoff
            const delay = 1000 * (retryCount + 1); // 1s, 2s, 3s
            message.info(`CLI正在初始化，${delay/1000}秒后自动重试...`);
            setIsInitializing(true);
            setTimeout(() => {
              setLoading(false);
              setStreaming(false);
              // Remove the user message before retry
              setMessages((prev: Message[]) => prev.slice(0, -1));
              // Re-add the input value for retry
              setInputValue(userMessage.content);
              // Retry
              handleSendInternal(retryCount + 1);
            }, delay);
          } else {
            message.error(`发送失败: ${error}`);
            setIsInitializing(false);
          }
        } else {
          setIsInitializing(false);
        }
        // 消息添加由 ChatManager 订阅回调处理
      }
    );
  };

  const handleSend = async () => {
    await handleSendInternal(0);
  };

  // Handle dismiss truncation warning
  const handleDismissTruncation = () => {
    setTruncated(false);
    setTruncatedInfo(null);
  };

  const handleStop = () => {
    if (streaming && sessionId) {
      chatManager.abortChat(sessionId);
      
      // Clear truncation state when stopping
      setTruncated(false);
      setTruncatedInfo(null);
      
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
      currentResponseRef.current = '';
      message.info('已停止生成');
    }
  };

  const handlePause = async () => {
    if (!sessionId || !api.pauseSession) {
      message.warning('暂停功能不可用');
      return;
    }
    try {
      await api.pauseSession(sessionId);
      message.info('暂停请求已发送，执行将在下一次工具调用前暂停');
    } catch (err: any) {
      message.error(`暂停失败: ${err.message}`);
    }
  };

  const handleResume = async (userInput?: string) => {
    if (!sessionId || !api.resumeSession) {
      message.warning('恢复功能不可用');
      return;
    }
    try {
      await api.resumeSession(sessionId, userInput);
      message.success(userInput ? '已注入用户输入并恢复执行' : '已恢复执行');
      setPauseInputModalVisible(false);
      setPauseInputValue('');
    } catch (err: any) {
      message.error(`恢复失败: ${err.message}`);
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
    if (e.key === 'Escape') {
      if (promptSelectorVisible) {
        setPromptSelectorVisible(false);
        setPromptSearchQuery('');
      }
      if (fileSelectorVisible) {
        setFileSelectorVisible(false);
        setFileSearchQuery('');
      }
    }
    
    // Tab key path completion
    if (e.key === 'Tab' && !e.shiftKey && !promptSelectorVisible && !fileSelectorVisible) {
      const value = inputValue;
      const cursorPos = e.currentTarget.selectionStart;
      const beforeCursor = value.substring(0, cursorPos);
      
      // Detect if cursor is in a path fragment
      const pathMatch = beforeCursor.match(/[\w\/\.\-_]*$/);
      
      if (pathMatch && pathMatch[0].length > 0) {
        e.preventDefault();
        setFileSelectorVisible(true);
        setFileSelectorMode('path-only');
        setFileSearchQuery(pathMatch[0]);
        setTabTriggerPos(cursorPos - pathMatch[0].length);
      }
    }
  };

  // Handle paste event for screenshot/image paste
  const handlePaste = useCallback((e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = e.clipboardData?.items;
    if (!items) return;

    for (const item of Array.from(items)) {
      if (item.type.startsWith('image/')) {
        e.preventDefault();
        const file = item.getAsFile();
        if (!file) continue;

        // Size limit: 10MB
        if (file.size > 10 * 1024 * 1024) {
          message.warning('图片大小不能超过 10MB');
          continue;
        }

        const mimeType = item.type;
        const reader = new FileReader();
        reader.onload = () => {
          const dataUrl = reader.result as string;
          // Extract base64 data (remove "data:image/xxx;base64," prefix)
          const base64 = dataUrl.split(',')[1];
          if (base64) {
            setPastedImages(prev => [...prev, {
              data: base64,
              mime_type: mimeType,
              name: `screenshot-${Date.now()}.${mimeType.split('/')[1] || 'png'}`,
            }]);
          }
        };
        reader.readAsDataURL(file);
        return; // Only handle the first image
      }
    }
  }, []);

  // Handle input change with prompt and file detection
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    setInputValue(value);
    // 用户手动输入时退出历史浏览模式
    isNavigatingHistoryRef.current = false;

    // 用户手动输入时重置历史导航
    resetNavigation();

    // ===== Detect / command for prompt selector =====
    const slashMatch = value.match(/^\/(\S*)$/);
    if (slashMatch) {
      setPromptSelectorVisible(true);
      setPromptSearchQuery(slashMatch[1] || '');
      setFileSelectorVisible(false); // Mutually exclusive
    } else if (!value.startsWith('/')) {
      setPromptSelectorVisible(false);
      setPromptSearchQuery('');
    }
    
    // ===== Detect @ for file selector =====
    if (value.endsWith('@')) {
      const beforeAt = value.slice(0, -1);
      if (beforeAt === '' || beforeAt.endsWith(' ') || beforeAt.endsWith('\n')) {
        setFileSelectorVisible(true);
        setFileSearchQuery('');
        setFileSelectorMode('context'); // Default mode
        setPromptSelectorVisible(false); // Mutually exclusive
        return;
      }
    }
    
    const lastAtIndex = value.lastIndexOf('@');
    if (lastAtIndex !== -1 && fileSelectorVisible) {
      setFileSearchQuery(value.slice(lastAtIndex + 1));
    } else if (!fileSelectorVisible) {
      // Don't close if already closed
    }
  };

  // Handle prompt selection
  const handlePromptSelect = (content: string) => {
    setInputValue(content);
    setPromptSelectorVisible(false);
    setPromptSearchQuery('');
  };
  
  // Handle file selection
  const handleFileSelect = (filepath: string, mode: FileSelectorMode) => {
    if (fileSelectorMode === 'path-only' && tabTriggerPos !== null) {
      // Tab completion: replace path fragment
      const cursorPos = inputRef.current?.selectionStart || 0;
      const before = inputValue.substring(0, tabTriggerPos);
      const after = inputValue.substring(cursorPos);
      setInputValue(`${before}${filepath}${after}`);
      setTabTriggerPos(null);
    } else {
      // @ reference: insert at end
      const lastAtIndex = inputValue.lastIndexOf('@');
      const before = inputValue.substring(0, lastAtIndex);
      
      if (mode === 'context') {
        // Add to context mode
        _setFilesToContext((prev: Set<string>) => new Set(prev).add(filepath));
        setInputValue(`${before}@${filepath} `);
      } else {
        // Path-only mode (no @)
        setInputValue(`${before}${filepath} `);
      }
    }
    
    setFileSelectorVisible(false);
    setFileSearchQuery('');
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

  // Memoize callbacks to prevent unnecessary re-renders
  const handleCopyCallback = useCallback((content: string) => {
    handleCopyMessage(content);
  }, []);

  const handleEditCallback = useCallback((index: number, content: string) => {
    handleEditMessage(index, content);
  }, []);

  const handleDeleteCallback = useCallback((index: number) => {
    handleDeleteMessage(index);
  }, []);

  // Memoized message list to prevent re-rendering when only streaming content changes
  const renderedMessages = useMemo(() => {
    return messages.map((msg, index) => {
      const isUser = msg.role === 'user';
      const isJsonContent = !isUser && msg.content && msg.content.trim().startsWith('{') && msg.content.trim().endsWith('}');
      const hasToolCalls = msg.tool_calls && msg.tool_calls.length > 0;
      
      // Hide JSON-only assistant messages
      if (!isUser && isJsonContent && !hasToolCalls) {
        return null;
      }
      
      return (
        <MessageItem
          key={`msg-${index}-${msg.timestamp || index}`}
          msg={msg}
          index={index}
          isUser={isUser}
          effectiveTheme={effectiveTheme}
          tokenColors={tokenColors}
          onCopy={handleCopyCallback}
          onEdit={handleEditCallback}
          onDelete={handleDeleteCallback}
        />
      );
    });
  }, [messages, effectiveTheme, tokenColors, handleCopyCallback, handleEditCallback, handleDeleteCallback]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Error Banner - shown when there's a connection or stream error */}
      {(connectionStatus?.status.activeError || streamError) && (
        <div style={{ padding: '8px 24px 0', background: tokenColors.colorBgContainer }}>
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
      
      {/* Truncation Banner - shown when response is cut off due to max_tokens */}
      {truncated && truncatedInfo && (
        <div style={{ 
          padding: '12px 24px', 
          background: tokenColors.colorWarningBg, 
          borderBottom: `1px solid ${tokenColors.colorWarningBorder}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1 }}>
            <span style={{ color: tokenColors.colorWarning, fontSize: 16 }}>⚠️</span>
            <Text style={{ color: tokenColors.colorWarningText }}>
              已达到单次响应的 token 上限，正在继续执行中...（共触发 {truncatedInfo.pendingToolCalls} 次）
            </Text>
          </div>
          <Space>
            <Button 
              danger
              size="small"
              onClick={handleStop}
              disabled={!streaming}
            >
              停止执行
            </Button>
            <Button 
              size="small"
              onClick={handleDismissTruncation}
            >
              关闭提示
            </Button>
          </Space>
        </div>
      )}
      
      {/* Header - 无标题 */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${tokenColors.colorBorderSecondary}`, background: tokenColors.colorBgContainer }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          {/* Left side: Skills, Model, Workspace */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Space>
              {/* Skills selector */}
              {skills.length > 0 && (
                <Dropdown
                  menu={{
                    items: skills.filter(s => s.state === 'active').map(skill => ({
                      key: skill.name,
                      label: (
                        <Tooltip title={skill.description} placement="left">
                          <div style={{ fontSize: '12px', fontWeight: 'normal' }}>
                            {skill.name}
                          </div>
                        </Tooltip>
                      ),
                    })),
                    selectable: true,
                    multiple: true,
                    selectedKeys: selectedSkills.length > 0 ? selectedSkills : skills.filter(s => s.state === 'active').map(s => s.name),
                    onSelect: ({ selectedKeys }) => {
                      const activeSkills = skills.filter(s => s.state === 'active');
                      const allSelected = selectedKeys.length === activeSkills.length;
                      handleSkillsChange(allSelected ? [] : selectedKeys);
                    },
                    onDeselect: ({ selectedKeys }) => {
                      handleSkillsChange(selectedKeys);
                    },
                  }}
                  trigger={['click']}
                  disabled={!sessionId}
                >
                  <Button 
                    icon={<ThunderboltOutlined />} 
                    style={{ width: 44 }}
                    loading={skillsLoading}
                  />
                </Dropdown>
              )}
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
                          {(provider === 'copilot' || provider === 'copilot-acp') ? <GithubOutlined /> : provider === 'minimax' ? <MinimaxIcon size={14} /> : <OllamaIcon />}
                          {provider === 'copilot' ? 'Copilot API' : provider === 'copilot-acp' ? 'Copilot ACP' : provider === 'minimax' ? 'MiniMax' : 'Ollama'}
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
                    style={{ color: '#52c41a', maxWidth: 200 }}
                    className="page-header-btn"
                  >
                    <span style={{ 
                      overflow: 'hidden', 
                      textOverflow: 'ellipsis', 
                      whiteSpace: 'nowrap',
                      display: 'inline-block',
                      maxWidth: '100%'
                    }}>
                      {workspace.alias || workspace.path.split('/').pop()}
                    </span>
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
            </Space>
          </div>
          
          {/* Right side: Connection Status, New Chat, Clear */}
          <Space>
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
            <Space.Compact style={{ width: '100%' }}>
              <Input
                style={{ flex: 1 }}
                placeholder="输入工作区路径，如 /path/to/project"
                value={workspacePath}
                onChange={(e) => setWorkspacePath(e.target.value)}
                onPressEnter={handleBindWorkspace}
              />
              <Button
                icon={<FolderOpenOutlined />}
                onClick={() => setDirectoryPickerVisible(true)}
              >
                浏览
              </Button>
            </Space.Compact>
          </div>
        )}
      </Modal>

      {/* Directory Picker */}
      <DirectoryPicker
        open={directoryPickerVisible}
        onCancel={() => setDirectoryPickerVisible(false)}
        onSelect={(path) => {
          setWorkspacePath(path);
          setDirectoryPickerVisible(false);
        }}
        initialPath={workspacePath || undefined}
        title="选择工作区目录"
      />

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
      <div ref={messagesContainerRef} style={{ flex: 1, overflowX: 'hidden', overflowY: 'auto', padding: '0 8px' }}>
        {sessionLoading ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <Spin tip="加载中..." />
          </div>
        ) : (
          <>
            {/* Empty state - show only when no messages and not streaming */}
            {messages.length === 0 && !streaming && (
              <div style={{ textAlign: 'center', padding: 48, color: '#999' }}>
                <img src={moteLogo} alt="Mote" style={{ width: 48, height: 48, marginBottom: 16 }} />
                <div>开始对话吧！</div>
              </div>
            )}
            
            {/* Message list - always render to avoid remounting */}
            <div style={{ display: messages.length === 0 && !streaming ? 'none' : 'block' }}>
              {renderedMessages}
            </div>

            {/* Streaming Response - 使用独立订阅，避免父组件重渲染 */}
            {streaming && sessionId && (
              <StreamingContent
                sessionId={sessionId}
                tokenColors={tokenColors}
                effectiveTheme={effectiveTheme}
                onScrollRequest={handleScrollRequest}
              />
            )}

            {loading && !streaming && (
              <div style={{ textAlign: 'center', padding: 16 }}>
                <Spin tip="思考中..." />
              </div>
            )}
          </>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Area */}
      <div style={{ padding: 16, borderTop: `1px solid ${tokenColors.colorBorderSecondary}`, background: tokenColors.colorBgContainer, position: 'relative' }}>
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
        
        {/* File Selector */}
        {sessionId && (
          <FileSelector
            visible={fileSelectorVisible}
            searchQuery={fileSearchQuery}
            sessionId={sessionId}
            mode={fileSelectorMode}
            onSelect={handleFileSelect}
            onCancel={() => {
              setFileSelectorVisible(false);
              setFileSearchQuery('');
            }}
          />
        )}

        <Space.Compact style={{ width: '100%' }} direction="vertical">
          {/* Pasted Images Preview */}
          {pastedImages.length > 0 && (
            <div style={{ 
              display: 'flex', gap: 8, padding: '8px 8px 4px', flexWrap: 'wrap',
              background: tokenColors.colorBgLayout, borderRadius: '8px 8px 0 0',
              border: `1px solid ${tokenColors.colorBorderSecondary}`, borderBottom: 'none',
            }}>
              {pastedImages.map((img, i) => (
                <div key={i} style={{ position: 'relative', display: 'inline-block' }}>
                  <img
                    src={`data:${img.mime_type};base64,${img.data}`}
                    alt="preview"
                    style={{ height: 64, borderRadius: 6, border: `1px solid ${tokenColors.colorBorderSecondary}`, objectFit: 'cover' }}
                  />
                  <CloseCircleFilled
                    style={{ 
                      position: 'absolute', top: -6, right: -6, cursor: 'pointer', 
                      fontSize: 16, color: '#ff4d4f', background: 'white', borderRadius: '50%',
                    }}
                    onClick={() => setPastedImages(prev => prev.filter((_, idx) => idx !== i))}
                  />
                </div>
              ))}
            </div>
          )}
          <Space.Compact style={{ width: '100%' }}>
          <TextArea
            ref={inputRef}
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyPress}
            onPaste={handlePaste}
            placeholder={isInitializing ? "CLI初始化中，请稍候..." : pastedImages.length > 0 ? "添加说明文字（可选）... (Ctrl+V 粘贴截图)" : "输入消息... (/ 引用提示词, @ 引用文件, Ctrl+V 粘贴截图, Shift+Enter 换行)"}
            autoSize={{ minRows: 1, maxRows: 10 }}
            disabled={loading || isInitializing}
            style={{ resize: 'none', borderRadius: pastedImages.length > 0 ? '0 0 0 6px' : undefined }}
            className="mote-input"
          />
          {paused ? (
            <>
              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                onClick={() => handleResume()}
              >
                继续
              </Button>
              <Button
                onClick={() => setPauseInputModalVisible(true)}
              >
                注入输入
              </Button>
            </>
          ) : streaming ? (
            <>
              {/* Hide pause button in ACP mode (Copilot models) as it doesn't work properly yet */}
              {(() => {
                const currentModelObj = models.find(m => m.id === currentModel);
                const isACPMode = currentModelObj?.provider === 'copilot';
                return !isACPMode && (
                  <Button
                    icon={<PauseOutlined />}
                    onClick={handlePause}
                    disabled={!api.pauseSession}
                  >
                    暂停
                  </Button>
                );
              })()}
              <Button
                type="primary"
                danger
                icon={<StopOutlined />}
                onClick={handleStop}
              >
                停止
              </Button>
            </>
          ) : (
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={handleSend}
              loading={loading || isInitializing}
              disabled={(!inputValue.trim() && pastedImages.length === 0) || isInitializing}
            >
              {isInitializing ? '初始化中...' : '发送'}
            </Button>
          )}
        </Space.Compact>
        </Space.Compact>

        {/* Pause Input Modal */}
        <Modal
          title="注入用户输入"
          open={pauseInputModalVisible}
          onOk={() => handleResume(pauseInputValue)}
          onCancel={() => {
            setPauseInputModalVisible(false);
            setPauseInputValue('');
          }}
          okText="注入并恢复"
          cancelText="取消"
        >
          <p style={{ marginBottom: 12, color: tokenColors.colorTextSecondary }}>
            输入内容将替代即将执行的工具调用结果，作为回复内容传递给 LLM。
          </p>
          {pausePendingTools.length > 0 && (
            <p style={{ marginBottom: 12, fontSize: 12, color: tokenColors.colorWarning }}>
              待执行工具: {pausePendingTools.join(', ')}
            </p>
          )}
          <TextArea
            value={pauseInputValue}
            onChange={(e) => setPauseInputValue(e.target.value)}
            placeholder="输入要注入的内容..."
            autoSize={{ minRows: 3, maxRows: 10 }}
          />
        </Modal>
      </div>
    </div>
  );
};
