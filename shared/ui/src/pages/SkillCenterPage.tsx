/**
 * SkillCenterPage - 技能中心页面
 *
 * 整合 Skills/Tools/Prompts 三个功能模块为 Tab 页
 */
import React, { useState, useRef } from 'react';
import { Tabs, Button, Space, theme } from 'antd';
import { AppstoreOutlined, ThunderboltOutlined, FileTextOutlined, FolderOpenOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { SkillsPage } from './SkillsPage';
import { ToolsPage } from './ToolsPage';
import { PromptsPage } from './PromptsPage';

export type SkillCenterTab = 'skills' | 'tools' | 'prompts';

// Ref types for child page methods
export interface SkillsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  handleReload: () => void;
}

export interface ToolsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  fetchTools: () => void;
}

export interface PromptsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  fetchPrompts: () => void;
}

export interface SkillCenterPageProps {
  /** 默认激活的 Tab */
  defaultTab?: SkillCenterTab;
  /** Tab 变化回调 */
  onTabChange?: (tab: SkillCenterTab) => void;
}

export const SkillCenterPage: React.FC<SkillCenterPageProps> = ({
  defaultTab = 'skills',
  onTabChange,
}) => {
  const { token } = theme.useToken();
  const [activeTab, setActiveTab] = useState<SkillCenterTab>(defaultTab);
  
  // Refs for child pages
  const skillsRef = useRef<SkillsPageRef>(null);
  const toolsRef = useRef<ToolsPageRef>(null);
  const promptsRef = useRef<PromptsPageRef>(null);

  const handleTabChange = (key: string) => {
    const tab = key as SkillCenterTab;
    setActiveTab(tab);
    onTabChange?.(tab);
  };

  // Tab bar extra content - buttons that change based on active tab
  const renderTabBarExtra = () => {
    switch (activeTab) {
      case 'skills':
        return (
          <Space>
            <Button icon={<FolderOpenOutlined />} onClick={() => skillsRef.current?.handleOpenDir('user')} className="page-header-btn">
              打开目录
            </Button>
            <Button icon={<PlusOutlined />} onClick={() => skillsRef.current?.setCreateModalVisible(true)} className="page-header-btn">
              创建
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => skillsRef.current?.handleReload()} className="page-header-btn">
              刷新
            </Button>
          </Space>
        );
      case 'tools':
        return (
          <Space>
            <Button icon={<FolderOpenOutlined />} onClick={() => toolsRef.current?.handleOpenDir('user')} className="page-header-btn">
              打开目录
            </Button>
            <Button icon={<PlusOutlined />} onClick={() => toolsRef.current?.setCreateModalVisible(true)} className="page-header-btn">
              创建
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => toolsRef.current?.fetchTools()} className="page-header-btn">
              刷新
            </Button>
          </Space>
        );
      case 'prompts':
        return (
          <Space>
            <Button icon={<FolderOpenOutlined />} onClick={() => promptsRef.current?.handleOpenDir('user')} className="page-header-btn">
              打开目录
            </Button>
            <Button icon={<PlusOutlined />} onClick={() => promptsRef.current?.setCreateModalVisible(true)} className="page-header-btn">
              创建
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => promptsRef.current?.fetchPrompts()} className="page-header-btn">
              刷新
            </Button>
          </Space>
        );
      default:
        return null;
    }
  };

  const tabItems = [
    {
      key: 'skills',
      label: (
        <span>
          <AppstoreOutlined />
          Skills
        </span>
      ),
      children: <SkillsPage ref={skillsRef} hideToolbar />,
    },
    {
      key: 'tools',
      label: (
        <span>
          <ThunderboltOutlined />
          Tools
        </span>
      ),
      children: <ToolsPage ref={toolsRef} hideToolbar />,
    },
    {
      key: 'prompts',
      label: (
        <span>
          <FileTextOutlined />
          Prompts
        </span>
      ),
      children: <PromptsPage ref={promptsRef} hideToolbar />,
    },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header with Tab Bar and Buttons */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          {/* Tab Selector */}
          <Tabs
            activeKey={activeTab}
            onChange={handleTabChange}
            items={tabItems.map(item => ({ key: item.key, label: item.label }))}
            size="small"
            style={{ marginBottom: 0, minHeight: 0 }}
            tabBarStyle={{ marginBottom: 0 }}
          />
          {/* Action Buttons */}
          {renderTabBarExtra()}
        </div>
      </div>

      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {activeTab === 'skills' && <SkillsPage ref={skillsRef} hideToolbar />}
        {activeTab === 'tools' && <ToolsPage ref={toolsRef} hideToolbar />}
        {activeTab === 'prompts' && <PromptsPage ref={promptsRef} hideToolbar />}
      </div>
    </div>
  );
};

export default SkillCenterPage;
