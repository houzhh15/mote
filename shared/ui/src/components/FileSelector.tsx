// ================================================================
// FileSelector - File reference selector component
// Inspired by PromptSelector, reusing similar structure and patterns
// ================================================================

import React, { useState, useEffect } from 'react';
import { List, Typography, Tag, Empty, Spin, Button, Space, theme } from 'antd';
import {
  FileOutlined,
  FileImageOutlined,
  FileTextOutlined,
  CodeOutlined,
  FilePdfOutlined,
  FileZipOutlined,
  FolderOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';

const { Text } = Typography;

// ================================================================
// Type Definitions
// ================================================================

export type FileType = 'image' | 'code' | 'text' | 'pdf' | 'archive' | 'other';
export type FileSelectorMode = 'context' | 'path-only';

// File item for display
export interface FileItem {
  name: string;
  path: string;
  type: FileType;
  size: number;
  icon: React.ReactNode;
  isDir: boolean;
}

// Workspace file from API
interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  children?: WorkspaceFile[];
}

// Component props
export interface FileSelectorProps {
  visible: boolean;
  searchQuery: string;
  sessionId: string;
  mode: FileSelectorMode;
  onSelect: (filepath: string, mode: FileSelectorMode) => void;
  onCancel: () => void;
}

// ================================================================
// File Type Detection
// ================================================================

const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico'];
const codeExts = [
  'js', 'ts', 'jsx', 'tsx', 'py', 'go', 'rs', 'java',
  'cpp', 'c', 'h', 'hpp', 'cs', 'rb', 'php', 'swift',
  'kt', 'scala', 'sh', 'bash', 'zsh',
];
const textExts = [
  'txt', 'md', 'json', 'yaml', 'yml', 'xml', 'csv',
  'toml', 'ini', 'conf', 'log',
];
const pdfExts = ['pdf'];
const archiveExts = ['zip', 'tar', 'gz', 'rar', '7z', 'bz2'];

/**
 * Detect file type based on extension
 */
export const detectFileType = (filename: string): FileType => {
  const ext = filename.split('.').pop()?.toLowerCase() ?? '';
  
  if (imageExts.includes(ext)) return 'image';
  if (codeExts.includes(ext)) return 'code';
  if (textExts.includes(ext)) return 'text';
  if (pdfExts.includes(ext)) return 'pdf';
  if (archiveExts.includes(ext)) return 'archive';
  
  return 'other';
};

/**
 * Get icon for file type
 */
const getFileIcon = (type: FileType): React.ReactNode => {
  const iconStyle = { fontSize: 16 };
  
  switch (type) {
    case 'image':
      return <FileImageOutlined style={{ ...iconStyle, color: '#52c41a' }} />;
    case 'code':
      return <CodeOutlined style={{ ...iconStyle, color: '#1890ff' }} />;
    case 'text':
      return <FileTextOutlined style={{ ...iconStyle, color: '#faad14' }} />;
    case 'pdf':
      return <FilePdfOutlined style={{ ...iconStyle, color: '#ff4d4f' }} />;
    case 'archive':
      return <FileZipOutlined style={{ ...iconStyle, color: '#722ed1' }} />;
    default:
      return <FileOutlined style={iconStyle} />;
  }
};

// ================================================================
// FileSelector Component
// ================================================================

