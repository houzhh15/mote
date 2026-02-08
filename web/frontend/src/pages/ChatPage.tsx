import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Input, Button, Spin, Typography, message, List, Avatar, Card } from 'antd';
import { SendOutlined, UserOutlined, RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';

const { TextArea } = Input;
const { Text } = Typography;

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp?: string;
}

interface ChatPageProps {
  ws: WebSocket | null;
}

export const ChatPage: React.FC<ChatPageProps> = ({ ws }) => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState<string>('');
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Create new session on mount
  useEffect(() => {
    const createSession = async () => {
      try {
        const response = await fetch('/api/v1/sessions', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ title: 'New Chat' }),
        });
        const data = await response.json();
        if (data.session?.id) {
          setSessionId(data.session.id);
        }
      } catch (error) {
        console.error('Failed to create session:', error);
        message.error('创建会话失败');
      }
    };
    createSession();
  }, []);

  // WebSocket message handler
  useEffect(() => {
    if (!ws) return;

    const handleMessage = (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        
        if (data.type === 'chat.message.delta') {
          setMessages(prev => {
            const lastMessage = prev[prev.length - 1];
            if (lastMessage?.role === 'assistant') {
              const updated = [...prev];
              updated[updated.length - 1] = {
                ...lastMessage,
                content: lastMessage.content + (data.payload?.content || ''),
              };
              return updated;
            } else {
              return [...prev, { role: 'assistant', content: data.payload?.content || '' }];
            }
          });
        } else if (data.type === 'chat.message.complete') {
          setLoading(false);
        } else if (data.type === 'chat.error') {
          setLoading(false);
          message.error(data.payload?.message || '发生错误');
        }
      } catch (error) {
        console.error('Failed to parse WS message:', error);
      }
    };

    ws.addEventListener('message', handleMessage);
    return () => ws.removeEventListener('message', handleMessage);
  }, [ws]);

  const sendMessage = useCallback(async () => {
    if (!inputValue.trim() || loading) return;

    const userMessage: Message = { role: 'user', content: inputValue };
    setMessages(prev => [...prev, userMessage]);
    setInputValue('');
    setLoading(true);

    try {
      // Send via HTTP API for better reliability
      const response = await fetch('/api/v1/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: sessionId,
          message: inputValue,
          stream: true,
        }),
      });

      if (!response.ok) {
        throw new Error('Chat request failed');
      }

      // Handle streaming response
      const reader = response.body?.getReader();
      if (!reader) throw new Error('No reader available');

      const decoder = new TextDecoder();
      let assistantContent = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value, { stream: true });
        const lines = chunk.split('\n');

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));
              if (data.type === 'content' && data.content) {
                assistantContent += data.content;
                setMessages(prev => {
                  const lastMessage = prev[prev.length - 1];
                  if (lastMessage?.role === 'assistant') {
                    const updated = [...prev];
                    updated[updated.length - 1] = {
                      ...lastMessage,
                      content: assistantContent,
                    };
                    return updated;
                  } else {
                    return [...prev, { role: 'assistant', content: assistantContent }];
                  }
                });
              }
            } catch (e) {
              // Ignore parse errors for incomplete chunks
            }
          }
        }
      }
    } catch (error) {
      console.error('Failed to send message:', error);
      message.error('发送消息失败');
    } finally {
      setLoading(false);
    }
  }, [inputValue, loading, sessionId]);

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ 
        padding: '16px 24px', 
        borderBottom: '1px solid #f0f0f0',
        background: '#fafafa'
      }}>
        <Typography.Title level={4} style={{ margin: 0 }}>对话</Typography.Title>
      </div>
      
      <div style={{ flex: 1, overflow: 'auto', padding: '16px 24px' }}>
        {messages.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '48px', color: '#999' }}>
            <RobotOutlined style={{ fontSize: 48, marginBottom: 16 }} />
            <div>开始对话吧！</div>
          </div>
        ) : (
          <List
            dataSource={messages}
            renderItem={(item, index) => (
              <List.Item style={{ 
                justifyContent: item.role === 'user' ? 'flex-end' : 'flex-start',
                padding: '8px 0',
                border: 'none'
              }}>
                <div style={{ 
                  maxWidth: '80%',
                  display: 'flex',
                  flexDirection: item.role === 'user' ? 'row-reverse' : 'row',
                  alignItems: 'flex-start',
                  gap: 8
                }}>
                  <Avatar 
                    icon={item.role === 'user' ? <UserOutlined /> : <RobotOutlined />}
                    style={{ 
                      backgroundColor: item.role === 'user' ? '#1890ff' : '#52c41a',
                      flexShrink: 0
                    }}
                  />
                  <Card 
                    size="small" 
                    style={{ 
                      background: item.role === 'user' ? '#1890ff' : '#f5f5f5',
                      color: item.role === 'user' ? 'white' : 'inherit',
                      borderRadius: 12,
                      border: 'none'
                    }}
                    styles={{ body: { padding: '8px 12px' } }}
                  >
                    {item.role === 'assistant' ? (
                      <ReactMarkdown>{item.content}</ReactMarkdown>
                    ) : (
                      <div style={{ whiteSpace: 'pre-wrap' }}>{item.content}</div>
                    )}
                  </Card>
                </div>
              </List.Item>
            )}
          />
        )}
        {loading && (
          <div style={{ textAlign: 'center', padding: 16 }}>
            <Spin tip="思考中..." />
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <div style={{ 
        padding: '16px 24px', 
        borderTop: '1px solid #f0f0f0',
        background: '#fafafa'
      }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <TextArea
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyPress}
            placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
            autoSize={{ minRows: 1, maxRows: 4 }}
            disabled={loading}
            style={{ flex: 1 }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={sendMessage}
            loading={loading}
            disabled={!inputValue.trim()}
          >
            发送
          </Button>
        </div>
      </div>
    </div>
  );
};
