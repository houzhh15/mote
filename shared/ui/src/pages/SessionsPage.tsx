// ================================================================
// SessionsPage - Shared sessions management page
// ================================================================

import React, { useState, useEffect, useMemo } from 'react';
import { Typography, List, Card, Button, Space, Spin, Empty, message, Modal, Tag, theme, Input, Checkbox, Segmented } from 'antd';
import { DeleteOutlined, MessageOutlined, ClockCircleOutlined, GithubOutlined, SearchOutlined, RobotOutlined, CheckSquareOutlined, CloseOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import { OllamaIcon } from '../components/OllamaIcon';
import { MinimaxIcon } from '../components/MinimaxIcon';
import { GlmIcon } from '../components/GlmIcon';
import type { Session } from '../types';

const { Text } = Typography;

// Helper function to extract provider from model ID
const getProviderFromModel = (model?: string): 'copilot' | 'ollama' | 'minimax' | 'glm' | null => {
  if (!model) return null;
  if (model.startsWith('ollama:')) return 'ollama';
  if (model.startsWith('minimax:')) return 'minimax';
  if (model.startsWith('glm:')) return 'glm';
  return 'copilot';
};

// Source filter options
type SourceFilter = 'all' | 'chat' | 'cron' | 'delegate';

interface SessionsPageProps {
  onSelectSession?: (sessionId: string) => void;
}

export const SessionsPage: React.FC<SessionsPageProps> = ({ onSelectSession }) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('all');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);
  const [batchDeleting, setBatchDeleting] = useState(false);

  const fetchSessions = async () => {
    setLoading(true);
    try {
      const data = await api.getSessions();
      setSessions(data);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
      message.error('获取会话列表失败');
    } finally {
      setLoading(false);
    }
  };

  const deleteSession = async (id: string) => {
    Modal.confirm({
      title: <div style={{ color: token.colorText }}>确认删除</div>,
      content: <div style={{ color: token.colorText }}>删除后无法恢复，确定要删除这个会话吗？</div>,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.deleteSession(id);
          message.success('删除成功');
          fetchSessions();
        } catch (error) {
          console.error('Failed to delete session:', error);
          message.error('删除失败');
        }
      },
    });
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    Modal.confirm({
      title: <div style={{ color: token.colorText }}>批量删除</div>,
      content: <div style={{ color: token.colorText }}>确定要删除选中的 {selectedIds.size} 个会话吗？此操作不可恢复。</div>,
      okText: `删除 ${selectedIds.size} 个`,
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setBatchDeleting(true);
        try {
          if (api.batchDeleteSessions) {
            const result = await api.batchDeleteSessions(Array.from(selectedIds));
            message.success(`已删除 ${result.deleted} 个会话`);
          } else {
            // Fallback: delete one by one
            let deleted = 0;
            for (const id of selectedIds) {
              try {
                await api.deleteSession(id);
                deleted++;
              } catch { /* continue */ }
            }
            message.success(`已删除 ${deleted} 个会话`);
          }
          setSelectedIds(new Set());
          setSelectMode(false);
          fetchSessions();
        } catch (error) {
          console.error('Batch delete failed:', error);
          message.error('批量删除失败');
        } finally {
          setBatchDeleting(false);
        }
      },
    });
  };

  // Filtered sessions
  const filteredSessions = useMemo(() => {
    return sessions.filter(s => {
      // Source filter
      if (sourceFilter !== 'all') {
        const source = s.source || 'chat';
        if (source !== sourceFilter) return false;
      }
      // Text search
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const title = (s.title || '').toLowerCase();
        const preview = (s.preview || '').toLowerCase();
        const model = (s.model || '').toLowerCase();
        if (!title.includes(query) && !preview.includes(query) && !model.includes(query)) return false;
      }
      return true;
    });
  }, [sessions, sourceFilter, searchQuery]);

  // Count by source
  const sourceCounts = useMemo(() => {
    const counts = { all: sessions.length, chat: 0, cron: 0, delegate: 0 };
    for (const s of sessions) {
      const source = (s.source || 'chat') as 'chat' | 'cron' | 'delegate';
      if (counts[source] !== undefined) counts[source]++;
    }
    return counts;
  }, [sessions]);

  // Toggle selection
  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // Select all visible
  const selectAllVisible = () => {
    const allVisible = new Set(filteredSessions.map(s => s.id));
    setSelectedIds(allVisible);
  };

  // Exit select mode
  const exitSelectMode = () => {
    setSelectMode(false);
    setSelectedIds(new Set());
  };

  useEffect(() => {
    fetchSessions();
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          {/* Source filter */}
          <Segmented
            size="small"
            value={sourceFilter}
            onChange={(v) => setSourceFilter(v as SourceFilter)}
            options={[
              { label: `全部 (${sourceCounts.all})`, value: 'all' },
              { label: `对话 (${sourceCounts.chat})`, value: 'chat' },
              { label: `定时 (${sourceCounts.cron})`, value: 'cron' },
              { label: `子代理 (${sourceCounts.delegate})`, value: 'delegate' },
            ]}
          />
          <Space>
            <Input
              placeholder="搜索会话..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              style={{ width: 220 }}
              allowClear
              prefix={<SearchOutlined style={{ color: token.colorTextSecondary }} />}
            />
            {!selectMode ? (
              <Button
                size="small"
                icon={<CheckSquareOutlined />}
                onClick={() => setSelectMode(true)}
              >
                多选
              </Button>
            ) : (
              <Space>
                <Button size="small" onClick={selectAllVisible}>全选</Button>
                <Button
                  size="small"
                  type="primary"
                  danger
                  icon={<DeleteOutlined />}
                  disabled={selectedIds.size === 0}
                  loading={batchDeleting}
                  onClick={handleBatchDelete}
                >
                  删除 ({selectedIds.size})
                </Button>
                <Button size="small" icon={<CloseOutlined />} onClick={exitSelectMode}>取消</Button>
              </Space>
            )}
          </Space>
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ maxWidth: 900 }}>
      <Spin spinning={loading}>
        {filteredSessions.length === 0 ? (
          <Empty description={sessions.length === 0 ? "暂无会话记录" : "没有匹配的会话"} />
        ) : (
          <List
            dataSource={filteredSessions}
            renderItem={(session) => (
              <Card
                size="small"
                style={{
                  marginBottom: 12,
                  cursor: selectMode ? 'pointer' : (onSelectSession ? 'pointer' : 'default'),
                  borderColor: selectedIds.has(session.id) ? token.colorPrimary : undefined,
                  background: selectedIds.has(session.id) ? token.colorPrimaryBg : undefined,
                }}
                onClick={() => {
                  if (selectMode) {
                    toggleSelect(session.id);
                  } else if (onSelectSession) {
                    onSelectSession(session.id);
                  }
                }}
                hoverable={selectMode || !!onSelectSession}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  {/* Checkbox in select mode */}
                  {selectMode && (
                    <Checkbox
                      checked={selectedIds.has(session.id)}
                      onChange={() => toggleSelect(session.id)}
                      onClick={(e) => e.stopPropagation()}
                      style={{ marginRight: 12 }}
                    />
                  )}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4, flexWrap: 'wrap', gap: 4 }}>
                      <MessageOutlined style={{ marginRight: 8, flexShrink: 0 }} />
                      <Text strong ellipsis style={{ flex: 1, minWidth: 0 }}>
                        {session.title || session.preview || `会话 ${session.id.slice(0, 8)}`}
                      </Text>
                      {/* Source tags */}
                      {(session.source === 'cron' || session.scenario === 'cron') && (
                        <Tag color="cyan" style={{ fontSize: 11, lineHeight: '18px' }}>
                          <ClockCircleOutlined style={{ marginRight: 2 }} />定时
                        </Tag>
                      )}
                      {session.source === 'delegate' && (
                        <Tag color="geekblue" style={{ fontSize: 11, lineHeight: '18px' }}>
                          <RobotOutlined style={{ marginRight: 2 }} />子代理
                        </Tag>
                      )}
                      {session.model && (
                        <Tag 
                          color={getProviderFromModel(session.model) === 'ollama' ? 'orange' : getProviderFromModel(session.model) === 'minimax' ? 'purple' : getProviderFromModel(session.model) === 'glm' ? 'cyan' : 'blue'}
                          style={{ display: 'flex', alignItems: 'center', gap: 4 }}
                        >
                          {getProviderFromModel(session.model) === 'ollama' ? <OllamaIcon size={10} /> : getProviderFromModel(session.model) === 'minimax' ? <MinimaxIcon size={10} /> : getProviderFromModel(session.model) === 'glm' ? <GlmIcon size={10} /> : <GithubOutlined style={{ fontSize: 10 }} />}
                          {session.model}
                        </Tag>
                      )}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        <ClockCircleOutlined style={{ marginRight: 4 }} />
                        {new Date(session.updated_at).toLocaleString()}
                      </Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {session.message_count || 0} 条消息
                      </Text>
                    </div>
                  </div>
                  {!selectMode && (
                    <Space>
                      <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={(e) => {
                          e.stopPropagation();
                          deleteSession(session.id);
                        }}
                      />
                    </Space>
                  )}
                </div>
              </Card>
            )}
          />
        )}
      </Spin>
        </div>
      </div>
    </div>
  );
};
