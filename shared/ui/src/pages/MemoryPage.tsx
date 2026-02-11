// ================================================================
// MemoryPage - Shared memory management page with grouping support
// ================================================================

import React, { useState, useEffect, useMemo } from 'react';
import { Typography, Tag, Button, Space, Input, Spin, Empty, message, Modal, Form, Select, theme, Tooltip, Badge, Pagination } from 'antd';
import { SearchOutlined, ReloadOutlined, DeleteOutlined, EyeOutlined, PlusOutlined, EditOutlined, FileOutlined, FolderOutlined, DownOutlined, RightOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Memory } from '../types';

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

const CATEGORIES = [
  { value: 'preference', label: '偏好' },
  { value: 'fact', label: '事实' },
  { value: 'decision', label: '决策' },
  { value: 'entity', label: '实体' },
  { value: 'other', label: '其他' },
];

// Memory group type for organizing chunks
interface MemoryGroup {
  key: string;
  sourceFile?: string;
  baseId: string;
  memories: Memory[];
  totalChunks: number;
  category: string;
  created_at: string;
}

// Extract base ID from chunk ID
function getBaseId(id: string): string {
  const match = id.match(/^(.+)-chunk-\d+$/);
  return match ? match[1] : id;
}

// Group memories by source document or base ID
function groupMemories(memories: Memory[]): MemoryGroup[] {
  const groups = new Map<string, MemoryGroup>();

  for (const memory of memories) {
    const baseId = getBaseId(memory.id);
    const groupKey = memory.source_file || baseId;

    if (!groups.has(groupKey)) {
      groups.set(groupKey, {
        key: groupKey,
        sourceFile: memory.source_file,
        baseId,
        memories: [],
        totalChunks: memory.chunk_total || 1,
        category: memory.category,
        created_at: memory.created_at,
      });
    }

    const group = groups.get(groupKey)!;
    group.memories.push(memory);
    if (memory.created_at < group.created_at) {
      group.created_at = memory.created_at;
    }
  }

  for (const group of groups.values()) {
    group.memories.sort((a, b) => (a.chunk_index ?? 0) - (b.chunk_index ?? 0));
  }

  return Array.from(groups.values()).sort((a, b) =>
    new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  );
}

