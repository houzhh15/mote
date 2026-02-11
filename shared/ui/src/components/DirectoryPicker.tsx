/**
 * DirectoryPicker - 目录选择器组件
 *
 * 类似资源管理器的目录浏览与选择组件。
 * 支持导航文件系统、选择目录、返回上级。
 * 在 Windows 上会显示磁盘驱动器列表作为根节点。
 */
import React, { useState, useEffect, useCallback } from 'react';
import { Modal, List, Button, Breadcrumb, Spin, Typography, Input, Space, message } from 'antd';
import {
  FolderOutlined,
  FolderOpenOutlined,
  ArrowLeftOutlined,
  HomeOutlined,
  CheckOutlined,
  EditOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { DirectoryEntry } from '../types';

const { Text } = Typography;

export interface DirectoryPickerProps {
  /** 是否显示 */
  open: boolean;
  /** 关闭回调 */
  onCancel: () => void;
  /** 选择目录后的回调，传递选中的目录路径 */
  onSelect: (path: string) => void;
  /** 初始路径（可选） */
  initialPath?: string;
  /** Modal 标题 */
  title?: string;
}

export const DirectoryPicker: React.FC<DirectoryPickerProps> = ({
  open,
  onCancel,
  onSelect,
  initialPath,
  title = '选择目录',
}) => {
  const api = useAPI();
  const [currentPath, setCurrentPath] = useState<string>('');
  const [parentPath, setParentPath] = useState<string>('');
  const [entries, setEntries] = useState<DirectoryEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>('');
  const [manualInput, setManualInput] = useState(false);
  const [inputPath, setInputPath] = useState('');

  // 加载目录内容
  const loadDirectory = useCallback(async (path?: string) => {
    if (!api.browseDirectory) {
      setError('目录浏览功能不可用');
      return;
    }

    setLoading(true);
    setError('');
    try {
      const result = await api.browseDirectory(path || '');
      setCurrentPath(result.path);
      setParentPath(result.parent);
      setEntries(result.entries || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '加载目录失败';
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [api]);

  // 打开时加载初始目录
  useEffect(() => {
    if (open) {
      loadDirectory(initialPath || '');
    }
  }, [open, initialPath, loadDirectory]);

  // 进入子目录
  const handleNavigate = (entry: DirectoryEntry) => {
    if (entry.is_dir) {
      loadDirectory(entry.path);
    }
  };

  // 返回上级目录
  const handleGoUp = () => {
    if (parentPath) {
      loadDirectory(parentPath);
    }
  };

  // 返回根目录（Home）
  const handleGoHome = () => {
    loadDirectory('');
  };

  // 确认选择当前目录
  const handleConfirm = () => {
    if (currentPath) {
      onSelect(currentPath);
    }
  };

  // 手动输入路径后跳转
  const handleManualNavigate = () => {
    const trimmed = inputPath.trim();
    if (!trimmed) {
      message.warning('请输入目录路径');
      return;
    }
    loadDirectory(trimmed);
    setManualInput(false);
    setInputPath('');
  };

  // 面包屑导航数据
  const breadcrumbItems = React.useMemo(() => {
    if (!currentPath) return [];
    
    // 按路径分隔符分割，支持 Windows 和 Unix
    const separator = currentPath.includes('\\') ? '\\' : '/';
    const parts = currentPath.split(separator).filter(Boolean);
    
    const items: { title: React.ReactNode; onClick?: () => void }[] = [];
    
    // 根节点
    items.push({
      title: <HomeOutlined />,
      onClick: handleGoHome,
    });
    
    // 各级路径
    let accumulated = '';
    parts.forEach((part, index) => {
      // Windows: 第一段如 "C:" 需要加回 \
      if (index === 0 && currentPath.includes('\\')) {
        accumulated = part + '\\';
      } else if (index === 0 && currentPath.startsWith('/')) {
        accumulated = '/' + part;
      } else {
        accumulated = accumulated + separator + part;
      }
      const pathForNav = accumulated;
      const isLast = index === parts.length - 1;
      
      items.push({
        title: isLast ? <Text strong>{part}</Text> : <a onClick={() => loadDirectory(pathForNav)}>{part}</a>,
      });
    });
    
    return items;
  }, [currentPath]);

  return (
    <Modal
      title={title}
      open={open}
      onCancel={onCancel}
      width={600}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button
          key="select"
          type="primary"
          icon={<CheckOutlined />}
          disabled={!currentPath}
          onClick={handleConfirm}
        >
          选择此目录
        </Button>,
      ]}
    >
      {/* 工具栏 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          size="small"
          disabled={!parentPath}
          onClick={handleGoUp}
          title="返回上级"
        />
        <Button
          icon={<HomeOutlined />}
          size="small"
          onClick={handleGoHome}
          title="主目录"
        />
        <Button
          icon={<EditOutlined />}
          size="small"
          onClick={() => {
            setManualInput(!manualInput);
            setInputPath(currentPath);
          }}
          title="手动输入路径"
          type={manualInput ? 'primary' : 'default'}
        />
      </div>

      {/* 手动输入路径 */}
      {manualInput && (
        <div style={{ marginBottom: 12 }}>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              placeholder="输入目录路径，如 /Users/name/projects"
              value={inputPath}
              onChange={(e) => setInputPath(e.target.value)}
              onPressEnter={handleManualNavigate}
            />
            <Button type="primary" onClick={handleManualNavigate}>
              跳转
            </Button>
          </Space.Compact>
        </div>
      )}

      {/* 面包屑路径 */}
      {breadcrumbItems.length > 0 && (
        <Breadcrumb
          style={{ marginBottom: 12, fontSize: 13 }}
          items={breadcrumbItems}
        />
      )}

      {/* 当前路径显示 */}
      {currentPath && (
        <div
          style={{
            background: 'var(--ant-color-bg-layout, #f5f5f5)',
            padding: '6px 12px',
            borderRadius: 6,
            marginBottom: 12,
            fontSize: 12,
            color: 'var(--ant-color-text-secondary, #666)',
            wordBreak: 'break-all',
          }}
        >
          {currentPath}
        </div>
      )}

      {/* 目录列表 */}
      {loading ? (
        <div style={{ textAlign: 'center', padding: '40px 0' }}>
          <Spin tip="加载中..." />
        </div>
      ) : error ? (
        <div style={{ textAlign: 'center', padding: '40px 0', color: 'red' }}>
          <Text type="danger">{error}</Text>
          <br />
          <Button size="small" style={{ marginTop: 8 }} onClick={() => loadDirectory(currentPath || '')}>
            重试
          </Button>
        </div>
      ) : (
        <List
          size="small"
          style={{ maxHeight: 400, overflowY: 'auto' }}
          dataSource={entries}
          locale={{ emptyText: '空目录' }}
          renderItem={(entry) => (
            <List.Item
              style={{
                cursor: entry.is_dir ? 'pointer' : 'default',
                padding: '8px 12px',
                borderRadius: 4,
              }}
              onClick={() => handleNavigate(entry)}
              onDoubleClick={() => handleNavigate(entry)}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
                {entry.is_dir ? (
                  <FolderOutlined style={{ color: '#faad14', fontSize: 16 }} />
                ) : (
                  <FolderOpenOutlined style={{ color: '#999', fontSize: 16 }} />
                )}
                <Text
                  style={{ flex: 1 }}
                  ellipsis={{ tooltip: entry.name }}
                >
                  {entry.name}
                </Text>
                {entry.is_dir && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    ›
                  </Text>
                )}
              </div>
            </List.Item>
          )}
        />
      )}
    </Modal>
  );
};
