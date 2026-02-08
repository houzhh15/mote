import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Button, Spin, Empty, message, Input } from 'antd';
import { SearchOutlined, MessageOutlined, DeleteOutlined } from '@ant-design/icons';

const { Title, Text, Paragraph } = Typography;

interface Session {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  message_count?: number;
}

interface SessionsPageProps {
  onSelectSession: (id: string) => void;
}

export const SessionsPage: React.FC<SessionsPageProps> = ({ onSelectSession }) => {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const fetchSessions = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/sessions');
      const data = await response.json();
      setSessions(data.sessions || []);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
      message.error('获取会话列表失败');
    } finally {
      setLoading(false);
    }
  };

  const deleteSession = async (id: string) => {
    try {
      await fetch(`/api/v1/sessions/${id}`, { method: 'DELETE' });
      message.success('删除成功');
      fetchSessions();
    } catch (error) {
      console.error('Failed to delete session:', error);
      message.error('删除失败');
    }
  };

  useEffect(() => {
    fetchSessions();
  }, []);

  const filteredSessions = sessions.filter(
    (session) =>
      session.title.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="page-container">
      <div className="page-header">
        <Title level={4}>会话历史</Title>
        <Input
          placeholder="搜索会话..."
          prefix={<SearchOutlined />}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          style={{ width: 300, marginTop: 16 }}
        />
      </div>

      <Spin spinning={loading}>
        {filteredSessions.length === 0 ? (
          <Empty description="暂无会话" />
        ) : (
          <List
            dataSource={filteredSessions}
            renderItem={(session) => (
              <Card
                size="small"
                style={{ marginBottom: 12, cursor: 'pointer' }}
                hoverable
                onClick={() => onSelectSession(session.id)}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ flex: 1 }}>
                    <div style={{ marginBottom: 4 }}>
                      <MessageOutlined style={{ marginRight: 8 }} />
                      <Text strong>{session.title || '新会话'}</Text>
                    </div>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {new Date(session.updated_at || session.created_at).toLocaleString()}
                    </Text>
                  </div>
                  <Button
                    type="text"
                    danger
                    icon={<DeleteOutlined />}
                    onClick={(e) => {
                      e.stopPropagation();
                      deleteSession(session.id);
                    }}
                  />
                </div>
              </Card>
            )}
          />
        )}
      </Spin>
    </div>
  );
};
