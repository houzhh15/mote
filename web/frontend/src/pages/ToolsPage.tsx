import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Tag, Spin, Empty, message, Collapse, Badge } from 'antd';
import { ToolOutlined, ApiOutlined } from '@ant-design/icons';

const { Title, Text, Paragraph } = Typography;

interface Tool {
  name: string;
  description: string;
  type: string;
  source?: string;
  parameters?: Record<string, unknown>;
}

export const ToolsPage: React.FC = () => {
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchTools = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/tools');
      const data = await response.json();
      setTools(data.tools || []);
    } catch (error) {
      console.error('Failed to fetch tools:', error);
      message.error('获取工具列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTools();
  }, []);

  const getTypeColor = (type: string) => {
    const colors: Record<string, string> = {
      builtin: 'blue',
      mcp: 'purple',
      custom: 'green',
      skill: 'orange',
    };
    return colors[type] || 'default';
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <Title level={4}>工具列表</Title>
        <Text type="secondary">共 {tools.length} 个工具</Text>
      </div>

      <Spin spinning={loading}>
        {tools.length === 0 ? (
          <Empty description="暂无工具" />
        ) : (
          <List
            grid={{ gutter: 16, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={tools}
            renderItem={(tool) => (
              <List.Item>
                <Card
                  size="small"
                  title={
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <ToolOutlined />
                      <Text strong ellipsis style={{ flex: 1 }}>{tool.name}</Text>
                    </div>
                  }
                  extra={<Tag color={getTypeColor(tool.type)}>{tool.type}</Tag>}
                >
                  <Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 8 }}>
                    {tool.description || '无描述'}
                  </Paragraph>
                  {tool.source && (
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      来源: {tool.source}
                    </Text>
                  )}
                </Card>
              </List.Item>
            )}
          />
        )}
      </Spin>
    </div>
  );
};
