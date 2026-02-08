// ================================================================
// MainLayout - Shared main layout component
// ================================================================

import React, { useState } from 'react';
import { Layout, Typography, theme } from 'antd';
import { Sidebar, type PageKey } from './Sidebar';

const { Content, Header } = Layout;
const { Text } = Typography;

// Re-export PageKey type for consumers
export type { PageKey } from './Sidebar';

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

export const MainLayout: React.FC<MainLayoutProps> = ({
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

  return (
    <Layout style={{ height: '100vh', overflow: 'hidden' }}>
      {/* Sidebar */}
      <div
        style={{
          width: actualWidth,
          minWidth: actualWidth,
          maxWidth: actualWidth,
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          zIndex: 10,
          transition: 'width 0.2s ease',
        }}
      >
        <Sidebar
          currentPage={currentPage}
          currentSessionId={currentSessionId}
          onNavigate={onNavigate}
          onSelectSession={onSelectSession}
          onSettingsClick={() => onNavigate('settings')}
          collapsed={collapsed}
          onCollapse={setCollapsed}
          refreshTrigger={refreshSessionsTrigger}
        />
      </div>

      {/* Main Content */}
      <Layout style={{ marginLeft: actualWidth, transition: 'margin-left 0.2s' }}>
        {showHeader && (
          <Header style={{
            background: token.colorBgContainer,
            padding: '0 24px',
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            position: 'sticky',
            top: 0,
            zIndex: 1,
          }}>
            <Text strong style={{ fontSize: 16 }}>
              {getPageTitle(currentPage)}
            </Text>
            {headerExtra}
          </Header>
        )}

        <Content style={{
          background: token.colorBgLayout,
          overflow: 'auto',
          height: showHeader ? 'calc(100vh - 64px)' : '100vh',
        }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  );
};

function getPageTitle(page: PageKey): string {
  switch (page) {
    case 'new-chat': return '新对话';
    case 'chat': return '对话';
    case 'sessions': return '会话历史';
    case 'memory': return '记忆';
    case 'skill': return '技能中心';
    case 'cron': return '定时任务';
    case 'mcp': return 'MCP 服务';
    case 'settings': return '设置';
    case 'workspace': return '工作区';
    default: return 'Mote';
  }
}
