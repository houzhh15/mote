// ================================================================
// AgentsPage - Multi-Agent Delegate management page
// ================================================================

import { useState, useEffect, useCallback } from 'react';
import {
  Typography, Card, Tag, Spin, Empty, message, Button, Space, Modal,
  Input, Select, InputNumber, List, Badge, Popconfirm, Tabs, Tooltip, theme, Switch, Checkbox,
} from 'antd';
import {
  TeamOutlined, PlusOutlined, ReloadOutlined, DeleteOutlined,
  EditOutlined, ClockCircleOutlined, CheckCircleOutlined,
  CloseCircleOutlined, LoadingOutlined, ThunderboltOutlined,
  CrownOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { AgentConfig, DelegationRecord, ModelsResponse, Model } from '../types';

const { Text, Paragraph } = Typography;

// ================================================================
// Primary Agent Card (read-only)
// ================================================================

interface PrimaryAgentCardProps {
  modelsInfo: ModelsResponse | null;
}

const PrimaryAgentCard: React.FC<PrimaryAgentCardProps> = ({ modelsInfo }) => {
  const currentModel = modelsInfo?.current || modelsInfo?.default || '(未设置)';
  const currentModelObj = modelsInfo?.models?.find(m => m.id === currentModel);
  const displayName = currentModelObj?.display_name || currentModel;
  const providerName = currentModelObj?.provider || '';

  return (
    <Card
      size="small"
      style={{
        width: '100%',
        minWidth: 0,
        minHeight: 180,
        display: 'flex',
        flexDirection: 'column',
        borderColor: '#d4b106',
        background: 'linear-gradient(135deg, rgba(250,219,20,0.04) 0%, transparent 100%)',
      }}
      styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column' } }}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <CrownOutlined style={{ color: '#d4b106' }} />
          <Text strong style={{ flex: 1 }}>主代理</Text>
          <Tooltip title="主代理是处理所有对话的入口代理，子代理通过 delegate 工具被主代理调用。在系统设置中管理 Provider 和模型。">
            <InfoCircleOutlined style={{ color: '#999', fontSize: 14 }} />
          </Tooltip>
        </div>
      }
    >
      <div style={{ flex: 1 }}>
        <Paragraph style={{ marginBottom: 8, minHeight: '2.8em', color: '#666' }}>
          默认对话代理，所有用户消息首先由此代理处理。可通过 delegate 工具自动调用子代理。
        </Paragraph>
        <Space wrap size={[4, 4]}>
          <Tag color="gold">{displayName}</Tag>
          {providerName && <Tag color="cyan">{providerName}</Tag>}
          <Tag color="default">主入口</Tag>
        </Space>
      </div>
      <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
        不可编辑 · 在系统设置中管理
      </Text>
    </Card>
  );
};

// ================================================================
// Agent Card Component
// ================================================================

interface AgentCardProps {
  name: string;
  config: AgentConfig;
  onEdit: (name: string, config: AgentConfig) => void;
  onDelete: (name: string) => void;
  onToggleEnabled: (name: string, enabled: boolean) => void;
}

const AgentCard: React.FC<AgentCardProps> = ({ name, config, onEdit, onDelete, onToggleEnabled }) => {
  const toolCount = config.tools?.length || 0;
  const toolLabel = toolCount === 0 ? '继承全部' : config.tools?.includes('*') ? '全部' : `${toolCount} 个`;
  const isEnabled = config.enabled !== false;

  return (
    <Card
      size="small"
      style={{
        width: '100%', minWidth: 0, minHeight: 180, display: 'flex', flexDirection: 'column',
        opacity: isEnabled ? 1 : 0.55,
        borderColor: isEnabled ? undefined : '#d9d9d9',
      }}
      styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column' } }}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <TeamOutlined style={{ color: isEnabled ? undefined : '#bfbfbf' }} />
          <Text strong ellipsis style={{ flex: 1, color: isEnabled ? undefined : '#bfbfbf' }}>{name}</Text>
        </div>
      }
      extra={
        <Space size={4}>
          <Tooltip title={isEnabled ? '禁用' : '启用'}>
            <Switch
              size="small"
              checked={isEnabled}
              onChange={(checked) => onToggleEnabled(name, checked)}
            />
          </Tooltip>
          <Button
            type="text" size="small" icon={<EditOutlined />}
            onClick={() => onEdit(name, config)}
          />
          <Popconfirm title={`删除代理 ${name}?`} onConfirm={() => onDelete(name)} okText="删除" cancelText="取消">
            <Button type="text" size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      }
    >
      <div style={{ flex: 1 }}>
        <Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 8, minHeight: '2.8em' }}>
          {config.description || '无描述'}
        </Paragraph>
        <Space wrap size={[4, 4]}>
          <Tag color="blue">{config.model || '(默认模型)'}</Tag>
          <Tag color="green">深度 {config.max_depth || 3}</Tag>
          <Tag color="orange">超时 {config.timeout || '5m'}</Tag>
          <Tag color="purple">工具: {toolLabel}</Tag>
        </Space>
      </div>
      {config.system_prompt && (
        <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }} ellipsis>
          Prompt: {config.system_prompt.slice(0, 60)}...
        </Text>
      )}
    </Card>
  );
};

