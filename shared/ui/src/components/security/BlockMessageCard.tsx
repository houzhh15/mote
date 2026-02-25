import React from 'react';
import { Card, Form, Input, InputNumber, Typography, Space, Tag } from 'antd';
import type { PolicyConfig } from '../../types/policy';

const { Text } = Typography;
const { TextArea } = Input;

export interface BlockMessageCardProps {
  policy: PolicyConfig;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

export const BlockMessageCard: React.FC<BlockMessageCardProps> = ({
  policy,
  onChange,
}) => {
  return (
    <Card title="拦截消息与熔断器" size="small">
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <div>
          <Text strong>自定义拦截消息模板</Text>
          <Text type="secondary" style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>
            工具被策略拦截时显示的消息。可用占位符：<Tag>{'{tool}'}</Tag> 工具名，<Tag>{'{reason}'}</Tag> 拦截原因。
            留空使用默认消息。
          </Text>
          <TextArea
            value={policy.block_message_template || ''}
            onChange={(e) => onChange({ block_message_template: e.target.value })}
            placeholder="例如：⛔ {tool} 被拦截：{reason}"
            autoSize={{ minRows: 2, maxRows: 4 }}
          />
        </div>

        <div>
          <Text strong>熔断器阈值</Text>
          <Text type="secondary" style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>
            同一会话中同一工具被连续拦截达到此次数后，将向 LLM 注入强制停止指令，防止无限重试循环。
            设为 0 禁用。
          </Text>
          <Form.Item style={{ marginBottom: 0 }}>
            <InputNumber
              min={0}
              max={100}
              value={policy.circuit_breaker_threshold ?? 3}
              onChange={(v) => onChange({ circuit_breaker_threshold: v ?? 0 })}
              addonAfter="次"
              style={{ width: 150 }}
            />
          </Form.Item>
        </div>
      </Space>
    </Card>
  );
};
