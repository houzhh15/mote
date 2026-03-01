// ================================================================
// ContextDetailModal - Detailed context window analysis view
//
// Shows three tiers of context data:
//   Total     = all DB messages (full history including compressed-away)
//   Effective = BuildContext output (summary + kept + new messages)
//   Budgeted  = after BudgetMessages (what LLM actually receives)
// ================================================================

import React, { useEffect, useState } from 'react';
import { Modal, Spin, Typography, Progress, Tag, Tooltip, Divider, Empty } from 'antd';
import {
  CompressOutlined,
  MessageOutlined,
  UserOutlined,
  RobotOutlined,
  ToolOutlined,
  CheckCircleOutlined,
  EyeInvisibleOutlined,
  MinusCircleOutlined,
} from '@ant-design/icons';
import type { SessionContextResponse, ContextSegment } from '../types';
import { useAPI } from '../context/APIContext';

const { Text } = Typography;

function formatTokenCount(tokens: number): string {
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)}K`;
  }
  return `${tokens}`;
}

function formatBytes(chars: number): string {
  if (chars >= 1024 * 1024) return `${(chars / (1024 * 1024)).toFixed(1)} MB`;
  if (chars >= 1024) return `${(chars / 1024).toFixed(1)} KB`;
  return `${chars} B`;
}

const segmentTypeLabels: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  compressed_summary:  { label: '压缩摘要', color: 'purple', icon: <CompressOutlined /> },
  kept_message:        { label: '保留消息', color: 'blue', icon: <MessageOutlined /> },
  history_message:     { label: '历史消息', color: 'default', icon: <MessageOutlined /> },
  compressed_history:  { label: '已压缩', color: 'volcano', icon: <EyeInvisibleOutlined /> },
  user_input:          { label: '用户输入', color: 'green', icon: <UserOutlined /> },
  system_prompt:       { label: '系统提示', color: 'gold', icon: <RobotOutlined /> },
};

const roleColors: Record<string, string> = {
  user: '#8B5CF6',
  assistant: '#52c41a',
  system: '#faad14',
  tool: '#1890ff',
};

interface ContextDetailModalProps {
  open: boolean;
  onClose: () => void;
  sessionId?: string;
}

export const ContextDetailModal: React.FC<ContextDetailModalProps> = ({ open, onClose, sessionId }) => {
  const api = useAPI();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<SessionContextResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCompressedHistory, setShowCompressedHistory] = useState(false);

  useEffect(() => {
    if (!open || !sessionId) return;
    setLoading(true);
    setError(null);

    const fetchContext = async () => {
      try {
        if (!api.getSessionContext) {
          setError('此功能暂不可用');
          return;
        }
        const resp = await api.getSessionContext(sessionId);
        setData(resp);
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : '获取上下文详情失败');
      } finally {
        setLoading(false);
      }
    };

    fetchContext();
  }, [open, sessionId, api]);

  // Budgeted usage percent (what LLM actually sees)
  const budgetedPercent = data && data.context_window > 0
    ? Math.min((data.budgeted_tokens / data.context_window) * 100, 100)
    : 0;

  // Effective (pre-budget) usage percent
  const effectivePercent = data && data.context_window > 0
    ? Math.min((data.effective_tokens / data.context_window) * 100, 100)
    : 0;

  const getProgressColor = (pct: number): string => {
    if (pct >= 80) return '#ff4d4f';
    if (pct >= 60) return '#faad14';
    return '#1677ff';
  };

  // Aggregate segment stats by type
  const segmentStats = React.useMemo(() => {
    if (!data) return [];
    const map = new Map<string, { count: number; tokens: number; chars: number; inContextCount: number; budgetedCount: number }>();
    for (const seg of data.segments) {
      const existing = map.get(seg.type) || { count: 0, tokens: 0, chars: 0, inContextCount: 0, budgetedCount: 0 };
      existing.count++;
      existing.tokens += seg.estimated_tokens;
      existing.chars += seg.char_count;
      if (seg.in_context) existing.inContextCount++;
      if (seg.budgeted) existing.budgetedCount++;
      map.set(seg.type, existing);
    }
    return Array.from(map.entries()).map(([type, stats]) => ({ type, ...stats }));
  }, [data]);

  // Count of compressed_history segments (for toggle button)
  const compressedHistoryCount = data
    ? data.segments.filter(s => s.type === 'compressed_history').length
    : 0;

  // Filtered segment list
  const visibleSegments = React.useMemo(() => {
    if (!data) return [];
    if (showCompressedHistory) return data.segments;
    return data.segments.filter(s => s.type !== 'compressed_history');
  }, [data, showCompressedHistory]);

  return (
    <Modal
      title="上下文窗口详情"
      open={open}
      onCancel={onClose}
      footer={null}
      width={680}
      styles={{
        body: {
          maxHeight: '70vh',
          overflowY: 'auto',
          paddingRight: 8,
          marginRight: -8,
          scrollbarGutter: 'stable',
        },
      }}
    >
      {loading && (
        <div style={{ textAlign: 'center', padding: 48 }}>
          <Spin tip="加载中..." />
        </div>
      )}

      {error && (
        <div style={{ textAlign: 'center', padding: 48, color: '#ff4d4f' }}>
          {error}
        </div>
      )}

      {!loading && !error && !data && (
        <Empty description="无数据" />
      )}

      {!loading && !error && data && (
        <>
          {/* === Overview: Context Window Progress === */}
          <div style={{ marginBottom: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
              <div>
                <Text strong style={{ fontSize: 15 }}>Context Window</Text>
                <Text type="secondary" style={{ marginLeft: 8 }}>{data.model || '未知模型'}</Text>
              </div>
              <Text style={{ fontSize: 13 }}>
                {formatTokenCount(data.budgeted_tokens)} / {formatTokenCount(data.context_window || 0)} tokens
              </Text>
            </div>
            <Progress
              percent={effectivePercent}
              success={{ percent: budgetedPercent, strokeColor: getProgressColor(budgetedPercent) }}
              showInfo={false}
              strokeColor="rgba(0,0,0,0.06)"
              size="small"
            />
            <div style={{ fontSize: 11, color: '#aaa', marginTop: 2, display: 'flex', gap: 12 }}>
              <span style={{ color: getProgressColor(budgetedPercent) }}>
                ■ Budgeted {budgetedPercent.toFixed(1)}%
              </span>
              <span style={{ color: '#ccc' }}>
                ■ Effective {effectivePercent.toFixed(1)}%
              </span>
            </div>
          </div>

          {/* === Three-tier token breakdown === */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr 1fr',
            gap: 10,
            marginBottom: 16,
            fontSize: 12,
          }}>
            <div style={{ padding: '8px 10px', background: 'rgba(0,0,0,0.02)', borderRadius: 6, border: '1px solid rgba(0,0,0,0.06)' }}>
              <div style={{ color: '#999', marginBottom: 2 }}>全部历史</div>
              <div style={{ fontSize: 15, fontWeight: 600 }}>{formatTokenCount(data.total_tokens)}</div>
              <div style={{ color: '#bbb' }}>{data.total_messages} 条 · {formatBytes(data.total_chars)}</div>
            </div>
            <Tooltip title="BuildContext 组装结果：压缩摘要 + 保留消息 + 压缩后新消息">
              <div style={{ padding: '8px 10px', background: 'rgba(22,119,255,0.04)', borderRadius: 6, border: '1px solid rgba(22,119,255,0.15)' }}>
                <div style={{ color: '#1677ff', marginBottom: 2 }}>有效上下文</div>
                <div style={{ fontSize: 15, fontWeight: 600 }}>{formatTokenCount(data.effective_tokens)}</div>
                <div style={{ color: '#bbb' }}>{data.effective_count} 条 · {formatBytes(data.effective_chars)}</div>
              </div>
            </Tooltip>
            <Tooltip title="BudgetMessages 裁剪后，实际发送给 LLM 的内容">
              <div style={{ padding: '8px 10px', background: 'rgba(82,196,26,0.04)', borderRadius: 6, border: '1px solid rgba(82,196,26,0.15)' }}>
                <div style={{ color: '#52c41a', marginBottom: 2 }}>Budgeted</div>
                <div style={{ fontSize: 15, fontWeight: 600 }}>{formatTokenCount(data.budgeted_tokens)}</div>
                <div style={{ color: '#bbb' }}>{data.budgeted_count} 条 · {formatBytes(data.budgeted_chars)}</div>
              </div>
            </Tooltip>
          </div>

          {/* === Compression info === */}
          {data.compression.has_compression && (
            <>
              <Divider style={{ margin: '12px 0' }} />
              <div style={{ marginBottom: 12 }}>
                <Text strong style={{ fontSize: 13 }}>
                  <CompressOutlined style={{ marginRight: 4 }} />
                  压缩上下文 (v{data.compression.version})
                </Text>
                <div style={{ fontSize: 12, color: '#888', marginTop: 4 }}>
                  <span>原始: {formatTokenCount(data.compression.original_tokens || 0)} tokens</span>
                  <span style={{ margin: '0 8px' }}>→</span>
                  <span>压缩后: {formatTokenCount(data.compression.total_tokens || 0)} tokens</span>
                  <span style={{ margin: '0 8px' }}>·</span>
                  <span>保留 {data.compression.kept_messages || 0} 条消息</span>
                  {data.compression.original_tokens && data.compression.total_tokens && (
                    <span style={{ margin: '0 8px' }}>
                      · 压缩比 {((data.compression.total_tokens / data.compression.original_tokens) * 100).toFixed(0)}%
                    </span>
                  )}
                </div>
                {data.compression.summary && (
                  <div style={{
                    marginTop: 6,
                    padding: '6px 10px',
                    background: 'rgba(114, 46, 209, 0.05)',
                    borderRadius: 6,
                    fontSize: 12,
                    color: '#666',
                    maxHeight: 80,
                    overflow: 'auto',
                    border: '1px solid rgba(114, 46, 209, 0.15)',
                  }}>
                    {data.compression.summary.length > 300
                      ? data.compression.summary.slice(0, 300) + '...'
                      : data.compression.summary}
                  </div>
                )}
              </div>
            </>
          )}

          {/* === Segment type summary tags === */}
          <Divider style={{ margin: '12px 0' }} />
          <div style={{ marginBottom: 12 }}>
            <Text strong style={{ fontSize: 13 }}>分段统计</Text>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
              {segmentStats.map(s => {
                const meta = segmentTypeLabels[s.type] || { label: s.type, color: 'default', icon: <MessageOutlined /> };
                return (
                  <Tooltip
                    key={s.type}
                    title={`${s.count} 条 · ${formatTokenCount(s.tokens)} tokens · ${formatBytes(s.chars)} · 有效 ${s.inContextCount} 条 · Budget ${s.budgetedCount} 条`}
                  >
                    <Tag icon={meta.icon} color={meta.color} style={{ cursor: 'pointer' }}>
                      {meta.label}: {s.count} ({formatTokenCount(s.tokens)})
                    </Tag>
                  </Tooltip>
                );
              })}
            </div>
          </div>

          {/* === Segment list === */}
          <Divider style={{ margin: '12px 0' }} />
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text strong style={{ fontSize: 13 }}>消息列表</Text>
              {compressedHistoryCount > 0 && (
                <span
                  style={{ fontSize: 12, color: '#1677ff', cursor: 'pointer', userSelect: 'none' }}
                  onClick={() => setShowCompressedHistory(v => !v)}
                >
                  {showCompressedHistory
                    ? `隐藏已压缩历史 (${compressedHistoryCount})`
                    : `显示已压缩历史 (${compressedHistoryCount})`}
                </span>
              )}
            </div>
            <div>
              {visibleSegments.map((seg, idx) => (
                <SegmentRow key={idx} segment={seg} contextWindow={data.context_window} />
              ))}
            </div>
          </div>
        </>
      )}
    </Modal>
  );
};

// Individual segment row
const SegmentRow: React.FC<{ segment: ContextSegment; contextWindow: number }> = ({ segment, contextWindow }) => {
  const meta = segmentTypeLabels[segment.type] || { label: segment.type, color: 'default', icon: <MessageOutlined /> };
  const pct = contextWindow > 0 ? ((segment.estimated_tokens / contextWindow) * 100) : 0;

  // Determine visual state:
  //   in_context + budgeted  → full opacity, green check
  //   in_context + !budgeted → medium opacity, orange minus (budget-truncated)
  //   !in_context            → low opacity, grey eye-invisible (compressed away)
  let statusIcon: React.ReactNode;
  let statusTip: string;
  let opacity: number;
  let bg: string;

  if (!segment.in_context) {
    statusIcon = <EyeInvisibleOutlined style={{ color: '#ccc', fontSize: 11 }} />;
    statusTip = '已压缩（不在有效上下文中）';
    opacity = 0.4;
    bg = 'rgba(0,0,0,0.02)';
  } else if (!segment.budgeted) {
    statusIcon = <MinusCircleOutlined style={{ color: '#faad14', fontSize: 11 }} />;
    statusTip = '在有效上下文中，但被 BudgetMessages 裁剪';
    opacity = 0.65;
    bg = 'rgba(250,173,20,0.04)';
  } else {
    statusIcon = <CheckCircleOutlined style={{ color: '#52c41a', fontSize: 11 }} />;
    statusTip = '在 Budget 内（会发送给 LLM）';
    opacity = 1;
    bg = 'transparent';
  }

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      padding: '5px 8px',
      borderRadius: 6,
      marginBottom: 2,
      background: bg,
      opacity,
      fontSize: 12,
    }}>
      {/* Status indicator */}
      <Tooltip title={statusTip}>
        {statusIcon}
      </Tooltip>

      {/* Role badge */}
      <span style={{
        width: 50,
        textAlign: 'center',
        color: roleColors[segment.role || ''] || '#999',
        fontWeight: 500,
        flexShrink: 0,
      }}>
        {segment.role || '-'}
      </span>

      {/* Type tag */}
      <Tag color={meta.color} style={{ fontSize: 10, lineHeight: '16px', padding: '0 4px', margin: 0, flexShrink: 0 }}>
        {meta.label}
      </Tag>

      {/* Tool calls indicator */}
      {segment.has_tool_calls && (
        <Tooltip title={`${segment.tool_call_count} 个工具调用`}>
          <ToolOutlined style={{ color: '#1890ff', fontSize: 11, flexShrink: 0 }} />
        </Tooltip>
      )}

      {/* Content preview */}
      <Tooltip title={segment.content_preview}>
        <span style={{
          flex: 1,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          color: '#888',
        }}>
          {segment.content_preview || '(空)'}
        </span>
      </Tooltip>

      {/* Token count */}
      <span style={{ flexShrink: 0, color: '#aaa', minWidth: 52, textAlign: 'right' }}>
        {formatTokenCount(segment.estimated_tokens)}
        {pct >= 1 && <span style={{ marginLeft: 2, fontSize: 10 }}>({pct.toFixed(0)}%)</span>}
      </span>
    </div>
  );
};
