// ================================================================
// CronPage - Shared cron jobs management page
// ================================================================

import React, { useState, useEffect, useRef } from 'react';
import { Typography, List, Card, Tag, Button, Space, Spin, Empty, message, Switch, Modal, Form, Input, Select, TimePicker, Radio, Checkbox, Tooltip, theme } from 'antd';
import { PlusOutlined, DeleteOutlined, ClockCircleOutlined, EditOutlined, GithubOutlined, QuestionCircleOutlined, MessageOutlined, LoadingOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useAPI } from '../context/APIContext';
import { OllamaIcon } from '../components/OllamaIcon';
import { MinimaxIcon } from '../components/MinimaxIcon';
import type { CronJob, CronExecutingJob, Model } from '../types';

const { Text } = Typography;

// 调度类型
type ScheduleType = 'every-minute' | 'hourly' | 'daily' | 'weekly' | 'monthly' | 'custom';

// 星期选项
const WEEKDAY_OPTIONS = [
  { label: '周一', value: 1 },
  { label: '周二', value: 2 },
  { label: '周三', value: 3 },
  { label: '周四', value: 4 },
  { label: '周五', value: 5 },
  { label: '周六', value: 6 },
  { label: '周日', value: 0 },
];

// 月份日期选项
const MONTH_DAY_OPTIONS = Array.from({ length: 31 }, (_, i) => ({
  label: `${i + 1}日`,
  value: i + 1,
}));

// 将 Cron 表达式解析为 UI 配置
const parseCronToUI = (cron: string): { type: ScheduleType; time?: dayjs.Dayjs; weekdays?: number[]; monthDay?: number } => {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return { type: 'custom' };

  const [minute, hour, dayOfMonth, month, dayOfWeek] = parts;

  // 每5分钟: */5 * * * *
  if (minute === '*/5' && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return { type: 'every-minute' };
  }

  // 每小时: N * * * *
  if (minute !== '*' && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return { type: 'hourly', time: dayjs().minute(parseInt(minute)).second(0) };
  }

  // 每天: N N * * *
  if (minute !== '*' && hour !== '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return { type: 'daily', time: dayjs().hour(parseInt(hour)).minute(parseInt(minute)).second(0) };
  }

  // 每周: N N * * N,N,N
  if (minute !== '*' && hour !== '*' && dayOfMonth === '*' && month === '*' && dayOfWeek !== '*') {
    const weekdays = dayOfWeek.split(',').map(d => parseInt(d));
    return { type: 'weekly', time: dayjs().hour(parseInt(hour)).minute(parseInt(minute)).second(0), weekdays };
  }

  // 每月: N N N * *
  if (minute !== '*' && hour !== '*' && dayOfMonth !== '*' && month === '*' && dayOfWeek === '*') {
    return { type: 'monthly', time: dayjs().hour(parseInt(hour)).minute(parseInt(minute)).second(0), monthDay: parseInt(dayOfMonth) };
  }

  return { type: 'custom' };
};

// 将 UI 配置转换为 Cron 表达式
const uiToCron = (type: ScheduleType, time?: dayjs.Dayjs, weekdays?: number[], monthDay?: number): string => {
  const minute = time ? time.minute() : 0;
  const hour = time ? time.hour() : 9;

  switch (type) {
    case 'every-minute':
      return '*/5 * * * *';
    case 'hourly':
      return `${minute} * * * *`;
    case 'daily':
      return `${minute} ${hour} * * *`;
    case 'weekly':
      return `${minute} ${hour} * * ${weekdays?.join(',') || '1'}`;
    case 'monthly':
      return `${minute} ${hour} ${monthDay || 1} * *`;
    default:
      return '0 9 * * *'; // 默认每天9点
  }
};

