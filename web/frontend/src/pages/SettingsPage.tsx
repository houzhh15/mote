import React, { useState, useEffect } from 'react';
import { Typography, Form, Input, Button, Card, Space, message, Divider, Switch, Select } from 'antd';
import { SaveOutlined } from '@ant-design/icons';

const { Title, Text } = Typography;

interface Settings {
  provider: {
    type: string;
    api_key?: string;
    model?: string;
    base_url?: string;
  };
  gateway: {
    host: string;
    port: number;
  };
  memory: {
    enabled: boolean;
    auto_capture: boolean;
    auto_recall: boolean;
  };
}

export const SettingsPage: React.FC = () => {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  const fetchSettings = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/settings');
      const data = await response.json();
      setSettings(data);
      form.setFieldsValue({
        provider_type: data.provider?.type,
        provider_model: data.provider?.model,
        provider_api_key: data.provider?.api_key,
        provider_base_url: data.provider?.base_url,
        gateway_host: data.gateway?.host,
        gateway_port: data.gateway?.port,
        memory_enabled: data.memory?.enabled,
        memory_auto_capture: data.memory?.auto_capture,
        memory_auto_recall: data.memory?.auto_recall,
      });
    } catch (error) {
      console.error('Failed to fetch settings:', error);
      message.error('获取设置失败');
    } finally {
      setLoading(false);
    }
  };

  const saveSettings = async (values: Record<string, unknown>) => {
    setSaving(true);
    try {
      const payload = {
        provider: {
          type: values.provider_type,
          model: values.provider_model,
          api_key: values.provider_api_key,
          base_url: values.provider_base_url,
        },
        gateway: {
          host: values.gateway_host,
          port: values.gateway_port,
        },
        memory: {
          enabled: values.memory_enabled,
          auto_capture: values.memory_auto_capture,
          auto_recall: values.memory_auto_recall,
        },
      };
      await fetch('/api/v1/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      message.success('设置已保存');
    } catch (error) {
      console.error('Failed to save settings:', error);
      message.error('保存设置失败');
    } finally {
      setSaving(false);
    }
  };

  useEffect(() => {
    fetchSettings();
  }, []);

  return (
    <div className="page-container">
      <div className="page-header">
        <Title level={4}>设置</Title>
      </div>

      <Form form={form} layout="vertical" onFinish={saveSettings} style={{ maxWidth: 600 }}>
        <Card title="AI 提供者" style={{ marginBottom: 16 }}>
          <Form.Item name="provider_type" label="提供者类型">
            <Select placeholder="选择提供者">
              <Select.Option value="openai">OpenAI</Select.Option>
              <Select.Option value="anthropic">Anthropic</Select.Option>
              <Select.Option value="ollama">Ollama</Select.Option>
              <Select.Option value="xai">xAI</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="provider_model" label="模型">
            <Input placeholder="例如: gpt-4, claude-3-opus" />
          </Form.Item>
          <Form.Item name="provider_api_key" label="API Key">
            <Input.Password placeholder="API 密钥" />
          </Form.Item>
          <Form.Item name="provider_base_url" label="Base URL (可选)">
            <Input placeholder="自定义 API 端点" />
          </Form.Item>
        </Card>

        <Card title="记忆设置" style={{ marginBottom: 16 }}>
          <Form.Item name="memory_enabled" label="启用记忆" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="memory_auto_capture" label="自动捕获" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="memory_auto_recall" label="自动召回" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Card>

        <Card title="网关设置" style={{ marginBottom: 16 }}>
          <Form.Item name="gateway_host" label="主机">
            <Input placeholder="127.0.0.1" />
          </Form.Item>
          <Form.Item name="gateway_port" label="端口">
            <Input type="number" placeholder="9080" />
          </Form.Item>
        </Card>

        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
            保存设置
          </Button>
        </Form.Item>
      </Form>
    </div>
  );
};
