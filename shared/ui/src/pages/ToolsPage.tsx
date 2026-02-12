// ================================================================
// ToolsPage - Shared tools list page
// ================================================================

import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Typography, List, Card, Tag, Spin, Empty, message, Button, Space, Modal, Input, Select } from 'antd';
import { ToolOutlined, FolderOpenOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Tool } from '../types';

const { Text, Paragraph } = Typography;

export interface ToolsPageProps {
  hideToolbar?: boolean;
}

export interface ToolsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  fetchTools: () => void;
}

export const ToolsPage = forwardRef<ToolsPageRef, ToolsPageProps>(({ hideToolbar = false }, ref) => {
  const api = useAPI();
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(false);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [newToolName, setNewToolName] = useState('');
  const [newToolRuntime, setNewToolRuntime] = useState('javascript');
  const [newToolTarget] = useState<'user'>('user');

  const fetchTools = async () => {
    setLoading(true);
    try {
      const data = await api.getTools();
      // Sort tools by name for consistent ordering
      const sortedData = [...data].sort((a, b) => a.name.localeCompare(b.name));
      setTools(sortedData);
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

  const handleOpenDir = async (target: 'user' | 'workspace') => {
    try {
      await api.openToolsDir?.(target);
      message.success('已在文件管理器中打开工具目录');
    } catch (error) {
      console.error('Failed to open tools dir:', error);
      message.error('打开目录失败');
    }
  };

  // Expose methods to parent via ref
  useImperativeHandle(ref, () => ({
    handleOpenDir,
    setCreateModalVisible,
    fetchTools,
  }));

  const handleCreateTool = async () => {
    if (!newToolName.trim()) {
      message.warning('请输入工具名称');
      return;
    }
    try {
      const result = await api.createTool?.(newToolName.trim(), newToolRuntime, newToolTarget);
      message.success(`工具模板已创建: ${result?.path}`);
      setCreateModalVisible(false);
      setNewToolName('');
      fetchTools();
    } catch (error) {
      console.error('Failed to create tool:', error);
      message.error('创建工具失败');
    }
  };

  const getTypeColor = (type: string) => {
    const colors: Record<string, string> = {
      builtin: 'blue',
      mcp: 'purple',
      custom: 'green',
      skill: 'orange',
      'script-javascript': 'cyan',
      'script-python': 'gold',
      'script-shell': 'lime',
      'script-powershell': 'magenta',
    };
    return colors[type] || 'default';
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
            创建工具
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchTools} loading={loading} size="small" className="page-header-btn">
            刷新
          </Button>
          </Space>
        </div>
      </div>
      )}

      <Spin spinning={loading}>
        {tools.length === 0 ? (
          <Empty description="暂无工具" />
        ) : (
          <List
            grid={{ gutter: 8, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={tools}
            style={{ padding: '0 8px' }}
            renderItem={(tool) => (
              <List.Item style={{ display: 'flex' }}>
                <Card
                  size="small"
                  style={{ width: '100%', minWidth: 220, minHeight: 160, height: '100%', display: 'flex', flexDirection: 'column' }}
                  styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column' } }}
                  title={
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <ToolOutlined />
                      <Text strong ellipsis style={{ flex: 1 }}>{tool.name}</Text>
                    </div>
                  }
                  extra={<Tag color={getTypeColor(tool.type)}>{tool.type}</Tag>}
                >
                  <div style={{ flex: 1 }}>
                    <Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 8, minHeight: '2.8em' }}>
                      {tool.description || '无描述'}
                    </Paragraph>
                  </div>
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

      {/* Create Tool Modal */}
      <Modal
        title="创建新工具"
        open={createModalVisible}
        onOk={handleCreateTool}
        onCancel={() => {
          setCreateModalVisible(false);
          setNewToolName('');
        }}
        okText="创建"
        cancelText="取消"
      >
        <div style={{ marginBottom: 16 }}>
          <Text>工具名称</Text>
          <Input
            placeholder="输入工具名称（英文，如 my-tool）"
            value={newToolName}
            onChange={(e) => setNewToolName(e.target.value)}
            style={{ marginTop: 8 }}
          />
        </div>
        <div style={{ marginBottom: 16 }}>
          <Text>运行时</Text>
          <Select
            value={newToolRuntime}
            onChange={setNewToolRuntime}
            style={{ width: '100%', marginTop: 8 }}
          >
            <Select.Option value="javascript">JavaScript</Select.Option>
            <Select.Option value="python">Python</Select.Option>
            <Select.Option value="shell">Shell/Bash</Select.Option>
            <Select.Option value="powershell">PowerShell</Select.Option>
          </Select>
        </div>
        <div>
          <Text type="secondary">工具将创建在用户目录（~/.mote/tools）</Text>
        </div>
      </Modal>
    </div>
  );
});

export default ToolsPage;
