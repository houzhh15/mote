// ================================================================
// AppleRemindersConfig - Apple Reminders channel configuration modal
// ================================================================

import React, { useState, useEffect } from 'react';
import { Modal, Form, Input, Switch, Divider, message } from 'antd';
import type { AppleRemindersChannelConfig } from '../../types';

export interface AppleRemindersConfigProps {
  visible: boolean;
  config: AppleRemindersChannelConfig | null;
  onSave: (config: AppleRemindersChannelConfig) => Promise<void>;
  onCancel: () => void;
}

export const AppleRemindersConfig: React.FC<AppleRemindersConfigProps> = ({
  visible,
  config,
  onSave,
  onCancel,
}) => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (visible && config) {
      form.setFieldsValue(config);
    }
  }, [visible, config, form]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      await onSave(values);
      message.success('配置已保存');
      onCancel();
    } catch (error) {
      if (error instanceof Error) {
        message.error('保存失败: ' + error.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  return (
    <Modal
      title="Apple Reminders 渠道配置"
      open={visible}
      onOk={handleSave}
      onCancel={handleCancel}
      confirmLoading={loading}
      okText="保存配置"
      cancelText="取消"
      width={500}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={config || {
          enabled: false,
          trigger: { prefix: '@mote:', caseSensitive: false },
          reply: { prefix: '[Mote]', separator: '\n' },
          watchList: 'Mote Tasks',
          pollInterval: '5s',
        }}
      >
        {/* 监控设置 */}
        <Divider orientation="left">监控设置</Divider>
        <Form.Item
          name="watchList"
          label="监控列表"
          rules={[{ required: true, message: '请输入要监控的提醒事项列表名称' }]}
          tooltip="只会处理此列表中的提醒"
        >
          <Input placeholder="Mote Tasks" />
        </Form.Item>
        <Form.Item
          name="pollInterval"
          label="轮询间隔"
          tooltip="检查新提醒的时间间隔"
        >
          <Input placeholder="5s" />
        </Form.Item>

        {/* 触发设置 */}
        <Divider orientation="left">触发设置</Divider>
        <Form.Item
          name={['trigger', 'prefix']}
          label="触发前缀"
          rules={[{ required: true, message: '请输入触发前缀' }]}
          tooltip="提醒标题必须以此前缀开头才会被处理"
        >
          <Input placeholder="@mote:" />
        </Form.Item>
        <Form.Item
          name={['trigger', 'caseSensitive']}
          label="区分大小写"
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>

        {/* 回复设置 */}
        <Divider orientation="left">回复设置</Divider>
        <Form.Item
          name={['reply', 'prefix']}
          label="回复前缀"
          tooltip="AI 回复会以此前缀开头"
        >
          <Input placeholder="[Mote]" />
        </Form.Item>
        <Form.Item
          name={['reply', 'separator']}
          label="分隔符"
          tooltip="前缀与内容之间的分隔符"
        >
          <Input placeholder="\n" />
        </Form.Item>
      </Form>
    </Modal>
  );
};