// 获取调度类型的描述
const getScheduleDescription = (type: ScheduleType, time?: dayjs.Dayjs, weekdays?: number[], monthDay?: number): string => {
  const timeStr = time ? time.format('HH:mm') : '09:00';
  switch (type) {
    case 'every-minute':
      return '每5分钟执行';
    case 'hourly':
      return `每小时的第 ${time?.minute() || 0} 分执行`;
    case 'daily':
      return `每天 ${timeStr} 执行`;
    case 'weekly':
      const dayNames = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
      const days = weekdays?.map(d => dayNames[d]).join('、') || '周一';
      return `每${days} ${timeStr} 执行`;
    case 'monthly':
      return `每月 ${monthDay || 1} 日 ${timeStr} 执行`;
    default:
      return '自定义';
  }
};

// Helper function to extract provider from model ID
// Ollama models have "ollama:" prefix (e.g., "ollama:llama3.2")
// Copilot models don't have prefix (e.g., "gpt-4", "claude-3.5-sonnet")
const getProviderFromModel = (model?: string): 'copilot' | 'ollama' | 'minimax' | null => {
  if (!model) return null;
  if (model.startsWith('ollama:')) return 'ollama';
  if (model.startsWith('minimax:')) return 'minimax';
  return 'copilot';
};

interface CronPageProps {
  onSelectSession?: (sessionId: string) => void;
}

