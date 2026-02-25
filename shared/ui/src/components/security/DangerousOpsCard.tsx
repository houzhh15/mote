import React, { useState } from 'react';
import { Card, Table, Tag, Button, Modal, Form, Input, Select, Space, Popconfirm, Switch, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons';
import type { PolicyConfig } from '../../types/policy';

const { Option } = Select;

export interface DangerousOpsCardProps {
  policy: PolicyConfig;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

const severityColors: Record<string, string> = {
  low: 'blue',
  medium: 'orange',
  high: 'red',
  critical: 'magenta',
};

const actionColors: Record<string, string> = {
  block: 'red',
  approve: 'orange',
  warn: 'blue',
};

export const DangerousOpsCard: React.FC<DangerousOpsCardProps> = ({
  policy,
  onChange,
}) => {
  const [modalVisible, setModalVisible] = useState(false);
  const [editIndex, setEditIndex] = useState<number | null>(null);
  const [form] = Form.useForm();

  const handleAdd = () => {
    setEditIndex(null);
    form.resetFields();
    form.setFieldsValue({ tool: 'shell', severity: 'medium', action: 'approve' });
    setModalVisible(true);
  };

  const handleEdit = (index: number) => {
    setEditIndex(index);
    form.setFieldsValue(policy.dangerous_ops[index]);
    setModalVisible(true);
  };

  const handleDelete = (index: number) => {
    const newOps = [...policy.dangerous_ops];
    newOps.splice(index, 1);
    onChange({ dangerous_ops: newOps });
  };

  const handleToggleEnabled = (index: number, enabled: boolean) => {
    const newOps = [...policy.dangerous_ops];
    newOps[index] = { ...newOps[index], enabled };
    onChange({ dangerous_ops: newOps });
  };

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      const newOps = [...policy.dangerous_ops];
      if (editIndex !== null) {
        newOps[editIndex] = values;
      } else {
        newOps.push(values);
      }
      onChange({ dangerous_ops: newOps });
      setModalVisible(false);
    } catch {
      // validation failed
    }
  };

  const columns = [
    {
      title: '启用',
      key: 'enabled',
      width: 60,
      render: (_: unknown, record: { enabled?: boolean }, index: number) => (
        <Tooltip title={record.enabled !== false ? '点击禁用' : '点击启用'}>
          <Switch
            size="small"
            checked={record.enabled !== false}
            onChange={(checked) => handleToggleEnabled(index, checked)}
          />
        </Tooltip>
      ),
    },
    {
      title: '工具名',
      dataIndex: 'tool',
      key: 'tool',
      width: 100,
      render: (text: string, record: { enabled?: boolean }) => (
        <span style={{ opacity: record.enabled === false ? 0.45 : 1 }}>{text}</span>
      ),
    },
    {
      title: '匹配模式',
      dataIndex: 'pattern',
      key: 'pattern',
      render: (text: string, record: { enabled?: boolean }) => (
        <code style={{ fontSize: 12, opacity: record.enabled === false ? 0.45 : 1 }}>{text}</code>
      ),
    },
    {
      title: '严重级别',
      dataIndex: 'severity',
      key: 'severity',
      width: 100,
      render: (text: string, record: { enabled?: boolean }) => (
        <Tag color={record.enabled === false ? 'default' : (severityColors[text] || 'default')}>{text}</Tag>
      ),
    },
    {
      title: '动作',
      dataIndex: 'action',
      key: 'action',
      width: 100,
      render: (text: string, record: { enabled?: boolean }) => (
        <Tag color={record.enabled === false ? 'default' : (actionColors[text] || 'default')}>{text}</Tag>
      ),
    },
    {
      title: '描述',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
      render: (text: string, record: { enabled?: boolean }) => (
        <span style={{ opacity: record.enabled === false ? 0.45 : 1 }}>{text}</span>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_: unknown, __: unknown, index: number) => (
        <Space size="small">
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(index)} />
          <Popconfirm title="确定删除此规则？" onConfirm={() => handleDelete(index)} okText="确定" cancelText="取消">
            <Button type="link" size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="危险操作规则"
      size="small"
      style={{ marginBottom: 16 }}
      extra={<Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleAdd}>添加规则</Button>}
    >
      <Table
        dataSource={policy.dangerous_ops}
        columns={columns}
        rowKey={(_, index) => String(index)}
        size="small"
        pagination={false}
      />

      <Modal
        title={editIndex !== null ? '编辑规则' : '添加规则'}
        open={modalVisible}
        onOk={handleSave}
        onCancel={() => setModalVisible(false)}
        okText="保存"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="tool" label="工具名" rules={[{ required: true, message: '请输入工具名' }]}>
            <Input placeholder="例如: shell" />
          </Form.Item>
          <Form.Item name="pattern" label="匹配模式 (正则)" rules={[{ required: true, message: '请输入匹配模式' }]}>
            <Input placeholder="例如: curl\\s+.*\\|\\s*(ba)?sh" />
          </Form.Item>
          <Form.Item name="severity" label="严重级别" rules={[{ required: true }]}>
            <Select>
              <Option value="low">Low</Option>
              <Option value="medium">Medium</Option>
              <Option value="high">High</Option>
              <Option value="critical">Critical</Option>
            </Select>
          </Form.Item>
          <Form.Item name="action" label="动作" rules={[{ required: true }]}>
            <Select>
              <Option value="block">Block（阻止）</Option>
              <Option value="approve">Approve（需审批）</Option>
              <Option value="warn">Warn（警告）</Option>
            </Select>
          </Form.Item>
          <Form.Item name="message" label="描述">
            <Input placeholder="规则描述" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};
