import React from 'react';
import { Card, Switch, Space, Statistic, Divider, Typography, Alert } from 'antd';
import type { PolicyConfig } from '../../types/policy';

const { Text } = Typography;

export interface PolicyOverviewCardProps {
  policy: PolicyConfig;
  pendingCount: number;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

export const PolicyOverviewCard: React.FC<PolicyOverviewCardProps> = ({
  policy,
  pendingCount,
  onChange,
}) => {
  return (
    <Card title="策略概览" size="small" style={{ marginBottom: 16 }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <Text>默认允许工具执行</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              开启：工具未在黑名单中即可执行（跳过白名单检查）。关闭：仅白名单中的工具可执行。
            </Text>
          </div>
          <Switch
            checked={policy.default_allow}
            onChange={(checked) => onChange({ default_allow: checked })}
          />
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <Text>所有调用需要审批</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              开启：所有通过策略检查的工具调用都需要人工审批后才会执行。
            </Text>
          </div>
          <Switch
            checked={policy.require_approval}
            onChange={(checked) => onChange({ require_approval: checked })}
          />
        </div>

        {policy.require_approval && (
          <Alert
            type="info"
            showIcon
            style={{ marginTop: 4 }}
            message="审批说明"
            description={'开启「所有调用需要审批」后，效果与危险操作规则中的「需审批」(approve)动作叠加：全局开关使所有工具调用都需审批，而危险操作规则仅对匹配的特定调用要求审批。'}
          />
        )}

        <Divider style={{ margin: '8px 0' }} />
        <Space size="large">
          <Statistic title="黑名单" value={policy.blocklist?.length || 0} />
          <Statistic title="白名单" value={policy.allowlist?.length || 0} />
          <Statistic title="危险规则" value={policy.dangerous_ops?.length || 0} />
          <Statistic title="待审批" value={pendingCount} />
        </Space>
      </Space>
    </Card>
  );
};
