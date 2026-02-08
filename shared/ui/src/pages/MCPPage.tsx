// ================================================================
// MCPPage - Shared MCP servers management page
// ================================================================

import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Tag, Button, Space, Spin, Empty, message, Modal, Form, Input, Select, Tooltip, theme } from 'antd';
import { PlusOutlined, DeleteOutlined, PlayCircleOutlined, ReloadOutlined, ApiOutlined, ImportOutlined, ToolOutlined, FileTextOutlined, StopOutlined, EditOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { MCPServer } from '../types';

const { Text } = Typography;

export const MCPPage: React.FC = () => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingServer, setEditingServer] = useState<MCPServer | null>(null);
  const [importJson, setImportJson] = useState('');
  const [importLoading, setImportLoading] = useState(false);
  const [form] = Form.useForm();
  const [editForm] = Form.useForm();

  const fetchServers = async () => {
    setLoading(true);
    try {
      const data = await api.getMCPServers();
      setServers(data);
    } catch (error) {
      console.error('Failed to fetch MCP servers:', error);
      message.error('获取 MCP 服务器列表失败');
    } finally {
      setLoading(false);
    }
  };

  const startServer = async (name: string) => {
    try {
      await api.startMCPServer(name);
      message.success('启动成功');
      fetchServers();
    } catch (error) {
      console.error('Failed to start server:', error);
      message.error('启动失败');
    }
  };

  const stopServer = async (name: string) => {
    try {
      await api.stopMCPServer(name);
      message.success('已停止');
      fetchServers();
    } catch (error) {
      console.error('Failed to stop server:', error);
      message.error('停止失败');
    }
  };

  const deleteServer = async (name: string) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除 MCP 服务器 "${name}" 吗？`,
      onOk: async () => {
        try {
          await api.deleteMCPServer(name);
          message.success('删除成功');
          fetchServers();
        } catch (error) {
          console.error('Failed to delete server:', error);
          message.error('删除失败');
        }
      },
    });
  };

  const createServer = async (values: Partial<MCPServer>) => {
    try {
      await api.createMCPServer(values);
      message.success('创建成功');
      setModalVisible(false);
      form.resetFields();
      fetchServers();
    } catch (error) {
      console.error('Failed to create server:', error);
      message.error('创建失败');
    }
  };

  const openEditModal = (server: MCPServer) => {
    setEditingServer(server);
    const transport = server.transport || server.type || 'http';
    editForm.setFieldsValue({
      type: transport,
      url: server.url || '',
      headers: server.headers ? JSON.stringify(server.headers, null, 2) : '',
      command: server.command || '',
      args: server.args?.join(' ') || '',
    });
    setEditModalVisible(true);
  };

  const updateServer = async (values: Record<string, unknown>) => {
    if (!editingServer) return;
    try {
      const updates: Partial<MCPServer> = {
        type: values.type as 'stdio' | 'http',
      };
      if (values.type === 'http') {
        updates.url = values.url as string;
        if (values.headers) {
          try {
            updates.headers = JSON.parse(values.headers as string);
          } catch {
            message.error('Headers JSON 格式无效');
            return;
          }
        }
      } else {
        updates.command = values.command as string;
        updates.args = values.args ? (values.args as string).split(/\s+/).filter(Boolean) : [];
      }
      await api.updateMCPServer(editingServer.name, updates);
      message.success('更新成功');
      setEditModalVisible(false);
      setEditingServer(null);
      editForm.resetFields();
      fetchServers();
    } catch (error) {
      console.error('Failed to update server:', error);
      message.error('更新失败');
    }
  };

  const handleImport = async () => {
    if (!importJson.trim()) {
      message.warning('请输入 JSON 配置');
      return;
    }

    let config: Record<string, unknown>;
    try {
      config = JSON.parse(importJson);
    } catch {
      message.error('JSON 格式无效');
      return;
    }

    if (typeof config !== 'object' || config === null || Array.isArray(config)) {
      message.error('JSON 格式必须是对象，例如: {"server_name": {...}}');
      return;
    }

    setImportLoading(true);
    try {
      const result = await api.importMCPServers(config);
      const { imported, errors } = result;
      
      if (imported.length > 0) {
        message.success(`成功导入 ${imported.length} 个服务器: ${imported.join(', ')}`);
      }
      
      const errorKeys = Object.keys(errors || {});
      if (errorKeys.length > 0) {
        errorKeys.forEach(name => {
          message.error(`导入 ${name} 失败: ${errors[name]}`);
        });
      }
      
      if (imported.length > 0) {
        setImportModalVisible(false);
        setImportJson('');
        fetchServers();
      }
    } catch (error) {
      console.error('Failed to import servers:', error);
      message.error(`导入失败: ${error instanceof Error ? error.message : '未知错误'}`);
    } finally {
      setImportLoading(false);
    }
  };

  useEffect(() => {
    fetchServers();
  }, []);

  const getStatusText = (status: string) => {
    switch (status) {
      case 'running':
      case 'connected':
        return '运行中';
      case 'stopped':
      case 'disconnected':
        return '已停止';
      case 'error':
        return '错误';
      default:
        return status;
    }
  };

  const isServerRunning = (status: string) => {
    return status === 'running' || status === 'connected';
  };

  const getTransportDisplay = (server: MCPServer) => {
    const transport = server.transport || server.type || 'stdio';
    return transport.toUpperCase();
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Space>
            <Button icon={<ImportOutlined />} onClick={() => setImportModalVisible(true)} className="page-header-btn">
              导入 JSON
            </Button>
          <Button icon={<PlusOutlined />} onClick={() => setModalVisible(true)} className="page-header-btn">
              添加服务器
            </Button>
          </Space>
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <Spin spinning={loading}>
        {servers.length === 0 ? (
          <Empty description="暂无 MCP 服务器" />
        ) : (
          <List
            dataSource={servers}
            renderItem={(server) => (
              <Card size="small" style={{ marginBottom: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                      <ApiOutlined style={{ color: isServerRunning(server.status) ? '#52c41a' : '#999' }} />
                      <Text strong style={{ fontSize: 14 }}>{server.name}</Text>
                      <Tag style={{ margin: 0 }}>{getTransportDisplay(server)}</Tag>
                      {isServerRunning(server.status) ? (
                        <>
                          <Tag color="green" style={{ margin: 0 }}>运行中</Tag>
                          {(server.tool_count !== undefined && server.tool_count > 0) && (
                            <Tooltip title="工具数量">
                              <Tag icon={<ToolOutlined />} style={{ margin: 0 }}>
                                {server.tool_count} 工具
                              </Tag>
                            </Tooltip>
                          )}
                          {(server.prompt_count !== undefined && server.prompt_count > 0) && (
                            <Tooltip title="提示词数量">
                              <Tag icon={<FileTextOutlined />} style={{ margin: 0 }}>
                                {server.prompt_count} 提示
                              </Tag>
                            </Tooltip>
                          )}
                        </>
                      ) : (
                        <Tag color={server.status === 'error' ? 'red' : 'default'} style={{ margin: 0 }}>
                          {getStatusText(server.status)}
                        </Tag>
                      )}
                    </div>
                    {server.error && (
                      <div style={{ marginTop: 4 }}>
                        <Text type="danger" style={{ fontSize: 12 }}>{server.error}</Text>
                      </div>
                    )}
                  </div>
                  <Space size="small">
                    {isServerRunning(server.status) ? (
                      <>
                        <Tooltip title="停止">
                          <Button
                            type="text"
                            size="small"
                            icon={<StopOutlined />}
                            onClick={() => stopServer(server.name)}
                          />
                        </Tooltip>
                        <Tooltip title="重启">
                          <Button
                            type="text"
                            size="small"
                            icon={<ReloadOutlined />}
                            onClick={() => startServer(server.name)}
                          />
                        </Tooltip>
                      </>
                    ) : (
                      <Tooltip title="启动">
                        <Button
                          type="text"
                          size="small"
                          icon={<PlayCircleOutlined />}
                          onClick={() => startServer(server.name)}
                        />
                      </Tooltip>
                    )}
                    <Tooltip title="编辑">
                      <Button
                        type="text"
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() => openEditModal(server)}
                      />
                    </Tooltip>
                    <Tooltip title="删除">
                      <Button
                        type="text"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => deleteServer(server.name)}
                      />
                    </Tooltip>
                  </Space>
                </div>
              </Card>
            )}
          />
        )}
      </Spin>
      </div>

      <Modal
        title="添加 MCP 服务器"
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false);
          form.resetFields();
        }}
        onOk={() => form.submit()}
        width={560}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(values) => {
            // Convert args string to array
            const serverData = {
              name: values.name,
              type: values.type,
              command: values.command,
              args: values.args ? values.args.split(/\s+/).filter(Boolean) : undefined,
              url: values.url,
              headers: values.headers ? JSON.parse(values.headers) : undefined,
            };
            createServer(serverData);
          }}
          initialValues={{ type: 'stdio' }}
        >
          <Form.Item name="name" label="服务器名称" rules={[{ required: true, message: '请输入服务器名称' }]}>
            <Input placeholder="例如: filesystem" />
          </Form.Item>
          <Form.Item name="type" label="传输类型" rules={[{ required: true }]}>
            <Select placeholder="选择传输类型">
              <Select.Option value="stdio">stdio (本地命令)</Select.Option>
              <Select.Option value="http">HTTP (远程服务)</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prevValues, curValues) => prevValues.type !== curValues.type}
          >
            {({ getFieldValue }) =>
              getFieldValue('type') === 'stdio' ? (
                <>
                  <Form.Item
                    name="command"
                    label="启动命令"
                    rules={[{ required: true, message: '请输入启动命令' }]}
                  >
                    <Input placeholder="例如: npx -y @modelcontextprotocol/server-filesystem" />
                  </Form.Item>
                  <Form.Item name="args" label="命令参数">
                    <Input placeholder="用空格分隔多个参数，例如: /tmp /home" />
                  </Form.Item>
                </>
              ) : (
                <>
                  <Form.Item
                    name="url"
                    label="服务器 URL"
                    rules={[{ required: true, message: '请输入服务器 URL' }]}
                  >
                    <Input placeholder="例如: http://localhost:8080/mcp" />
                  </Form.Item>
                  <Form.Item name="headers" label="请求头 (JSON)">
                    <Input.TextArea
                      rows={2}
                      placeholder='例如: {"Authorization": "Bearer xxx"}'
                    />
                  </Form.Item>
                </>
              )
            }
          </Form.Item>
        </Form>
      </Modal>

      {/* JSON 导入 Modal */}
      <Modal
        title="导入 MCP 服务器 (JSON)"
        open={importModalVisible}
        onCancel={() => {
          setImportModalVisible(false);
          setImportJson('');
        }}
        onOk={handleImport}
        confirmLoading={importLoading}
        okText="导入"
        cancelText="取消"
        width={640}
      >
        <div style={{ marginBottom: 16 }}>
          <Text type="secondary">
            支持批量导入多个 MCP 服务器，格式如下：
          </Text>
          <pre style={{ 
            background: token.colorBgLayout, 
            padding: 12, 
            borderRadius: 4, 
            fontSize: 12,
            marginTop: 8,
            overflow: 'auto'
          }}>
{`{
  "local": {
    "type": "http",
    "url": "http://127.0.0.1:8001/mcp",
    "headers": {
      "Authorization": "Bearer your-token"
    }
  },
  "filesystem": {
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
  }
}`}
          </pre>
        </div>
        <Input.TextArea
          value={importJson}
          onChange={(e) => setImportJson(e.target.value)}
          rows={10}
          placeholder='粘贴 JSON 配置...'
          style={{ fontFamily: 'monospace' }}
        />
      </Modal>

      {/* 编辑 Modal */}
      <Modal
        title={`编辑 MCP 服务器: ${editingServer?.name || ''}`}
        open={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false);
          setEditingServer(null);
          editForm.resetFields();
        }}
        onOk={() => editForm.submit()}
        width={560}
      >
        <Form
          form={editForm}
          layout="vertical"
          onFinish={updateServer}
        >
          <Form.Item name="type" label="传输类型" rules={[{ required: true }]}>
            <Select placeholder="选择传输类型">
              <Select.Option value="stdio">stdio (本地命令)</Select.Option>
              <Select.Option value="http">HTTP (远程服务)</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prevValues, curValues) => prevValues.type !== curValues.type}
          >
            {({ getFieldValue }) =>
              getFieldValue('type') === 'stdio' ? (
                <>
                  <Form.Item
                    name="command"
                    label="启动命令"
                    rules={[{ required: true, message: '请输入启动命令' }]}
                  >
                    <Input placeholder="例如: npx -y @modelcontextprotocol/server-filesystem" />
                  </Form.Item>
                  <Form.Item name="args" label="命令参数">
                    <Input placeholder="用空格分隔多个参数，例如: /tmp /home" />
                  </Form.Item>
                </>
              ) : (
                <>
                  <Form.Item
                    name="url"
                    label="服务器 URL"
                    rules={[{ required: true, message: '请输入服务器 URL' }]}
                  >
                    <Input placeholder="例如: http://localhost:8080/mcp" />
                  </Form.Item>
                  <Form.Item name="headers" label="请求头 (JSON)">
                    <Input.TextArea
                      rows={4}
                      placeholder='例如: {"Authorization": "Bearer xxx"}'
                      style={{ fontFamily: 'monospace' }}
                    />
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
