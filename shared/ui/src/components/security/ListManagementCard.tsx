import React, { useState } from 'react';
import { Card, Tag, Input, Space, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { PolicyConfig } from '../../types/policy';

const { Text } = Typography;

export interface ListManagementCardProps {
  policy: PolicyConfig;
  onChange: (patch: Partial<PolicyConfig>) => void;
}

export const ListManagementCard: React.FC<ListManagementCardProps> = ({
  policy,
  onChange,
}) => {
  const [blockInput, setBlockInput] = useState('');
  const [allowInput, setAllowInput] = useState('');
  const [blockInputVisible, setBlockInputVisible] = useState(false);
  const [allowInputVisible, setAllowInputVisible] = useState(false);

  const handleBlockClose = (removed: string) => {
    onChange({ blocklist: policy.blocklist.filter((t) => t !== removed) });
  };

  const handleAllowClose = (removed: string) => {
    onChange({ allowlist: policy.allowlist.filter((t) => t !== removed) });
  };

  const handleBlockAdd = () => {
    const val = blockInput.trim();
    if (val && !policy.blocklist.includes(val)) {
      onChange({ blocklist: [...policy.blocklist, val] });
    }
    setBlockInput('');
    setBlockInputVisible(false);
  };

  const handleAllowAdd = () => {
    const val = allowInput.trim();
    if (val && !policy.allowlist.includes(val)) {
      onChange({ allowlist: [...policy.allowlist, val] });
    }
    setAllowInput('');
    setAllowInputVisible(false);
  };

  return (
    <Card title="黑名单 / 白名单" size="small" style={{ marginBottom: 16 }}>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {/* Blocklist */}
        <div>
          <Text strong style={{ display: 'block', marginBottom: 4 }}>黑名单（禁止使用的工具）</Text>
          <Text type="secondary" style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>
            黑名单中的工具将被直接拒绝，优先级最高。为空表示不做黑名单限制。
          </Text>
          <Space wrap>
            {policy.blocklist.map((item) => (
              <Tag key={item} closable onClose={() => handleBlockClose(item)} color="red">
                {item}
              </Tag>
            ))}
            {blockInputVisible ? (
              <Input
                size="small"
                style={{ width: 200 }}
                value={blockInput}
                onChange={(e) => setBlockInput(e.target.value)}
                onBlur={handleBlockAdd}
                onPressEnter={handleBlockAdd}
                autoFocus
                placeholder="shell, read_file, group:file_ops"
              />
            ) : (
              <Tag onClick={() => setBlockInputVisible(true)} style={{ cursor: 'pointer', borderStyle: 'dashed' }}>
                <PlusOutlined /> 添加
              </Tag>
            )}
          </Space>
        </div>

        {/* Allowlist */}
        <div>
          <Text strong style={{ display: 'block', marginBottom: 4 }}>白名单（允许的工具）</Text>
          <Text type="secondary" style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>
            仅在"默认允许工具执行"关闭时生效。关闭默认允许后，只有白名单中的工具才能被调用。
          </Text>
          <Space wrap>
            {policy.allowlist.map((item) => (
              <Tag key={item} closable onClose={() => handleAllowClose(item)} color="green">
                {item}
              </Tag>
            ))}
            {allowInputVisible ? (
              <Input
                size="small"
                style={{ width: 200 }}
                value={allowInput}
                onChange={(e) => setAllowInput(e.target.value)}
                onBlur={handleAllowAdd}
                onPressEnter={handleAllowAdd}
                autoFocus
                placeholder="shell, read_file, group:file_ops"
              />
            ) : (
              <Tag onClick={() => setAllowInputVisible(true)} style={{ cursor: 'pointer', borderStyle: 'dashed' }}>
                <PlusOutlined /> 添加
              </Tag>
            )}
          </Space>
        </div>
      </Space>
    </Card>
  );
};
