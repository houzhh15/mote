// ================================================================
// SkillsPage - Shared skills management page
// ================================================================

import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Typography, List, Card, Tag, Spin, Empty, message, Button, Modal, Descriptions, Space, Tooltip, Input } from 'antd';
import { ThunderboltOutlined, PlayCircleOutlined, PauseCircleOutlined, ReloadOutlined, InfoCircleOutlined, FolderOpenOutlined, PlusOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Skill } from '../types';

const { Text, Paragraph } = Typography;

export interface SkillsPageProps {
  hideToolbar?: boolean;
}

export interface SkillsPageRef {
  handleOpenDir: (target: 'user') => void;
  setCreateModalVisible: (visible: boolean) => void;
  handleReload: () => void;
}

export const SkillsPage = forwardRef<SkillsPageRef, SkillsPageProps>(({ hideToolbar = false }, ref) => {
  const api = useAPI();
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [newSkillName, setNewSkillName] = useState('');
  const [newSkillTarget] = useState<'user'>('user');

  const fetchSkills = async () => {
    setLoading(true);
    try {
      const data = await api.getSkills?.() ?? [];
      setSkills(data);
    } catch (error) {
      console.error('Failed to fetch skills:', error);
      message.error('获取技能列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSkills();
  }, []);

  const handleActivate = async (skill: Skill) => {
    setActionLoading(skill.id);
    try {
      await api.activateSkill?.(skill.id);
      message.success(`已激活技能: ${skill.name}`);
      fetchSkills();
    } catch (error) {
      console.error('Failed to activate skill:', error);
      message.error('激活技能失败');
    } finally {
      setActionLoading(null);
    }
  };

  const handleDeactivate = async (skill: Skill) => {
    setActionLoading(skill.id);
    try {
      await api.deactivateSkill?.(skill.id);
      message.success(`已停用技能: ${skill.name}`);
      fetchSkills();
    } catch (error) {
      console.error('Failed to deactivate skill:', error);
      message.error('停用技能失败');
    } finally {
      setActionLoading(null);
    }
  };

  const handleReload = async () => {
    setLoading(true);
    try {
      await api.reloadSkills?.();
      message.success('技能已重新加载');
      fetchSkills();
    } catch (error) {
      console.error('Failed to reload skills:', error);
      message.error('重新加载技能失败');
    } finally {
      setLoading(false);
    }
  };

  const handleOpenDir = async (target: 'user' | 'workspace') => {
    try {
      await api.openSkillsDir?.(target);
      message.success('已在文件管理器中打开技能目录');
    } catch (error) {
      console.error('Failed to open skills dir:', error);
      message.error('打开目录失败');
    }
  };

  // Expose methods to parent via ref
  useImperativeHandle(ref, () => ({
    handleOpenDir,
    setCreateModalVisible,
    handleReload,
  }));

  const handleCreateSkill = async () => {
    if (!newSkillName.trim()) {
      message.warning('请输入技能名称');
      return;
    }
    try {
      const result = await api.createSkill?.(newSkillName.trim(), newSkillTarget);
      message.success(`技能模板已创建: ${result?.path}`);
      setCreateModalVisible(false);
      setNewSkillName('');
      // Reload skills to pick up the new template
      await api.reloadSkills?.();
      fetchSkills();
    } catch (error) {
      console.error('Failed to create skill:', error);
      message.error('创建技能失败');
    }
  };

  const showDetail = (skill: Skill) => {
    setSelectedSkill(skill);
    setDetailVisible(true);
  };

  const getStateColor = (state: string) => {
    const colors: Record<string, string> = {
      active: 'green',
      inactive: 'default',
      error: 'red',
      loading: 'blue',
    };
    return colors[state] || 'default';
  };

  const getStateText = (state: string) => {
    const texts: Record<string, string> = {
      active: '已激活',
      inactive: '未激活',
      error: '错误',
      loading: '加载中',
    };
    return texts[state] || state;
  };

  return (
    <div style={{ padding: hideToolbar ? 0 : 24, paddingTop: hideToolbar ? 16 : 24, height: '100%', overflow: 'auto' }}>
      {!hideToolbar && (
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Space>
            <Button icon={<FolderOpenOutlined />} onClick={() => handleOpenDir('user')} size="small" className="page-header-btn">
              打开用户目录
            </Button>
            <Button icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)} size="small" className="page-header-btn">
              创建技能
            </Button>
            <Button icon={<ReloadOutlined />} onClick={handleReload} loading={loading} size="small" className="page-header-btn">
              重新加载
            </Button>
          </Space>
        </div>
      </div>
      )}

      <Spin spinning={loading}>
        {skills.length === 0 ? (
          <Empty description="暂无技能" />
        ) : (
          <List
            grid={{ gutter: 16, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={skills}
            style={{ maxWidth: '100%', overflow: 'hidden' }}
            renderItem={(skill) => (
              <List.Item style={{ maxWidth: '100%' }}>
                <Card
                  size="small"
                  title={
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <ThunderboltOutlined />
                      <Text strong ellipsis style={{ flex: 1 }}>{skill.name}</Text>
                    </div>
                  }
                  extra={<Tag color={getStateColor(skill.state)}>{getStateText(skill.state)}</Tag>}
                  actions={[
                    <Tooltip title="详情" key="detail">
                      <InfoCircleOutlined onClick={() => showDetail(skill)} />
                    </Tooltip>,
                    skill.state === 'active' ? (
                      <Tooltip title="停用" key="deactivate">
                        <PauseCircleOutlined
                          onClick={() => handleDeactivate(skill)}
                          style={{ color: actionLoading === skill.id ? '#999' : undefined }}
                        />
                      </Tooltip>
                    ) : (
                      <Tooltip title="激活" key="activate">
                        <PlayCircleOutlined
                          onClick={() => handleActivate(skill)}
                          style={{ color: actionLoading === skill.id ? '#999' : '#52c41a' }}
                        />
                      </Tooltip>
                    ),
                  ]}
                >
                  <Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 8 }}>
                    {skill.description || '无描述'}
                  </Paragraph>
                  <Space size="small">
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      v{skill.version}
                    </Text>
                    {skill.tools && skill.tools.length > 0 && (
                      <Tag style={{ fontSize: 11 }}>{skill.tools.length} 工具</Tag>
                    )}
                  </Space>
                </Card>
              </List.Item>
            )}
          />
        )}
      </Spin>

      <Modal
        title={selectedSkill?.name || '技能详情'}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={600}
      >
        {selectedSkill && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="ID">{selectedSkill.id}</Descriptions.Item>
            <Descriptions.Item label="名称">{selectedSkill.name}</Descriptions.Item>
            <Descriptions.Item label="版本">{selectedSkill.version}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={getStateColor(selectedSkill.state)}>{getStateText(selectedSkill.state)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="描述">{selectedSkill.description || '-'}</Descriptions.Item>
            <Descriptions.Item label="作者">{selectedSkill.author || '-'}</Descriptions.Item>
            <Descriptions.Item label="路径">{selectedSkill.path || '-'}</Descriptions.Item>
            {selectedSkill.tools && selectedSkill.tools.length > 0 && (
              <Descriptions.Item label="工具">
                <Space wrap>
                  {selectedSkill.tools.map((tool, idx) => (
                    <Tag key={idx}>{tool}</Tag>
                  ))}
                </Space>
              </Descriptions.Item>
            )}
            {selectedSkill.prompts && selectedSkill.prompts.length > 0 && (
              <Descriptions.Item label="提示词">
                <Space wrap>
                  {selectedSkill.prompts.map((prompt, idx) => (
                    <Tag key={idx}>{prompt}</Tag>
                  ))}
                </Space>
              </Descriptions.Item>
            )}
            {selectedSkill.error && (
              <Descriptions.Item label="错误">
                <Text type="danger">{selectedSkill.error}</Text>
              </Descriptions.Item>
            )}
          </Descriptions>
        )}
      </Modal>

      {/* Create Skill Modal */}
      <Modal
        title="创建新技能"
        open={createModalVisible}
        onOk={handleCreateSkill}
        onCancel={() => {
          setCreateModalVisible(false);
          setNewSkillName('');
        }}
        okText="创建"
        cancelText="取消"
      >
        <div style={{ marginBottom: 16 }}>
          <Text>技能名称</Text>
          <Input
            placeholder="输入技能名称（英文，如 my-skill）"
            value={newSkillName}
            onChange={(e) => setNewSkillName(e.target.value)}
            style={{ marginTop: 8 }}
          />
        </div>
        <div>
          <Text type="secondary">技能将创建在用户目录（~/.mote/skills）</Text>
        </div>
      </Modal>
    </div>
  );
});

export default SkillsPage;
