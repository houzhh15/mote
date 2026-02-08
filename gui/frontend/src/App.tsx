// ================================================================
// Mote GUI App - Desktop application using shared UI
// ================================================================

import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { App as AntdApp, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {
  MainLayout,
  ChatPage,
  SettingsPage,
  MemoryPage,
  MCPPage,
  CronPage,
  NewChatPage,
  SkillCenterPage,
  SessionsPage,
  APIProvider,
  ThemeProvider,
  ChatManagerProvider,
  createWailsAdapter,
  type PageKey,
} from '@mote/shared-ui';
import './App.css';
// 显式导入 shared-ui 的样式（确保在 Wails 中正确加载）
import '@mote/shared-ui/style.css';

// Get Wails bindings (may not be available immediately in production)
const getWailsApp = () => {
  const win = window as unknown as { go?: { main?: { App?: unknown } } };
  if (win.go?.main?.App) {
    return win.go.main.App;
  }
  return null;
};

const LAST_SESSION_KEY = 'mote_last_session_id';

const App: React.FC = () => {
  const [currentPage, setCurrentPage] = useState<PageKey>('new-chat');
  // Try to restore last session ID from localStorage
  const [selectedSessionId, setSelectedSessionId] = useState<string | undefined>(() => {
    try {
      return localStorage.getItem(LAST_SESSION_KEY) || undefined;
    } catch {
      return undefined;
    }
  });
  const [wailsReady, setWailsReady] = useState(false);
  const [wailsApp, setWailsApp] = useState<unknown>(null);
  const [refreshSessionsTrigger, setRefreshSessionsTrigger] = useState(0);

  // Wait for Wails runtime to be ready
  useEffect(() => {
    const checkWailsReady = () => {
      const app = getWailsApp();
      if (app) {
        setWailsApp(app);
        setWailsReady(true);
        return true;
      }
      return false;
    };

    // Check immediately
    if (checkWailsReady()) return;

    // Poll for Wails runtime (production build may load async)
    const interval = setInterval(() => {
      if (checkWailsReady()) {
        clearInterval(interval);
      }
    }, 50);

    // Timeout after 5 seconds
    const timeout = setTimeout(() => {
      clearInterval(interval);
      console.error('Wails runtime not available after 5s');
      setWailsReady(true); // Allow error UI to show
    }, 5000);

    return () => {
      clearInterval(interval);
      clearTimeout(timeout);
    };
  }, []);

  // Create Wails adapter
  const adapter = useMemo(() => {
    if (!wailsApp) return null;
    return createWailsAdapter(wailsApp as Parameters<typeof createWailsAdapter>[0]);
  }, [wailsApp]);

  const handleSelectSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    // Persist to localStorage
    try {
      localStorage.setItem(LAST_SESSION_KEY, sessionId);
    } catch (e) {
      console.error('Failed to save session ID:', e);
    }
    setCurrentPage('chat');
  };

  const handleSessionCreated = useCallback((sessionId: string) => {
    // Store the session ID so we can restore it when navigating back
    setSelectedSessionId(sessionId);
    // Persist to localStorage
    try {
      localStorage.setItem(LAST_SESSION_KEY, sessionId);
    } catch (e) {
      console.error('Failed to save session ID:', e);
    }
    // 触发会话列表刷新
    setRefreshSessionsTrigger(prev => prev + 1);
  }, []);

  const handleNavigate = (page: PageKey) => {
    // Don't clear session when navigating - preserve chat state
    setCurrentPage(page);
  };

  const handleNavigateToChat = useCallback((sessionId: string) => {
    setSelectedSessionId(sessionId);
    try {
      localStorage.setItem(LAST_SESSION_KEY, sessionId);
    } catch (e) {
      console.error('Failed to save session ID:', e);
    }
    setCurrentPage('chat');
  }, []);

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
        // Use stable key to avoid remounting when sessionId changes
        return (
          <ChatPage 
            key="chat-page"
            sessionId={selectedSessionId} 
            onSessionCreated={handleSessionCreated}
          />
        );
      case 'sessions':
        return (
          <SessionsPage
            onSelectSession={handleSelectSession}
          />
        );
      case 'settings':
        return <SettingsPage hideStatusCard hideHelpCard />;
      case 'memory':
        return <MemoryPage />;
      case 'skill':
        return <SkillCenterPage />;
      case 'mcp':
        return <MCPPage />;
      case 'cron':
        return <CronPage />;
      default:
        return (
          <NewChatPage
            onSessionCreated={handleSessionCreated}
            onNavigateToChat={handleNavigateToChat}
          />
        );
    }
  };

  // Show loading while waiting for Wails runtime
  if (!wailsReady) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spin size="large" tip="Loading..." />
      </div>
    );
  }

  if (!adapter) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', flexDirection: 'column', gap: 16 }}>
        <div style={{ fontSize: 24, color: '#ff4d4f' }}>⚠️ Wails 环境未检测到</div>
        <div style={{ color: '#666' }}>请确保在 Wails 应用中运行</div>
      </div>
    );
  }

  return (
    <ThemeProvider locale={zhCN}>
      <AntdApp>
        <APIProvider adapter={adapter}>
          <ChatManagerProvider>
            <MainLayout
              currentPage={currentPage}
              currentSessionId={selectedSessionId}
              onNavigate={handleNavigate}
              onSelectSession={handleSelectSession}
              refreshSessionsTrigger={refreshSessionsTrigger}
              showHeader={false}
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
