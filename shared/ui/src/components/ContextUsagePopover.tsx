// ================================================================
// ContextUsagePopover - Compact context window usage indicator
// ================================================================

import React, { useMemo, useState } from 'react';
import { Popover, Progress, Typography } from 'antd';
import { DatabaseOutlined, BarChartOutlined } from '@ant-design/icons';
import type { Message, Model } from '../types';
import { ContextDetailModal } from './ContextDetailModal';

const { Text } = Typography;

/** Estimate token count: ~3 chars/token heuristic */
function estimateTokens(content: string): number {
  if (!content) return 0;
  return Math.ceil((content.length + 2) / 3);
}

interface ContextStats {
  conversationTokens: number;  // user + assistant content + tool call args
  toolResultTokens: number;    // tool_calls[].result
  totalTokens: number;
  contextWindow: number;
}

function computeContextStats(
  messages: Message[],
  model: Model | undefined,
  streamingTokens: number,
): ContextStats {
  let conversationTokens = 0;
  let toolResultTokens = 0;

  for (const msg of messages) {
    // Message content (user + assistant)
    if (msg.content) {
      conversationTokens += estimateTokens(msg.content);
    }
    // Per-message overhead (~4 tokens for role/keys)
    conversationTokens += 4;

    // Tool calls
    if (msg.tool_calls) {
      for (const tc of msg.tool_calls) {
        if (tc.arguments) {
          conversationTokens += estimateTokens(tc.arguments);
        }
        if (tc.result) {
          const resultStr = typeof tc.result === 'string' ? tc.result : JSON.stringify(tc.result);
          toolResultTokens += estimateTokens(resultStr);
        }
      }
    }
  }

  // Add streaming response tokens (current assistant reply being generated)
  conversationTokens += streamingTokens;

  const totalTokens = conversationTokens + toolResultTokens;
  const contextWindow = model?.context_window || 128000;

  return { conversationTokens, toolResultTokens, totalTokens, contextWindow };
}

function formatTokenCount(tokens: number): string {
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)}K`;
  }
  return `${tokens}`;
}

interface ContextUsagePopoverProps {
  messages: Message[];
  currentModel: string;
  models: Model[];
  streamingTokens?: number;
  backendEstimatedTokens?: number;  // Backend-computed token estimate (accurate, byte-based)
  sessionId?: string;               // Session ID for context detail API
  style?: React.CSSProperties;
}

export const ContextUsagePopover: React.FC<ContextUsagePopoverProps> = ({
  messages,
  currentModel,
  models,
  streamingTokens = 0,
  backendEstimatedTokens,
  sessionId,
  style,
}) => {
  const [detailOpen, setDetailOpen] = useState(false);
  const model = useMemo(() => models.find(m => m.id === currentModel), [models, currentModel]);

  // Use backend-provided token estimate when:
  // 1. Available (> 0)
  // 2. NOT currently streaming (during streaming, backend count is stale)
  // During streaming, fall back to local estimation for real-time updates
  const useBackend = backendEstimatedTokens && backendEstimatedTokens > 0 && streamingTokens === 0;

  const stats = useMemo(() => {
    if (useBackend) {
      // Use backend tokens as totalTokens; break down is approximate
      const contextWindow = model?.context_window || 128000;
      return {
        conversationTokens: backendEstimatedTokens!,
        toolResultTokens: 0,
        totalTokens: backendEstimatedTokens!,
        contextWindow,
      };
    }
    return computeContextStats(messages, model, streamingTokens);
  }, [useBackend, backendEstimatedTokens, messages, model, streamingTokens]);

  const usagePercent = stats.contextWindow > 0
    ? Math.min((stats.totalTokens / stats.contextWindow) * 100, 100)
    : 0;

  const convPct = stats.contextWindow > 0 ? (stats.conversationTokens / stats.contextWindow) * 100 : 0;
  const toolResPct = stats.contextWindow > 0 ? (stats.toolResultTokens / stats.contextWindow) * 100 : 0;

  const getProgressColor = (pct: number): string => {
    if (pct >= 80) return '#ff4d4f';
    if (pct >= 60) return '#faad14';
    return '#1677ff';
  };

  const popoverContent = (
    <div style={{ width: 220, padding: '2px 0' }}>
      <div style={{ marginBottom: 4 }}>
        <Text strong style={{ fontSize: 13 }}>Context Window</Text>
        <div style={{ fontSize: 12, color: '#888' }}>
          {formatTokenCount(stats.totalTokens)} / {formatTokenCount(stats.contextWindow)} · {usagePercent.toFixed(0)}%
        </div>
      </div>
      <Progress
        percent={usagePercent}
        showInfo={false}
        strokeColor={getProgressColor(usagePercent)}
        trailColor="rgba(0,0,0,0.06)"
        size="small"
        style={{ marginBottom: 6 }}
      />
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#666' }}>
        <span>Messages</span><span>{convPct.toFixed(1)}%</span>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#666' }}>
        <span>Tool Results</span><span>{toolResPct.toFixed(1)}%</span>
      </div>
      {sessionId && (
        <div
          style={{
            marginTop: 6,
            paddingTop: 6,
            borderTop: '1px solid rgba(0,0,0,0.06)',
            textAlign: 'center',
          }}
        >
          <span
            onClick={(e) => { e.stopPropagation(); setDetailOpen(true); }}
            style={{ fontSize: 11, color: '#1677ff', cursor: 'pointer', userSelect: 'none' }}
          >
            <BarChartOutlined style={{ marginRight: 3 }} />
            查看详情
          </span>
        </div>
      )}
    </div>
  );

  return (
    <>
      <Popover content={popoverContent} trigger="hover" placement="topLeft" overlayInnerStyle={{ borderRadius: 8 }}>
        <div
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 3,
            cursor: 'pointer',
            padding: '0 4px',
            fontSize: 11,
            color: '#999',
            lineHeight: '16px',
            ...style,
          }}
        >
          <DatabaseOutlined style={{ fontSize: 10 }} />
          <span>{formatTokenCount(stats.totalTokens)}</span>
          <Progress
            percent={usagePercent}
            showInfo={false}
            strokeColor={getProgressColor(usagePercent)}
            trailColor="rgba(0,0,0,0.06)"
            size={[36, 3]}
            style={{ margin: 0 }}
          />
        </div>
      </Popover>
      <ContextDetailModal
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        sessionId={sessionId}
      />
    </>
  );
};