// ================================================================
// Agent Edit Modal Component
// ================================================================

interface AgentEditModalProps {
  visible: boolean;
  editingName: string | null;
  config: AgentConfig;
  modelsInfo: ModelsResponse | null;
  onOk: (name: string, config: AgentConfig) => void;
  onCancel: () => void;
}

const AgentEditModal: React.FC<AgentEditModalProps> = ({ visible, editingName, config, modelsInfo, onOk, onCancel }) => {
  const [name, setName] = useState('');
  const [agentConfig, setAgentConfig] = useState<AgentConfig>({});

  useEffect(() => {
    if (visible) {
      setName(editingName || '');
      setAgentConfig({ ...config });
    }
  }, [visible, editingName, config]);

  const handleOk = () => {
    const n = editingName || name.trim();
    if (!n) {
      message.warning('请输入代理名称');
      return;
    }
    onOk(n, agentConfig);
  };

  const update = (field: keyof AgentConfig, value: unknown) => {
    setAgentConfig(prev => ({ ...prev, [field]: value }));
  };

  return (
    <Modal
      title={editingName ? `编辑代理: ${editingName}` : '创建新代理'}
      open={visible}
      onOk={handleOk}
      onCancel={onCancel}
      okText={editingName ? '保存' : '创建'}
      cancelText="取消"
      width={560}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 16 }}>
        {!editingName && (
          <div>
            <Text strong>名称</Text>
            <Input
              placeholder="agent 名称（英文，如 researcher）"
              value={name}
              onChange={e => setName(e.target.value)}
              style={{ marginTop: 4 }}
            />
          </div>
        )}
        <div>
          <Text strong>描述</Text>
          <Input
            placeholder="代理的简要说明"
            value={agentConfig.description || ''}
            onChange={e => update('description', e.target.value)}
            style={{ marginTop: 4 }}
          />
        </div>
        <div>
          <Text strong>模型</Text>
          <Select
            placeholder="留空则继承主代理模型"
            value={agentConfig.model || undefined}
            onChange={v => update('model', v || '')}
            allowClear
            showSearch
            style={{ width: '100%', marginTop: 4 }}
            optionFilterProp="label"
          >
            {(modelsInfo?.models || []).filter(m => m.available !== false).map((m: Model) => (
              <Select.Option key={m.id} value={m.id} label={m.display_name}>
                {m.display_name}{m.provider ? ` (${m.provider})` : ''}
              </Select.Option>
            ))}
          </Select>
        </div>
        <div>
          <Text strong>System Prompt</Text>
          <Input.TextArea
            placeholder="子代理的 system prompt"
            value={agentConfig.system_prompt || ''}
            onChange={e => update('system_prompt', e.target.value)}
            rows={3}
            style={{ marginTop: 4 }}
          />
        </div>
        <div style={{ display: 'flex', gap: 16 }}>
          <div style={{ flex: 1 }}>
            <Text strong>最大深度</Text>
            <InputNumber
              min={1} max={10}
              value={agentConfig.max_depth || 3}
              onChange={v => update('max_depth', v)}
              style={{ width: '100%', marginTop: 4 }}
            />
          </div>
          <div style={{ flex: 1 }}>
            <Text strong>超时</Text>
            <Input
              placeholder="5m"
              value={agentConfig.timeout || ''}
              onChange={e => update('timeout', e.target.value)}
              style={{ marginTop: 4 }}
            />
          </div>
          <div style={{ flex: 1 }}>
            <Text strong>温度</Text>
            <InputNumber
              min={0} max={2} step={0.1}
              value={agentConfig.temperature || undefined}
              onChange={v => update('temperature', v)}
              placeholder="默认"
              style={{ width: '100%', marginTop: 4 }}
            />
          </div>
        </div>
        <div>
          <Text strong>工具白名单</Text>
          <Select
            mode="tags"
            placeholder="留空=继承全部，输入工具名称回车添加"
            value={agentConfig.tools || []}
            onChange={v => update('tools', v)}
            style={{ width: '100%', marginTop: 4 }}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            输入 * 表示允许全部工具，留空表示继承主代理全部工具
          </Text>
        </div>
      </div>
    </Modal>
  );
};

