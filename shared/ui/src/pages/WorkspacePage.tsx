// ================================================================
// WorkspacePage - Shared workspace management page
// 工作区管理：默认工作区设置、Session 级工作区、工作区列表
// ================================================================

import React, { useState, useEffect } from 'react';
import { Typography, Button, Modal, Form, Input, Space, message, Popconfirm, Empty, Card, List, Tooltip, Divider, theme } from 'antd';
import { FolderOutlined, FolderOpenOutlined, PlusOutlined, DeleteOutlined, SettingOutlined, CheckCircleOutlined, HomeFilled } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';

const { Title, Text, Paragraph } = Typography;

interface Workspace {
  id: string;
  name: string;
  path: string;
  description?: string;
  is_default?: boolean;
  created_at?: string;
}

interface RecentWorkspace {
  path: string;
  name: string;
  last_used?: string;
}

export const WorkspacePage: React.FC = () => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [loading, setLoading] = useState(false);
  const [defaultWorkspace, setDefaultWorkspace] = useState<Workspace | null>(null);
  const [recentWorkspaces, setRecentWorkspaces] = useState<RecentWorkspace[]>([]);
  const [initModalVisible, setInitModalVisible] = useState(false);
  const [setDefaultModalVisible, setSetDefaultModalVisible] = useState(false);
  const [form] = Form.useForm();
  const [defaultForm] = Form.useForm();

  const getDirName = (path: string) => {
    // Handle both Windows and Unix separators
    const normalized = path.replace(/\\/g, '/');
    // Remove trailing slash if present (unless it's root)
    const trimmed = normalized.endsWith('/') && normalized.length > 1 
      ? normalized.slice(0, -1) 
      : normalized;
    return trimmed.split('/').pop() || path;
  };

  const fetchWorkspaces = async () => {
    setLoading(true);
    try {
      const data = await api.getWorkspaces?.() ?? [];
      // Adapt to different API response shapes
      if (Array.isArray(data)) {
        // Old API format: returns array of bound workspaces
        const defaultWs = (data as Workspace[]).find(ws => ws.is_default);
        if (defaultWs && !defaultWs.name) {
          defaultWs.name = getDirName(defaultWs.path);
        }
        setDefaultWorkspace(defaultWs || null);
        setRecentWorkspaces((data as Workspace[]).map(ws => ({ 
          path: ws.path, 
          name: ws.name || getDirName(ws.path) 
        })));
      } else {
        // New API format with default property
        const responseData = data as { default?: Workspace; workspaces?: RecentWorkspace[] };
        const defaultWs = responseData.default;
        if (defaultWs && !defaultWs.name) {
          defaultWs.name = getDirName(defaultWs.path);
        }
        setDefaultWorkspace(defaultWs || null);
        
        const workspaces = (responseData.workspaces || []).map(ws => ({
          ...ws,
          name: ws.name || getDirName(ws.path)
        }));
        setRecentWorkspaces(workspaces);
      }
    } catch (error) {
      console.error('Failed to fetch workspaces:', error);
      // Don't show error for missing API
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchWorkspaces();
  }, []);

  const handleSetDefault = async (values: { path: string }) => {
    try {
      await api.bindWorkspace?.('default', values.path, '默认工作区', false);
      message.success('默认工作区设置成功');
      setSetDefaultModalVisible(false);
      defaultForm.resetFields();
      fetchWorkspaces();
    } catch (error) {
      console.error('Failed to set default workspace:', error);
      message.error('设置默认工作区失败');
    }
  };

  const handleInitWorkspace = async (values: { path: string; name?: string }) => {
    try {
      // Initialize workspace by creating .mote directory structure
      await api.bindWorkspace?.('init', values.path, values.name, false);
      message.success('工作区初始化成功');
      setInitModalVisible(false);
      form.resetFields();
      fetchWorkspaces();
    } catch (error) {
      console.error('Failed to initialize workspace:', error);
      message.error('初始化工作区失败');
    }
  };

  const handleOpenWorkspace = async (_path: string) => {
    try {
      // Open workspace in file manager using skills/open API pattern
      await api.openSkillsDir?.('workspace');
      message.success('已打开工作区目录');
    } catch (error) {
      message.error('打开目录失败');
    }
  };

  const handleRemoveRecent = async (path: string) => {
    try {
      await api.unbindWorkspace?.(path);
      message.success('已从最近列表移除');
      fetchWorkspaces();
    } catch (error) {
      message.error('移除失败');
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <Title level={4} style={{ margin: 0 }}>工作区管理</Title>
          </div>
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ marginBottom: 16 }}>
          <Text type="secondary">
            工作区是项目级别的配置单元，包含技能、工具、提示词和 Hooks
          </Text>
        </div>

        {/* Default Workspace Card */}
      <Card 
        title={
          <Space>
            <HomeFilled style={{ color: '#1890ff' }} />
            <span>默认工作区</span>
          </Space>
        }
        extra={
          <Button 
            type="primary" 
            icon={<SettingOutlined />}
            onClick={() => setSetDefaultModalVisible(true)}
          >
            设置默认
          </Button>
        }
        style={{ marginBottom: 24 }}
      >
        {defaultWorkspace ? (
          <div>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>名称：</Text>
                <Text>{defaultWorkspace.name}</Text>
              </div>
              <div>
                <Text strong>路径：</Text>
                <Text copyable code>{defaultWorkspace.path}</Text>
              </div>
              {defaultWorkspace.description && (
                <div>
                  <Text strong>描述：</Text>
                  <Text>{defaultWorkspace.description}</Text>
                </div>
              )}
              <Space style={{ marginTop: 8 }}>
                <Button 
                  icon={<FolderOpenOutlined />}
                  onClick={() => handleOpenWorkspace(defaultWorkspace.path)}
                >
                  打开目录
                </Button>
              </Space>
            </Space>
          </div>
        ) : (
          <Empty 
            description="未设置默认工作区"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            <Button type="primary" onClick={() => setSetDefaultModalVisible(true)}>
              设置默认工作区
            </Button>
          </Empty>
        )}
      </Card>

      {/* Actions */}
      <Space style={{ marginBottom: 16 }}>
        <Button 
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setInitModalVisible(true)}
        >
          初始化工作区
        </Button>
      </Space>

      {/* Recent Workspaces */}
      <Divider orientation="left">最近使用的工作区</Divider>
      
      <List
        loading={loading}
        dataSource={recentWorkspaces}
        locale={{ emptyText: <Empty description="暂无最近使用的工作区" /> }}
        renderItem={(item) => (
          <List.Item
            actions={[
              <Tooltip title="设为默认" key="default">
                <Button 
                  type="text" 
                  icon={<CheckCircleOutlined />}
                  onClick={() => {
                    defaultForm.setFieldValue('path', item.path);
                    setSetDefaultModalVisible(true);
                  }}
                />
              </Tooltip>,
              <Tooltip title="打开目录" key="open">
                <Button 
                  type="text" 
                  icon={<FolderOpenOutlined />}
                  onClick={() => handleOpenWorkspace(item.path)}
                />
              </Tooltip>,
              <Popconfirm
                key="remove"
                title="确定从列表中移除？"
                onConfirm={() => handleRemoveRecent(item.path)}
                okText="确定"
                cancelText="取消"
              >
                <Button type="text" danger icon={<DeleteOutlined />} />
              </Popconfirm>,
            ]}
          >
            <List.Item.Meta
              avatar={<FolderOutlined style={{ fontSize: 24, color: '#1890ff' }} />}
              title={item.name}
              description={<Text type="secondary" copyable>{item.path}</Text>}
            />
          </List.Item>
        )}
      />

      {/* Set Default Modal */}
      <Modal
        title={<Space><SettingOutlined /> 设置默认工作区</Space>}
        open={setDefaultModalVisible}
        onCancel={() => {
          setSetDefaultModalVisible(false);
          defaultForm.resetFields();
        }}
        footer={null}
        width={500}
      >
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          默认工作区将自动应用于新创建的会话。工作区中的技能、工具、提示词和 Hooks 将自动加载。
        </Paragraph>
        <Form
          form={defaultForm}
          layout="vertical"
          onFinish={handleSetDefault}
        >
          <Form.Item
            name="path"
            label="工作区路径"
            rules={[{ required: true, message: '请输入工作区路径' }]}
          >
            <Input placeholder="输入工作区绝对路径，如 /home/user/my-project" />
          </Form.Item>
          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button onClick={() => {
                setSetDefaultModalVisible(false);
                defaultForm.resetFields();
              }}>
                取消
              </Button>
              <Button type="primary" htmlType="submit">
                确定
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* Init Workspace Modal */}
      <Modal
        title={<Space><PlusOutlined /> 初始化工作区</Space>}
        open={initModalVisible}
        onCancel={() => {
          setInitModalVisible(false);
          form.resetFields();
        }}
        footer={null}
        width={500}
      >
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          初始化将在指定目录创建 .mote 配置目录结构，包括 skills、tools、prompts、hooks 子目录。
        </Paragraph>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleInitWorkspace}
        >
          <Form.Item
            name="path"
            label="目录路径"
            rules={[{ required: true, message: '请输入目录路径' }]}
          >
            <Input placeholder="输入要初始化的目录绝对路径" />
          </Form.Item>
          <Form.Item
            name="name"
            label="工作区名称（可选）"
          >
            <Input placeholder="为工作区设置一个友好名称" />
          </Form.Item>
          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button onClick={() => {
                setInitModalVisible(false);
                form.resetFields();
              }}>
                取消
              </Button>
              <Button type="primary" htmlType="submit">
                初始化
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
      </div>
    </div>
  );
};
