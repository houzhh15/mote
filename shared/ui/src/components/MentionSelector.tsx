// ================================================================
// MentionSelector - Unified @ mention selector (Agents + Files)
// Shows a tabbed popup when user types @ in the chat input.
// Tab 1: Agents (direct delegate to sub-agent)
// Tab 2: Files (file context reference)
// ================================================================

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { List, Typography, Tag, Empty, Spin, theme, Segmented } from 'antd';
import {
  RobotOutlined,
  FileOutlined,
  FileImageOutlined,
  FileTextOutlined,
  CodeOutlined,
  FilePdfOutlined,
  FileZipOutlined,
  FolderOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { AgentConfig, DirectoryEntry } from '../types';
import type { FileSelectorMode } from './FileSelector';
import { detectFileType } from './FileSelector';

const { Text } = Typography;

// ================================================================
// Types
// ================================================================

type MentionTab = 'agents' | 'files';

interface AgentItem {
  name: string;
  description: string;
  model: string;
  enabled: boolean;
  hasSteps: boolean;
  isEntryPoint: boolean;
}

interface FileItem {
  name: string;
  path: string;
  type: string;
  size: number;
  icon: React.ReactNode;
  isDir: boolean;
}

interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  children?: WorkspaceFile[];
}

export interface MentionSelectorProps {
  visible: boolean;
  searchQuery: string;
  sessionId?: string;
  workspacePath?: string;
  /** Called when user selects an agent */
  onSelectAgent: (agentName: string) => void;
  /** Called when user selects a file */
  onSelectFile: (filepath: string, mode: FileSelectorMode) => void;
  onCancel: () => void;
}

// ================================================================
// File helpers (reused from FileSelector)
// ================================================================

const getFileIcon = (type: string): React.ReactNode => {
  const iconStyle = { fontSize: 16 };
  switch (type) {
    case 'image': return <FileImageOutlined style={{ ...iconStyle, color: '#52c41a' }} />;
    case 'code': return <CodeOutlined style={{ ...iconStyle, color: '#1890ff' }} />;
    case 'text': return <FileTextOutlined style={{ ...iconStyle, color: '#faad14' }} />;
    case 'pdf': return <FilePdfOutlined style={{ ...iconStyle, color: '#ff4d4f' }} />;
    case 'archive': return <FileZipOutlined style={{ ...iconStyle, color: '#722ed1' }} />;
    default: return <FileOutlined style={iconStyle} />;
  }
};

const flattenFiles = (files: WorkspaceFile[]): FileItem[] => {
  const result: FileItem[] = [];
  for (const file of files) {
    if (file.is_dir) {
      result.push({
        name: file.name, path: file.path, type: 'other',
        size: 0, icon: <FolderOutlined style={{ fontSize: 16, color: '#faad14' }} />, isDir: true,
      });
      if (file.children) result.push(...flattenFiles(file.children));
    } else {
      const type = detectFileType(file.name);
      result.push({
        name: file.name, path: file.path, type,
        size: file.size, icon: getFileIcon(type), isDir: false,
      });
    }
  }
  return result;
};

// ================================================================
// MentionSelector Component
// ================================================================

