/**
 * SidebarMenu - 侧边栏菜单组件
 *
 * 渲染功能菜单项：New Chat、Cron、Skill、MCP、Memory
 */
import React from 'react';
import { Divider } from 'antd';
import {
  PlusOutlined,
  ClockCircleOutlined,
  AppstoreOutlined,
  ApiOutlined,
  BulbOutlined,
  TeamOutlined,
} from '@ant-design/icons';

export type PageKey =
  | 'new-chat'
  | 'chat'
  | 'sessions'
  | 'cron'
  | 'skill'
  | 'mcp'
  | 'memory'
  | 'agents'
  | 'settings'
  | 'workspace';

export interface MenuItem {
  key: PageKey;
  icon: React.ReactNode;
  label: string;
  dividerAfter?: boolean;
}

export interface SidebarMenuProps {
  /** 当前选中的页面 */
  currentPage?: PageKey;
  /** 页面导航回调 */
  onNavigate?: (page: PageKey) => void;
  /** 是否折叠 */
  collapsed?: boolean;
}

const menuItems: MenuItem[] = [
  {
    key: 'new-chat',
    icon: <PlusOutlined />,
    label: 'New Chat',
    dividerAfter: true,
  },
  {
    key: 'cron',
    icon: <ClockCircleOutlined />,
    label: 'Cron',
  },
  {
    key: 'skill',
    icon: <AppstoreOutlined />,
    label: 'Skill',
  },
  {
    key: 'mcp',
    icon: <ApiOutlined />,
    label: 'MCP',
  },
  {
    key: 'memory',
    icon: <BulbOutlined />,
    label: 'Memory',
  },
  {
    key: 'agents',
    icon: <TeamOutlined />,
    label: 'Agents',
    dividerAfter: true,
  },
];

export const SidebarMenu: React.FC<SidebarMenuProps> = ({
  currentPage,
  onNavigate,
  collapsed = false,
}) => {
  const handleClick = (key: PageKey) => {
    onNavigate?.(key);
  };

  return (
    <div className="sidebar-menu">
      {menuItems.map((item) => (
        <React.Fragment key={item.key}>
          <div
            className={`sidebar-menu-item ${
              currentPage === item.key ? 'sidebar-menu-item-active' : ''
            }`}
            onClick={() => handleClick(item.key)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                handleClick(item.key);
              }
            }}
          >
            <span className="sidebar-menu-icon">{item.icon}</span>
            {!collapsed && (
              <span className="sidebar-menu-label">{item.label}</span>
            )}
          </div>
          {item.dividerAfter && <Divider className="sidebar-menu-divider" />}
        </React.Fragment>
      ))}
    </div>
  );
};

export default SidebarMenu;
