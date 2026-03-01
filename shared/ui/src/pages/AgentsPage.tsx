// ================================================================
// AgentsPage - Multi-Agent Delegate management page
// ================================================================

import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Typography, Card, Tag, Spin, Empty, message, Button, Space, Modal,
  Input, Select, InputNumber, List, Badge, Popconfirm, Tabs, Tooltip, theme, Switch, Checkbox, Radio, Alert,
} from 'antd';
import {
  TeamOutlined, PlusOutlined, ReloadOutlined, DeleteOutlined,
  EditOutlined, ClockCircleOutlined, CheckCircleOutlined,
  CloseCircleOutlined, LoadingOutlined, ThunderboltOutlined,
  CrownOutlined, InfoCircleOutlined, SaveOutlined, BranchesOutlined,
  FolderOutlined, TagOutlined, RightOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { AgentConfig, DelegationRecord, ModelsResponse, Model, Step, ValidationResult } from '../types';
import { StepEditor } from '../components/StepEditor';
import { CFGPreview } from '../components/CFGPreview';
import OrchestrationView from './OrchestrationView';

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
          {config.entry_point && <Tag color="gold">入口</Tag>}
          {config.stealth && <Tag color="default">隐身</Tag>}
        </Space>
        {config.tags && config.tags.length > 0 && (
          <div style={{ marginTop: 6 }}>
            <Space wrap size={[2, 2]}>
              {config.tags.map(tag => (
                <Tag key={tag} icon={<TagOutlined />} style={{ fontSize: 11, margin: 0 }}>{tag}</Tag>
              ))}
            </Space>
          </div>
        )}
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
  agents: Record<string, AgentConfig>;
  onOk: (name: string, config: AgentConfig) => void;
  onCancel: () => void;
  /** Called when user clicks "+ 新建" inside a step editor agent_ref */
  onCreateAgent?: () => void;
}

