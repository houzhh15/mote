import React, { useState } from 'react';
import { Card, Table, Button, Modal, Form, Input, Switch, Popconfirm, Typography, Space, Tag } from 'antd';
import { PlusOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons';
import type { PolicyConfig, ScrubRule } from '../../types/policy';

const { Text } = Typography;

export interface ScrubRulesCardProps {
  policy: PolicyConfig;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

export const ScrubRulesCard: React.FC<ScrubRulesCardProps> = ({
  policy,
  onChange,
}) => {
  const rules = policy.scrub_rules || [];
  const [modalVisible, setModalVisible] = useState(false);
  const [editIndex, setEditIndex] = useState<number | null>(null);
  const [form] = Form.useForm();

  const handleAdd = () => {
    setEditIndex(null);
    form.resetFields();
    form.setFieldsValue({ enabled: true, replacement: '' });
    setModalVisible(true);
  };

  const handleEdit = (index: number) => {
    setEditIndex(index);
    form.setFieldsValue(rules[index]);
    setModalVisible(true);
  };

  const handleDelete = (index: number) => {
    const newRules = [...rules];
    newRules.splice(index, 1);
    onChange({ scrub_rules: newRules });
  };

  const handleToggle = (index: number, enabled: boolean) => {
    const newRules = [...rules];
    newRules[index] = { ...newRules[index], enabled };
    onChange({ scrub_rules: newRules });
  };

  const handleSave = () => {
    form.validateFields().then((values) => {
      // Validate regex
      try {
        new RegExp(values.pattern);
      } catch {
        form.setFields([{ name: 'pattern', errors: ['无效的正则表达式'] }]);
        return;
      }

      const newRules = [...rules];
      const rule: ScrubRule = {
        name: values.name,
        pattern: values.pattern,
        replacement: values.replacement || '',
        enabled: values.enabled ?? true,
      };

      if (editIndex !== null) {
        newRules[editIndex] = rule;
      } else {
        newRules.push(rule);
      }

      onChange({ scrub_rules: newRules });
      setModalVisible(false);
      form.resetFields();
    });
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 150,
      render: (name: string) => <Text strong>{name}</Text>,
    },
    {
      title: '模式 (正则)',
      dataIndex: 'pattern',
      key: 'pattern',
      ellipsis: true,
      render: (pattern: string) => (
        <Text code style={{ fontSize: 12 }}>{pattern}</Text>
      ),
    },
    {
      title: '替换文本',
      dataIndex: 'replacement',
      key: 'replacement',
      width: 150,
      render: (text: string) => text ? <Tag>{text}</Tag> : <Text type="secondary">[REDACTED]</Text>,
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 70,
      render: (enabled: boolean, _: ScrubRule, index: number) => (
        <Switch size="small" checked={enabled} onChange={(v) => handleToggle(index, v)} />
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_: unknown, __: ScrubRule, index: number) => (
        <Space size="small">
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(index)} />
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(index)} okText="确定" cancelText="取消">
            <Button type="link" size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="自定义凭证脱敏规则"
      size="small"
      extra={
        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleAdd}>
          添加规则
        </Button>
      }
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
        除内置检测模式（API Key、Bearer Token、AWS Key 等）外，可添加自定义正则表达式匹配并脱敏敏感信息。
        留空「替换文本」字段将使用默认的 [REDACTED] 脱敏方式。
      </Text>

      <Table
        dataSource={rules.map((r, i) => ({ ...r, key: i }))}
        columns={columns}
        size="small"
        pagination={false}
        locale={{ emptyText: '尚未添加自定义脱敏规则（内置规则始终生效）' }}
      />

      <Modal
        title={editIndex !== null ? '编辑脱敏规则' : '添加脱敏规则'}
        open={modalVisible}
        onOk={handleSave}
        onCancel={() => { setModalVisible(false); form.resetFields(); }}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="规则名称"
            rules={[{ required: true, message: '请输入规则名称' }]}
          >
            <Input placeholder="例如：SSN、信用卡号" />
          </Form.Item>
          <Form.Item
            name="pattern"
            label="正则表达式"
            rules={[{ required: true, message: '请输入正则表达式' }]}
            extra="用于匹配需要脱敏的内容，例如：\b\d{3}-\d{2}-\d{4}\b"
          >
            <Input placeholder="正则表达式模式" />
          </Form.Item>
          <Form.Item
            name="replacement"
            label="替换文本"
            extra="留空则使用默认的部分隐藏脱敏 ([REDACTED])"
          >
            <Input placeholder="例如：***-**-****" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};
