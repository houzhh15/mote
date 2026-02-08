import React from 'react';
import { App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {
  APIProvider,
  createHttpAdapter,
  MainLayout,
  ChatPage,
  MemoryPage,
  CronPage,
  MCPPage,
  SettingsPage,
  NewChatPage,
  SkillCenterPage,
  ThemeProvider,
  ChatManagerProvider,
} from '@mote/shared-ui';
import type { PageKey } from '@mote/shared-ui';
import './App.css';

// Create HTTP adapter for web frontend
const apiAdapter = createHttpAdapter({});

// Storage keys for persistence
const STORAGE_KEY_SESSION = 'mote_current_session_id';
const STORAGE_KEY_PAGE = 'mote_current_page';

const App: React.FC = () => {
  // Initialize from localStorage
  const [currentPage, setCurrentPage] = React.useState<PageKey>(() => {
    const saved = localStorage.getItem(STORAGE_KEY_PAGE);
    return (saved as PageKey) || 'new-chat';
  });
  const [selectedSessionId, setSelectedSessionId] = React.useState<string | undefined>(() => {
    return localStorage.getItem(STORAGE_KEY_SESSION) || undefined;
  });
  const [refreshSessionsTrigger, setRefreshSessionsTrigger] = React.useState(0);

  const handleSelectSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    localStorage.setItem(STORAGE_KEY_SESSION, sessionId);
    setCurrentPage('chat');
  };

  const handleSessionCreated = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    localStorage.setItem(STORAGE_KEY_SESSION, sessionId);
    // 触发会话列表刷新
    setRefreshSessionsTrigger(prev => prev + 1);
  };

  const handleNavigateToChat = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    localStorage.setItem(STORAGE_KEY_SESSION, sessionId);
    setCurrentPage('chat');
    localStorage.setItem(STORAGE_KEY_PAGE, 'chat');
  };

  const handleNavigate = (page: PageKey) => {
    setCurrentPage(page);
    localStorage.setItem(STORAGE_KEY_PAGE, page);
    // Don't clear session when navigating - keep it for when user returns
  };

  const renderPage = () => {
    switch (currentPage) {
      case 'new-chat':
        return (
          <NewChatPage
            onSessionCreated={handleSessionCreated}
            onNavigateToChat={handleNavigateToChat}
          />
        );
      case 'chat':
        return (
          <ChatPage 
            key={selectedSessionId || 'new'} 
            sessionId={selectedSessionId} 
            onSessionCreated={handleSessionCreated}
          />
        );
      case 'memory':
        return <MemoryPage />;
      case 'skill':
        return <SkillCenterPage />;
      case 'cron':
        return <CronPage />;
      case 'mcp':
        return <MCPPage />;
      case 'settings':
        return <SettingsPage hideStatusCard hideHelpCard />;
      default:
        return (
          <NewChatPage
            onSessionCreated={handleSessionCreated}
            onNavigateToChat={handleNavigateToChat}
          />
        );
    }
  };

  return (
    <ThemeProvider locale={zhCN}>
      <AntdApp>
        <APIProvider adapter={apiAdapter}>
          <ChatManagerProvider>
            <MainLayout
              currentPage={currentPage}
              currentSessionId={selectedSessionId}
              onNavigate={handleNavigate}
              onSelectSession={handleSelectSession}
              title="Mote"
              showHeader={false}
              refreshSessionsTrigger={refreshSessionsTrigger}
            >
              {renderPage()}
            </MainLayout>
          </ChatManagerProvider>
        </APIProvider>
      </AntdApp>
    </ThemeProvider>
  );
};

export default App;
