// ================================================================
// MemoryPage - Shared memory management page with grouping support
// Performance: Each tab is a separate React.memo component to avoid
// cross-tab re-renders when state changes in one tab.
// ================================================================

import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { Typography, Tag, Button, Space, Input, Spin, Empty, message, Modal, Form, Select, theme, Tooltip, Badge, Pagination, Tabs, Statistic, Row, Col, Card } from 'antd';
import { SearchOutlined, ReloadOutlined, DeleteOutlined, EyeOutlined, PlusOutlined, EditOutlined, FileOutlined, FolderOutlined, DownOutlined, RightOutlined, SyncOutlined, BarChartOutlined, CalendarOutlined, ExportOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Memory } from '../types';
import type { MemoryStats, MemorySyncResult, APIAdapter } from '../services/adapter';

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

const CATEGORIES = [
  { value: 'preference', label: '偏好' },
  { value: 'fact', label: '事实' },
  { value: 'decision', label: '决策' },
  { value: 'entity', label: '实体' },
  { value: 'other', label: '其他' },
];

// === Stable style constants (avoid re-creating objects each render) ===
const STYLES = {
  searchInput: { width: 260 } as React.CSSProperties,
  filterSelect: { width: 120 } as React.CSSProperties,
  maxWidth900: { maxWidth: 900 } as React.CSSProperties,
  fullWidth: { width: '100%' } as React.CSSProperties,
  flexColumn: { display: 'flex', flexDirection: 'column', height: '100%' } as React.CSSProperties,
  listHeader: { padding: '12px 0', flexShrink: 0 } as React.CSSProperties,
  listHeaderInner: { display: 'flex', justifyContent: 'space-between', alignItems: 'center' } as React.CSSProperties,
  listContent: { flex: 1, overflow: 'auto' } as React.CSSProperties,
  paginationWrap: { display: 'flex', justifyContent: 'center', marginTop: 24, paddingBottom: 16 } as React.CSSProperties,
  chunkContainer: { marginLeft: 24, marginBottom: 16 } as React.CSSProperties,
  fontSize12: { fontSize: 12 } as React.CSSProperties,
  mb8: { marginBottom: 8 } as React.CSSProperties,
  mb16: { marginBottom: 16 } as React.CSSProperties,
  mb24: { marginBottom: 24 } as React.CSSProperties,
  mt8Block: { marginTop: 8, display: 'block' } as React.CSSProperties,
  mt16: { marginTop: 16 } as React.CSSProperties,
  mb16Block: { marginBottom: 16, display: 'block' } as React.CSSProperties,
  preWrap: { whiteSpace: 'pre-wrap', marginBottom: 0 } as React.CSSProperties,
  preWrapScroll: { whiteSpace: 'pre-wrap', maxHeight: 400, overflow: 'auto' } as React.CSSProperties,
  textEllipsis: { maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block' } as React.CSSProperties,
  contentEllipsis: { maxWidth: 400 } as React.CSSProperties,
  flex1: { flex: 1 } as React.CSSProperties,
  flexBetween: { display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' } as React.CSSProperties,
  pageRoot: { display: 'flex', flexDirection: 'column', height: '100%' } as React.CSSProperties,
  pageContent: { flex: 1, overflow: 'auto', padding: '0 24px 24px' } as React.CSSProperties,
  tabsFull: { height: '100%' } as React.CSSProperties,
  searchPrefix: { color: '#bfbfbf' } as React.CSSProperties,
  folderIcon: { marginRight: 8 } as React.CSSProperties,
  codeBlock: { background: 'rgba(0,0,0,0.04)', padding: '8px 12px', borderRadius: 6, fontFamily: 'monospace', fontSize: 13, wordBreak: 'break-all' } as React.CSSProperties,
} as const;

const CATEGORY_FILTER_OPTIONS = [{ value: null, label: '全部分类' }, ...CATEGORIES];

// === Category helpers (module-level, no re-creation) ===
const CATEGORY_COLORS: Record<string, string> = {
  preference: 'blue', fact: 'green', decision: 'orange', entity: 'purple', other: 'default',
};
function getCategoryColor(category: string): string {
  return CATEGORY_COLORS[category] || 'default';
}
function getCategoryLabel(category: string): string {
  const found = CATEGORIES.find(c => c.value === category);
  return found?.label || category;
}

// === Memory group type ===
interface MemoryGroup {
  key: string;
  sourceFile?: string;
  baseId: string;
  memories: Memory[];
  totalChunks: number;
  category: string;
  created_at: string;
}

function getBaseId(id: string): string {
  const match = id.match(/^(.+)-chunk-\d+$/);
  return match ? match[1] : id;
}

function groupMemories(memories: Memory[]): MemoryGroup[] {
  const groups = new Map<string, MemoryGroup>();
  for (const memory of memories) {
    const baseId = getBaseId(memory.id);
    const groupKey = memory.source_file || baseId;
    if (!groups.has(groupKey)) {
      groups.set(groupKey, {
        key: groupKey, sourceFile: memory.source_file, baseId,
        memories: [], totalChunks: memory.chunk_total || 1,
        category: memory.category, created_at: memory.created_at,
      });
    }
    const group = groups.get(groupKey)!;
    group.memories.push(memory);
    if (memory.created_at < group.created_at) group.created_at = memory.created_at;
  }
  for (const group of groups.values()) {
    group.memories.sort((a, b) => (a.chunk_index ?? 0) - (b.chunk_index ?? 0));
  }
  return Array.from(groups.values()).sort((a, b) =>
    new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  );
}

// ================================================================
// Memory List Tab (separate component to isolate re-renders)
// ================================================================
interface MemoryListTabProps {
  api: APIAdapter;
  colorBgContainer: string;
  borderRadius: number;
  colorBorderSecondary: string;
  colorTextSecondary: string;
  colorPrimary: string;
  colorBgTextHover: string;
}

const MemoryListTab = React.memo<MemoryListTabProps>(({ api, colorBgContainer, borderRadius, colorBorderSecondary, colorTextSecondary, colorPrimary, colorBgTextHover }) => {
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [filterCategory, setFilterCategory] = useState<string | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(100);
  const [total, setTotal] = useState(0);

  // Modal state
  const [viewModalVisible, setViewModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [selectedMemory, setSelectedMemory] = useState<Memory | null>(null);
  const [form] = Form.useForm();

  const canCreate = typeof api.createMemory === 'function';
  const canUpdate = typeof api.updateMemory === 'function';
  const canExport = typeof api.exportMemories === 'function';

  const fetchMemories = useCallback(async (page = 1, size = 100) => {
    setLoading(true);
    try {
      const offset = (page - 1) * size;
      const data = await api.getMemories({ limit: size, offset });
      setMemories(data.memories);
      setTotal(data.total);
    } catch (error) {
      console.error('Failed to fetch memories:', error);
      message.error('获取记忆失败');
    } finally {
      setLoading(false);
    }
  }, [api]);

  // Single initial load + debounced search
  const initialLoadDone = useRef(false);
  useEffect(() => {
    if (!initialLoadDone.current) {
      initialLoadDone.current = true;
      fetchMemories(1, pageSize);
    }
  }, [fetchMemories, pageSize]);

  // Debounced search — skip initial empty string to avoid duplicate load
  useEffect(() => {
    if (!initialLoadDone.current) return;
    const timer = setTimeout(() => {
      if (!searchQuery.trim()) {
        fetchMemories(1, pageSize);
      } else {
        setLoading(true);
        api.searchMemories(searchQuery, 500).then(data => {
          setMemories(data);
          setTotal(data.length);
          setCurrentPage(1);
        }).catch(error => {
          console.error('Failed to search memories:', error);
          message.error('搜索记忆失败');
        }).finally(() => setLoading(false));
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [searchQuery, api, fetchMemories, pageSize]);

  const handlePageChange = useCallback((page: number, size?: number) => {
    const newSize = size || pageSize;
    setCurrentPage(page);
    if (size && size !== pageSize) setPageSize(size);
    if (!searchQuery.trim()) fetchMemories(page, newSize);
  }, [pageSize, searchQuery, fetchMemories]);

  const deleteMemory = useCallback((id: string) => {
    Modal.confirm({
      title: '确认删除',
      content: '删除后无法恢复，确定要删除这条记忆吗？',
      onOk: async () => {
        try {
          await api.deleteMemory(id);
          message.success('删除成功');
          fetchMemories(currentPage, pageSize);
        } catch (error) {
          console.error('Failed to delete memory:', error);
          message.error('删除失败');
        }
      },
    });
  }, [api, fetchMemories, currentPage, pageSize]);

  const deleteGroup = useCallback((group: MemoryGroup) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除这组记忆吗？共 ${group.memories.length} 个分片。`,
      onOk: async () => {
        try {
          for (const memory of group.memories) await api.deleteMemory(memory.id);
          message.success('删除成功');
          fetchMemories(currentPage, pageSize);
        } catch (error) {
          console.error('Failed to delete group:', error);
          message.error('删除失败');
        }
      },
    });
  }, [api, fetchMemories, currentPage, pageSize]);

  const handleSave = useCallback(async (values: { content: string; category: string }) => {
    try {
      if (selectedMemory && canUpdate) {
        await api.updateMemory!(selectedMemory.id, values.content, values.category);
        message.success('更新成功');
      } else if (canCreate) {
        await api.createMemory!(values.content, values.category);
        message.success('添加成功');
      }
      setEditModalVisible(false);
      form.resetFields();
      setSearchQuery('');
      fetchMemories(1, pageSize);
    } catch (error) {
      console.error('Failed to save memory:', error);
      message.error(selectedMemory ? '更新失败' : '添加失败');
    }
  }, [api, selectedMemory, canUpdate, canCreate, form, fetchMemories, pageSize]);

  const handleExport = useCallback(async () => {
    if (!canExport) return;
    try {
      const data = await api.exportMemories!();
      const jsonStr = JSON.stringify(data, null, 2);
      const blob = new Blob([jsonStr], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `memory-export-${new Date().toISOString().slice(0, 10)}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      // Fallback: if download didn't trigger (e.g. in Wails WebView), copy to clipboard
      setTimeout(() => {
        URL.revokeObjectURL(url);
      }, 1000);
      message.success(`导出 ${data.count} 条记忆（已下载到浏览器）`);
    } catch (error) {
      console.error('Export failed:', error);
      message.error('导出失败');
    }
  }, [api, canExport]);

  const filteredMemories = useMemo(() => {
    if (!filterCategory) return memories;
    return memories.filter(m => m.category === filterCategory);
  }, [memories, filterCategory]);

  const memoryGroups = useMemo(() => groupMemories(filteredMemories), [filteredMemories]);

  const toggleGroup = useCallback((key: string) => {
    setExpandedGroups(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  }, []);

  // Stable group header style (depends on theme tokens)
  const groupHeaderStyle = useMemo(() => ({
    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
    padding: '12px 16px', background: colorBgContainer,
    borderRadius, border: `1px solid ${colorBorderSecondary}`, marginBottom: 8,
  } as React.CSSProperties), [colorBgContainer, borderRadius, colorBorderSecondary]);

  const chunkItemStyle = useMemo(() => ({
    padding: '12px 16px', background: colorBgTextHover,
    borderRadius, marginBottom: 8, borderLeft: `3px solid ${colorPrimary}`,
  } as React.CSSProperties), [colorBgTextHover, borderRadius, colorPrimary]);

  return (
    <>
      <div style={STYLES.flexColumn}>
        {/* Header */}
        <div style={STYLES.listHeader}>
          <div style={STYLES.listHeaderInner}>
            <Space>
              <Input placeholder="搜索记忆..." value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)} style={STYLES.searchInput}
                allowClear prefix={<SearchOutlined style={STYLES.searchPrefix} />} />
              <Select placeholder="分类筛选" value={filterCategory}
                onChange={setFilterCategory} allowClear style={STYLES.filterSelect}
                options={CATEGORY_FILTER_OPTIONS} />
              <Button icon={<ReloadOutlined />} onClick={() => fetchMemories(currentPage, pageSize)} className="page-header-btn">刷新</Button>
            </Space>
            <Space>
              {canExport && <Button icon={<ExportOutlined />} onClick={handleExport} className="page-header-btn">导出</Button>}
              {canCreate && <Button icon={<PlusOutlined />} onClick={() => { setSelectedMemory(null); form.resetFields(); form.setFieldsValue({ category: 'other' }); setEditModalVisible(true); }} className="page-header-btn">添加记忆</Button>}
            </Space>
          </div>
        </div>

        {/* Content */}
        <div style={STYLES.listContent}>
          <div style={STYLES.maxWidth900}>
            <Spin spinning={loading}>
              {memoryGroups.length === 0 ? <Empty description="暂无记忆" /> : (
                <div>
                  {memoryGroups.map(group => {
                    const isExpanded = expandedGroups.has(group.key);
                    const hasChunks = group.memories.length > 1 || (group.memories[0]?.chunk_total ?? 0) > 1;
                    return (
                      <div key={group.key}>
                        {/* Group Header */}
                        <div style={{ ...groupHeaderStyle, cursor: hasChunks ? 'pointer' : 'default' }}
                          onClick={() => hasChunks && toggleGroup(group.key)}>
                          <Space>
                            {hasChunks ? (isExpanded ? <DownOutlined style={STYLES.fontSize12} /> : <RightOutlined style={STYLES.fontSize12} />) : <FileOutlined style={{ color: colorTextSecondary }} />}
                            <div>
                              {group.sourceFile ? (
                                <Tooltip title={group.sourceFile}>
                                  <Text strong style={STYLES.textEllipsis}><FolderOutlined style={STYLES.folderIcon} />{group.sourceFile.split('/').pop()}</Text>
                                </Tooltip>
                              ) : <Text ellipsis style={STYLES.contentEllipsis}>{group.memories[0]?.content.slice(0, 60)}...</Text>}
                            </div>
                            <Tag color={getCategoryColor(group.category)}>{getCategoryLabel(group.category)}</Tag>
                            {hasChunks && <Badge count={`${group.memories.length}/${group.totalChunks} 分片`} style={{ backgroundColor: colorPrimary }} />}
                          </Space>
                          <Space size="small" onClick={e => e.stopPropagation()}>
                            <Text type="secondary" style={STYLES.fontSize12}>{new Date(group.created_at).toLocaleDateString()}</Text>
                            {!hasChunks && (
                              <>
                                <Button type="text" size="small" icon={<EyeOutlined />} onClick={() => { setSelectedMemory(group.memories[0]); setViewModalVisible(true); }} title="查看" />
                                {canUpdate && <Button type="text" size="small" icon={<EditOutlined />} onClick={() => { setSelectedMemory(group.memories[0]); form.setFieldsValue({ content: group.memories[0].content, category: group.memories[0].category }); setEditModalVisible(true); }} title="编辑" />}
                              </>
                            )}
                            <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => hasChunks ? deleteGroup(group) : deleteMemory(group.memories[0].id)} title="删除" />
                          </Space>
                        </div>
                        {/* Chunk List */}
                        {isExpanded && (
                          <div style={STYLES.chunkContainer}>
                            {group.memories.map((mem, index) => (
                              <div key={mem.id} style={chunkItemStyle}>
                                <div style={STYLES.flexBetween}>
                                  <div style={STYLES.flex1}>
                                    <Space style={STYLES.mb8}>
                                      <Tag>分片 {(mem.chunk_index ?? index) + 1}/{mem.chunk_total || group.memories.length}</Tag>
                                      {mem.relevance !== undefined && <Tag color="gold">相关度: {(mem.relevance * 100).toFixed(0)}%</Tag>}
                                    </Space>
                                    <Paragraph ellipsis={{ rows: 3, expandable: true, symbol: '展开' }} style={STYLES.mb8}>{mem.content}</Paragraph>
                                    <Text type="secondary" style={STYLES.fontSize12}>{new Date(mem.created_at).toLocaleString()}</Text>
                                  </div>
                                  <Space size="small">
                                    <Button type="text" size="small" icon={<EyeOutlined />} onClick={() => { setSelectedMemory(mem); setViewModalVisible(true); }} title="查看" />
                                    {canUpdate && <Button type="text" size="small" icon={<EditOutlined />} onClick={() => { setSelectedMemory(mem); form.setFieldsValue({ content: mem.content, category: mem.category }); setEditModalVisible(true); }} title="编辑" />}
                                    <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => deleteMemory(mem.id)} title="删除" />
                                  </Space>
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })}
                  {!searchQuery.trim() && total > pageSize && (
                    <div style={STYLES.paginationWrap}>
                      <Pagination current={currentPage} pageSize={pageSize} total={total}
                        onChange={handlePageChange} showSizeChanger showQuickJumper
                        showTotal={(t, range) => `第 ${range[0]}-${range[1]} 条，共 ${t} 条记忆`}
                        pageSizeOptions={['50', '100', '200', '500']} />
                    </div>
                  )}
                </div>
              )}
            </Spin>
          </div>
        </div>
      </div>

      {/* View Modal */}
      <Modal
        title={<Space><span>记忆详情</span>{selectedMemory && <Tag color={getCategoryColor(selectedMemory.category)}>{getCategoryLabel(selectedMemory.category)}</Tag>}</Space>}
        open={viewModalVisible} onCancel={() => setViewModalVisible(false)} width={700}
        footer={[
          <Button key="close" onClick={() => setViewModalVisible(false)}>关闭</Button>,
          canUpdate && <Button key="edit" type="primary" onClick={() => { setViewModalVisible(false); if (selectedMemory) { form.setFieldsValue({ content: selectedMemory.content, category: selectedMemory.category }); setEditModalVisible(true); } }}>编辑</Button>,
          <Button key="delete" danger onClick={() => { if (selectedMemory) { setViewModalVisible(false); deleteMemory(selectedMemory.id); } }}>删除</Button>,
        ].filter(Boolean)}
      >
        {selectedMemory && (
          <div>
            <Paragraph style={STYLES.preWrapScroll}>{selectedMemory.content}</Paragraph>
            <div style={{ marginTop: 16, borderTop: `1px solid ${colorBorderSecondary}`, paddingTop: 16 }}>
              <Space direction="vertical" size="small">
                <Text type="secondary" style={STYLES.fontSize12}>创建时间: {new Date(selectedMemory.created_at).toLocaleString()}</Text>
                {selectedMemory.source_file && <Text type="secondary" style={STYLES.fontSize12}>来源文件: {selectedMemory.source_file}</Text>}
                {selectedMemory.chunk_total && selectedMemory.chunk_total > 1 && <Text type="secondary" style={STYLES.fontSize12}>分片信息: {(selectedMemory.chunk_index ?? 0) + 1} / {selectedMemory.chunk_total}</Text>}
                {selectedMemory.relevance !== undefined && <Text type="secondary" style={STYLES.fontSize12}>相关度: {(selectedMemory.relevance * 100).toFixed(0)}%</Text>}
                {selectedMemory.importance !== undefined && <Text type="secondary" style={STYLES.fontSize12}>重要性: {(selectedMemory.importance * 100).toFixed(0)}%</Text>}
              </Space>
            </div>
          </div>
        )}
      </Modal>

      {/* Edit Modal */}
      <Modal title={selectedMemory ? '编辑记忆' : '添加记忆'} open={editModalVisible}
        onCancel={() => { setEditModalVisible(false); form.resetFields(); }}
        onOk={() => form.submit()} okText={selectedMemory ? '保存' : '添加'} cancelText="取消">
        <Form form={form} layout="vertical" onFinish={handleSave}>
          <Form.Item name="content" label="内容" rules={[{ required: true, message: '请输入记忆内容' }]}>
            <TextArea rows={4} placeholder="输入记忆内容..." />
          </Form.Item>
          <Form.Item name="category" label="分类" rules={[{ required: true, message: '请选择分类' }]}>
            <Select options={CATEGORIES} placeholder="选择分类" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
});
MemoryListTab.displayName = 'MemoryListTab';

// ================================================================
// Stats Tab
// ================================================================
interface StatsTabProps {
  api: APIAdapter;
}

const StatsTab = React.memo<StatsTabProps>(({ api }) => {
  const [stats, setStats] = useState<MemoryStats | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchStats = useCallback(async () => {
    if (typeof api.getMemoryStats !== 'function') return;
    setLoading(true);
    try {
      const data = await api.getMemoryStats();
      setStats(data);
    } catch (error) {
      console.error('Failed to fetch stats:', error);
      message.error('获取统计失败');
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => { fetchStats(); }, [fetchStats]);

  return (
    <Spin spinning={loading}>
      <div style={STYLES.maxWidth900}>
        {stats ? (
          <>
            <Row gutter={[16, 16]} style={STYLES.mb24}>
              <Col span={6}><Card size="small"><Statistic title="总记忆数" value={stats.total} /></Card></Col>
              <Col span={6}><Card size="small"><Statistic title="今日自动捕获" value={stats.auto_capture_today ?? 0} /></Card></Col>
              <Col span={6}><Card size="small"><Statistic title="今日自动召回" value={stats.auto_recall_today ?? 0} /></Card></Col>
              <Col span={6}><Card size="small"><Statistic title="索引条目" value={stats.index_entries ?? stats.total} /></Card></Col>
            </Row>
            {stats.by_category && Object.keys(stats.by_category).length > 0 && (
              <Card title="按分类统计" size="small" style={STYLES.mb16}>
                <Space wrap>
                  {Object.entries(stats.by_category).map(([cat, count]) => (
                    <Tag key={cat} color={getCategoryColor(cat)}>{getCategoryLabel(cat)}: {count}</Tag>
                  ))}
                </Space>
              </Card>
            )}
            {stats.by_capture_method && Object.keys(stats.by_capture_method).length > 0 && (
              <Card title="按捕获方式统计" size="small" style={STYLES.mb16}>
                <Space wrap>
                  {Object.entries(stats.by_capture_method).map(([method, count]) => (
                    <Tag key={method}>{method}: {count}</Tag>
                  ))}
                </Space>
              </Card>
            )}
            <Button icon={<ReloadOutlined />} onClick={fetchStats}>刷新统计</Button>
          </>
        ) : (
          <Empty description="暂无统计数据" />
        )}
      </div>
    </Spin>
  );
});
StatsTab.displayName = 'StatsTab';

// ================================================================
// Sync Tab
// ================================================================
interface SyncTabProps {
  api: APIAdapter;
}

const SyncTab = React.memo<SyncTabProps>(({ api }) => {
  const [syncing, setSyncing] = useState(false);
  const [syncResult, setSyncResult] = useState<MemorySyncResult | null>(null);

  const handleSync = useCallback(async () => {
    if (typeof api.syncMemory !== 'function') return;
    setSyncing(true);
    setSyncResult(null);
    try {
      const result = await api.syncMemory();
      setSyncResult(result);
      message.success('同步完成');
    } catch (error) {
      console.error('Sync failed:', error);
      message.error('同步失败');
    } finally {
      setSyncing(false);
    }
  }, [api]);

  return (
    <div style={STYLES.maxWidth900}>
      <Card size="small" style={STYLES.mb16}>
        <Space direction="vertical" style={STYLES.fullWidth}>
          <Text>从 Markdown 文件同步记忆到向量索引。系统将读取以下文件并建立索引：</Text>
          <div style={STYLES.codeBlock}>
            <div>📄 长期记忆：<Text code copyable>~/.mote/MEMORY.md</Text></div>
            <div style={STYLES.mt16}>📁 每日日志：<Text code copyable>~/.mote/memory/YYYY-MM-DD.md</Text></div>
          </div>
          <Text type="secondary">提示：你可以直接编辑上述 Markdown 文件来管理记忆内容，然后点击同步按钮更新索引。</Text>
          <Button type="primary" icon={<SyncOutlined spin={syncing} />} loading={syncing} onClick={handleSync}>
            {syncing ? '同步中...' : '开始同步'}
          </Button>
        </Space>
      </Card>
      {syncResult && (
        <Card title="同步结果" size="small">
          <Row gutter={[16, 16]}>
            {syncResult.synced !== undefined && <Col span={6}><Statistic title="已同步" value={syncResult.synced} /></Col>}
            {syncResult.created !== undefined && <Col span={6}><Statistic title="新增" value={syncResult.created} /></Col>}
            {syncResult.updated !== undefined && <Col span={6}><Statistic title="更新" value={syncResult.updated} /></Col>}
            {syncResult.deleted !== undefined && <Col span={6}><Statistic title="删除" value={syncResult.deleted} /></Col>}
          </Row>
          {syncResult.errors !== undefined && syncResult.errors > 0 && <Text type="danger" style={STYLES.mt8Block}>错误数: {syncResult.errors}</Text>}
          {syncResult.duration && <Text type="secondary" style={STYLES.mt8Block}>耗时: {syncResult.duration}</Text>}
        </Card>
      )}
    </div>
  );
});
SyncTab.displayName = 'SyncTab';

// ================================================================
// Daily Log Tab
// ================================================================
interface DailyLogTabProps {
  api: APIAdapter;
}

const DailyLogTab = React.memo<DailyLogTabProps>(({ api }) => {
  const [content, setContent] = useState('');
  const [date, setDate] = useState('');
  const [loading, setLoading] = useState(false);
  const [appendContent, setAppendContent] = useState('');
  const [appendSection, setAppendSection] = useState('');

  const canAppend = typeof api.appendDailyLog === 'function';

  const fetchDailyLog = useCallback(async (targetDate?: string) => {
    if (typeof api.getDailyLog !== 'function') return;
    setLoading(true);
    try {
      const data = await api.getDailyLog(targetDate);
      setContent(data.content || '');
      setDate(data.date || '');
    } catch (error) {
      console.error('Failed to fetch daily log:', error);
      message.error('获取每日日志失败');
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => { fetchDailyLog(); }, [fetchDailyLog]);

  const handleAppend = useCallback(async () => {
    if (!canAppend || !appendContent.trim()) return;
    try {
      await api.appendDailyLog!(appendContent, appendSection || undefined);
      message.success('日志追加成功');
      setAppendContent('');
      setAppendSection('');
      fetchDailyLog();
    } catch (error) {
      console.error('Failed to append daily log:', error);
      message.error('日志追加失败');
    }
  }, [api, appendContent, appendSection, canAppend, fetchDailyLog]);

  return (
    <Spin spinning={loading}>
      <div style={STYLES.maxWidth900}>
        <Text type="secondary" style={STYLES.mb16Block}>
          每日日志会在对话过程中自动记录会话摘要。你也可以手动追加内容。
          {date && <>（当前日期: {date}）</>}
        </Text>
        <div style={STYLES.codeBlock}>
          📁 日志文件：<Text code copyable>~/.mote/memory/{date || 'YYYY-MM-DD'}.md</Text>
        </div>
        <Card size="small" style={{ marginTop: 16, marginBottom: 16 }}>
          {content ? (
            <Paragraph style={STYLES.preWrap}>{content}</Paragraph>
          ) : (
            <Empty description="今日暂无日志内容" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
        </Card>
        {canAppend && (
          <Card title="追加日志" size="small">
            <Space direction="vertical" style={STYLES.fullWidth}>
              <Input placeholder="分节标题（可选）" value={appendSection} onChange={e => setAppendSection(e.target.value)} />
              <TextArea rows={3} placeholder="输入日志内容..." value={appendContent} onChange={e => setAppendContent(e.target.value)} />
              <Button type="primary" disabled={!appendContent.trim()} onClick={handleAppend}>追加</Button>
            </Space>
          </Card>
        )}
        <div style={STYLES.mt16}>
          <Button icon={<ReloadOutlined />} onClick={() => fetchDailyLog()}>刷新</Button>
        </div>
      </div>
    </Spin>
  );
});
DailyLogTab.displayName = 'DailyLogTab';

// ================================================================
// Main MemoryPage — thin shell, only manages active tab
// ================================================================
export const MemoryPage: React.FC = () => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [activeTab, setActiveTab] = useState('list');

  const canSync = typeof api.syncMemory === 'function';
  const canStats = typeof api.getMemoryStats === 'function';
  const canDaily = typeof api.getDailyLog === 'function';

  // Stable tab items — only depends on capability flags and theme tokens (rarely change)
  const tabItems = useMemo(() => {
    const items: Array<{ key: string; label: React.ReactNode; children: React.ReactNode }> = [
      {
        key: 'list',
        label: <span><FileOutlined /> 记忆列表</span>,
        children: <MemoryListTab api={api}
          colorBgContainer={token.colorBgContainer} borderRadius={token.borderRadius}
          colorBorderSecondary={token.colorBorderSecondary} colorTextSecondary={token.colorTextSecondary}
          colorPrimary={token.colorPrimary} colorBgTextHover={token.colorBgTextHover} />,
      },
    ];
    if (canStats) {
      items.push({ key: 'stats', label: <span><BarChartOutlined /> 统计</span>, children: <StatsTab api={api} /> });
    }
    if (canSync) {
      items.push({ key: 'sync', label: <span><SyncOutlined /> 同步</span>, children: <SyncTab api={api} /> });
    }
    if (canDaily) {
      items.push({ key: 'daily', label: <span><CalendarOutlined /> 每日日志</span>, children: <DailyLogTab api={api} /> });
    }
    return items;
  }, [api, canStats, canSync, canDaily, token.colorBgContainer, token.borderRadius, token.colorBorderSecondary, token.colorTextSecondary, token.colorPrimary, token.colorBgTextHover]);

  return (
    <div style={STYLES.pageRoot}>
      <div style={STYLES.pageContent}>
        <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} style={STYLES.tabsFull} />
      </div>
    </div>
  );
};
