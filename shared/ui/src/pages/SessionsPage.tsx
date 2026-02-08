// ================================================================
// SessionsPage - Shared sessions management page
// ================================================================

import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Button, Space, Spin, Empty, message, Modal, Tag, theme, Input } from 'antd';
import { DeleteOutlined, MessageOutlined, ClockCircleOutlined, GithubOutlined, SearchOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import { OllamaIcon } from '../components/OllamaIcon';
import type { Session } from '../types';

const { Text } = Typography;

// Helper function to extract provider from model ID
// Ollama models have "ollama:" prefix (e.g., "ollama:llama3.2")
// Copilot models don't have prefix (e.g., "gpt-4", "claude-3.5-sonnet")
const getProviderFromModel = (model?: string): 'copilot' | 'ollama' | null => {
  if (!model) return null;
  if (model.startsWith('ollama:')) {
    return 'ollama';
  }
  return 'copilot';
};

interface SessionsPageProps {
  onSelectSession?: (sessionId: string) => void;
}

export const SessionsPage: React.FC<SessionsPageProps> = ({ onSelectSession }) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const fetchSessions = async () => {
    setLoading(true);
    try {
      const data = await api.getSessions();
      setSessions(data);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
      message.error('获取会话列表失败');
    } finally {
      setLoading(false);
    }
  };

  const deleteSession = async (id: string) => {
    Modal.confirm({
      title: <div style={{ color: token.colorText }}>确认删除</div>,
      content: <div style={{ color: token.colorText }}>删除后无法恢复，确定要删除这个会话吗？</div>,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.deleteSession(id);
          message.success('删除成功');
          fetchSessions();
        } catch (error) {
          console.error('Failed to delete session:', error);
          message.error('删除失败');
        }
      },
    });
  };

  useEffect(() => {
    fetchSessions();
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Input
            placeholder="搜索会话..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ width: 260 }}
            allowClear
            prefix={<SearchOutlined style={{ color: token.colorTextSecondary }} />}
          />
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
      <Spin spinning={loading}>
        {sessions.length === 0 ? (
          <Empty description="暂无会话记录" />
        ) : (
          <List
            dataSource={sessions.filter(s => {
              if (!searchQuery) return true;
              const query = searchQuery.toLowerCase();
              const title = (s.title || '').toLowerCase();
              const preview = (s.preview || '').toLowerCase();
              const model = (s.model || '').toLowerCase();
              return title.includes(query) || preview.includes(query) || model.includes(query);
            })}
            renderItem={(session) => (
              <Card
                size="small"
                style={{ marginBottom: 12, cursor: onSelectSession ? 'pointer' : 'default' }}
                onClick={() => onSelectSession && onSelectSession(session.id)}
                hoverable={!!onSelectSession}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
                      <MessageOutlined style={{ marginRight: 8 }} />
                      <Text strong ellipsis style={{ flex: 1 }}>
                        {session.title || session.preview || `会话 ${session.id.slice(0, 8)}`}
                      </Text>
                      {session.model && (
                        <Tag 
                          color={getProviderFromModel(session.model) === 'ollama' ? 'orange' : 'blue'}
                          style={{ marginLeft: 8, display: 'flex', alignItems: 'center', gap: 4 }}
                        >
                          {getProviderFromModel(session.model) === 'ollama' ? <OllamaIcon size={10} /> : <GithubOutlined style={{ fontSize: 10 }} />}
                          {session.model}
                        </Tag>
                      )}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        <ClockCircleOutlined style={{ marginRight: 4 }} />
                        {new Date(session.updated_at).toLocaleString()}
                      </Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {session.message_count || 0} 条消息
                      </Text>
                    </div>
                  </div>
                  <Space>
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={(e) => {
                        e.stopPropagation();
                        deleteSession(session.id);
                      }}
                    />
                  </Space>
                </div>
              </Card>
            )}
          />
        )}
      </Spin>
      </div>
    </div>
  );
};
