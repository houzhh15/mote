/**
 * ThemeContext - 主题管理 Context
 *
 * 提供深色/浅色模式切换功能
 */
import React, { createContext, useContext, useState, useEffect, useMemo } from 'react';
import { ConfigProvider, theme as antdTheme } from 'antd';
import type { Locale } from 'antd/lib/locale';

export type ThemeMode = 'light' | 'dark' | 'system';

interface ThemeContextType {
  /** 当前主题模式设置 */
  themeMode: ThemeMode;
  /** 实际生效的主题（考虑 system 设置） */
  effectiveTheme: 'light' | 'dark';
  /** 设置主题模式 */
  setThemeMode: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const THEME_STORAGE_KEY = 'mote-theme-mode';

interface ThemeProviderProps {
  children: React.ReactNode;
  /** Ant Design locale */
  locale?: Locale;
}

export const ThemeProvider: React.FC<ThemeProviderProps> = ({ children, locale }) => {
  // 从 localStorage 读取保存的主题设置
  const [themeMode, setThemeModeState] = useState<ThemeMode>(() => {
    const saved = localStorage.getItem(THEME_STORAGE_KEY);
    return (saved as ThemeMode) || 'system';
  });

  // 系统主题偏好
  const [systemTheme, setSystemTheme] = useState<'light' | 'dark'>(() => {
    if (typeof window !== 'undefined' && window.matchMedia) {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    return 'light';
  });

  // 监听系统主题变化
  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = (e: MediaQueryListEvent) => {
      setSystemTheme(e.matches ? 'dark' : 'light');
    };
    mediaQuery.addEventListener('change', handler);
    return () => mediaQuery.removeEventListener('change', handler);
  }, []);

  // 计算实际生效的主题
  const effectiveTheme = useMemo(() => {
    if (themeMode === 'system') {
      return systemTheme;
    }
    return themeMode;
  }, [themeMode, systemTheme]);

  // 设置主题并保存
  const setThemeMode = (mode: ThemeMode) => {
    setThemeModeState(mode);
    localStorage.setItem(THEME_STORAGE_KEY, mode);
  };

  // 更新 document 类名以支持 CSS 变量
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', effectiveTheme);
    if (effectiveTheme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [effectiveTheme]);

  const contextValue = useMemo(
    () => ({ themeMode, effectiveTheme, setThemeMode }),
    [themeMode, effectiveTheme]
  );

  // Ant Design 主题配置
  const antdThemeConfig = useMemo(() => ({
    algorithm: effectiveTheme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      // 可以在这里自定义更多主题 token
      colorPrimary: '#1890ff',
    },
  }), [effectiveTheme]);

  return (
    <ThemeContext.Provider value={contextValue}>
      <ConfigProvider theme={antdThemeConfig} locale={locale}>
        {children}
      </ConfigProvider>
    </ThemeContext.Provider>
  );
};

export const useTheme = (): ThemeContextType => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
};
