// ================================================================
// MainLayout - Shared main layout component
// ================================================================

import React, { useState, useMemo, useCallback } from 'react';
import { Layout, Typography, theme } from 'antd';
import { Sidebar, type PageKey } from './Sidebar';

const { Content, Header } = Layout;
const { Text } = Typography;

// Re-export PageKey type for consumers
export type { PageKey } from './Sidebar';

// === Stable style constants ===
const LAYOUT_ROOT: React.CSSProperties = { height: '100vh', overflow: 'hidden' };
const SIDEBAR_BASE: React.CSSProperties = {
  height: '100vh',
  position: 'fixed',
  left: 0,
  top: 0,
  zIndex: 10,
  transition: 'width 0.2s ease',
};
const HEADER_BASE: React.CSSProperties = {
  padding: '0 24px',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  position: 'sticky',
  top: 0,
  zIndex: 1,
};
const TITLE_STYLE: React.CSSProperties = { fontSize: 16 };
const CONTENT_FULL: React.CSSProperties = { overflow: 'auto', height: '100vh' };
const CONTENT_WITH_HEADER: React.CSSProperties = { overflow: 'auto', height: 'calc(100vh - 64px)' };

interface MainLayoutProps {
  children: React.ReactNode;
  currentPage: PageKey;
  currentSessionId?: string;
  onNavigate: (page: PageKey) => void;
  onSelectSession?: (sessionId: string) => void;
  title?: string;
  showHeader?: boolean;
  headerExtra?: React.ReactNode;
  siderWidth?: number;
  collapsible?: boolean;
  refreshSessionsTrigger?: number;
}

export const MainLayout: React.FC<MainLayoutProps> = React.memo(({
  children,
  currentPage,
  currentSessionId,
  onNavigate,
  onSelectSession,
  title: _title = 'Mote',
  showHeader = true,
  headerExtra,
  siderWidth = 280,
  collapsible: _collapsible = true,
  refreshSessionsTrigger,
}) => {
  const [collapsed, setCollapsed] = useState(false);
  const { token } = theme.useToken();

  const actualWidth = collapsed ? 60 : siderWidth;

  // Memoize dynamic styles that depend on actualWidth / token
  const sidebarStyle = useMemo<React.CSSProperties>(() => ({
    ...SIDEBAR_BASE,
    width: actualWidth,
    minWidth: actualWidth,
    maxWidth: actualWidth,
  }), [actualWidth]);

  const mainLayoutStyle = useMemo<React.CSSProperties>(() => ({
    marginLeft: actualWidth,
    transition: 'margin-left 0.2s',
  }), [actualWidth]);

  const headerStyle = useMemo<React.CSSProperties>(() => ({
    ...HEADER_BASE,
    background: token.colorBgContainer,
    borderBottom: `1px solid ${token.colorBorderSecondary}`,
  }), [token.colorBgContainer, token.colorBorderSecondary]);

  const contentStyle = useMemo<React.CSSProperties>(() => {
    const base = showHeader ? CONTENT_WITH_HEADER : CONTENT_FULL;
    return { ...base, background: token.colorBgLayout };
  }, [showHeader, token.colorBgLayout]);

  const handleSettingsClick = useCallback(() => onNavigate('settings'), [onNavigate]);

  return (
    <Layout style={LAYOUT_ROOT}>
      {/* Sidebar */}
      <div style={sidebarStyle}>
        <Sidebar
          currentPage={currentPage}
          currentSessionId={currentSessionId}
          onNavigate={onNavigate}
          onSelectSession={onSelectSession}
          onSettingsClick={handleSettingsClick}
          collapsed={collapsed}
          onCollapse={setCollapsed}
          refreshTrigger={refreshSessionsTrigger}
        />
      </div>

      {/* Main Content */}
      <Layout style={mainLayoutStyle}>
        {showHeader && (
          <Header style={headerStyle}>
            <Text strong style={TITLE_STYLE}>
              {getPageTitle(currentPage)}
            </Text>
            {headerExtra}
          </Header>
        )}

        <Content style={contentStyle}>
          {children}
        </Content>
      </Layout>
    </Layout>
  );
});

function getPageTitle(page: PageKey): string {
  switch (page) {
    case 'new-chat': return '新对话';
    case 'chat': return '对话';
    case 'sessions': return '会话历史';
    case 'memory': return '记忆';
    case 'skill': return '技能中心';
    case 'cron': return '定时任务';
    case 'mcp': return 'MCP 服务';
    case 'agents': return '代理';
    case 'settings': return '设置';
    case 'workspace': return '工作区';
    default: return 'Mote';
  }
}
