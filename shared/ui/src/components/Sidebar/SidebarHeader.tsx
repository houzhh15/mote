/**
 * SidebarHeader - 侧边栏头部组件
 *
 * 包含 Logo、搜索框、设置按钮、折叠按钮、主题切换
 */
import React, { useState } from 'react';
import { Input, Button, Space, Typography, Tooltip, Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import {
  SearchOutlined,
  SettingOutlined,
  CloseOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  BulbOutlined,
  BulbFilled,
} from '@ant-design/icons';
import moteLogo from '../../assets/mote_logo.png';
import { useTheme } from '../../context/ThemeContext';

const { Text } = Typography;

export interface SidebarHeaderProps {
  /** 搜索关键词 */
  searchKeyword?: string;
  /** 搜索关键词变化回调 */
  onSearchChange?: (keyword: string) => void;
  /** 设置按钮点击回调 */
  onSettingsClick?: () => void;
  /** 是否折叠 */
  collapsed?: boolean;
  /** 折叠状态变化回调 */
  onCollapse?: (collapsed: boolean) => void;
}

export const SidebarHeader: React.FC<SidebarHeaderProps> = ({
  searchKeyword = '',
  onSearchChange,
  onSettingsClick,
  collapsed = false,
  onCollapse,
}) => {
  const [showSearch, setShowSearch] = useState(false);
  const { themeMode, effectiveTheme, setThemeMode } = useTheme();

  const handleSearchToggle = () => {
    if (showSearch) {
      // 关闭搜索时清空关键词
      onSearchChange?.('');
    }
    setShowSearch(!showSearch);
  };

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onSearchChange?.(e.target.value);
  };

  const handleCollapseToggle = () => {
    onCollapse?.(!collapsed);
  };

  const themeMenuItems: MenuProps['items'] = [
    {
      key: 'light',
      label: '浅色模式',
      onClick: () => setThemeMode('light'),
    },
    {
      key: 'dark',
      label: '深色模式',
      onClick: () => setThemeMode('dark'),
    },
  ];

  const getThemeIcon = () => {
    return effectiveTheme === 'dark' ? <BulbFilled /> : <BulbOutlined />;
  };

  const getThemeTooltip = () => {
    const modeLabels: Record<string, string> = {
      light: '浅色模式',
      dark: '深色模式',
    };
    return `主题: ${modeLabels[themeMode] || '浅色模式'}`;
  };

  if (collapsed) {
    return (
      <div className="sidebar-header sidebar-header-collapsed">
        <Tooltip title="展开侧边栏" placement="right">
          <Button
            type="text"
            icon={<MenuUnfoldOutlined />}
            onClick={handleCollapseToggle}
            className="sidebar-action-btn"
          />
        </Tooltip>
      </div>
    );
  }

  return (
    <div className="sidebar-header">
      {showSearch ? (
        <div className="sidebar-search-bar">
          <Input
            placeholder="Search sessions..."
            value={searchKeyword}
            onChange={handleSearchChange}
            prefix={<SearchOutlined />}
            suffix={
              <CloseOutlined
                onClick={handleSearchToggle}
                style={{ cursor: 'pointer' }}
              />
            }
            autoFocus
          />
        </div>
      ) : (
        <>          <div className="sidebar-logo">
            <img src={moteLogo} alt="Mote" className="sidebar-logo-icon" style={{ width: 20, height: 20 }} />
            <Text strong className="sidebar-logo-text">
              Mote
            </Text>
          </div>
          <Space className="sidebar-header-actions">
            <Tooltip title="搜索">
              <Button
                type="text"
                icon={<SearchOutlined />}
                onClick={handleSearchToggle}
                className="sidebar-action-btn"
              />
            </Tooltip>
            <Dropdown menu={{ items: themeMenuItems, selectedKeys: [themeMode] }} trigger={['click']}>
              <Tooltip title={getThemeTooltip()}>
                <Button
                  type="text"
                  icon={getThemeIcon()}
                  className="sidebar-action-btn"
                />
              </Tooltip>
            </Dropdown>
            <Tooltip title="设置">
              <Button
                type="text"
                icon={<SettingOutlined />}
                onClick={onSettingsClick}
                className="sidebar-action-btn"
              />
            </Tooltip>
            <Tooltip title="收起侧边栏">
              <Button
                type="text"
                icon={<MenuFoldOutlined />}
                onClick={handleCollapseToggle}
                className="sidebar-action-btn"
              />
            </Tooltip>
          </Space>
        </>
      )}
    </div>
  );
};

export default SidebarHeader;
