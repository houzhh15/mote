import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Tag, Button, Space, Spin, Empty, message, Switch, Modal, Form, Input, Select } from 'antd';
import { PlusOutlined, ApiOutlined, DeleteOutlined, PlayCircleOutlined, PauseCircleOutlined } from '@ant-design/icons';

const { Title, Text } = Typography;

interface MCPServer {
  name: string;
  type: 'http' | 'stdio';
  url?: string;
  command?: string;
  args?: string[];
  status?: 'running' | 'stopped' | 'error';
  tool_count?: number;
}

export const MCPPage: React.FC = () => {
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();

  const fetchServers = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/mcp/servers');
      const data = await response.json();
      setServers(data.servers || []);
    } catch (error) {
      console.error('Failed to fetch MCP servers:', error);
      message.error('获取 MCP 服务器列表失败');
    } finally {
      setLoading(false);
    }
  };

  const startServer = async (name: string) => {
    try {
      await fetch(`/api/v1/mcp/servers/${name}/start`, { method: 'POST' });
      message.success('启动成功');
      fetchServers();
    } catch (error) {
      console.error('Failed to start server:', error);
      message.error('启动失败');
    }
  };

  const stopServer = async (name: string) => {
    try {
      await fetch(`/api/v1/mcp/servers/${name}/stop`, { method: 'POST' });
      message.success('停止成功');
      fetchServers();
    } catch (error) {
      console.error('Failed to stop server:', error);
      message.error('停止失败');
    }
  };

  const addServer = async (values: Partial<MCPServer>) => {
    try {
      await fetch('/api/v1/mcp/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      message.success('添加成功');
      setModalVisible(false);
      form.resetFields();
      fetchServers();
    } catch (error) {
      console.error('Failed to add server:', error);
      message.error('添加失败');
    }
  };

  const deleteServer = async (name: string) => {
    try {
      await fetch(`/api/v1/mcp/servers/${name}`, { method: 'DELETE' });
      message.success('删除成功');
      fetchServers();
    } catch (error) {
      console.error('Failed to delete server:', error);
      message.error('删除失败');
    }
  };

  useEffect(() => {
    fetchServers();
  }, []);

  const getStatusColor = (status?: string) => {
    switch (status) {
      case 'running':
        return 'green';
      case 'error':
        return 'red';
      default:
        return 'default';
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Title level={4}>MCP 服务器</Title>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
            添加服务器
          </Button>
        </div>
      </div>

      <Spin spinning={loading}>
        {servers.length === 0 ? (
          <Empty description="暂无 MCP 服务器" />
        ) : (
          <List
            grid={{ gutter: 16, xs: 1, sm: 2, md: 2, lg: 3, xl: 3 }}
            dataSource={servers}
            renderItem={(server) => (
              <List.Item>
                <Card
                  title={
                    <Space>
                      <ApiOutlined />
                      <Text strong>{server.name}</Text>
                    </Space>
                  }
                  extra={
                    <Tag color={getStatusColor(server.status)}>
                      {server.status || 'stopped'}
                    </Tag>
                  }
                  actions={[
                    server.status === 'running' ? (
                      <Button
                        type="text"
                        icon={<PauseCircleOutlined />}
                        onClick={() => stopServer(server.name)}
                      >
                        停止
                      </Button>
                    ) : (
                      <Button
                        type="text"
                        icon={<PlayCircleOutlined />}
                        onClick={() => startServer(server.name)}
                      >
                        启动
                      </Button>
                    ),
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => deleteServer(server.name)}
                    >
                      删除
                    </Button>,
                  ]}
                >
                  <div style={{ marginBottom: 8 }}>
                    <Tag>{server.type}</Tag>
                    {server.tool_count !== undefined && (
                      <Text type="secondary">{server.tool_count} 个工具</Text>
                    )}
                  </div>
                  <Text type="secondary" ellipsis>
                    {server.url || server.command}
                  </Text>
                </Card>
              </List.Item>
            )}
          />
        )}
      </Spin>

      <Modal
        title="添加 MCP 服务器"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={addServer}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="服务器名称" />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select placeholder="选择类型">
              <Select.Option value="http">HTTP</Select.Option>
              <Select.Option value="stdio">StdIO</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) => prevValues.type !== currentValues.type}
          >
            {({ getFieldValue }) =>
              getFieldValue('type') === 'http' ? (
                <Form.Item name="url" label="URL" rules={[{ required: true }]}>
                  <Input placeholder="http://localhost:8080/mcp" />
                </Form.Item>
              ) : (
                <>
                  <Form.Item name="command" label="命令" rules={[{ required: true }]}>
                    <Input placeholder="python" />
                  </Form.Item>
                  <Form.Item name="args" label="参数">
                    <Input placeholder="-m mcp_server (逗号分隔)" />
                  </Form.Item>
                </>
              )
            }
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};
