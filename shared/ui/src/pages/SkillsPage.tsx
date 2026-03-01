// ================================================================
// SkillsPage - Shared skills management page
// ================================================================

import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Typography, List, Card, Tag, Spin, Empty, message, Button, Modal, Descriptions, Space, Tooltip, Input, Alert, Badge } from 'antd';
import { ThunderboltOutlined, PlayCircleOutlined, PauseCircleOutlined, ReloadOutlined, InfoCircleOutlined, FolderOpenOutlined, PlusOutlined, UploadOutlined, DeleteOutlined } from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { Skill, SkillVersionInfo } from '../types';

const { Text, Paragraph } = Typography;

// Update Confirm Dialog Component
interface UpdateConfirmDialogProps {
  skill: Skill;
  versionInfo: SkillVersionInfo;
  visible: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  loading?: boolean;
}

const UpdateConfirmDialog: React.FC<UpdateConfirmDialogProps> = ({
  skill,
  versionInfo,
  visible,
  onConfirm,
  onCancel,
  loading = false,
}) => {
  return (
    <Modal
      title={`更新 ${skill.name}`}
      open={visible}
      onOk={onConfirm}
      onCancel={onCancel}
      okText={loading ? '更新中...' : '确认更新'}
      cancelText="取消"
      confirmLoading={loading}
      okButtonProps={{ disabled: loading }}
    >
      <div style={{ marginBottom: 16 }}>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="当前版本">
            <strong>v{versionInfo.local_version}</strong>
          </Descriptions.Item>
          <Descriptions.Item label="最新版本">
            <strong style={{ color: '#52c41a' }}>v{versionInfo.embed_version}</strong>
          </Descriptions.Item>
        </Descriptions>
      </div>

      {versionInfo.local_modified && (
        <Alert
          message="警告"
          description="检测到本地文件已被修改，更新将覆盖您的修改。系统会自动备份旧版本。"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {versionInfo.description && (
        <div>
          <Text type="secondary">更新说明:</Text>
          <Paragraph style={{ marginTop: 8 }}>{versionInfo.description}</Paragraph>
        </div>
      )}
    </Modal>
  );
};

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
  
  // Version update states
  const [versionInfoMap, setVersionInfoMap] = useState<Record<string, SkillVersionInfo>>({});
  const [updateDialogVisible, setUpdateDialogVisible] = useState(false);
  const [skillToUpdate, setSkillToUpdate] = useState<Skill | null>(null);
  const [updateLoading, setUpdateLoading] = useState(false);

  const fetchSkills = async () => {
    setLoading(true);
    try {
      const data = await api.getSkills?.() ?? [];
      // Sort skills by state (active first) then by name for consistent ordering
      const sortedData = [...data].sort((a, b) => {
        // Active skills first
        if (a.state === 'active' && b.state !== 'active') return -1;
        if (a.state !== 'active' && b.state === 'active') return 1;
        // Then sort by name (case-insensitive)
        return a.name.localeCompare(b.name, 'zh-CN', { sensitivity: 'base' });
      });
      setSkills(sortedData);
    } catch (error) {
      console.error('Failed to fetch skills:', error);
      message.error('获取技能列表失败');
    } finally {
      setLoading(false);
    }
  };

  const checkUpdates = async () => {
    if (api.checkSkillUpdates) {
      try {
        const result = await api.checkSkillUpdates();
        const versionMap: Record<string, SkillVersionInfo> = {};
        result.updates.forEach(info => {
          versionMap[info.skill_id] = info;
        });
        setVersionInfoMap(versionMap);
      } catch (error) {
        console.error('Failed to check skill updates:', error);
        // Don't show error message for version check failure
      }
    }
  };

  useEffect(() => {
    fetchSkills();
    checkUpdates();
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

  const handleDelete = (skill: Skill) => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除技能 "${skill.name}" 吗？此操作将同时删除磁盘上的技能文件，不可恢复。`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await api.deleteSkill?.(skill.id);
          message.success(`已删除技能: ${skill.name}`);
          fetchSkills();
        } catch (error) {
          console.error('Failed to delete skill:', error);
          message.error('删除技能失败');
        }
      },
    });
  };

  // Builtin skills cannot be deleted
  const isBuiltinSkill = (skill: Skill) => {
    const builtinIds = ['mote-mcp-config', 'mote-self', 'mote-memory', 'mote-cron', 'mote-agents'];
    return builtinIds.includes(skill.id);
  };

  const handleReload = async () => {
    setLoading(true);
    try {
      await api.reloadSkills?.();
      message.success('技能已重新加载');
      await fetchSkills();
      
      // Check for updates
      if (api.checkSkillUpdates) {
        try {
          const result = await api.checkSkillUpdates();
          const versionMap: Record<string, SkillVersionInfo> = {};
          result.updates.forEach(info => {
            versionMap[info.skill_id] = info;
          });
          setVersionInfoMap(versionMap);
          
          const updateCount = result.updates.filter(info => info.update_available).length;
          if (updateCount > 0) {
            message.info(`发现 ${updateCount} 个技能有可用更新`);
          }
        } catch (error) {
          console.error('Failed to check skill updates:', error);
        }
      }
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

  const handleShowUpdateDialog = (skill: Skill) => {
    setSkillToUpdate(skill);
    setUpdateDialogVisible(true);
  };

  const handleConfirmUpdate = async () => {
    if (!skillToUpdate || !api.updateSkill) return;

    setUpdateLoading(true);
    try {
      const result = await api.updateSkill(skillToUpdate.id, { backup: true });
      
      if (result.success) {
        message.success(`技能 ${skillToUpdate.name} 已更新至 v${result.new_version}`);
        setUpdateDialogVisible(false);
        setSkillToUpdate(null);
        
        // Refresh skills and version info
        await handleReload();
      } else {
        message.error(`更新失败: ${result.error || '未知错误'}`);
      }
    } catch (error) {
      console.error('Failed to update skill:', error);
      message.error('更新技能失败');
    } finally {
      setUpdateLoading(false);
    }
  };

  const handleCancelUpdate = () => {
    setUpdateDialogVisible(false);
    setSkillToUpdate(null);
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
            grid={{ gutter: 8, xs: 1, sm: 2, md: 2, lg: 3, xl: 3, xxl: 4 }}
            dataSource={skills}
            style={{ maxWidth: '100%', overflow: 'hidden', padding: '0 8px' }}
            renderItem={(skill) => (
              <List.Item style={{ display: 'flex' }}>
                <Card
                  size="small"
                  style={{ width: '100%', minWidth: 220, minHeight: 200, height: '100%', display: 'flex', flexDirection: 'column' }}
                  styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column' } }}
                  title={
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <ThunderboltOutlined />
                      <Text strong ellipsis style={{ flex: 1 }}>{skill.name}</Text>
                      {versionInfoMap[skill.id]?.update_available && (
                        <Badge count="更新" style={{ backgroundColor: '#1890ff' }} />
                      )}
                    </div>
                  }
                  extra={<Tag color={getStateColor(skill.state)}>{getStateText(skill.state)}</Tag>}
                  actions={[
                    versionInfoMap[skill.id]?.update_available ? (
                      <Tooltip title={`v${versionInfoMap[skill.id].local_version} → v${versionInfoMap[skill.id].embed_version}`} key="update">
                        <UploadOutlined 
                          onClick={() => handleShowUpdateDialog(skill)}
                          style={{ color: '#1890ff', fontSize: 16 }}
                        />
                      </Tooltip>
                    ) : (
                      <Tooltip title="详情" key="detail">
                        <InfoCircleOutlined onClick={() => showDetail(skill)} />
                      </Tooltip>
                    ),
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
                    ...(!isBuiltinSkill(skill) ? [
                      <Tooltip title="删除" key="delete">
                        <DeleteOutlined
                          onClick={() => handleDelete(skill)}
                          style={{ color: '#ff4d4f' }}
                        />
                      </Tooltip>
                    ] : []),
                  ]}
                >
                  <div style={{ flex: 1 }}>
                    <Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 8, minHeight: '2.8em' }}>
                      {skill.description || '无描述'}
                    </Paragraph>
                  </div>
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

      {/* Update Confirm Dialog */}
      {skillToUpdate && versionInfoMap[skillToUpdate.id] && (
        <UpdateConfirmDialog
          skill={skillToUpdate}
          versionInfo={versionInfoMap[skillToUpdate.id]}
          visible={updateDialogVisible}
          onConfirm={handleConfirmUpdate}
          onCancel={handleCancelUpdate}
          loading={updateLoading}
        />
      )}

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
