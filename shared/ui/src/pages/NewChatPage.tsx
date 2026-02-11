/**
 * NewChatPage - 新对话入口页面
 *
 * 居中布局，延迟创建 Session，发送第一条消息时才创建
 */
import React, { useState, useEffect, useCallback } from 'react';
import { Input, Button, Select, Space, Typography, message, Modal, Tag, Tooltip, Dropdown } from 'antd';
import { SendOutlined, GithubOutlined, FolderOutlined, LinkOutlined, DisconnectOutlined, FolderOpenOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import { useInputHistory } from '../hooks';
import { PromptSelector } from '../components/PromptSelector';
import { DirectoryPicker } from '../components/DirectoryPicker';
import { OllamaIcon } from '../components/OllamaIcon';
import type { Model, Workspace, Skill } from '../types';

const { TextArea } = Input;
const { Text, Title } = Typography;

export interface NewChatPageProps {
  /** 会话创建成功后的回调，传递 sessionId */
  onSessionCreated?: (sessionId: string) => void;
  /** 导航到 ChatPage 的回调，传递 sessionId */
  onNavigateToChat?: (sessionId: string) => void;
}

export const NewChatPage: React.FC<NewChatPageProps> = ({
  onSessionCreated,
  onNavigateToChat,
}) => {
  const api = useAPI();
  const { navigatePrev, navigateNext, resetNavigation } = useInputHistory();

  // 状态
  const [inputValue, setInputValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [models, setModels] = useState<Model[]>([]);
  const [currentModel, setCurrentModel] = useState<string>('');
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState<Workspace | null>(null);
  const [modelsLoading, setModelsLoading] = useState(true);
  const [workspaceModalVisible, setWorkspaceModalVisible] = useState(false);
  const [workspacePath, setWorkspacePath] = useState('');
  const [directoryPickerVisible, setDirectoryPickerVisible] = useState(false);
  const [promptSelectorVisible, setPromptSelectorVisible] = useState(false);
  const [promptSearchQuery, setPromptSearchQuery] = useState('');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]); // empty = all

  // 加载模型列表
  const loadModels = useCallback(async () => {
    setModelsLoading(true);
    try {
      const response = await api.getModels();
      setModels(response.models || []);
      setCurrentModel(response.current || response.default || '');
    } catch (error) {
      console.error('Failed to load models:', error);
      message.error('加载模型失败');
    } finally {
      setModelsLoading(false);
    }
  }, [api]);

  // 加载工作区列表
  const loadWorkspaces = useCallback(async () => {
    if (!api.getWorkspaces) return;
    try {
      const data = await api.getWorkspaces();
      setWorkspaces(data || []);
    } catch (error) {
      console.error('Failed to load workspaces:', error);
    }
  }, [api]);

  // 加载技能列表
  const loadSkills = useCallback(async () => {
    if (!api.getSkills) return;
    try {
      const skillsList = await api.getSkills();
      setSkills(skillsList || []);
    } catch (error) {
      console.error('Failed to load skills:', error);
    }
  }, [api]);

  useEffect(() => {
    loadModels();
    loadWorkspaces();
    loadSkills();
  }, [loadModels, loadWorkspaces, loadSkills]);

  // 获取模型图标
  const getModelIcon = (model: Model) => {
    if (model.provider === 'ollama') {
      return <OllamaIcon size={12} style={{ marginRight: 8 }} />;
    }
    return <GithubOutlined style={{ fontSize: 12, marginRight: 8 }} />;
  };

  // 判断模型是否免费（根据 model.is_free 字段）
  const isModelFree = (model: Model) => {
    return model.is_free === true;
  };

  // 选择工作区 - 使用 path 作为唯一标识
  const handleSelectWorkspace = (workspacePath: string) => {
    const ws = workspaces.find((w) => w.path === workspacePath);
    setSelectedWorkspace(ws || null);
    setWorkspaceModalVisible(false);
  };

  // 获取去重后的工作区列表（按 path 去重）
  const uniqueWorkspaces: Workspace[] = React.useMemo(() => {
    const seen = new Set<string>();
    return workspaces.filter((ws) => {
      if (seen.has(ws.path)) return false;
      seen.add(ws.path);
      return true;
    });
  }, [workspaces]);

  // 绑定新工作区（通过路径）
  const handleBindWorkspacePath = () => {
    if (!workspacePath.trim()) {
      message.warning('请输入工作区路径');
      return;
    }
    // 创建临时工作区对象，实际绑定在发送时进行
    setSelectedWorkspace({
      id: '',
      path: workspacePath.trim(),
      name: workspacePath.trim().split('/').pop() || workspacePath.trim(),
    } as Workspace);
    setWorkspaceModalVisible(false);
    setWorkspacePath('');
  };

  // 清除工作区绑定
  const handleClearWorkspace = () => {
    setSelectedWorkspace(null);
    setWorkspaceModalVisible(false);
  };

  // 发送消息（创建 Session 后发送）
  const handleSend = async () => {
    const content = inputValue.trim();
    if (!content) {
      return;
    }

    setLoading(true);

    try {
      // 1. 创建新 Session
      const title = content.slice(0, 50) + (content.length > 50 ? '...' : '');
      const session = await api.createSession(title, 'chat');
      const sessionId = session.id;

      // 2. 如果选择了模型，设置会话模型（等待完成）
      if (currentModel && api.setSessionModel) {
        try {
          await api.setSessionModel(sessionId, currentModel);
        } catch (e) {
          console.error('Failed to set session model:', e);
          message.warning('模型设置失败，但会话已创建');
          // 不阻塞，继续
        }
      }

      // 3. 如果选择了工作区，绑定工作区（等待完成）
      if (selectedWorkspace?.path && api.bindWorkspace) {
        try {
          await api.bindWorkspace(sessionId, selectedWorkspace.path, selectedWorkspace.alias);
        } catch (e) {
          console.error('Failed to bind workspace:', e);
          message.warning('工作区绑定失败，但会话已创建');
          // 不阻塞，继续发送
        }
      }

      // 3.5. 如果选择了技能，设置会话技能
      if (selectedSkills.length > 0 && api.setSessionSkills) {
        try {
          await api.setSessionSkills(sessionId, selectedSkills);
        } catch (e) {
          console.error('Failed to set session skills:', e);
          // 不阻塞，继续
        }
      }

      // 4. 通知 Session 创建成功
      onSessionCreated?.(sessionId);

      // 5. 发送消息（发送消息前先跳转，让 ChatPage 处理响应）
      // 这里我们将消息内容存储，让 ChatPage 在挂载时发送
      // 或者直接跳转，让 ChatPage 自动发送
      // 为了更好的用户体验，我们先跳转，然后 ChatPage 会自动加载

      // 注意：需要传递初始消息给 ChatPage
      // 这里通过 sessionStorage 临时存储待发送的消息
      sessionStorage.setItem(`mote_pending_message_${sessionId}`, content);

      // 6. 跳转到 ChatPage
      onNavigateToChat?.(sessionId);
    } catch (error) {
      console.error('Failed to create session:', error);
      message.error('创建会话失败，请重试');
      setLoading(false);
    }
  };

  // 处理键盘事件
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter 发送，Shift+Enter 换行
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
    // 上键导航到上一条历史
    if (e.key === 'ArrowUp' && !e.shiftKey && inputValue === '') {
      e.preventDefault();
      const prev = navigatePrev();
      setInputValue(prev ?? '');
    }
    // 下键导航到下一条历史
    if (e.key === 'ArrowDown' && !e.shiftKey && inputValue === '') {
      e.preventDefault();
      const next = navigateNext();
      setInputValue(next ?? '');
    }
    // Close prompt selector on Escape
    if (e.key === 'Escape' && promptSelectorVisible) {
      setPromptSelectorVisible(false);
      setPromptSearchQuery('');
    }
  };

  // 处理输入变化，检测 / 命令
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    setInputValue(value);
    resetNavigation();

    // Detect / command at the start of input
    const match = value.match(/^\/(.*)$/);
    if (match) {
      setPromptSelectorVisible(true);
      setPromptSearchQuery(match[1] || '');
    } else if (!value.startsWith('/')) {
      setPromptSelectorVisible(false);
      setPromptSearchQuery('');
    }
  };

  // 处理提示词选择
  const handlePromptSelect = (content: string) => {
    setInputValue(content);
    setPromptSelectorVisible(false);
    setPromptSearchQuery('');
  };

  return (
    <div className="new-chat-page" style={styles.container}>
      <div style={styles.content}>
        {/* 标题 */}
        <div style={styles.header}>
          <Title level={2} style={styles.title}>
            Message Mote
          </Title>
          <Text type="secondary">
            开始一个新的对话
          </Text>
        </div>

        {/* 输入区域 */}
        <div style={styles.inputContainer}>
          <div style={{ position: 'relative' }}>
            <TextArea
              value={inputValue}
              onChange={handleInputChange}
              onKeyDown={handleKeyDown}
              placeholder="输入消息开始对话... (输入 / 使用提示词)"
              autoSize={{ minRows: 3, maxRows: 8 }}
              style={styles.textArea}
              disabled={loading}
              className="mote-input"
            />
            <PromptSelector
              visible={promptSelectorVisible}
              searchQuery={promptSearchQuery}
              onSelect={(content) => {
                handlePromptSelect(content);
              }}
              onCancel={() => {
                setPromptSelectorVisible(false);
                setPromptSearchQuery('');
              }}
            />
          </div>

          {/* 底部控制栏：模型选择 + 工作区绑定 + 发送按钮，同一行 */}
          <div style={styles.controlBar}>
            <Space size="middle">
              {/* 技能选择 */}
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
                      setSelectedSkills(allSelected ? [] : selectedKeys);
                    },
                    onDeselect: ({ selectedKeys }) => {
                      setSelectedSkills(selectedKeys);
                    },
                  }}
                  trigger={['click']}
                >
                  <Button 
                    icon={<ThunderboltOutlined />} 
                    style={{ width: 44 }}
                  />
                </Dropdown>
              )}

              {/* 模型选择 */}
              <Select
                value={currentModel}
                onChange={setCurrentModel}
                style={{ width: 220 }}
                loading={modelsLoading}
                placeholder="选择模型"
                options={models.map((model) => ({
                  value: model.id,
                  label: (
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
                      <Space>
                        {getModelIcon(model)}
                        <span>{model.display_name || model.id}</span>
                      </Space>
                      {isModelFree(model) && (
                        <Tag color="green" style={{ marginLeft: 8, fontSize: 10 }}>免费</Tag>
                      )}
                    </div>
                  ),
                }))}
              />



              {/* 工作区绑定按钮 */}
              {selectedWorkspace ? (
                <Button
                  icon={<LinkOutlined />}
                  onClick={() => setWorkspaceModalVisible(true)}
                  style={{ color: '#52c41a' }}
                >
                  {selectedWorkspace.name || selectedWorkspace.path.split('/').pop()}
                </Button>
              ) : (
                <Button
                  icon={<FolderOutlined />}
                  onClick={() => setWorkspaceModalVisible(true)}
                >
                  绑定工作区
                </Button>
              )}
            </Space>

            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={handleSend}
              loading={loading}
              disabled={!inputValue.trim()}
            >
              发送
            </Button>
          </div>
        </div>
      </div>

      {/* 工作区绑定 Modal */}
      <Modal
        title={selectedWorkspace ? '工作区设置' : '绑定工作区'}
        open={workspaceModalVisible}
        onCancel={() => {
          setWorkspaceModalVisible(false);
          setWorkspacePath('');
        }}
        footer={selectedWorkspace ? [
          <Button key="open" type="primary" icon={<FolderOpenOutlined />} onClick={() => {
            navigator.clipboard.writeText(selectedWorkspace.path).then(() => {
              message.success('工作区路径已复制到剪贴板');
            }).catch(() => {
              message.info(`工作区路径: ${selectedWorkspace.path}`);
            });
          }}>
            复制路径
          </Button>,
          <Button key="unbind" danger icon={<DisconnectOutlined />} onClick={handleClearWorkspace}>
            解除绑定
          </Button>,
          <Button key="cancel" onClick={() => setWorkspaceModalVisible(false)}>
            关闭
          </Button>,
        ] : [
          <Button key="cancel" onClick={() => setWorkspaceModalVisible(false)}>
            取消
          </Button>,
          <Button key="bind" type="primary" onClick={handleBindWorkspacePath}>
            绑定
          </Button>,
        ]}
      >
        {selectedWorkspace ? (
          <div>
            <Typography.Paragraph>
              <strong>路径:</strong> {selectedWorkspace.path}
            </Typography.Paragraph>
            {selectedWorkspace.name && (
              <Typography.Paragraph>
                <strong>名称:</strong> {selectedWorkspace.name}
              </Typography.Paragraph>
            )}
            <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
              工作区绑定后，对话中可以访问该目录下的文件
            </Typography.Paragraph>
          </div>
        ) : (
          <div>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
              绑定工作区后，对话中可以访问该目录下的文件。
            </Typography.Paragraph>
            {uniqueWorkspaces.length > 0 && (
              <div style={{ marginBottom: 16 }}>
                <Typography.Text strong>选择已有工作区：</Typography.Text>
                <Select
                  style={{ width: '100%', marginTop: 8 }}
                  placeholder="选择工作区"
                  options={uniqueWorkspaces.map((ws) => ({
                    value: ws.path,
                    label: ws.alias || ws.path.split('/').pop() || ws.path,
                  }))}
                  onChange={handleSelectWorkspace}
                />
              </div>
            )}
            <div>
              <Typography.Text strong>或输入/浏览工作区路径：</Typography.Text>
              <Space.Compact style={{ width: '100%', marginTop: 8 }}>
                <Input
                  style={{ flex: 1 }}
                  placeholder="输入工作区路径，如 /path/to/project"
                  value={workspacePath}
                  onChange={(e) => setWorkspacePath(e.target.value)}
                  onPressEnter={handleBindWorkspacePath}
                />
                <Button
                  icon={<FolderOpenOutlined />}
                  onClick={() => setDirectoryPickerVisible(true)}
                >
                  浏览
                </Button>
              </Space.Compact>
            </div>
          </div>
        )}
      </Modal>

      {/* 目录选择器 */}
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
    </div>
  );
};

// 内联样式
const styles: { [key: string]: React.CSSProperties } = {
  container: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100%',
    padding: '24px',
    backgroundColor: 'var(--content-bg, #fafafa)',
  },
  content: {
    maxWidth: 600,
    width: '100%',
    textAlign: 'center',
  },
  header: {
    marginBottom: 32,
  },
  title: {
    marginBottom: 8,
  },
  inputContainer: {
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  },
  textArea: {
    borderRadius: 12,
    padding: 16,
    fontSize: 16,
    resize: 'none',
  },
  controlBar: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
};

export default NewChatPage;