const AgentEditModal: React.FC<AgentEditModalProps> = ({ visible, editingName, config, modelsInfo, agents, onOk, onCancel, onCreateAgent }) => {
  const api = useAPI();
  const [name, setName] = useState('');
  const [agentConfig, setAgentConfig] = useState<AgentConfig>({});
  const [mode, setMode] = useState<'simple' | 'structured'>('simple');
  const [steps, setSteps] = useState<Step[]>([]);
  const [maxRecursion, setMaxRecursion] = useState(5);
  const [validationResults, setValidationResults] = useState<ValidationResult[]>();
  const [draftPromptDismissed, setDraftPromptDismissed] = useState(false);

  const agentName = editingName || name.trim();
  const availableAgents = useMemo(
    () => Object.entries(agents).map(([n, cfg]) => ({ name: n, tags: cfg.tags })),
    [agents],
  );

  // Detect self-referencing routes → show recursion config
  const hasSelfRoute = useMemo(
    () =>
      steps.some(
        s => s.type === 'route' && s.branches && Object.values(s.branches).includes(agentName),
      ),
    [steps, agentName],
  );

  useEffect(() => {
    if (visible) {
      setName(editingName || '');
      setAgentConfig({ ...config });
      setDraftPromptDismissed(false);
      if (config.steps && config.steps.length > 0) {
        setMode('structured');
        setSteps([...config.steps]);
        setMaxRecursion(config.max_recursion ?? 5);
      } else {
        setMode('simple');
        setSteps([]);
        setMaxRecursion(5);
      }
      setValidationResults(undefined);
    }
  }, [visible, editingName, config]);

  const handleRestoreDraft = useCallback(() => {
    if (config.draft?.steps) {
      setSteps([...config.draft.steps]);
      setMode('structured');
      message.success('已恢复草稿');
    }
    setDraftPromptDismissed(true);
  }, [config]);

  const handleSaveDraft = useCallback(async () => {
    if (!agentName) {
      message.warning('请先输入代理名称');
      return;
    }
    try {
      await api.saveAgentDraft?.(agentName, { steps });
      message.success('草稿已保存');
    } catch {
      message.error('保存草稿失败');
    }
  }, [api, agentName, steps]);

  const handleValidate = useCallback(async () => {
    if (!agentName) return;
    try {
      const results = await api.validateAgentCFG?.(agentName, steps);
      setValidationResults(results || []);
    } catch {
      message.error('验证失败');
    }
  }, [api, agentName, steps]);

  const handleOk = () => {
    const n = editingName || name.trim();
    if (!n) {
      message.warning('请输入代理名称');
      return;
    }
    const finalConfig: AgentConfig = { ...agentConfig };
    if (mode === 'structured' && steps.length > 0) {
      finalConfig.steps = steps;
      finalConfig.max_recursion = hasSelfRoute ? maxRecursion : undefined;
    } else {
      delete finalConfig.steps;
      delete finalConfig.max_recursion;
    }
    onOk(n, finalConfig);
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
      width={mode === 'structured' ? 720 : 560}
      footer={(_, { OkBtn, CancelBtn }) => (
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <Space>
            {mode === 'structured' && steps.length > 0 && (
              <Button icon={<SaveOutlined />} onClick={handleSaveDraft}>保存草稿</Button>
            )}
          </Space>
          <Space>
            <CancelBtn />
            <OkBtn />
          </Space>
        </div>
      )}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 16 }}>
        {/* Draft restore prompt */}
        {config.draft?.steps && !draftPromptDismissed && (
          <Alert
            type="info"
            showIcon
            message={`存在未保存的草稿（${new Date(config.draft.saved_at).toLocaleString()}）`}
            action={
              <Space>
                <Button size="small" type="primary" onClick={handleRestoreDraft}>恢复</Button>
                <Button size="small" onClick={() => setDraftPromptDismissed(true)}>忽略</Button>
              </Space>
            }
          />
        )}

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
          <Text strong>标签</Text>
          <Select
            mode="tags"
            placeholder="输入标签名称回车添加，或从已有标签中选择"
            value={agentConfig.tags || []}
            onChange={v => update('tags', v)}
            style={{ width: '100%', marginTop: 4 }}
            options={Array.from(new Set(
              Object.values(agents).flatMap(a => a.tags || [])
            )).sort().map(t => ({ label: t, value: t }))}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            标签用于分组管理代理，类似文件夹
          </Text>
        </div>
        <div style={{ display: 'flex', gap: 24 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Switch
              size="small"
              checked={agentConfig.entry_point || false}
              onChange={v => update('entry_point', v)}
            />
            <Text>入口</Text>
            <Tooltip title="入口 Agent 在 @ 引用中优先显示，通常是面向用户的交互入口">
              <InfoCircleOutlined style={{ color: '#999', fontSize: 14 }} />
            </Tooltip>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Switch
              size="small"
              checked={agentConfig.stealth || false}
              onChange={v => update('stealth', v)}
            />
            <Text>隐身</Text>
            <Tooltip title="隐身 Agent 的信息不会注入到主 Agent 的系统提示词中，但仍可通过 delegate 调用">
              <InfoCircleOutlined style={{ color: '#999', fontSize: 14 }} />
            </Tooltip>
          </div>
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

        {/* ---- Orchestration mode toggle ---- */}
        <div style={{ borderTop: '1px solid #f0f0f0', paddingTop: 12 }}>
          <Text strong>编排模式</Text>
          <div style={{ marginTop: 4 }}>
            <Radio.Group value={mode} onChange={e => setMode(e.target.value)} optionType="button" buttonStyle="solid">
              <Radio.Button value="simple">简单模式</Radio.Button>
              <Radio.Button value="structured">
                <BranchesOutlined /> 结构化编排
              </Radio.Button>
            </Radio.Group>
          </div>
        </div>

        {mode === 'structured' && (
          <>
            <Tabs
              size="small"
              items={[
                {
                  key: 'steps',
                  label: '编排步骤',
                  children: (
                    <StepEditor
                      steps={steps}
                      onChange={setSteps}
                      availableAgents={availableAgents}
                      onCreateAgent={onCreateAgent}
                    />
                  ),
                },
                {
                  key: 'preview',
                  label: 'CFG 预览',
                  children: (
                    <CFGPreview
                      steps={steps}
                      agentName={agentName || '(未命名)'}
                      validationResults={validationResults}
                      onValidate={handleValidate}
                    />
                  ),
                },
              ]}
            />

            {hasSelfRoute && (
              <div style={{ background: '#fffbe6', padding: '8px 12px', borderRadius: 6 }}>
                <Space>
                  <Text strong>自引用路由检测</Text>
                  <Text type="secondary">最大递归次数</Text>
                  <InputNumber
                    min={1} max={100}
                    value={maxRecursion}
                    onChange={v => setMaxRecursion(v ?? 5)}
                    style={{ width: 80 }}
                  />
                </Space>
              </div>
            )}
          </>
        )}
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
  const [activeTagFolder, setActiveTagFolder] = useState<string | null>(null);

  const agentEntries = Object.entries(agents).sort(([a], [b]) => a.localeCompare(b));

  // Compute tag-based folder groups
  const { tagGroups, othersEntries, allTags } = useMemo(() => {
    const groups: Record<string, [string, AgentConfig][]> = {};
    const others: [string, AgentConfig][] = [];
    const tagsSet = new Set<string>();

    for (const [name, cfg] of agentEntries) {
      const tags = cfg.tags;
      if (tags && tags.length > 0) {
        for (const tag of tags) {
          tagsSet.add(tag);
          if (!groups[tag]) groups[tag] = [];
          groups[tag].push([name, cfg]);
        }
      } else {
        others.push([name, cfg]);
      }
    }
    return {
      tagGroups: groups,
      othersEntries: others,
      allTags: Array.from(tagsSet).sort(),
    };
  }, [agentEntries]);

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
      key: 'orchestration',
      label: (
        <span>
          <BranchesOutlined /> 编排视图
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

  const handleReloadAgents = useCallback(async () => {
    setLoading(true);
    try {
      if (api.reloadAgents) {
        const result = await api.reloadAgents();
        message.success(`已重新加载 ${result.count} 个代理配置`);
      }
      await fetchAgents();
      fetchRecentDelegations();
      fetchModelsInfo();
    } catch (error) {
      console.error('Failed to reload agents:', error);
      message.error('重新加载代理配置失败');
    } finally {
      setLoading(false);
    }
  }, [api, fetchAgents, fetchRecentDelegations, fetchModelsInfo]);

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
            onClick={handleReloadAgents}
            loading={loading}
            size="small" className="page-header-btn"
          >
            重新加载
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

            {/* 子代理列表 - 标签文件夹模式 */}
            {agentEntries.length === 0 ? (
              <Empty
                description="暂未配置子代理"
                style={{ padding: '40px 0' }}
              >
                <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>
                  创建子代理
                </Button>
              </Empty>
            ) : activeTagFolder !== null ? (
              /* ---- 在标签文件夹内部 ---- */
              <div>
                <Button
                  type="link"
                  size="small"
                  onClick={() => setActiveTagFolder(null)}
                  style={{ marginBottom: 12, padding: 0 }}
                >
                  ← 返回
                </Button>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16,
                  fontSize: 16, fontWeight: 600,
                }}>
                  <FolderOutlined />
                  {activeTagFolder === '__others__' ? 'Others' : activeTagFolder}
                  <Tag style={{ marginLeft: 4 }}>
                    {activeTagFolder === '__others__'
                      ? othersEntries.length
                      : (tagGroups[activeTagFolder] || []).length} 个代理
                  </Tag>
                </div>
                <List
                  grid={{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 3, xxl: 4 }}
                  dataSource={
                    activeTagFolder === '__others__'
                      ? othersEntries
                      : (tagGroups[activeTagFolder] || [])
                  }
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
              </div>
            ) : allTags.length === 0 ? (
              /* ---- 没有任何标签时只显示 Others 文件夹 ---- */
              <div>
                <div
                  onClick={() => setActiveTagFolder('__others__')}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 12, padding: '14px 18px',
                    borderRadius: 10, cursor: 'pointer', marginBottom: 8,
                    border: `1px solid ${token.colorBorderSecondary}`,
                    background: token.colorBgContainer,
                    transition: 'all 0.2s',
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = token.colorBgLayout; e.currentTarget.style.borderColor = token.colorPrimary; }}
                  onMouseLeave={e => { e.currentTarget.style.background = token.colorBgContainer; e.currentTarget.style.borderColor = token.colorBorderSecondary; }}
                >
                  <FolderOutlined style={{ fontSize: 22, color: token.colorTextSecondary }} />
                  <div style={{ flex: 1 }}>
                    <Text strong style={{ fontSize: 14 }}>Others</Text>
                    <Text type="secondary" style={{ marginLeft: 6 }}>({agentEntries.length})</Text>
                  </div>
                  <RightOutlined style={{ color: token.colorTextQuaternary }} />
                </div>
              </div>
            ) : (
              /* ---- 有标签时显示标签文件夹 + Others ---- */
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {allTags.map(tag => {
                  const count = tagGroups[tag]?.length || 0;
                  return (
                    <div
                      key={tag}
                      onClick={() => setActiveTagFolder(tag)}
                      style={{
                        display: 'flex', alignItems: 'center', gap: 12, padding: '14px 18px',
                        borderRadius: 10, cursor: 'pointer',
                        border: `1px solid ${token.colorBorderSecondary}`,
                        background: token.colorBgContainer,
                        transition: 'all 0.2s',
                      }}
                      onMouseEnter={e => { e.currentTarget.style.background = token.colorBgLayout; e.currentTarget.style.borderColor = token.colorPrimary; }}
                      onMouseLeave={e => { e.currentTarget.style.background = token.colorBgContainer; e.currentTarget.style.borderColor = token.colorBorderSecondary; }}
                    >
                      <FolderOutlined style={{ fontSize: 22, color: '#1890ff' }} />
                      <div style={{ flex: 1 }}>
                        <Text strong style={{ fontSize: 14 }}>{tag}</Text>
                        <Text type="secondary" style={{ marginLeft: 6 }}>({count})</Text>
                      </div>
                      <RightOutlined style={{ color: token.colorTextQuaternary }} />
                    </div>
                  );
                })}
                {/* Others 文件夹 (无标签的代理) */}
                <div
                  onClick={() => setActiveTagFolder('__others__')}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 12, padding: '14px 18px',
                    borderRadius: 10, cursor: 'pointer',
                    border: `1px solid ${token.colorBorderSecondary}`,
                    background: token.colorBgContainer,
                    transition: 'all 0.2s',
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = token.colorBgLayout; e.currentTarget.style.borderColor = token.colorPrimary; }}
                  onMouseLeave={e => { e.currentTarget.style.background = token.colorBgContainer; e.currentTarget.style.borderColor = token.colorBorderSecondary; }}
                >
                  <FolderOutlined style={{ fontSize: 22, color: token.colorTextSecondary }} />
                  <div style={{ flex: 1 }}>
                    <Text strong style={{ fontSize: 14 }}>Others</Text>
                    <Text type="secondary" style={{ marginLeft: 6 }}>({othersEntries.length})</Text>
                  </div>
                  <RightOutlined style={{ color: token.colorTextQuaternary }} />
                </div>
              </div>
            )}
          </Spin>
        )}

        {activeTab === 'orchestration' && (
          <OrchestrationView agents={agents} onEditAgent={handleEdit} />
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
        agents={agents}
        onOk={handleModalOk}
        onCancel={() => setModalVisible(false)}
        onCreateAgent={handleAdd}
      />
    </div>
  );
};

export default AgentsPage;
