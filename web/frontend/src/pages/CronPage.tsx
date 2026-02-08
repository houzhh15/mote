import React, { useState, useEffect } from 'react';
import { Typography, List, Card, Tag, Button, Space, Spin, Empty, message, Switch, Modal, Form, Input, Select } from 'antd';
import { PlusOutlined, PlayCircleOutlined, PauseCircleOutlined, DeleteOutlined, ClockCircleOutlined } from '@ant-design/icons';

const { Title, Text } = Typography;

interface CronJob {
  id: string;
  name: string;
  schedule: string;
  prompt: string;
  enabled: boolean;
  next_run?: string;
  last_run?: string;
}

export const CronPage: React.FC = () => {
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();

  const fetchJobs = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/cron');
      const data = await response.json();
      setJobs(data.jobs || []);
    } catch (error) {
      console.error('Failed to fetch cron jobs:', error);
      message.error('获取定时任务失败');
    } finally {
      setLoading(false);
    }
  };

  const toggleJob = async (id: string, enabled: boolean) => {
    try {
      await fetch(`/api/v1/cron/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      });
      message.success(enabled ? '已启用' : '已禁用');
      fetchJobs();
    } catch (error) {
      console.error('Failed to toggle job:', error);
      message.error('操作失败');
    }
  };

  const deleteJob = async (id: string) => {
    try {
      await fetch(`/api/v1/cron/${id}`, { method: 'DELETE' });
      message.success('删除成功');
      fetchJobs();
    } catch (error) {
      console.error('Failed to delete job:', error);
      message.error('删除失败');
    }
  };

  const createJob = async (values: Partial<CronJob>) => {
    try {
      await fetch('/api/v1/cron', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      message.success('创建成功');
      setModalVisible(false);
      form.resetFields();
      fetchJobs();
    } catch (error) {
      console.error('Failed to create job:', error);
      message.error('创建失败');
    }
  };

  useEffect(() => {
    fetchJobs();
  }, []);

  return (
    <div className="page-container">
      <div className="page-header">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Title level={4}>定时任务</Title>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
            添加任务
          </Button>
        </div>
      </div>

      <Spin spinning={loading}>
        {jobs.length === 0 ? (
          <Empty description="暂无定时任务" />
        ) : (
          <List
            dataSource={jobs}
            renderItem={(job) => (
              <Card size="small" style={{ marginBottom: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ flex: 1 }}>
                    <div style={{ marginBottom: 8 }}>
                      <Text strong style={{ fontSize: 16 }}>{job.name}</Text>
                      <Tag color={job.enabled ? 'green' : 'default'} style={{ marginLeft: 8 }}>
                        {job.enabled ? '运行中' : '已停止'}
                      </Tag>
                    </div>
                    <div style={{ marginBottom: 8 }}>
                      <ClockCircleOutlined style={{ marginRight: 8 }} />
                      <Text type="secondary">{job.schedule}</Text>
                    </div>
                    <Text ellipsis style={{ color: '#666' }}>{job.prompt}</Text>
                    {job.next_run && (
                      <div style={{ marginTop: 8 }}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          下次执行: {new Date(job.next_run).toLocaleString()}
                        </Text>
                      </div>
                    )}
                  </div>
                  <Space>
                    <Switch
                      checked={job.enabled}
                      onChange={(checked) => toggleJob(job.id, checked)}
                    />
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => deleteJob(job.id)}
                    />
                  </Space>
                </div>
              </Card>
            )}
          />
        )}
      </Spin>

      <Modal
        title="添加定时任务"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={createJob}>
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
            <Input placeholder="例如: 每日总结" />
          </Form.Item>
          <Form.Item name="schedule" label="Cron 表达式" rules={[{ required: true }]}>
            <Input placeholder="例如: 0 9 * * * (每天9点)" />
          </Form.Item>
          <Form.Item name="prompt" label="执行提示词" rules={[{ required: true }]}>
            <Input.TextArea rows={3} placeholder="AI 将执行的任务描述" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};
