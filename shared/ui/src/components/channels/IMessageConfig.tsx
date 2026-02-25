// ================================================================
// IMessageConfig - iMessage channel configuration modal
// ================================================================

import React, { useState, useEffect } from 'react';
import { Modal, Form, Input, Switch, Select, Divider, message } from 'antd';
import type { IMessageChannelConfig } from '../../types';
import type { Model } from '../../types';

export interface IMessageConfigProps {
  visible: boolean;
  config: IMessageChannelConfig | null;
  models?: Model[];
  onSave: (config: IMessageChannelConfig) => Promise<void>;
  onCancel: () => void;
}

export const IMessageConfig: React.FC<IMessageConfigProps> = ({
  visible,
  config,
  models,
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
      title="iMessage 渠道配置"
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
          trigger: { prefix: '@mote', caseSensitive: false, selfTrigger: true },
          reply: { prefix: '[Mote]', separator: '\n' },
          allowFrom: [],
        }}
      >
        {/* 模型设置 */}
        <Form.Item
          name="model"
          label="专属模型"
          tooltip="为此渠道指定专属模型，留空则使用全局默认模型"
        >
          <Select
            placeholder="使用默认模型"
            allowClear
            showSearch
            options={(models || []).map(m => ({ label: `${m.display_name || m.id} (${m.provider || ''})`, value: m.id }))}
          />
        </Form.Item>

        {/* 触发设置 */}
        <Divider orientation="left">触发设置</Divider>
        <Form.Item
          name={['trigger', 'prefix']}
          label="触发前缀"
          rules={[{ required: true, message: '请输入触发前缀' }]}
          tooltip="消息必须以此前缀开头才会被处理"
        >
          <Input placeholder="@mote" />
        </Form.Item>
        <Form.Item
          name={['trigger', 'caseSensitive']}
          label="区分大小写"
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name={['trigger', 'selfTrigger']}
          label="处理自己发送的消息"
          valuePropName="checked"
          tooltip="允许给自己发送消息来触发 AI"
        >
          <Switch />
        </Form.Item>

        {/* 回复设置 */}
        <Divider orientation="left">回复设置</Divider>
        <Form.Item
          name={['reply', 'prefix']}
          label="回复前缀"
          tooltip="AI 回复消息会带有此前缀"
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

        {/* 安全设置 */}
        <Divider orientation="left">安全设置</Divider>
        <Form.Item
          name="allowFrom"
          label="发信人白名单"
          tooltip="留空表示允许所有发信人，否则只处理白名单中的发信人消息"
        >
          <Select
            mode="tags"
            placeholder="输入邮箱或手机号后按回车添加"
            tokenSeparators={[',']}
            style={{ width: '100%' }}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};
