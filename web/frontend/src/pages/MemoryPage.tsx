import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Tag, Button, Space, Input, Spin, Empty, message } from 'antd';
import { SearchOutlined, ReloadOutlined, DeleteOutlined } from '@ant-design/icons';

const { Title, Text, Paragraph } = Typography;

interface Memory {
  id: string;
  content: string;
  category: string;
  created_at: string;
  relevance?: number;
}

export const MemoryPage: React.FC = () => {
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const fetchMemories = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/memory');
      const data = await response.json();
      setMemories(data.memories || []);
    } catch (error) {
      console.error('Failed to fetch memories:', error);
      message.error('获取记忆失败');
    } finally {
      setLoading(false);
    }
  };

  const searchMemories = async () => {
    if (!searchQuery.trim()) {
      fetchMemories();
      return;
    }
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/memory/search?q=${encodeURIComponent(searchQuery)}`);
      const data = await response.json();
      setMemories(data.memories || []);
    } catch (error) {
      console.error('Failed to search memories:', error);
      message.error('搜索记忆失败');
    } finally {
      setLoading(false);
    }
  };

  const deleteMemory = async (id: string) => {
    try {
      await fetch(`/api/v1/memory/${id}`, { method: 'DELETE' });
      message.success('删除成功');
      fetchMemories();
    } catch (error) {
      console.error('Failed to delete memory:', error);
      message.error('删除失败');
    }
  };

  useEffect(() => {
    fetchMemories();
  }, []);

  const getCategoryColor = (category: string) => {
    const colors: Record<string, string> = {
      preference: 'blue',
      fact: 'green',
      event: 'orange',
      instruction: 'purple',
    };
    return colors[category] || 'default';
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <Title level={4}>记忆管理</Title>
        <Space style={{ marginTop: 16 }}>
          <Input.Search
            placeholder="搜索记忆..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onSearch={searchMemories}
            style={{ width: 300 }}
            enterButton={<SearchOutlined />}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchMemories}>
            刷新
          </Button>
        </Space>
      </div>

      <Spin spinning={loading}>
        {memories.length === 0 ? (
          <Empty description="暂无记忆" />
        ) : (
          <List
            grid={{ gutter: 16, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={memories}
            renderItem={(memory) => (
              <List.Item>
                <Card
                  size="small"
                  title={
                    <Space>
                      <Tag color={getCategoryColor(memory.category)}>{memory.category}</Tag>
                      {memory.relevance && (
                        <Tag color="gold">相关度: {(memory.relevance * 100).toFixed(0)}%</Tag>
                      )}
                    </Space>
                  }
                  extra={
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => deleteMemory(memory.id)}
                    />
                  }
                >
                  <Paragraph ellipsis={{ rows: 3 }}>{memory.content}</Paragraph>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {new Date(memory.created_at).toLocaleString()}
                  </Text>
                </Card>
              </List.Item>
            )}
          />
        )}
      </Spin>
    </div>
  );
};
