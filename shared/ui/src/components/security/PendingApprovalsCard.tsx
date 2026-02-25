import React from 'react';
import { Card, List, Tag, Button, Empty, Badge, Typography } from 'antd';
import type { ApprovalRequest } from '../../types/policy';

const { Text } = Typography;

export interface PendingApprovalsCardProps {
  pending: ApprovalRequest[];
  onApprove: (id: string) => void;
  onReject: (id: string) => void;
}

const formatTime = (t: string) => {
  try {
    return new Date(t).toLocaleString();
  } catch {
    return t;
  }
};

export const PendingApprovalsCard: React.FC<PendingApprovalsCardProps> = ({
  pending,
  onApprove,
  onReject,
}) => {
  return (
    <Card
      title={
        <>
          待审批请求 <Badge count={pending.length} style={{ marginLeft: 8 }} />
        </>
      }
      size="small"
      style={{ marginBottom: 16 }}
    >
      {pending.length === 0 ? (
        <Empty description="暂无待审批请求" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      ) : (
        <List
          dataSource={pending}
          renderItem={(item) => (
            <List.Item
              actions={[
                <Button
                  key="approve"
                  type="primary"
                  size="small"
                  onClick={() => onApprove(item.id)}
                >
                  批准
                </Button>,
                <Button
                  key="reject"
                  danger
                  size="small"
                  onClick={() => onReject(item.id)}
                >
                  拒绝
                </Button>,
              ]}
            >
              <List.Item.Meta
                title={
                  <>
                    <Tag>{item.tool_name}</Tag> {item.reason}
                  </>
                }
                description={
                  <Text type="secondary">
                    创建: {formatTime(item.created_at)} · 过期: {formatTime(item.expires_at)}
                  </Text>
                }
              />
            </List.Item>
          )}
        />
      )}
    </Card>
  );
};
