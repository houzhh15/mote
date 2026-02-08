/**
 * Sidebar - 侧边栏主组件
 *
 * 组合 SidebarHeader/SidebarMenu/SessionList 三个子组件
 */
import React, { useState, useEffect } from 'react';
import { SidebarHeader } from './SidebarHeader';
import { SidebarMenu, type PageKey } from './SidebarMenu';
import { SessionList } from './SessionList';
import { useAPI } from '../../context/APIContext';
import type { Session } from '../../types';
import './styles.css';

export interface SidebarProps {
  /** 当前选中的页面 */
  currentPage: PageKey;
  /** 当前选中的会话 ID */
  currentSessionId?: string;
  /** 页面导航回调 */
  onNavigate: (page: PageKey) => void;
  /** 选择会话回调 */
  onSelectSession?: (sessionId: string) => void;
  /** 设置按钮点击回调 */
  onSettingsClick?: () => void;
  /** 是否折叠 */
  collapsed?: boolean;
  /** 折叠状态变化回调 */
  onCollapse?: (collapsed: boolean) => void;
  /** 会话删除后回调 */
  onSessionDeleted?: (sessionId: string) => void;
  /** 刷新触发器，每次值变化时重新加载会话列表 */
  refreshTrigger?: number;
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentPage,
  currentSessionId,
  onNavigate,
  onSelectSession,
  onSettingsClick,
  collapsed = false,
  onCollapse,
  onSessionDeleted,
  refreshTrigger,
}) => {
  const api = useAPI();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');

  // 获取会话列表
  const fetchSessions = async () => {
    setLoading(true);
    try {
      const data = await api.getSessions();
      setSessions(data);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSessions();
    // 每 30 秒刷新一次
    const interval = setInterval(fetchSessions, 30000);
    return () => clearInterval(interval);
  }, []);

  // 监听 refreshTrigger 变化，触发刷新
  useEffect(() => {
    if (refreshTrigger !== undefined) {
      fetchSessions();
    }
  }, [refreshTrigger]);

  const handleNavigate = (page: PageKey) => {
    onNavigate(page);
  };

  const handleSettingsClick = () => {
    if (onSettingsClick) {
      onSettingsClick();
    } else {
      onNavigate('settings');
    }
  };

  const handleSelectSession = (sessionId: string) => {
    onSelectSession?.(sessionId);
  };

  const handleSearchChange = (keyword: string) => {
    setSearchKeyword(keyword);
  };

  return (
    <div
      className={`sidebar-container ${
        collapsed ? 'sidebar-container-collapsed' : ''
      }`}
    >
      <SidebarHeader
        searchKeyword={searchKeyword}
        onSearchChange={handleSearchChange}
        onSettingsClick={handleSettingsClick}
        collapsed={collapsed}
        onCollapse={onCollapse}
      />
      <div className="sidebar-content">
        <SidebarMenu
          currentPage={currentPage}
          onNavigate={handleNavigate}
          collapsed={collapsed}
        />
        <SessionList
          sessions={sessions}
          currentSessionId={currentSessionId}
          searchKeyword={searchKeyword}
          onSelectSession={handleSelectSession}
          onSessionDeleted={(sessionId) => {
            // 刷新会话列表
            fetchSessions();
            // 通知外部
            onSessionDeleted?.(sessionId);
          }}
          onSessionRenamed={() => {
            // 刷新会话列表以显示新名称
            fetchSessions();
          }}
          loading={loading}
          collapsed={collapsed}
          onViewHistory={() => onNavigate('sessions')}
        />
      </div>
    </div>
  );
};

// Re-export types and sub-components
export type { PageKey } from './SidebarMenu';
export { SidebarHeader } from './SidebarHeader';
export { SidebarMenu } from './SidebarMenu';
export { SessionList } from './SessionList';

export default Sidebar;