export const FileSelector: React.FC<FileSelectorProps> = ({
  visible,
  searchQuery,
  sessionId,
  mode,
  onSelect,
  onCancel,
}) => {
  const api = useAPI();
  const { token } = theme.useToken();
  
  const [workspaceFiles, setWorkspaceFiles] = useState<FileItem[]>([]);
  const [filteredFiles, setFilteredFiles] = useState<FileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [currentPath, setCurrentPath] = useState<string>('/');

  /**
   * Recursively flatten file tree into a flat array of files AND directories
   */
  const flattenFiles = (files: WorkspaceFile[]): FileItem[] => {
    const result: FileItem[] = [];
    
    for (const file of files) {
      if (file.is_dir) {
        // Add directory itself
        result.push({
          name: file.name,
          path: file.path,
          type: 'other',
          size: 0,
          icon: <FolderOutlined style={{ fontSize: 16, color: '#faad14' }} />,
          isDir: true,
        });
        
        // Recursively process subdirectories
        if (file.children) {
          result.push(...flattenFiles(file.children));
        }
      } else {
        // Add file to result
        const type = detectFileType(file.name);
        result.push({
          name: file.name,
          path: file.path,
          type,
          size: file.size,
          icon: getFileIcon(type),
          isDir: false,
        });
      }
    }
    
    return result;
  };

  /**
   * Load workspace files from API
   */
  const loadFiles = async (path: string = '/') => {
    setLoading(true);
    try {
      const data = await api.listWorkspaceFiles?.(sessionId, path);
      if (data) {
        const files = flattenFiles(data);
        setWorkspaceFiles(files);
      }
    } catch (error) {
      console.error('Failed to load workspace files:', error);
      setWorkspaceFiles([]);
    } finally {
      setLoading(false);
    }
  };

  // Load files when selector becomes visible
  useEffect(() => {
    if (visible) {
      loadFiles(currentPath);
    }
  }, [api, visible, sessionId, currentPath]);

  // Filter and sort files based on search query
  useEffect(() => {
    let items = workspaceFiles;
    
    // When searching, show all matching files from all directories
    // When not searching, only show files in current directory
    if (!searchQuery) {
      // Filter to current directory only
      items = items.filter((f) => {
        const dir = f.path.substring(0, f.path.lastIndexOf('/')) || '/';
        return dir === currentPath;
      });
    } else {
      // Apply search filter across all directories
      const query = searchQuery.toLowerCase();
      items = items.filter((f) =>
        f.name.toLowerCase().includes(query) ||
        f.path.toLowerCase().includes(query)
      );
    }
    
    // Sort: directories first, then by type priority
    items.sort((a, b) => {
      // Directories come first
      if (a.isDir && !b.isDir) return -1;
      if (!a.isDir && b.isDir) return 1;
      
      // For files, sort by type priority: image > code > text > pdf > archive > other
      if (!a.isDir && !b.isDir) {
        const typeOrder: Record<FileType, number> = {
          image: 0,
          code: 1,
          text: 2,
          pdf: 3,
          archive: 4,
          other: 5,
        };
        return typeOrder[a.type] - typeOrder[b.type];
      }
      
      return 0;
    });
    
    setFilteredFiles(items);
    setSelectedIndex(0); // Reset selection when filter changes
  }, [searchQuery, workspaceFiles, currentPath]);

  // Handle file/directory selection
  const handleFileClick = (file: FileItem, selectedMode: FileSelectorMode) => {
    if (file.isDir) {
      // Navigate into directory
      setCurrentPath(file.path);
    } else {
      // Select file
      onSelect(file.path, selectedMode);
      onCancel();
    }
  };

  // Keyboard navigation (similar to PromptSelector)
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      setSelectedIndex((prev) => Math.min(prev + 1, filteredFiles.length - 1));
      e.preventDefault();
    } else if (e.key === 'ArrowUp') {
      setSelectedIndex((prev) => Math.max(prev - 1, 0));
      e.preventDefault();
    } else if (e.key === 'Enter') {
      if (filteredFiles[selectedIndex]) {
        handleFileClick(filteredFiles[selectedIndex], mode);
      }
      e.preventDefault();
    } else if (e.key === 'Escape') {
      onCancel();
      e.preventDefault();
    }
  };

  if (!visible) return null;

  // Calculate parent path for "go up" functionality
  const parentPath = currentPath === '/' ? null : currentPath.substring(0, currentPath.lastIndexOf('/')) || '/';

  // UI will be implemented in step-05
  return (
    <div
      style={{
        position: 'absolute',
        bottom: '100%',
        left: 0,
        right: 0,
        maxHeight: 350,
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
      {/* Current path header */}
      {!searchQuery && (
        <div
          style={{
            padding: '8px 12px',
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            background: token.colorBgTextActive,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Text strong style={{ fontSize: 12 }}>
            📁 {currentPath}
          </Text>
          {parentPath && (
            <Button
              size="small"
              type="link"
              style={{ fontSize: 11 }}
              onClick={() => setCurrentPath(parentPath)}
            >
              ↑ Up
            </Button>
          )}
        </div>
      )}
      {loading ? (
        <div style={{ padding: 24, textAlign: 'center' }}>
          <Spin size="small" tip="Loading files..." />
        </div>
      ) : filteredFiles.length === 0 ? (
        <Empty
          description="No matching files"
          style={{ padding: 24 }}
          imageStyle={{ height: 40 }}
        />
      ) : (
        <List
          size="small"
          dataSource={filteredFiles}
          renderItem={(file, index) => (
            <List.Item
              onClick={() => handleFileClick(file, mode)}
              className={index === selectedIndex ? 'file-selector-item-selected' : ''}
              style={{
                cursor: 'pointer',
                padding: '8px 12px',
                background: index === selectedIndex ? token.colorBgTextHover : 'transparent',
              }}
            >
              <List.Item.Meta
                style={{ textAlign: 'left' }}
                avatar={file.icon}
                title={
                  <span style={{ fontSize: 13 }}>
                    {file.isDir ? file.name : `@${file.name}`}
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
                    {file.path}{!file.isDir && ` · ${(file.size / 1024).toFixed(1)} KB`}
                  </Text>
                }
              />
              {/* Show buttons only for files, not directories */}
              {!file.isDir && (
                <Space>
                  <Button
                    size="small"
                    type="primary"
                    style={{ fontSize: 12 }}
                    onClick={(e) => {
                      e.stopPropagation();
                      handleFileClick(file, 'context');
                    }}
                  >
                    Add to Context
                  </Button>
                  <Button
                    size="small"
                    style={{ fontSize: 12 }}
                    onClick={(e) => {
                      e.stopPropagation();
                      handleFileClick(file, 'path-only');
                    }}
                  >
                    Path Only
                  </Button>
                </Space>
              )}
            </List.Item>
          )}
        />
      )}
      <style>{`
        .file-selector-item-selected {
          background: ${token.colorBgTextHover} !important;
        }
      `}</style>
    </div>
  );
};