// ================================================================
// Delegation History Component
// ================================================================

const statusConfig: Record<string, { color: string; icon: React.ReactNode }> = {
  running: { color: 'processing', icon: <LoadingOutlined /> },
  completed: { color: 'success', icon: <CheckCircleOutlined /> },
  failed: { color: 'error', icon: <CloseCircleOutlined /> },
  timeout: { color: 'warning', icon: <ClockCircleOutlined /> },
};

interface DelegationHistoryProps {
  records: DelegationRecord[];
  loading: boolean;
  selectedIds: Set<string>;
  onSelectionChange: (ids: Set<string>) => void;
}

const DelegationHistory: React.FC<DelegationHistoryProps> = ({ records, loading, selectedIds, onSelectionChange }) => {
  if (loading) return <Spin />;
  if (records.length === 0) return <Empty description="暂无委托记录" />;

  const allSelected = records.length > 0 && records.every(r => selectedIds.has(r.id));
  const someSelected = records.some(r => selectedIds.has(r.id)) && !allSelected;

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      onSelectionChange(new Set(records.map(r => r.id)));
    } else {
      onSelectionChange(new Set());
    }
  };

  const handleToggle = (id: string, checked: boolean) => {
    const next = new Set(selectedIds);
    if (checked) {
      next.add(id);
    } else {
      next.delete(id);
    }
    onSelectionChange(next);
  };

  return (
    <div>
      <div style={{ marginBottom: 8, paddingLeft: 4 }}>
        <Checkbox
          indeterminate={someSelected}
          checked={allSelected}
          onChange={(e) => handleSelectAll(e.target.checked)}
        >
          全选
        </Checkbox>
      </div>
      <List
        dataSource={records}
        renderItem={(record) => {
        const st = statusConfig[record.status] || statusConfig.completed;
        let chain: string[] = [];
        try { chain = JSON.parse(record.chain); } catch { /* ignore */ }
        const duration = record.completed_at
          ? `${((new Date(record.completed_at).getTime() - new Date(record.started_at).getTime()) / 1000).toFixed(1)}s`
          : '进行中...';

        return (
          <List.Item>
            <div style={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
              <Checkbox
                checked={selectedIds.has(record.id)}
                onChange={(e) => handleToggle(record.id, e.target.checked)}
                style={{ marginRight: 12, marginTop: 4 }}
              />
              <div style={{ flex: 1, minWidth: 0 }}>
                <List.Item.Meta
                  avatar={<Badge status={st.color as 'processing' | 'success' | 'error' | 'warning'} />}
                  title={
                    <Space>
                      <Text strong>{record.agent_name}</Text>
                      <Tag color={st.color === 'error' ? 'red' : st.color === 'warning' ? 'orange' : st.color === 'success' ? 'green' : 'blue'}>
                        {record.status}
                      </Tag>
                      <Text type="secondary">深度 {record.depth}</Text>
                    </Space>
                  }
                  description={
                    <div>
                      <Paragraph ellipsis={{ rows: 1 }} style={{ marginBottom: 4 }}>
                        {record.prompt}
                      </Paragraph>
                      <Space split="·">
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          <ClockCircleOutlined /> {duration}
                        </Text>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          <ThunderboltOutlined /> {record.tokens_used.toLocaleString()} tokens
                        </Text>
                        {chain.length > 0 && (
                          <Text type="secondary" style={{ fontSize: 12 }}>
                            链路: {chain.join(' → ')}
                          </Text>
                        )}
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {new Date(record.started_at).toLocaleString()}
                        </Text>
                      </Space>
                      {record.error_message && (
                        <div style={{ marginTop: 4 }}>
                          <Text type="danger" style={{ fontSize: 12 }}>{record.error_message}</Text>
                        </div>
                      )}
                    </div>
                  }
                />
              </div>
            </div>
          </List.Item>
        );
      }}
    />
    </div>
  );
};

