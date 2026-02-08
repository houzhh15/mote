// ================================================================
// ChannelCard - Generic channel status card component
// ================================================================

import React from 'react';
import { Card, Space, Badge, Button, Typography } from 'antd';
import { MessageOutlined } from '@ant-design/icons';
import type { ChannelStatus } from '../../types';

const { Text } = Typography;

export interface ChannelCardProps {
  channel: ChannelStatus;
  onConfigure: () => void;
  onToggle: (enabled: boolean) => void;
  loading?: boolean;
}

const statusBadgeMap: Record<ChannelStatus['status'], { status: 'success' | 'default' | 'error'; text: string }> = {
  running: { status: 'success', text: '运行中' },
  stopped: { status: 'default', text: '已停止' },
  error: { status: 'error', text: '错误' },
};

export const ChannelCard: React.FC<ChannelCardProps> = ({
  channel,
  onConfigure,
  onToggle,
  loading,
}) => {
  const statusBadge = statusBadgeMap[channel.status] || statusBadgeMap.stopped;
  // 使用 status 判断按钮状态，而不是 enabled
  const isRunning = channel.status === 'running';

  return (
    <Card size="small" style={{ marginBottom: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <Space>
            <MessageOutlined />
            <Text strong>{channel.name}</Text>
            <Badge status={statusBadge.status} text={statusBadge.text} />
          </Space>
          {channel.error && (
            <div>
              <Text type="danger" style={{ fontSize: 12 }}>{channel.error}</Text>
            </div>
          )}
        </div>
        <Space>
          <Button size="small" onClick={onConfigure}>
            配置
          </Button>
          <Button
            size="small"
            type={isRunning ? 'default' : 'primary'}
            danger={isRunning}
            onClick={() => onToggle(!isRunning)}
            loading={loading}
          >
            {isRunning ? '停止' : '启动'}
          </Button>
        </Space>
      </div>
    </Card>
  );
};
