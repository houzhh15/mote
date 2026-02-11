// ================================================================
// PromptSelector - Prompt reference selector component
// ================================================================

import React, { useState, useEffect } from 'react';
import { List, Typography, Tag, Empty, Spin, Tabs, message, Modal, Form, Input, theme } from 'antd';
import { FileTextOutlined, ApiOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Prompt, MCPPrompt } from '../types';

const { Text } = Typography;

// Unified prompt item for display
interface PromptItem {
  id: string;
  name: string;
  content: string;
  type: 'user' | 'mcp';
  source?: string; // MCP server name
  description?: string;
  hasArgs?: boolean;
  args?: { name: string; description?: string; required?: boolean }[];
}

interface PromptSelectorProps {
  visible: boolean;
  searchQuery: string;
  onSelect: (content: string) => void;
  onCancel: () => void;
}

export const PromptSelector: React.FC<PromptSelectorProps> = ({
  visible,
  searchQuery,
  onSelect,
  onCancel,
}) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [userPrompts, setUserPrompts] = useState<PromptItem[]>([]);
  const [mcpPrompts, setMCPPrompts] = useState<PromptItem[]>([]);
  const [filteredPrompts, setFilteredPrompts] = useState<PromptItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetchingPrompt, setFetchingPrompt] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('all');
  const [argsModalVisible, setArgsModalVisible] = useState(false);
  const [pendingPrompt, setPendingPrompt] = useState<PromptItem | null>(null);
  const [form] = Form.useForm();
  
  // Log when component visibility changes
  useEffect(() => {
    console.log('[PromptSelector] Visibility changed:', visible);
  }, [visible]);

  // Load prompts on mount
  useEffect(() => {
    const loadPrompts = async () => {
      setLoading(true);
      try {
        // Load user prompts
        const userData = await api.getPrompts?.() ?? [];
        console.log('[PromptSelector] Loaded user prompts:', userData.length);
        const userItems: PromptItem[] = userData
          .filter((p: Prompt) => p.enabled)
          .map((p: Prompt) => ({
            id: p.id,
            name: p.name,
            content: p.content,
            type: 'user' as const,
            description: p.description,
            hasArgs: p.arguments && p.arguments.length > 0,
            args: p.arguments,
          }));
        setUserPrompts(userItems);

        // Load MCP prompts
        const mcpData = await api.getMCPPrompts?.() ?? [];
        console.log('[PromptSelector] Loaded MCP prompts:', mcpData.length);
        const mcpItems: PromptItem[] = mcpData.map((p: MCPPrompt) => ({
          id: `mcp_${p.server}_${p.name}`,
          name: p.name,
          content: p.description || `MCP Prompt: ${p.name}`,
          type: 'mcp' as const,
          source: p.server,
          description: p.description,
          hasArgs: p.arguments && p.arguments.length > 0,
          args: p.arguments,
        }));
        setMCPPrompts(mcpItems);
        console.log('[PromptSelector] Total prompts loaded:', userItems.length + mcpItems.length);
      } catch (error) {
        console.error('[PromptSelector] Failed to load prompts:', error);
        setUserPrompts([]);
        setMCPPrompts([]);
      } finally {
        setLoading(false);
      }
    };
    if (visible) {
      console.log('[PromptSelector] Popup opened, loading prompts...');
      loadPrompts();
    }
  }, [api, visible]);

  // Filter prompts based on search query and active tab
  useEffect(() => {
    let items: PromptItem[] = [];
    
    if (activeTab === 'all') {
      items = [...userPrompts, ...mcpPrompts];
    } else if (activeTab === 'user') {
      items = userPrompts;
    } else if (activeTab === 'mcp') {
      items = mcpPrompts;
    }

    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      items = items.filter((p) =>
        p.name.toLowerCase().includes(query) ||
        (p.content && p.content.toLowerCase().includes(query)) ||
        (p.source && p.source.toLowerCase().includes(query))
      );
    }

    console.log('[PromptSelector] Filtered prompts:', {
      activeTab,
      searchQuery,
      total: items.length,
      userCount: userPrompts.length,
      mcpCount: mcpPrompts.length
    });
    setFilteredPrompts(items);
  }, [searchQuery, userPrompts, mcpPrompts, activeTab]);

  // Fetch MCP prompt content with arguments
  const fetchMCPPromptContent = async (prompt: PromptItem, args: Record<string, string>) => {
    if (!prompt.source) {
      message.error('无法获取 MCP 提示词：缺少服务器信息');
      return;
    }
    
    setFetchingPrompt(true);
    try {
      const result = await api.getMCPPromptContent?.(prompt.source, prompt.name, args);
      
      if (result && result.messages && result.messages.length > 0) {
        // Combine all message contents
        const content = result.messages.map(m => {
          if (m.role && m.role !== 'user') {
            return `[${m.role}]\n${m.content}`;
          }
          return m.content;
        }).join('\n\n');
        onSelect(content);
      } else {
        // Fallback to description if no messages
        onSelect(prompt.description || prompt.content);
      }
      onCancel();
    } catch (error: any) {
      console.error('Failed to get MCP prompt content:', error);
      message.error(`获取提示词失败: ${error.message || '未知错误'}`);
    } finally {
      setFetchingPrompt(false);
    }
  };

  const handlePromptClick = async (prompt: PromptItem) => {
    if (prompt.type === 'mcp') {
      // For MCP prompts with required args, show a dialog
      const requiredArgs = prompt.args?.filter(a => a.required) || [];
      if (requiredArgs.length > 0) {
        // Show dialog to collect arguments
        setPendingPrompt(prompt);
        form.resetFields();
        setArgsModalVisible(true);
        return;
      }
      
      // No required args, fetch directly
      await fetchMCPPromptContent(prompt, {});
    } else {
      // User prompt
      const requiredArgs = prompt.args?.filter(a => a.required) || [];
      if (requiredArgs.length > 0 || (prompt.args && prompt.args.length > 0)) {
        // Show dialog to collect arguments (for any args, not just required)
        setPendingPrompt(prompt);
        form.resetFields();
        setArgsModalVisible(true);
        return;
      }
      
      // No args, use content directly
      onSelect(prompt.content);
      onCancel();
    }
  };

  const handleArgsSubmit = async () => {
    if (!pendingPrompt) return;
    
    try {
      const values = await form.validateFields();
      setArgsModalVisible(false);
      
      if (pendingPrompt.type === 'mcp') {
        await fetchMCPPromptContent(pendingPrompt, values);
      } else {
        // User prompt - render with variables
        setFetchingPrompt(true);
        try {
          const result = await api.renderPrompt?.(pendingPrompt.id, values);
          if (result) {
            onSelect(result.content);
          } else {
            onSelect(pendingPrompt.content);
          }
          onCancel();
        } catch (error: any) {
          console.error('Failed to render prompt:', error);
          message.error(`渲染提示词失败: ${error.message || '未知错误'}`);
        } finally {
          setFetchingPrompt(false);
        }
      }
    } catch (error) {
      // Form validation error
      console.error('Form validation error:', error);
    }
  };

  const handleArgsCancel = () => {
    setArgsModalVisible(false);
    setPendingPrompt(null);
    form.resetFields();
  };

  if (!visible) {
    console.log('[PromptSelector] Component hidden, returning null');
    return null;
  }

  console.log('[PromptSelector] Rendering with data:', {
    loading,
    userCount: userPrompts.length,
    mcpCount: mcpPrompts.length,
    filteredCount: filteredPrompts.length,
    activeTab
  });

  const tabItems = [
    { key: 'all', label: <span style={{ fontSize: 13 }}>{`全部 (${userPrompts.length + mcpPrompts.length})`}</span> },
    { key: 'user', label: <span style={{ fontSize: 13 }}>{`用户 (${userPrompts.length})`}</span> },
    { key: 'mcp', label: <span style={{ fontSize: 13 }}>{`MCP (${mcpPrompts.length})`}</span> },
  ];

  return (
    <div
      style={{
        position: 'absolute',
        bottom: '100%',
        left: 0,
        right: 0,
        maxHeight: 350,
        overflowY: 'auto',
        background: token.colorBgContainer,
        border: `1px solid ${token.colorBorderSecondary}`,
        borderRadius: 8,
        boxShadow: '0 -4px 12px rgba(0, 0, 0, 0.1)',
        zIndex: 100,
        marginBottom: 8,
      }}
      onClick={(e) => e.stopPropagation()}
    >
      {fetchingPrompt && (
        <div style={{ 
          position: 'absolute', 
          top: 0, 
          left: 0, 
          right: 0, 
          bottom: 0, 
          background: token.colorBgMask, 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center',
          zIndex: 10,
        }}>
          <Spin tip="正在获取提示词内容..." />
        </div>
      )}
      <div style={{ padding: '4px 12px 0', borderBottom: `1px solid ${token.colorBorderSecondary}` }}>
        <Tabs 
          activeKey={activeTab} 
          onChange={setActiveTab}
          size="small"
          items={tabItems}
        />
      </div>

      {loading ? (
        <div style={{ padding: 24, textAlign: 'center' }}>
          <Spin size="small" tip="正在加载提示词..." />
        </div>
      ) : filteredPrompts.length === 0 ? (
        <div style={{ padding: 24, textAlign: 'center' }}>
          <Empty
            description={
              <div>
                <div style={{ marginBottom: 8 }}>无匹配的提示词</div>
                {userPrompts.length === 0 && mcpPrompts.length === 0 && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    请在设置页面添加用户提示词，或配置 MCP 服务器
                  </Text>
                )}
                {(userPrompts.length > 0 || mcpPrompts.length > 0) && searchQuery && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    尝试更改搜索关键词或切换标签页
                  </Text>
                )}
              </div>
            }
            imageStyle={{ height: 40 }}
          />
        </div>
      ) : (
        <List
          size="small"
          dataSource={filteredPrompts}
          renderItem={(prompt) => (
            <List.Item
              onClick={() => !fetchingPrompt && handlePromptClick(prompt)}
              style={{
                cursor: fetchingPrompt ? 'wait' : 'pointer',
                padding: '8px 12px',
              }}
              className="prompt-selector-item"
            >
              <List.Item.Meta
                style={{ textAlign: 'left' }}
                avatar={
                  prompt.type === 'mcp' 
                    ? <ApiOutlined style={{ fontSize: 16, color: '#722ed1' }} />
                    : <FileTextOutlined style={{ fontSize: 16, color: '#1890ff' }} />
                }
                title={
                  <span style={{ fontSize: 13 }}>
                    /{prompt.name}
                    {prompt.type === 'mcp' && (
                      <Tag color="purple" style={{ marginLeft: 8, fontSize: 11 }}>
                        {prompt.source}
                      </Tag>
                    )}
                    {prompt.hasArgs && (
                      <Tag color="orange" style={{ marginLeft: 4, fontSize: 11 }}>
                        需要参数
                      </Tag>
                    )}
                  </span>
                }
                description={
                  <Text
                    ellipsis
                    type="secondary"
                    style={{ fontSize: 12, display: 'block', textAlign: 'left' }}
                  >
                    {prompt.description || prompt.content?.substring(0, 80)}
                    {(!prompt.description && (prompt.content?.length || 0) > 80) ? '...' : ''}
                  </Text>
                }
              />
            </List.Item>
          )}
        />
      )}

      <style>{`
        .prompt-selector-item:hover {
          background: ${token.colorBgTextHover};
        }
      `}</style>

      {/* Arguments Modal for MCP prompts with required args */}
      <Modal
        title={`填写参数 - ${pendingPrompt?.name}`}
        open={argsModalVisible}
        onOk={handleArgsSubmit}
        onCancel={handleArgsCancel}
        okText="确定"
        cancelText="取消"
        confirmLoading={fetchingPrompt}
      >
        <Form form={form} layout="vertical">
          {pendingPrompt?.args?.map((arg) => (
            <Form.Item
              key={arg.name}
              name={arg.name}
              label={arg.name}
              rules={arg.required ? [{ required: true, message: `请输入 ${arg.name}` }] : []}
              tooltip={arg.description}
            >
              <Input.TextArea 
                placeholder={arg.description || `请输入 ${arg.name}`}
                rows={3}
              />
            </Form.Item>
          ))}
        </Form>
        {pendingPrompt?.description && (
          <div style={{ marginTop: 8, color: '#666', fontSize: 12 }}>
            提示词说明: {pendingPrompt.description}
          </div>
        )}
      </Modal>
    </div>
  );
};
