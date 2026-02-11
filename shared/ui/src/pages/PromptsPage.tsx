// ================================================================
// PromptsPage - Shared prompts management page
// ================================================================

import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Typography, List, Card, Tag, Spin, Empty, message, Button, Modal, Input, Switch, Space, Descriptions, Tooltip, Popconfirm } from 'antd';
import { FileTextOutlined, FolderOpenOutlined, PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Prompt } from '../types';

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

export interface PromptsPageProps {
  hideToolbar?: boolean;
}

export interface PromptsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  fetchPrompts: () => void;
}

export const PromptsPage = forwardRef<PromptsPageRef, PromptsPageProps>(({ hideToolbar = false }, ref) => {
  const api = useAPI();
  const [prompts, setPrompts] = useState<Prompt[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedPrompt, setSelectedPrompt] = useState<Prompt | null>(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [newPromptName, setNewPromptName] = useState('');
  const [newPromptContent, setNewPromptContent] = useState('');
  const [editContent, setEditContent] = useState('');

  const fetchPrompts = async () => {
    setLoading(true);
    try {
      // Reload prompts from disk first
      await api.reloadPrompts?.();
      // Then fetch the updated list
      const data = await api.getPrompts?.() ?? [];
      setPrompts(data);
    } catch (error) {
      console.error('Failed to fetch prompts:', error);
      message.error('获取提示词列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPrompts();
  }, []);

  const handleOpenDir = async (target: 'user' | 'workspace') => {
    try {
      await api.openPromptsDir?.(target);
      message.success('已在文件管理器中打开提示词目录');
    } catch (error) {
      console.error('Failed to open prompts dir:', error);
      message.error('打开目录失败');
    }
  };

  // Expose methods to parent via ref
  useImperativeHandle(ref, () => ({
    handleOpenDir,
    setCreateModalVisible,
    fetchPrompts,
  }));

  const handleCreatePrompt = async () => {
    if (!newPromptName.trim()) {
      message.warning('请输入提示词名称');
      return;
    }
    if (!newPromptContent.trim()) {
      message.warning('请输入提示词内容');
      return;
    }
    try {
      await api.createPrompt?.({
        name: newPromptName.trim(),
        content: newPromptContent.trim(),
        enabled: true,
      });
      message.success('提示词已创建');
      setCreateModalVisible(false);
      setNewPromptName('');
      setNewPromptContent('');
      fetchPrompts();
    } catch (error) {
      console.error('Failed to create prompt:', error);
      message.error('创建提示词失败');
    }
  };

  const handleToggle = async (prompt: Prompt) => {
    try {
      await api.togglePrompt?.(prompt.id);
      message.success(`已${prompt.enabled ? '禁用' : '启用'}提示词: ${prompt.name}`);
      fetchPrompts();
    } catch (error) {
      console.error('Failed to toggle prompt:', error);
      message.error('切换状态失败');
    }
  };

  const handleDelete = async (prompt: Prompt) => {
    try {
      await api.deletePrompt?.(prompt.id);
      message.success(`已删除提示词: ${prompt.name}`);
      fetchPrompts();
    } catch (error) {
      console.error('Failed to delete prompt:', error);
      message.error('删除提示词失败');
    }
  };

  const handleEdit = (prompt: Prompt) => {
    setSelectedPrompt(prompt);
    setEditContent(prompt.content);
    setEditModalVisible(true);
  };

  const handleSaveEdit = async () => {
    if (!selectedPrompt || !editContent.trim()) {
      message.warning('提示词内容不能为空');
      return;
    }
    try {
      await api.updatePrompt?.(selectedPrompt.id, { content: editContent.trim() });
      message.success('提示词已更新');
      setEditModalVisible(false);
      setSelectedPrompt(null);
      setEditContent('');
      fetchPrompts();
    } catch (error) {
      console.error('Failed to update prompt:', error);
      message.error('更新提示词失败');
    }
  };

  const showDetail = (prompt: Prompt) => {
    setSelectedPrompt(prompt);
    setDetailVisible(true);
  };

  const getTypeColor = (type?: string) => {
    const colors: Record<string, string> = {
      system: 'blue',
      user: 'green',
      skill: 'orange',
    };
    return colors[type || 'user'] || 'default';
  };

  return (
    <div style={{ padding: hideToolbar ? 0 : 24, paddingTop: hideToolbar ? 16 : 24, height: '100%', overflow: 'auto' }}>
      {!hideToolbar && (
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Space>
          <Button icon={<FolderOpenOutlined />} onClick={() => handleOpenDir('user')} size="small" className="page-header-btn">
            打开用户目录
          </Button>
          <Button icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)} size="small" className="page-header-btn">
            创建提示词
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchPrompts} loading={loading} size="small" className="page-header-btn">
            刷新
          </Button>
          </Space>
        </div>
      </div>
      )}

      <Spin spinning={loading}>
        {prompts.length === 0 ? (
          <Empty description="暂无提示词" />
        ) : (
          <List
            grid={{ gutter: 16, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={prompts}
            renderItem={(prompt) => (
              <List.Item>
                <Card
                  size="small"
                  title={
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <FileTextOutlined />
                      <Text strong ellipsis style={{ flex: 1 }}>{prompt.name}</Text>
                    </div>
                  }
                  extra={
                    <Space>
                      <Tag color={getTypeColor(prompt.type)}>{prompt.type || 'user'}</Tag>
                      <Switch
                        size="small"
                        checked={prompt.enabled}
                        onChange={() => handleToggle(prompt)}
                      />
                    </Space>
                  }
                  actions={[
                    <Tooltip title="详情" key="detail">
                      <InfoCircleOutlined onClick={() => showDetail(prompt)} />
                    </Tooltip>,
                    <Tooltip title="编辑" key="edit">
                      <EditOutlined onClick={() => handleEdit(prompt)} />
                    </Tooltip>,
                    <Popconfirm
                      title="确定删除此提示词？"
                      onConfirm={() => handleDelete(prompt)}
                      okText="确定"
                      cancelText="取消"
                      key="delete"
                    >
                      <DeleteOutlined style={{ color: '#ff4d4f' }} />
                    </Popconfirm>,
                  ]}
                >
                  <Paragraph ellipsis={{ rows: 3 }} style={{ marginBottom: 8 }}>
                    {prompt.content || '无内容'}
                  </Paragraph>
                  {prompt.priority !== undefined && prompt.priority > 0 && (
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      优先级: {prompt.priority}
                    </Text>
                  )}
                </Card>
              </List.Item>
            )}
          />
        )}
      </Spin>

      {/* Detail Modal */}
      <Modal
        title={selectedPrompt?.name || '提示词详情'}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={600}
      >
        {selectedPrompt && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="ID">{selectedPrompt.id}</Descriptions.Item>
            <Descriptions.Item label="名称">{selectedPrompt.name}</Descriptions.Item>
            <Descriptions.Item label="类型">
              <Tag color={getTypeColor(selectedPrompt.type)}>{selectedPrompt.type || 'user'}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={selectedPrompt.enabled ? 'green' : 'default'}>
                {selectedPrompt.enabled ? '已启用' : '已禁用'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="优先级">{selectedPrompt.priority ?? 0}</Descriptions.Item>
            <Descriptions.Item label="内容">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{selectedPrompt.content}</pre>
            </Descriptions.Item>
            <Descriptions.Item label="创建时间">{selectedPrompt.created_at}</Descriptions.Item>
            <Descriptions.Item label="更新时间">{selectedPrompt.updated_at}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>

      {/* Create Modal */}
      <Modal
        title="创建新提示词"
        open={createModalVisible}
        onOk={handleCreatePrompt}
        onCancel={() => {
          setCreateModalVisible(false);
          setNewPromptName('');
          setNewPromptContent('');
        }}
        okText="创建"
        cancelText="取消"
        width={600}
      >
        <div style={{ marginBottom: 16 }}>
          <Text>提示词名称</Text>
          <Input
            placeholder="输入提示词名称"
            value={newPromptName}
            onChange={(e) => setNewPromptName(e.target.value)}
            style={{ marginTop: 8 }}
          />
        </div>
        <div>
          <Text>提示词内容</Text>
          <TextArea
            placeholder="输入提示词内容..."
            value={newPromptContent}
            onChange={(e) => setNewPromptContent(e.target.value)}
            rows={8}
            style={{ marginTop: 8 }}
          />
        </div>
      </Modal>

      {/* Edit Modal */}
      <Modal
        title={`编辑提示词 - ${selectedPrompt?.name}`}
        open={editModalVisible}
        onOk={handleSaveEdit}
        onCancel={() => {
          setEditModalVisible(false);
          setSelectedPrompt(null);
          setEditContent('');
        }}
        okText="保存"
        cancelText="取消"
        width={600}
      >
        <TextArea
          value={editContent}
          onChange={(e) => setEditContent(e.target.value)}
          rows={12}
        />
      </Modal>
    </div>
  );
});

export default PromptsPage;