export const MemoryPage: React.FC = () => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [filterCategory, setFilterCategory] = useState<string | null>(null);
  const [viewModalVisible, setViewModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [selectedMemory, setSelectedMemory] = useState<Memory | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [form] = Form.useForm();
  
  // Pagination state
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(100);
  const [total, setTotal] = useState(0);

  const canCreate = typeof api.createMemory === 'function';
  const canUpdate = typeof api.updateMemory === 'function';

  const fetchMemories = async (page = currentPage, size = pageSize) => {
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
  };

  const searchMemories = async (query: string) => {
    if (!query.trim()) {
      fetchMemories(1, pageSize);
      return;
    }
    setLoading(true);
    try {
      const data = await api.searchMemories(query, 500); // Search returns more results
      setMemories(data);
      setTotal(data.length);
      setCurrentPage(1); // Reset to first page on search
    } catch (error) {
      console.error('Failed to search memories:', error);
      message.error('搜索记忆失败');
    } finally {
      setLoading(false);
    }
  };

  // Handle page change
  const handlePageChange = (page: number, size?: number) => {
    const newSize = size || pageSize;
    setCurrentPage(page);
    if (size && size !== pageSize) {
      setPageSize(size);
    }
    if (!searchQuery.trim()) {
      fetchMemories(page, newSize);
    }
  };

  // 搜索防抖
  useEffect(() => {
    const timer = setTimeout(() => {
      searchMemories(searchQuery);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchQuery]);

  // Initial load
  useEffect(() => {
    fetchMemories(1, pageSize);
  }, []);

  const deleteMemory = async (id: string) => {
    Modal.confirm({
      title: '确认删除',
      content: '删除后无法恢复，确定要删除这条记忆吗？',
      onOk: async () => {
        try {
          await api.deleteMemory(id);
          message.success('删除成功');
          fetchMemories();
        } catch (error) {
          console.error('Failed to delete memory:', error);
          message.error('删除失败');
        }
      },
    });
  };

  const deleteGroup = async (group: MemoryGroup) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除这组记忆吗？共 ${group.memories.length} 个分片。`,
      onOk: async () => {
        try {
          for (const memory of group.memories) {
            await api.deleteMemory(memory.id);
          }
          message.success('删除成功');
          fetchMemories();
        } catch (error) {
          console.error('Failed to delete group:', error);
          message.error('删除失败');
        }
      },
    });
  };

  const viewMemory = (memory: Memory) => {
    setSelectedMemory(memory);
    setViewModalVisible(true);
  };

  const openCreateModal = () => {
    setSelectedMemory(null);
    form.resetFields();
    form.setFieldsValue({ category: 'other' });
    setEditModalVisible(true);
  };

  const openEditModal = (memory: Memory) => {
    setSelectedMemory(memory);
    form.setFieldsValue({
      content: memory.content,
      category: memory.category,
    });
    setEditModalVisible(true);
  };

  const handleSave = async (values: { content: string; category: string }) => {
    try {
      if (selectedMemory && canUpdate) {
        // Update existing
        await api.updateMemory!(selectedMemory.id, values.content, values.category);
        message.success('更新成功');
      } else if (canCreate) {
        // Create new
        await api.createMemory!(values.content, values.category);
        message.success('添加成功');
      }
      setEditModalVisible(false);
      form.resetFields();
      setSearchQuery(''); // 清空搜索以显示所有记忆
      fetchMemories(); // 刷新数据列表
    } catch (error) {
      console.error('Failed to save memory:', error);
      message.error(selectedMemory ? '更新失败' : '添加失败');
    }
  };

  // 初始加载
  useEffect(() => {
    fetchMemories();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const getCategoryColor = (category: string) => {
    const colors: Record<string, string> = {
      preference: 'blue',
      fact: 'green',
      decision: 'orange',
      entity: 'purple',
      other: 'default',
    };
    return colors[category] || 'default';
  };

  const getCategoryLabel = (category: string) => {
    const found = CATEGORIES.find(c => c.value === category);
    return found?.label || category;
  };

  // Filter and group memories
  const filteredMemories = useMemo(() => {
    if (!filterCategory) return memories;
    return memories.filter(m => m.category === filterCategory);
  }, [memories, filterCategory]);

  const memoryGroups = useMemo(() => groupMemories(filteredMemories), [filteredMemories]);

  const toggleGroup = (key: string) => {
    setExpandedGroups(prev => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const renderGroupHeader = (group: MemoryGroup) => {
    const isExpanded = expandedGroups.has(group.key);
    const hasChunks = group.memories.length > 1 || (group.memories[0]?.chunk_total ?? 0) > 1;

    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '12px 16px',
          background: token.colorBgContainer,
          borderRadius: token.borderRadius,
          border: `1px solid ${token.colorBorderSecondary}`,
          marginBottom: 8,
          cursor: hasChunks ? 'pointer' : 'default',
        }}
        onClick={() => hasChunks && toggleGroup(group.key)}
      >
        <Space>
          {hasChunks ? (
            isExpanded ? <DownOutlined style={{ fontSize: 12 }} /> : <RightOutlined style={{ fontSize: 12 }} />
          ) : (
            <FileOutlined style={{ color: token.colorTextSecondary }} />
          )}
          <div>
            {group.sourceFile ? (
              <Tooltip title={group.sourceFile}>
                <Text strong style={{ maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block' }}>
                  <FolderOutlined style={{ marginRight: 8 }} />
                  {group.sourceFile.split('/').pop()}
                </Text>
              </Tooltip>
            ) : (
              <Text ellipsis style={{ maxWidth: 400 }}>
                {group.memories[0]?.content.slice(0, 60)}...
              </Text>
            )}
          </div>
          <Tag color={getCategoryColor(group.category)}>{getCategoryLabel(group.category)}</Tag>
          {hasChunks && (
            <Badge count={`${group.memories.length}/${group.totalChunks} 分片`} style={{ backgroundColor: token.colorPrimary }} />
          )}
        </Space>
        <Space size="small" onClick={e => e.stopPropagation()}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {new Date(group.created_at).toLocaleDateString()}
          </Text>
          {!hasChunks && (
            <>
              <Button type="text" size="small" icon={<EyeOutlined />} onClick={() => viewMemory(group.memories[0])} title="查看" />
              {canUpdate && <Button type="text" size="small" icon={<EditOutlined />} onClick={() => openEditModal(group.memories[0])} title="编辑" />}
            </>
          )}
          <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => hasChunks ? deleteGroup(group) : deleteMemory(group.memories[0].id)} title="删除" />
        </Space>
      </div>
    );
  };

  const renderChunkList = (group: MemoryGroup) => {
    if (!expandedGroups.has(group.key)) return null;

    return (
      <div style={{ marginLeft: 24, marginBottom: 16 }}>
        {group.memories.map((memory, index) => (
          <div
            key={memory.id}
            style={{
              padding: '12px 16px',
              background: token.colorBgTextHover,
              borderRadius: token.borderRadius,
              marginBottom: 8,
              borderLeft: `3px solid ${token.colorPrimary}`,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <div style={{ flex: 1 }}>
                <Space style={{ marginBottom: 8 }}>
                  <Tag>分片 {(memory.chunk_index ?? index) + 1}/{memory.chunk_total || group.memories.length}</Tag>
                  {memory.relevance !== undefined && <Tag color="gold">相关度: {(memory.relevance * 100).toFixed(0)}%</Tag>}
                </Space>
                <Paragraph ellipsis={{ rows: 3, expandable: true, symbol: '展开' }} style={{ marginBottom: 8 }}>
                  {memory.content}
                </Paragraph>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {new Date(memory.created_at).toLocaleString()}
                </Text>
              </div>
              <Space size="small">
                <Button type="text" size="small" icon={<EyeOutlined />} onClick={() => viewMemory(memory)} title="查看" />
                {canUpdate && <Button type="text" size="small" icon={<EditOutlined />} onClick={() => openEditModal(memory)} title="编辑" />}
                <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => deleteMemory(memory.id)} title="删除" />
              </Space>
            </div>
          </div>
        ))}
      </div>
    );
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Space>
            <Input
              placeholder="搜索记忆..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              style={{ width: 260 }}
              allowClear
              prefix={<SearchOutlined style={{ color: '#bfbfbf' }} />}
            />
            <Select
              placeholder="分类筛选"
              value={filterCategory}
              onChange={setFilterCategory}
              allowClear
              style={{ width: 120 }}
              options={[{ value: null, label: '全部分类' }, ...CATEGORIES]}
            />
            <Button icon={<ReloadOutlined />} onClick={() => fetchMemories()} className="page-header-btn">
              刷新
            </Button>
          </Space>
          <Space>
            {canCreate && (
              <Button icon={<PlusOutlined />} onClick={openCreateModal} className="page-header-btn">
                添加记忆
              </Button>
            )}
          </Space>
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <Spin spinning={loading}>
          {memoryGroups.length === 0 ? (
            <Empty description="暂无记忆" />
          ) : (
            <div>
              {memoryGroups.map(group => (
                <div key={group.key}>
                  {renderGroupHeader(group)}
                  {renderChunkList(group)}
                </div>
              ))}
              
              {/* Pagination - only show when not searching and total > pageSize */}
              {!searchQuery.trim() && total > pageSize && (
                <div style={{ display: 'flex', justifyContent: 'center', marginTop: 24, paddingBottom: 16 }}>
                  <Pagination
                    current={currentPage}
                    pageSize={pageSize}
                    total={total}
                    onChange={handlePageChange}
                    showSizeChanger
                    showQuickJumper
                    showTotal={(t, range) => `第 ${range[0]}-${range[1]} 条，共 ${t} 条记忆`}
                    pageSizeOptions={['50', '100', '200', '500']}
                  />
                </div>
              )}
            </div>
          )}
        </Spin>
      </div>

      {/* View Memory Modal */}
      <Modal
        title={
          <Space>
            <span>记忆详情</span>
            {selectedMemory && (
              <Tag color={getCategoryColor(selectedMemory.category)}>
                {getCategoryLabel(selectedMemory.category)}
              </Tag>
            )}
          </Space>
        }
        open={viewModalVisible}
        onCancel={() => setViewModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setViewModalVisible(false)}>关闭</Button>,
          canUpdate && (
            <Button key="edit" type="primary" onClick={() => { setViewModalVisible(false); if (selectedMemory) openEditModal(selectedMemory); }}>
              编辑
            </Button>
          ),
          <Button key="delete" danger onClick={() => { if (selectedMemory) { setViewModalVisible(false); deleteMemory(selectedMemory.id); } }}>
            删除
          </Button>,
        ].filter(Boolean)}
        width={700}
      >
        {selectedMemory && (
          <div>
            <Paragraph style={{ whiteSpace: 'pre-wrap', maxHeight: 400, overflow: 'auto' }}>
              {selectedMemory.content}
            </Paragraph>
            <div style={{ marginTop: 16, borderTop: `1px solid ${token.colorBorderSecondary}`, paddingTop: 16 }}>
              <Space direction="vertical" size="small">
                <Text type="secondary" style={{ fontSize: 12 }}>
                  创建时间: {new Date(selectedMemory.created_at).toLocaleString()}
                </Text>
                {selectedMemory.source_file && (
                  <Text type="secondary" style={{ fontSize: 12 }}>来源文件: {selectedMemory.source_file}</Text>
                )}
                {selectedMemory.chunk_total && selectedMemory.chunk_total > 1 && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    分片信息: {(selectedMemory.chunk_index ?? 0) + 1} / {selectedMemory.chunk_total}
                  </Text>
                )}
                {selectedMemory.relevance !== undefined && (
                  <Text type="secondary" style={{ fontSize: 12 }}>相关度: {(selectedMemory.relevance * 100).toFixed(0)}%</Text>
                )}
                {selectedMemory.importance !== undefined && (
                  <Text type="secondary" style={{ fontSize: 12 }}>重要性: {(selectedMemory.importance * 100).toFixed(0)}%</Text>
                )}
              </Space>
            </div>
          </div>
        )}
      </Modal>

      {/* Create/Edit Memory Modal */}
      <Modal
        title={selectedMemory ? '编辑记忆' : '添加记忆'}
        open={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false);
          form.resetFields();
        }}
        onOk={() => form.submit()}
        okText={selectedMemory ? '保存' : '添加'}
        cancelText="取消"
      >
        <Form form={form} layout="vertical" onFinish={handleSave}>
          <Form.Item
            name="content"
            label="内容"
            rules={[{ required: true, message: '请输入记忆内容' }]}
          >
            <TextArea rows={4} placeholder="输入记忆内容..." />
          </Form.Item>
          <Form.Item
            name="category"
            label="分类"
            rules={[{ required: true, message: '请选择分类' }]}
          >
            <Select options={CATEGORIES} placeholder="选择分类" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};
