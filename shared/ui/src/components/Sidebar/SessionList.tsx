/**
 * SessionList - 会话列表组件
 *
 * 显示历史会话列表，支持日期分组和搜索过滤
 */
import React, { useMemo, useState } from 'react';
import { Typography, Dropdown, Modal, Input, message, Button, Tooltip } from 'antd';
import { MessageOutlined, MoreOutlined, EditOutlined, DeleteOutlined, GithubOutlined, HistoryOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import type { Session } from '../../types';
import { useSessionGroups } from '../../hooks';
import { useAPI } from '../../context/APIContext';
import { OllamaIcon } from '../OllamaIcon';
import { MinimaxIcon } from '../MinimaxIcon';

const { Text } = Typography;

// Helper function to get provider from model
const getProviderFromModel = (model?: string): 'copilot' | 'ollama' | 'minimax' | null => {
  if (!model) return null;
  if (model.startsWith('ollama:')) return 'ollama';
  if (model.startsWith('minimax:')) return 'minimax';
  return 'copilot';
};

export interface SessionListProps {
  /** 会话列表 */
  sessions: Session[];
  /** 当前选中的会话 ID */
  currentSessionId?: string;
  /** 搜索关键词 */
  searchKeyword?: string;
  /** 选择会话回调 */
  onSelectSession?: (sessionId: string) => void;
  /** 会话删除后回调 */
  onSessionDeleted?: (sessionId: string) => void;
  /** 会话重命名后回调 */
  onSessionRenamed?: (sessionId: string, newTitle: string) => void;
  /** 是否加载中 */
  loading?: boolean;
  /** 是否折叠 */
  collapsed?: boolean;
  /** 查看历史回调（折叠模式下使用） */
  onViewHistory?: () => void;
}

/**
 * 获取会话显示标题
 */
const getSessionTitle = (session: Session): string => {
  if (session.title) {
    return session.title;
  }
  if (session.preview) {
    // 截取前 30 个字符
    return session.preview.length > 30
      ? session.preview.slice(0, 30) + '...'
      : session.preview;
  }
  return 'New Chat';
};

/**
 * 根据关键词过滤会话
 */
const filterSessions = (sessions: Session[], keyword: string): Session[] => {
  if (!keyword.trim()) {
    return sessions;
  }

  const lowerKeyword = keyword.toLowerCase();
  return sessions.filter((session) => {
    const title = getSessionTitle(session).toLowerCase();
    const preview = (session.preview || '').toLowerCase();
    return title.includes(lowerKeyword) || preview.includes(lowerKeyword);
  });
};

export const SessionList: React.FC<SessionListProps> = ({
  sessions,
  currentSessionId,
  searchKeyword = '',
  onSelectSession,
  onSessionDeleted,
  onSessionRenamed,
  loading = false,
  collapsed = false,
  onViewHistory,
}) => {
  const api = useAPI();
  const [renameModalVisible, setRenameModalVisible] = useState(false);
  const [renameSession, setRenameSession] = useState<Session | null>(null);
  const [newTitle, setNewTitle] = useState('');

  // 过滤会话
  const filteredSessions = useMemo(
    () => filterSessions(sessions, searchKeyword),
    [sessions, searchKeyword]
  );

  // 分组会话
  const groups = useSessionGroups(filteredSessions);

  const handleSessionClick = (sessionId: string) => {
    onSelectSession?.(sessionId);
  };

  // 处理重命名
  const handleRename = (session: Session) => {
    setRenameSession(session);
    setNewTitle(getSessionTitle(session));
    setRenameModalVisible(true);
  };

  const handleRenameConfirm = async () => {
    if (!renameSession || !newTitle.trim()) {
      message.warning('请输入新名称');
      return;
    }
    try {
      if (api.updateSession) {
        await api.updateSession(renameSession.id, { title: newTitle.trim() });
        message.success('重命名成功');
        onSessionRenamed?.(renameSession.id, newTitle.trim());
      }
    } catch (error) {
      console.error('Failed to rename session:', error);
      message.error('重命名失败');
    } finally {
      setRenameModalVisible(false);
      setRenameSession(null);
      setNewTitle('');
    }
  };

  // 处理删除
  const handleDelete = (session: Session) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除会话 "${getSessionTitle(session)}" 吗？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.deleteSession(session.id);
          message.success('删除成功');
          onSessionDeleted?.(session.id);
        } catch (error) {
          console.error('Failed to delete session:', error);
          message.error('删除失败');
        }
      },
    });
  };

  // 生成菜单项
  const getMenuItems = (session: Session): MenuProps['items'] => [
    {
      key: 'rename',
      icon: <EditOutlined />,
      label: '重命名',
      onClick: () => handleRename(session),
    },
    {
      key: 'delete',
      icon: <DeleteOutlined />,
      label: '删除',
      danger: true,
      onClick: () => handleDelete(session),
    },
  ];

  if (collapsed) {
    return (
      <div className="session-list-collapsed">
        <Tooltip title="会话历史" placement="right">
          <Button
            type="text"
            icon={<HistoryOutlined />}
            onClick={onViewHistory}
            className="session-history-btn"
          />
        </Tooltip>
      </div>
    );
  }

  // 优化：只在初始加载时显示 loading 状态，刷新时保持原有列表
  if (loading && groups.length === 0) {
    return (
      <div className="session-list session-list-loading">
        <Text type="secondary">Loading sessions...</Text>
      </div>
    );
  }

  if (groups.length === 0) {
    return (
      <div className="session-list session-list-empty">
        <Text type="secondary">
          {searchKeyword ? 'No sessions found' : 'No chat history'}
        </Text>
      </div>
    );
  }

  return (
    <>
    <div className="session-list">
      {groups.map((group) => (
        <div key={group.key} className="session-group">
          <div className="session-group-header">
            <Text type="secondary" className="session-group-label">
              {group.label}
            </Text>
          </div>
          <div className="session-group-items">
            {group.sessions.map((session) => (
              <div
                key={session.id}
                className={`session-item ${
                  currentSessionId === session.id ? 'session-item-active' : ''
                }`}
                onClick={() => handleSessionClick(session.id)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    handleSessionClick(session.id);
                  }
                }}
              >
                <div className="session-item-icon">
                  <MessageOutlined />
                </div>
                <div className="session-item-content">
                  <div className="session-item-title">
                    {getSessionTitle(session)}
                  </div>
                  <div className="session-item-meta">
                    {session.model && (
                      <span className="session-item-model">
                        {getProviderFromModel(session.model) === 'ollama' 
                          ? <OllamaIcon size={12} /> 
                          : getProviderFromModel(session.model) === 'minimax'
                          ? <MinimaxIcon size={12} />
                          : <GithubOutlined style={{ fontSize: 12 }} />}
                        {' '}{session.model.split('/').pop()?.replace('ollama:', '')}
                      </span>
                    )}
                  </div>
                </div>
                <Dropdown
                  menu={{ items: getMenuItems(session) }}
                  trigger={['click']}
                  placement="bottomRight"
                >
                  <div
                    className="session-item-more"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <MoreOutlined />
                  </div>
                </Dropdown>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>

    {/* Rename Modal */}
    <Modal
      title="重命名会话"
      open={renameModalVisible}
      onOk={handleRenameConfirm}
      onCancel={() => {
        setRenameModalVisible(false);
        setRenameSession(null);
        setNewTitle('');
      }}
      okText="确定"
      cancelText="取消"
    >
      <Input
        value={newTitle}
        onChange={(e) => setNewTitle(e.target.value)}
        placeholder="输入新名称"
        onPressEnter={handleRenameConfirm}
        autoFocus
      />
    </Modal>
    </>
  );
};

export default SessionList;