export const CronPage: React.FC<CronPageProps> = ({ onSelectSession }) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingJob, setEditingJob] = useState<CronJob | null>(null);
  const [models, setModels] = useState<Model[]>([]);
  const [executingJobs, setExecutingJobs] = useState<Map<string, CronExecutingJob>>(new Map());
  const [form] = Form.useForm();
  const executingPollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchJobs = async () => {
    setLoading(true);
    try {
      const data = await api.getCronJobs();
      setJobs(data);
    } catch (error) {
      console.error('Failed to fetch cron jobs:', error);
      message.error('获取定时任务失败');
    } finally {
      setLoading(false);
    }
  };

  const loadModels = async () => {
    try {
      const response = await api.getModels();
      setModels(response.models || []);
    } catch (error) {
      console.error('Failed to load models:', error);
    }
  };

  const fetchExecuting = async () => {
    if (!api.getCronExecuting) return;
    try {
      const executing = await api.getCronExecuting();
      const map = new Map<string, CronExecutingJob>();
      for (const ej of executing) {
        map.set(ej.name, ej);
      }
      setExecutingJobs(map);
    } catch {
      // silently ignore polling errors
    }
  };

  const toggleJob = async (id: string, enabled: boolean) => {
    try {
      await api.toggleCronJob(id, enabled);
      message.success(enabled ? '已启用' : '已禁用');
      fetchJobs();
    } catch (error) {
      console.error('Failed to toggle job:', error);
      message.error('操作失败');
    }
  };

  const deleteJob = async (id: string, name: string) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除定时任务 "${name}" 吗？`,
      onOk: async () => {
        try {
          await api.deleteCronJob(id);
          message.success('删除成功');
          fetchJobs();
        } catch (error) {
          console.error('Failed to delete job:', error);
          message.error('删除失败');
        }
      },
    });
  };

  const openAddModal = () => {
    setEditingJob(null);
    form.resetFields();
    setModalVisible(true);
  };

  const openEditModal = (job: CronJob) => {
    setEditingJob(job);
    
    // 解析现有的 cron 表达式
    const parsed = parseCronToUI(job.schedule);
    
    form.setFieldsValue({
      name: job.name,
      scheduleType: parsed.type,
      scheduleTime: parsed.time || dayjs().hour(9).minute(0),
      scheduleWeekdays: parsed.weekdays || [1],
      scheduleMonthDay: parsed.monthDay || 1,
      scheduleMinute: parsed.time?.minute() || 0,
      schedule: parsed.type === 'custom' ? job.schedule : undefined,
      prompt: job.prompt,
      model: job.model || undefined,
      workspace_path: job.workspace_path || undefined,
      workspace_alias: job.workspace_alias || undefined,
    });
    setModalVisible(true);
  };

  const handleModalOk = async (values: Record<string, unknown>) => {
    try {
      // 根据 scheduleType 构建 cron 表达式
      let cronSchedule: string;
      const scheduleType = values.scheduleType as ScheduleType;
      
      if (scheduleType === 'custom') {
        cronSchedule = values.schedule as string;
      } else {
        const time = values.scheduleTime as dayjs.Dayjs | undefined;
        const weekdays = values.scheduleWeekdays as number[] | undefined;
        const monthDay = values.scheduleMonthDay as number | undefined;
        const minute = values.scheduleMinute as number | undefined;
        
        // 对于 hourly 类型，构建只包含分钟的 time
        const effectiveTime = scheduleType === 'hourly' 
          ? dayjs().minute(minute || 0) 
          : time;
        
        cronSchedule = uiToCron(scheduleType, effectiveTime, weekdays, monthDay);
      }
      
      const jobData: Partial<CronJob> = {
        name: values.name as string,
        schedule: cronSchedule,
        prompt: values.prompt as string,
        model: values.model as string | undefined,
        workspace_path: (values.workspace_path as string) || undefined,
        workspace_alias: (values.workspace_alias as string) || undefined,
      };
      
      if (editingJob) {
        // 编辑模式：更新现有任务
        await api.updateCronJob(editingJob.name, {
          schedule: cronSchedule,
          prompt: values.prompt as string,
          model: values.model as string | undefined,
          workspace_path: (values.workspace_path as string) || undefined,
          workspace_alias: (values.workspace_alias as string) || undefined,
        });
        message.success('更新成功');
      } else {
        // 新建模式
        await api.createCronJob(jobData);
        message.success('创建成功');
      }
      setModalVisible(false);
      form.resetFields();
      setEditingJob(null);
      fetchJobs();
    } catch (error) {
      console.error('Failed to save job:', error);
      message.error(editingJob ? '更新失败' : '创建失败');
    }
  };

  const handleModalCancel = () => {
    setModalVisible(false);
    form.resetFields();
    setEditingJob(null);
  };

  // Group models by provider for OptGroup display
  const getModelsByProvider = () => {
    const grouped: Record<string, Model[]> = {};
    models.forEach(model => {
      const provider = model.provider || 'copilot';
      if (!grouped[provider]) {
        grouped[provider] = [];
      }
      grouped[provider].push(model);
    });
    return grouped;
  };

  useEffect(() => {
    fetchJobs();
    loadModels();
    fetchExecuting();
    // Poll executing status every 3 seconds
    executingPollRef.current = setInterval(fetchExecuting, 3000);
    return () => {
      if (executingPollRef.current) clearInterval(executingPollRef.current);
    };
  }, []);

  const grouped = getModelsByProvider();
  const providers = Object.keys(grouped);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Button icon={<PlusOutlined />} onClick={openAddModal} className="page-header-btn">
            添加任务
          </Button>
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ maxWidth: 900 }}>
        <Spin spinning={loading}>
        {jobs.length === 0 ? (
          <Empty description="暂无定时任务" />
        ) : (
          <List
            dataSource={jobs}
            renderItem={(job) => {
              const executing = executingJobs.get(job.name);
              const sessionId = job.session_id || `cron-${job.name}`;
              return (
              <Card size="small" style={{ marginBottom: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                    <div style={{ marginBottom: 8, display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 8 }}>
                      <Text strong style={{ fontSize: 13 }}>{job.name}</Text>
                      {executing ? (
                        <Tag color="processing" icon={<LoadingOutlined />} style={{ fontSize: 11 }}>
                          执行中 ({executing.running_for}s)
                        </Tag>
                      ) : (
                        <Tag color={job.enabled ? 'green' : 'default'} style={{ fontSize: 11 }}>
                          {job.enabled ? '已启用' : '已停止'}
                        </Tag>
                      )}
                      {job.model && (
                        <Tag 
                          color={getProviderFromModel(job.model) === 'ollama' ? 'orange' : getProviderFromModel(job.model) === 'minimax' ? 'purple' : 'blue'}
                          style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11 }}
                        >
                          {getProviderFromModel(job.model) === 'ollama' ? <OllamaIcon size={10} /> : getProviderFromModel(job.model) === 'minimax' ? <MinimaxIcon size={10} /> : <GithubOutlined style={{ fontSize: 10 }} />}
                          {job.model}
                        </Tag>
                      )}
                    </div>
                    <div style={{ marginBottom: 6 }}>
                      <ClockCircleOutlined style={{ marginRight: 8, fontSize: 12 }} />
                      <Tooltip title={`Cron: ${job.schedule}`}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {(() => {
                            const parsed = parseCronToUI(job.schedule);
                            return getScheduleDescription(parsed.type, parsed.time, parsed.weekdays, parsed.monthDay);
                          })()}
                        </Text>
                      </Tooltip>
                    </div>
                    <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      <Text style={{ color: '#666', fontSize: 12 }}>{job.prompt}</Text>
                    </div>
                    {job.workspace_path && (
                      <div style={{ marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        <Text type="secondary" style={{ fontSize: 11 }}>
                          📂 {job.workspace_alias ? `${job.workspace_alias} (${job.workspace_path})` : job.workspace_path}
                        </Text>
                      </div>
                    )}
                    {job.next_run && (
                      <div style={{ marginTop: 6 }}>
                        <Text type="secondary" style={{ fontSize: 11 }}>
                          下次执行: {new Date(job.next_run).toLocaleString()}
                        </Text>
                      </div>
                    )}
                  </div>
                  <Space size={4}>
                    {onSelectSession && (
                      <Tooltip title="查看会话记录">
                        <Button
                          type="text"
                          size="small"
                          icon={<MessageOutlined />}
                          onClick={() => onSelectSession(sessionId)}
                        />
                      </Tooltip>
                    )}
                    <Switch
                      size="small"
                      checked={job.enabled}
                      onChange={(checked) => toggleJob(job.name, checked)}
                    />
                    <Button
                      type="text"
                      size="small"
                      icon={<EditOutlined />}
                      onClick={() => openEditModal(job)}
                    />
                    <Button
                      type="text"
                      size="small"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => deleteJob(job.name, job.name)}
                    />
                  </Space>
                </div>
              </Card>
              );
            }}
          />
        )}
      </Spin>
        </div>
      </div>

      <Modal
        title={editingJob ? '编辑定时任务' : '添加定时任务'}
        open={modalVisible}
        onCancel={handleModalCancel}
        onOk={() => form.submit()}
        width={520}
      >
        <Form form={form} layout="vertical" onFinish={handleModalOk}>
          <Form.Item name="name" label="任务名称" rules={[{ required: true, message: '请输入任务名称' }]}>
            <Input placeholder="例如: 每日总结" disabled={!!editingJob} />
          </Form.Item>
          
          <Form.Item 
            label={
              <Space>
                执行频率
                <Tooltip title="设置任务的执行时间规则">
                  <QuestionCircleOutlined style={{ color: '#999' }} />
                </Tooltip>
              </Space>
            }
          >
            <Form.Item name="scheduleType" noStyle initialValue="daily">
              <Radio.Group style={{ marginBottom: 12 }}>
                <Radio.Button value="every-minute">每5分钟</Radio.Button>
                <Radio.Button value="hourly">每小时</Radio.Button>
                <Radio.Button value="daily">每天</Radio.Button>
                <Radio.Button value="weekly">每周</Radio.Button>
                <Radio.Button value="monthly">每月</Radio.Button>
                <Radio.Button value="custom">自定义</Radio.Button>
              </Radio.Group>
            </Form.Item>
            
            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.scheduleType !== cur.scheduleType}>
              {({ getFieldValue }) => {
                const scheduleType = getFieldValue('scheduleType') as ScheduleType;
                
                if (scheduleType === 'every-minute') {
                  return (
                    <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
                      任务将每5分钟执行一次
                    </Text>
                  );
                }
                
                if (scheduleType === 'hourly') {
                  return (
                    <Form.Item name="scheduleMinute" label="在每小时的第几分钟" initialValue={0}>
                      <Select style={{ width: 120 }}>
                        {Array.from({ length: 60 }, (_, i) => (
                          <Select.Option key={i} value={i}>{i} 分</Select.Option>
                        ))}
                      </Select>
                    </Form.Item>
                  );
                }
                
                if (scheduleType === 'daily') {
                  return (
                    <Form.Item name="scheduleTime" label="执行时间" initialValue={dayjs().hour(9).minute(0)}>
                      <TimePicker format="HH:mm" style={{ width: 120 }} />
                    </Form.Item>
                  );
                }
                
                if (scheduleType === 'weekly') {
                  return (
                    <>
                      <Form.Item name="scheduleWeekdays" label="选择星期" initialValue={[1]} rules={[{ required: true, message: '请至少选择一天' }]}>
                        <Checkbox.Group options={WEEKDAY_OPTIONS} />
                      </Form.Item>
                      <Form.Item name="scheduleTime" label="执行时间" initialValue={dayjs().hour(9).minute(0)}>
                        <TimePicker format="HH:mm" style={{ width: 120 }} />
                      </Form.Item>
                    </>
                  );
                }
                
                if (scheduleType === 'monthly') {
                  return (
                    <>
                      <Form.Item name="scheduleMonthDay" label="每月第几天" initialValue={1}>
                        <Select style={{ width: 120 }}>
                          {MONTH_DAY_OPTIONS.map(opt => (
                            <Select.Option key={opt.value} value={opt.value}>{opt.label}</Select.Option>
                          ))}
                        </Select>
                      </Form.Item>
                      <Form.Item name="scheduleTime" label="执行时间" initialValue={dayjs().hour(9).minute(0)}>
                        <TimePicker format="HH:mm" style={{ width: 120 }} />
                      </Form.Item>
                    </>
                  );
                }
                
                if (scheduleType === 'custom') {
                  return (
                    <Form.Item 
                      name="schedule" 
                      label={
                        <Space>
                          Cron 表达式
                          <Tooltip title="格式: 分 时 日 月 周 (例如: 0 9 * * * 表示每天9点)">
                            <QuestionCircleOutlined style={{ color: '#999' }} />
                          </Tooltip>
                        </Space>
                      }
                      rules={[{ required: true, message: '请输入 Cron 表达式' }]}
                    >
                      <Input placeholder="例如: 0 9 * * *" />
                    </Form.Item>
                  );
                }
                
                return null;
              }}
            </Form.Item>
          </Form.Item>

          <Form.Item name="model" label="使用模型">
            <Select placeholder="选择模型（可选，默认使用场景模型）" allowClear>
              {providers.length <= 1 ? (
                models.map((model) => (
                  <Select.Option key={model.id} value={model.id} disabled={model.available === false}>
                    <Space>
                      {model.provider === 'ollama' ? <OllamaIcon size={12} /> : model.provider === 'minimax' ? <MinimaxIcon size={12} /> : <GithubOutlined style={{ fontSize: 12 }} />}
                      {model.display_name}
                    </Space>
                  </Select.Option>
                ))
              ) : (
                providers.map(provider => (
                  <Select.OptGroup 
                    key={provider} 
                    label={
                      <Space>
                        {provider === 'ollama' ? <OllamaIcon /> : provider === 'minimax' ? <MinimaxIcon size={14} /> : <GithubOutlined />}
                        {provider === 'ollama' ? 'Ollama' : provider === 'minimax' ? 'MiniMax' : provider === 'copilot-acp' ? 'Copilot ACP' : 'GitHub Copilot'}
                      </Space>
                    }
                  >
                    {grouped[provider].map(model => (
                      <Select.Option key={model.id} value={model.id} disabled={model.available === false}>
                        {model.display_name}
                      </Select.Option>
                    ))}
                  </Select.OptGroup>
                ))
              )}
            </Select>
          </Form.Item>
          <Form.Item name="workspace_path" label="工作目录" tooltip="设置后，AI 将在此目录上下文中执行任务">
            <Input placeholder="例如: /Users/me/project（可选）" />
          </Form.Item>
          <Form.Item name="workspace_alias" label="目录别名" tooltip="可选，为工作目录设置一个友好名称">
            <Input placeholder="例如: my-project（可选）" />
          </Form.Item>
          <Form.Item name="prompt" label="执行提示词" rules={[{ required: true, message: '请输入执行提示词' }]}>
            <Input.TextArea rows={3} placeholder="AI 将执行的任务描述" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};
