import React, { useState } from 'react';
import { Card, Collapse, Tag, Typography, Space, InputNumber, Input, Button, Modal, Form, Popconfirm, Switch, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import type { PolicyConfig, ParamRule } from '../../types/policy';

const { Text } = Typography;

export interface ParamRulesCardProps {
  policy: PolicyConfig;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

export const ParamRulesCard: React.FC<ParamRulesCardProps> = ({
  policy,
  onChange,
}) => {
  const paramRules = policy.param_rules || {};
  const toolNames = Object.keys(paramRules);

  const [addModalVisible, setAddModalVisible] = useState(false);
  const [form] = Form.useForm();

  const handleRuleChange = (toolName: string, field: keyof ParamRule, value: unknown) => {
    const newRules = { ...paramRules };
    newRules[toolName] = { ...newRules[toolName], [field]: value };
    onChange({ param_rules: newRules });
  };

  const handleAddPathPrefix = (toolName: string, prefix: string) => {
    if (!prefix.trim()) return;
    const rule = paramRules[toolName];
    const current = rule.path_prefix || [];
    if (current.includes(prefix.trim())) return;
    handleRuleChange(toolName, 'path_prefix', [...current, prefix.trim()]);
  };

  const handleRemovePathPrefix = (toolName: string, prefix: string) => {
    const rule = paramRules[toolName];
    handleRuleChange(toolName, 'path_prefix', (rule.path_prefix || []).filter((p) => p !== prefix));
  };

  const handleAddForbidden = (toolName: string, word: string) => {
    if (!word.trim()) return;
    const rule = paramRules[toolName];
    const current = rule.forbidden || [];
    if (current.includes(word.trim())) return;
    handleRuleChange(toolName, 'forbidden', [...current, word.trim()]);
  };

  const handleRemoveForbidden = (toolName: string, word: string) => {
    const rule = paramRules[toolName];
    handleRuleChange(toolName, 'forbidden', (rule.forbidden || []).filter((f) => f !== word));
  };

  const handleDeleteRule = (toolName: string) => {
    const newRules = { ...paramRules };
    delete newRules[toolName];
    onChange({ param_rules: newRules });
  };

  const handleToggleEnabled = (toolName: string, enabled: boolean) => {
    const newRules = { ...paramRules };
    newRules[toolName] = { ...newRules[toolName], enabled };
    onChange({ param_rules: newRules });
  };

  const handleAddRule = async () => {
    try {
      const values = await form.validateFields();
      const toolName = values.tool_name.trim();
      if (paramRules[toolName]) {
        form.setFields([{ name: 'tool_name', errors: ['该工具名已存在规则'] }]);
        return;
      }
      const newRule: ParamRule = {};
      if (values.max_length) newRule.max_length = values.max_length;
      if (values.pattern) newRule.pattern = values.pattern;
      if (values.path_prefix) {
        newRule.path_prefix = values.path_prefix.split(',').map((s: string) => s.trim()).filter(Boolean);
      }
      if (values.forbidden) {
        newRule.forbidden = values.forbidden.split(',').map((s: string) => s.trim()).filter(Boolean);
      }
      const newRules = { ...paramRules, [toolName]: newRule };
      onChange({ param_rules: newRules });
      setAddModalVisible(false);
      form.resetFields();
    } catch {
      // validation failed
    }
  };

  const PathPrefixEditor: React.FC<{ toolName: string; prefixes: string[] }> = ({ toolName, prefixes }) => {
    const [inputVisible, setInputVisible] = useState(false);
    const [inputValue, setInputValue] = useState('');
    return (
      <div>
        <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>允许路径前缀：</Text>
        <Space wrap>
          {prefixes.map((p) => (
            <Tag key={p} color="blue" closable onClose={() => handleRemovePathPrefix(toolName, p)}>{p}</Tag>
          ))}
          {inputVisible ? (
            <Input
              size="small"
              style={{ width: 160 }}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onBlur={() => { handleAddPathPrefix(toolName, inputValue); setInputValue(''); setInputVisible(false); }}
              onPressEnter={() => { handleAddPathPrefix(toolName, inputValue); setInputValue(''); setInputVisible(false); }}
              autoFocus
              placeholder="$WORKSPACE, ~/project, /tmp"
            />
          ) : (
            <Tag onClick={() => setInputVisible(true)} style={{ cursor: 'pointer', borderStyle: 'dashed' }}>
              <PlusOutlined /> 添加
            </Tag>
          )}
        </Space>
      </div>
    );
  };

  const ForbiddenEditor: React.FC<{ toolName: string; words: string[] }> = ({ toolName, words }) => {
    const [inputVisible, setInputVisible] = useState(false);
    const [inputValue, setInputValue] = useState('');
    return (
      <div>
        <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>禁止关键词：</Text>
        <Space wrap>
          {words.map((f) => (
            <Tag key={f} color="red" closable onClose={() => handleRemoveForbidden(toolName, f)}>{f}</Tag>
          ))}
          {inputVisible ? (
            <Input
              size="small"
              style={{ width: 160 }}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onBlur={() => { handleAddForbidden(toolName, inputValue); setInputValue(''); setInputVisible(false); }}
              onPressEnter={() => { handleAddForbidden(toolName, inputValue); setInputValue(''); setInputVisible(false); }}
              autoFocus
              placeholder="关键词"
            />
          ) : (
            <Tag onClick={() => setInputVisible(true)} style={{ cursor: 'pointer', borderStyle: 'dashed' }}>
              <PlusOutlined /> 添加
            </Tag>
          )}
        </Space>
      </div>
    );
  };

  const items = toolNames.map((toolName) => {
    const rule = paramRules[toolName];
    const isEnabled = rule.enabled !== false;
    return {
      key: toolName,
      label: (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
          <Space>
            <Tooltip title={isEnabled ? '点击禁用' : '点击启用'}>
              <Switch
                size="small"
                checked={isEnabled}
                onChange={(checked, e) => { e.stopPropagation(); handleToggleEnabled(toolName, checked); }}
              />
            </Tooltip>
            <Text strong style={{ opacity: isEnabled ? 1 : 0.45 }}>{toolName}</Text>
            {!isEnabled && <Tag color="default">已禁用</Tag>}
          </Space>
          <Popconfirm title="确定删除此工具的参数规则？" onConfirm={() => handleDeleteRule(toolName)} okText="确定" cancelText="取消">
            <Button type="link" size="small" danger icon={<DeleteOutlined />} onClick={(e) => e.stopPropagation()} />
          </Popconfirm>
        </div>
      ),
      children: (
        <Space direction="vertical" style={{ width: '100%', opacity: isEnabled ? 1 : 0.45 }} size="small">
          {(rule.path_prefix !== undefined || toolName === 'read_file' || toolName === 'write_file') && (
            <PathPrefixEditor toolName={toolName} prefixes={rule.path_prefix || []} />
          )}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text type="secondary">最大长度：</Text>
            <InputNumber
              size="small"
              value={rule.max_length}
              onChange={(val) => handleRuleChange(toolName, 'max_length', val ?? undefined)}
              min={0}
              placeholder="无限制"
            />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text type="secondary">匹配模式：</Text>
            <Input
              size="small"
              value={rule.pattern || ''}
              onChange={(e) => handleRuleChange(toolName, 'pattern', e.target.value || undefined)}
              style={{ maxWidth: 300 }}
              placeholder="正则表达式（可选）"
            />
          </div>
          <ForbiddenEditor toolName={toolName} words={rule.forbidden || []} />
        </Space>
      ),
    };
  });

  return (
    <Card
      title="参数校验规则"
      size="small"
      style={{ marginBottom: 16 }}
      extra={<Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => setAddModalVisible(true)}>添加规则</Button>}
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 12, fontSize: 12 }}>
        对特定工具的参数进行校验。默认已配置 read_file 和 write_file 的路径前缀规则，限制文件访问范围。
      </Text>
      {toolNames.length === 0 ? (
        <Text type="secondary">暂无参数校验规则。点击"添加规则"为工具配置参数约束。</Text>
      ) : (
        <Collapse items={items} size="small" />
      )}

      <Modal
        title="添加参数校验规则"
        open={addModalVisible}
        onOk={handleAddRule}
        onCancel={() => { setAddModalVisible(false); form.resetFields(); }}
        okText="添加"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="tool_name" label="工具名" rules={[{ required: true, message: '请输入工具名' }]}>
            <Input placeholder="例如: shell, write_file, read_file" />
          </Form.Item>
          <Form.Item name="path_prefix" label="允许路径前缀（逗号分隔）" extra="支持 ~ (Home目录) 和 $WORKSPACE (当前工作目录)">
            <Input placeholder="$WORKSPACE, ~/project, /tmp, ~/.mote" />
          </Form.Item>
          <Form.Item name="max_length" label="最大参数长度">
            <InputNumber min={0} placeholder="不限制" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="pattern" label="参数匹配正则">
            <Input placeholder="可选，用于限制参数格式" />
          </Form.Item>
          <Form.Item name="forbidden" label="禁止关键词（逗号分隔）">
            <Input placeholder="rm -rf, sudo, chmod 777" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};