// ================================================================
// Main AgentsPage Component
// ================================================================

export const AgentsPage: React.FC = () => {
  const api = useAPI();
  const [agents, setAgents] = useState<Record<string, AgentConfig>>({});
  const [delegations, setDelegations] = useState<DelegationRecord[]>([]);
  const [modelsInfo, setModelsInfo] = useState<ModelsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [delegationsLoading, setDelegationsLoading] = useState(false);
  const [selectedDelegationIds, setSelectedDelegationIds] = useState<Set<string>>(new Set());
  const [modalVisible, setModalVisible] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [editingConfig, setEditingConfig] = useState<AgentConfig>({});

  const fetchAgents = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.getAgents?.();
      setAgents(data || {});
    } catch (error) {
      console.error('Failed to fetch agents:', error);
      message.error('获取代理列表失败');
    } finally {
      setLoading(false);
    }
  }, [api]);

  const fetchRecentDelegations = useCallback(async () => {
    // Use the global delegations endpoint to get recent records across all sessions
    if (!api.getDelegations) return;
    setDelegationsLoading(true);
    try {
      const records = await api.getDelegations(50);
      setDelegations(Array.isArray(records) ? records : []);
    } catch (error) {
      console.error('Failed to fetch delegations:', error);
    } finally {
      setDelegationsLoading(false);
    }
  }, [api]);

  const fetchModelsInfo = useCallback(async () => {
    try {
      const data = await api.getModels();
      setModelsInfo(data);
    } catch {
      // Models info is optional — don't block the page
    }
  }, [api]);

  useEffect(() => {
    fetchAgents();
    fetchRecentDelegations();
    fetchModelsInfo();
  }, [fetchAgents, fetchRecentDelegations, fetchModelsInfo]);

  const handleAdd = useCallback(() => {
    setEditingName(null);
    setEditingConfig({});
    setModalVisible(true);
  }, []);

  const handleEdit = useCallback((name: string, config: AgentConfig) => {
    setEditingName(name);
    setEditingConfig(config);
    setModalVisible(true);
  }, []);

  const handleDelete = useCallback(async (name: string) => {
    try {
      await api.deleteAgent?.(name);
      message.success(`已删除代理: ${name}`);
      fetchAgents();
    } catch (error) {
      console.error('Failed to delete agent:', error);
      message.error('删除代理失败');
    }
  }, [api, fetchAgents]);

  const handleToggleEnabled = useCallback(async (name: string, enabled: boolean) => {
    try {
      const existing = agents[name];
      if (!existing) return;
      await api.updateAgent?.(name, { ...existing, enabled });
      // Optimistic update
      setAgents((prev) => ({ ...prev, [name]: { ...prev[name], enabled } }));
      message.success(enabled ? `已启用代理: ${name}` : `已禁用代理: ${name}`);
    } catch (error) {
      console.error('Failed to toggle agent:', error);
      message.error('切换代理状态失败');
      fetchAgents(); // Revert on error
    }
  }, [api, agents, fetchAgents]);

  const handleBatchDeleteDelegations = useCallback(async () => {
    if (selectedDelegationIds.size === 0) return;
    try {
      const ids = Array.from(selectedDelegationIds);
      await api.batchDeleteDelegations?.(ids);
      message.success(`已删除 ${ids.length} 条委托记录`);
      setSelectedDelegationIds(new Set());
      fetchRecentDelegations();
    } catch (error) {
      console.error('Failed to batch delete delegations:', error);
      message.error('批量删除委托记录失败');
    }
  }, [api, selectedDelegationIds, fetchRecentDelegations]);

  const handleModalOk = useCallback(async (name: string, config: AgentConfig) => {
    try {
      if (editingName) {
        await api.updateAgent?.(name, config);
        message.success(`已更新代理: ${name}`);
      } else {
        await api.addAgent?.(name, config);
        message.success(`已创建代理: ${name}`);
      }
      setModalVisible(false);
      fetchAgents();
    } catch (error) {
      console.error('Failed to save agent:', error);
      message.error('保存代理失败');
    }
  }, [api, editingName, fetchAgents]);

  const { token } = theme.useToken();
  const [activeTab, setActiveTab] = useState('agents');

  const agentEntries = Object.entries(agents).sort(([a], [b]) => a.localeCompare(b));

  const tabItems = [
    {
      key: 'agents',
      label: (
        <span>
          <TeamOutlined /> 代理配置 ({agentEntries.length})
        </span>
      ),
    },
    {
      key: 'history',
      label: (
        <span>
          <ClockCircleOutlined /> 委托历史 ({delegations.length})
        </span>
      ),
    },
  ];

  const renderHeaderExtra = () => {
    if (activeTab === 'agents') {
      return (
        <Space>
          <Button
            icon={<PlusOutlined />} onClick={handleAdd}
            size="small" className="page-header-btn"
          >
            创建子代理
          </Button>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => { fetchAgents(); fetchRecentDelegations(); fetchModelsInfo(); }}
            loading={loading}
            size="small" className="page-header-btn"
          >
            刷新
          </Button>
        </Space>
      );
    }
    return (
      <Space>
        {selectedDelegationIds.size > 0 && (
          <Popconfirm
            title={`确定删除选中的 ${selectedDelegationIds.size} 条记录？`}
            onConfirm={handleBatchDeleteDelegations}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Button
              danger
              icon={<DeleteOutlined />}
              size="small"
              className="page-header-btn"
            >
              删除 ({selectedDelegationIds.size})
            </Button>
          </Popconfirm>
        )}
        <Button
          icon={<ReloadOutlined />}
          onClick={() => { fetchRecentDelegations(); }}
          loading={delegationsLoading}
          size="small" className="page-header-btn"
        >
          刷新
        </Button>
      </Space>
    );
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{
        padding: '12px 24px',
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
        background: token.colorBgContainer,
        flexShrink: 0,
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={tabItems}
            size="small"
            style={{ marginBottom: 0, minHeight: 0 }}
            tabBarStyle={{ marginBottom: 0 }}
          />
          {renderHeaderExtra()}
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: '16px 24px' }}>
        {activeTab === 'agents' && (
          <Spin spinning={loading}>
            {/* 主代理只读卡片 */}
            <div style={{ marginBottom: 16 }}>
              <List
                grid={{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 3, xxl: 4 }}
                dataSource={['primary']}
                style={{ maxWidth: '100%', overflow: 'hidden', padding: '0 8px' }}
                renderItem={() => (
                  <List.Item style={{ display: 'flex' }}>
                    <PrimaryAgentCard modelsInfo={modelsInfo} />
                  </List.Item>
                )}
              />
            </div>

            {/* 子代理列表 */}
            {agentEntries.length === 0 ? (
              <Empty
                description="暂未配置子代理"
                style={{ padding: '40px 0' }}
              >
                <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>
                  创建子代理
                </Button>
              </Empty>
            ) : (
              <List
                grid={{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 3, xxl: 4 }}
                dataSource={agentEntries}
                style={{ maxWidth: '100%', overflow: 'hidden', padding: '0 8px' }}
                renderItem={([name, config]) => (
                  <List.Item style={{ display: 'flex' }}>
                    <AgentCard
                      name={name}
                      config={config}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                      onToggleEnabled={handleToggleEnabled}
                    />
                  </List.Item>
                )}
              />
            )}
          </Spin>
        )}

        {activeTab === 'history' && (
          <DelegationHistory
            records={delegations}
            loading={delegationsLoading}
            selectedIds={selectedDelegationIds}
            onSelectionChange={setSelectedDelegationIds}
          />
        )}
      </div>

      <AgentEditModal
        visible={modalVisible}
        editingName={editingName}
        config={editingConfig}
        modelsInfo={modelsInfo}
        onOk={handleModalOk}
        onCancel={() => setModalVisible(false)}
      />
    </div>
  );
};

export default AgentsPage;
