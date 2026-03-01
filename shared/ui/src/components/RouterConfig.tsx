import React, { useCallback } from 'react';
import { Button, Select, Space, Table, Tag, Typography } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import type { AgentOption } from '../types';

const { Text } = Typography;

interface RouterConfigProps {
  branches: Record<string, string>;
  onChange: (branches: Record<string, string>) => void;
  availableAgents: AgentOption[];
}

interface BranchRow {
  key: string;
  target: string;
  isDefault: boolean;
}

export const RouterConfig: React.FC<RouterConfigProps> = ({
  branches,
  onChange,
  availableAgents,
}) => {
  // Build rows — each non-_default branch is keyed by its target agent name.
  // Legacy configs may have opaque keys (branch_xxx); we normalise on save.
  const rows: BranchRow[] = Object.entries(branches).map(([match, target]) => ({
    key: match,
    target,
    isDefault: match === '_default',
  }));

  // Ensure _default is always last
  rows.sort((a, b) => {
    if (a.isDefault) return 1;
    if (b.isDefault) return -1;
    return 0;
  });

  // Agents already used as targets (excluding the row being edited)
  const usedAgents = new Set(Object.values(branches));

  const updateTarget = useCallback(
    (oldKey: string, newTarget: string) => {
      const updated: Record<string, string> = {};
      for (const [k, v] of Object.entries(branches)) {
        if (k === oldKey) {
          // For non-default branches, use agent name as key for clarity.
          // _default keeps its special key.
          if (k === '_default') {
            updated['_default'] = newTarget;
          } else {
            updated[newTarget] = newTarget;
          }
        } else {
          updated[k] = v;
        }
      }
      onChange(updated);
    },
    [branches, onChange],
  );

  const removeBranch = useCallback(
    (key: string) => {
      const updated = { ...branches };
      delete updated[key];
      onChange(updated);
    },
    [branches, onChange],
  );

  const addBranch = useCallback(() => {
    // Find first available agent not already targeted
    const available = availableAgents.filter((a) => !usedAgents.has(a.name));
    const target = available.length > 0 ? available[0].name : '';
    const key = target || `_new_${Date.now()}`;
    onChange({ ...branches, [key]: target });
  }, [branches, onChange, availableAgents, usedAgents]);

  const columns = [
    {
      title: '目标代理',
      dataIndex: 'target',
      key: 'target',
      render: (value: string, record: BranchRow) => (
        <Space>
          {record.isDefault && (
            <Text type="secondary" italic style={{ whiteSpace: 'nowrap' }}>
              默认 →
            </Text>
          )}
          <Select
            size="small"
            value={value || undefined}
            onChange={(val) => updateTarget(record.key, val)}
            placeholder="选择代理"
            style={{ minWidth: 160 }}
            optionLabelProp="label"
          >
            {availableAgents.map((a) => (
              <Select.Option key={a.name} value={a.name} label={a.name}>
                <Space size={4}>
                  <span>{a.name}</span>
                  {a.tags?.map((t) => (
                    <Tag key={t} style={{ marginInlineEnd: 0, fontSize: 11, lineHeight: '18px', padding: '0 4px' }}>{t}</Tag>
                  ))}
                </Space>
              </Select.Option>
            ))}
            <Select.Option key="_end" value="_end" label="_end">
              <Text type="secondary" italic>_end（结束）</Text>
            </Select.Option>
          </Select>
        </Space>
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 40,
      render: (_: unknown, record: BranchRow) =>
        !record.isDefault ? (
          <Button
            type="text"
            danger
            size="small"
            icon={<DeleteOutlined />}
            onClick={() => removeBranch(record.key)}
          />
        ) : null,
    },
  ];

  return (
    <div>
      <Table
        size="small"
        dataSource={rows}
        columns={columns}
        pagination={false}
        bordered
        locale={{ emptyText: '暂无分支' }}
      />
      <Space style={{ marginTop: 8 }}>
        <Button size="small" icon={<PlusOutlined />} onClick={addBranch}>
          添加分支
        </Button>
        {!branches['_default'] && branches['_default'] !== '' && (
          <Button
            size="small"
            type="link"
            onClick={() => onChange({ ...branches, _default: '' })}
          >
            + 添加默认分支
          </Button>
        )}
      </Space>
    </div>
  );
};