export const MentionSelector: React.FC<MentionSelectorProps> = ({
  visible,
  searchQuery,
  sessionId,
  workspacePath,
  onSelectAgent,
  onSelectFile,
  onCancel,
}) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const containerRef = useRef<HTMLDivElement>(null);

  // Tab state
  const [activeTab, setActiveTab] = useState<MentionTab>('agents');

  // Agent state
  const [agents, setAgents] = useState<AgentItem[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(false);

  // File state
  const [files, setFiles] = useState<FileItem[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);

  // Navigation
  const [selectedIndex, setSelectedIndex] = useState(0);
  // Agent filter: 'entry' (default), 'orchestrated', or 'all'
  const [agentFilter, setAgentFilter] = useState<'entry' | 'orchestrated' | 'all'>('entry');

  // Load agents when visible
  useEffect(() => {
    if (!visible) return;
    setAgentsLoading(true);
    setAgentFilter('entry'); // reset filter on open
    api.getAgents?.()
      ?.then((agentsMap: Record<string, AgentConfig>) => {
        const items: AgentItem[] = Object.entries(agentsMap)
          .filter(([, cfg]) => cfg.enabled !== false)
          .map(([name, cfg]) => ({
            name,
            description: cfg.description || '',
            model: cfg.model || '',
            enabled: cfg.enabled !== false,
            hasSteps: Array.isArray(cfg.steps) && cfg.steps.length > 0,
            isEntryPoint: cfg.entry_point === true,
          }));
        setAgents(items);
      })
      .catch(() => setAgents([]))
      .finally(() => setAgentsLoading(false));
  }, [visible, api]);

  // Load files when visible and files tab active
  useEffect(() => {
    if (!visible || activeTab !== 'files') return;
    if (!sessionId && !workspacePath) return;

    setFilesLoading(true);

    const loadAndSetFiles = async () => {
      try {
        if (sessionId && api.listWorkspaceFiles) {
          // listWorkspaceFiles returns WorkspaceFile[] directly
          const wsFiles = await api.listWorkspaceFiles(sessionId, '/');
          const arr = Array.isArray(wsFiles) ? wsFiles : [];
          setFiles(flattenFiles(arr));
        } else if (workspacePath && api.browseDirectory) {
          // browseDirectory returns { path, parent, entries: DirectoryEntry[] }
          const result = await api.browseDirectory(workspacePath);
          if (result?.entries) {
            const items: FileItem[] = result.entries.map((entry: DirectoryEntry) => {
              const type = entry.is_dir ? 'other' : detectFileType(entry.name);
              return {
                name: entry.name,
                path: entry.path,
                type,
                size: 0,
                icon: entry.is_dir
                  ? <FolderOutlined style={{ fontSize: 16, color: '#faad14' }} />
                  : getFileIcon(type),
                isDir: entry.is_dir,
              };
            });
            setFiles(items);
          }
        }
      } catch {
        setFiles([]);
      } finally {
        setFilesLoading(false);
      }
    };

    loadAndSetFiles();
  }, [visible, activeTab, sessionId, workspacePath, api]);

  // Filter items based on search and agent filter
  const filteredAgents = agents.filter(a => {
    // Apply filter first
    if (agentFilter === 'entry' && !a.isEntryPoint) return false;
    if (agentFilter === 'orchestrated' && !a.hasSteps) return false;
    // Then apply search
    return !searchQuery || a.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      a.description.toLowerCase().includes(searchQuery.toLowerCase());
  });

  const entryCount = agents.filter(a => a.isEntryPoint).length;
  const orchestratedCount = agents.filter(a => a.hasSteps).length;
  const allCount = agents.length;

  const filteredFiles = files.filter(f =>
    !searchQuery || f.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    f.path.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const currentItems = activeTab === 'agents' ? filteredAgents : filteredFiles;

  // Reset selection on filter/tab change
  useEffect(() => {
    setSelectedIndex(0);
  }, [searchQuery, activeTab, agentFilter]);

  // Auto-switch to files tab if search matches a file path pattern
  useEffect(() => {
    if (!searchQuery) return;
    // If query starts with path-like chars, switch to files
    if (searchQuery.includes('/') || searchQuery.includes('.')) {
      setActiveTab('files');
    }
  }, [searchQuery]);

  // Keyboard navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      setSelectedIndex((prev) => Math.min(prev + 1, currentItems.length - 1));
      e.preventDefault();
    } else if (e.key === 'ArrowUp') {
      setSelectedIndex((prev) => Math.max(prev - 1, 0));
      e.preventDefault();
    } else if (e.key === 'Tab') {
      // Switch tabs with Tab key
      setActiveTab(prev => prev === 'agents' ? 'files' : 'agents');
      e.preventDefault();
    } else if (e.key === 'Enter') {
      if (activeTab === 'agents' && filteredAgents[selectedIndex]) {
        onSelectAgent(filteredAgents[selectedIndex].name);
      } else if (activeTab === 'files' && filteredFiles[selectedIndex]) {
        const file = filteredFiles[selectedIndex];
        if (file.isDir) {
          // Navigate into directory (simplified: just filter)
        } else {
          onSelectFile(file.path, 'context');
        }
      }
      e.preventDefault();
    } else if (e.key === 'Escape') {
      onCancel();
      e.preventDefault();
    }
  }, [activeTab, filteredAgents, filteredFiles, selectedIndex, onSelectAgent, onSelectFile, onCancel, currentItems.length]);

  // Focus container when visible
  useEffect(() => {
    if (visible && containerRef.current) {
      containerRef.current.focus();
    }
  }, [visible]);

  if (!visible) return null;

  return (
    <div
      ref={containerRef}
      style={{
        position: 'absolute',
        bottom: '100%',
        left: 0,
        right: 0,
        maxHeight: 300,
        overflowY: 'auto',
        background: token.colorBgContainer,
        border: `1px solid ${token.colorBorderSecondary}`,
        borderRadius: 8,
        boxShadow: '0 -4px 12px rgba(0, 0, 0, 0.1)',
        zIndex: 100,
        marginBottom: 8,
      }}
      onKeyDown={handleKeyDown}
      tabIndex={0}
    >
      {/* Tab header */}
      <div style={{
        padding: '8px 12px 4px',
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
        background: token.colorBgTextActive,
      }}>
        <Segmented
          size="small"
          value={activeTab}
          onChange={(val) => setActiveTab(val as MentionTab)}
          options={[
            {
              value: 'agents',
              label: (
                <span><RobotOutlined style={{ marginRight: 4 }} />Agents{agents.length > 0 ? ` (${agents.length})` : ''}</span>
              ),
            },
            {
              value: 'files',
              label: (
                <span><FileOutlined style={{ marginRight: 4 }} />Files</span>
              ),
            },
          ]}
          block
          style={{ marginBottom: 4 }}
        />
        <Text type="secondary" style={{ fontSize: 11 }}>
          {activeTab === 'agents' 
            ? '选择 Agent 直接执行，跳过主 Agent 路由' 
            : '选择文件添加到上下文 (@引用)'}
          <span style={{ float: 'right', opacity: 0.6 }}>Tab 切换 · Enter 选择 · Esc 关闭</span>
        </Text>
      </div>

      {/* Content */}
      {activeTab === 'agents' ? (
        // ============ Agents Tab ============
        agentsLoading ? (
          <div style={{ padding: 24, textAlign: 'center' }}>
            <Spin size="small" />
          </div>
        ) : (
          <>
            {/* Agent filter toggle */}
            {agents.length > 0 && (
              <div style={{
                padding: '6px 12px',
                borderBottom: `1px solid ${token.colorBorderSecondary}`,
                display: 'flex',
                alignItems: 'center',
                gap: 8,
              }}>
                <Segmented
                  size="small"
                  value={agentFilter}
                  onChange={(val) => setAgentFilter(val as 'entry' | 'orchestrated' | 'all')}
                  options={[
                    { value: 'entry', label: `入口 (${entryCount})` },
                    { value: 'orchestrated', label: `编排 (${orchestratedCount})` },
                    { value: 'all', label: `全部 (${allCount})` },
                  ]}
                />
              </div>
            )}
            {filteredAgents.length === 0 ? (
              <Empty
                description={agents.length === 0 ? '暂无配置的 Sub-Agent' : '无匹配的 Agent'}
                style={{ padding: 24 }}
                imageStyle={{ height: 40 }}
              />
            ) : (
              <List
                size="small"
                dataSource={filteredAgents}
                renderItem={(agent, index) => (
                  <List.Item
                    onClick={() => onSelectAgent(agent.name)}
                    style={{
                      cursor: 'pointer',
                      padding: '10px 12px',
                      background: index === selectedIndex ? token.colorBgTextHover : 'transparent',
                      transition: 'background 0.15s',
                    }}
                  >
                    <List.Item.Meta
                      style={{ textAlign: 'left' }}
                      avatar={
                        <RobotOutlined style={{ fontSize: 20, color: token.colorPrimary }} />
                      }
                      title={
                        <span style={{ fontSize: 13 }}>
                          <span style={{ fontWeight: 600 }}>{agent.name}</span>
                          {agent.isEntryPoint && (
                            <Tag 
                              color="gold" 
                              style={{ marginLeft: 8, fontSize: 11 }}
                            >
                              入口
                            </Tag>
                          )}
                          {agent.hasSteps && (
                            <Tag 
                              icon={<ThunderboltOutlined />} 
                              color="processing" 
                              style={{ marginLeft: 4, fontSize: 11 }}
                            >
                              编排
                            </Tag>
                          )}
                          {agent.model && (
                            <Tag style={{ marginLeft: 4, fontSize: 11 }}>{agent.model}</Tag>
                          )}
                        </span>
                      }
                      description={
                        agent.description ? (
                          <Text
                            ellipsis
                            type="secondary"
                            style={{ fontSize: 12, display: 'block', textAlign: 'left' }}
                          >
                            {agent.description}
                          </Text>
                        ) : null
                      }
                    />
                  </List.Item>
                )}
              />
            )}
          </>
        )
      ) : (
        // ============ Files Tab ============
        filesLoading ? (
          <div style={{ padding: 24, textAlign: 'center' }}>
            <Spin size="small" />
          </div>
        ) : filteredFiles.length === 0 ? (
          <Empty
            description="无匹配的文件"
            style={{ padding: 24 }}
            imageStyle={{ height: 40 }}
          />
        ) : (
          <List
            size="small"
            dataSource={filteredFiles.slice(0, 50)}
            renderItem={(file, index) => (
              <List.Item
                onClick={() => {
                  if (file.isDir) return; // skip dirs for now
                  onSelectFile(file.path, 'context');
                }}
                style={{
                  cursor: file.isDir ? 'default' : 'pointer',
                  padding: '8px 12px',
                  background: index === selectedIndex ? token.colorBgTextHover : 'transparent',
                  opacity: file.isDir ? 0.6 : 1,
                }}
              >
                <List.Item.Meta
                  style={{ textAlign: 'left' }}
                  avatar={file.icon}
                  title={
                    <span style={{ fontSize: 13 }}>
                      {file.isDir ? file.name + '/' : file.name}
                      {!file.isDir && (
                        <Tag
                          color={
                            file.type === 'image' ? 'green' :
                            file.type === 'code' ? 'blue' :
                            file.type === 'text' ? 'gold' :
                            'default'
                          }
                          style={{ marginLeft: 8, fontSize: 11 }}
                        >
                          {file.type}
                        </Tag>
                      )}
                    </span>
                  }
                  description={
                    <Text
                      ellipsis
                      type="secondary"
                      style={{ fontSize: 12, display: 'block', textAlign: 'left' }}
                    >
                      {file.path}{!file.isDir ? ` · ${(file.size / 1024).toFixed(1)} KB` : ''}
                    </Text>
                  }
                />
              </List.Item>
            )}
          />
        )
      )}
    </div>
  );
};
