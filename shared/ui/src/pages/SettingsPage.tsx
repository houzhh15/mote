// ================================================================
// SettingsPage - Shared settings page with GitHub Copilot auth
// ================================================================

import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  Card,
  Button,
  Space,
  message,
  Typography,
  Badge,
  Descriptions,
  Alert,
  Spin,
  Steps,
  Modal,
  Select,
  Checkbox,
  Tag,
  Input,
  theme,
} from 'antd';
import {
  ReloadOutlined,
  GithubOutlined,
  LogoutOutlined,
  CheckCircleOutlined,
  LoadingOutlined,
  CopyOutlined,
  ExclamationCircleOutlined,
  PoweroffOutlined,
  SettingOutlined,
  RocketOutlined,
} from '@ant-design/icons';
import { useAPI } from '../context/APIContext';
import type { ServiceStatus, AuthStatus, DeviceCodeResponse, ChannelStatus, IMessageChannelConfig, AppleNotesChannelConfig, AppleRemindersChannelConfig, ChannelConfig, Model, ScenarioModels, ProviderStatus } from '../types';
import { ChannelCard, IMessageConfig, AppleNotesConfig, AppleRemindersConfig } from '../components/channels';
import { OllamaIcon } from '../components/OllamaIcon';

const { Title, Text, Paragraph } = Typography;

export interface SettingsPageProps {
  /** 是否隐藏服务状态卡片 */
  hideStatusCard?: boolean;
  /** 是否隐藏帮助卡片 */
  hideHelpCard?: boolean;
}

export const SettingsPage: React.FC<SettingsPageProps> = ({
  hideStatusCard = false,
  hideHelpCard = false,
}) => {
  const api = useAPI();
  const { token } = theme.useToken();
  const [loading, setLoading] = useState(false);
  const [serviceStatus, setServiceStatus] = useState<ServiceStatus | null>(null);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [deviceCode, setDeviceCode] = useState<DeviceCodeResponse | null>(null);
  const [loginStep, setLoginStep] = useState<'idle' | 'waiting' | 'polling'>('idle');
  const pollingRef = useRef<number | null>(null);

  // Channel state
  const [channels, setChannels] = useState<ChannelStatus[]>([]);
  const [configModalVisible, setConfigModalVisible] = useState(false);
  const [currentChannelType, setCurrentChannelType] = useState<string | null>(null);
  const [currentConfig, setCurrentConfig] = useState<ChannelConfig | null>(null);
  const [channelLoading, setChannelLoading] = useState<Record<string, boolean>>({});

  // Scenario models state
  const [models, setModels] = useState<Model[]>([]);
  const [scenarioModels, setScenarioModels] = useState<ScenarioModels>({ chat: '', cron: '', channel: '' });
  const [scenarioLoading, setScenarioLoading] = useState<Record<string, boolean>>({});

  // Provider state - multi-select support
  const [enabledProviders, setEnabledProviders] = useState<string[]>(['copilot']);
  const [defaultProvider, setDefaultProvider] = useState<string>('copilot');
  const [providerStatuses, setProviderStatuses] = useState<ProviderStatus[]>([]);
  const [providerLoading, setProviderLoading] = useState(false);
  
  // Ollama config state
  const [ollamaEndpoint, setOllamaEndpoint] = useState('http://localhost:11434');
  const [ollamaLoading, setOllamaLoading] = useState(false);

  // Check capabilities based on API availability
  const hasAuthSupport = Boolean(api.getAuthStatus);
  const hasRestartSupport = Boolean(api.restartService);
  const hasQuitSupport = Boolean(api.quit);
  const hasChannelSupport = Boolean(api.getChannels);
  const hasScenarioModelsSupport = Boolean(api.getScenarioModels);

  useEffect(() => {
    loadServiceStatus();
    loadModels();
    loadConfig();
    if (hasAuthSupport) {
      loadAuthStatus();
    }
    if (hasChannelSupport) {
      loadChannels();
    }
    if (hasScenarioModelsSupport) {
      loadScenarioModels();
    }
    
    return () => {
      if (pollingRef.current) {
        clearTimeout(pollingRef.current);
      }
    };
  }, [hasAuthSupport, hasChannelSupport, hasScenarioModelsSupport]);

  const loadConfig = async () => {
    try {
      const config = await api.getConfig();
      setDefaultProvider(config.provider?.default || 'copilot');
      // Support both enabled array and legacy single provider
      const enabled = config.provider?.enabled;
      if (enabled && enabled.length > 0) {
        setEnabledProviders(enabled);
      } else {
        setEnabledProviders([config.provider?.default || 'copilot']);
      }
      // Load Ollama config
      if (config.ollama?.endpoint) {
        setOllamaEndpoint(config.ollama.endpoint);
      }
    } catch (error) {
      console.error('Failed to load config:', error);
    }
  };

  const handleOllamaEndpointSave = async () => {
    setOllamaLoading(true);
    try {
      await api.updateConfig({
        ollama: {
          endpoint: ollamaEndpoint,
        }
      });
      message.success('Ollama 地址已保存并生效');
      // Reload models to refresh Ollama models
      loadModels();
    } catch (error) {
      console.error('Failed to update Ollama endpoint:', error);
      message.error('保存失败');
    } finally {
      setOllamaLoading(false);
    }
  };

  const handleProvidersChange = async (providers: string[]) => {
    if (providers.length === 0) {
      message.warning('至少需要启用一个 Provider');
      return;
    }
    
    setProviderLoading(true);
    try {
      // Determine default provider (first enabled, or keep current if still enabled)
      let newDefault = defaultProvider;
      if (!providers.includes(defaultProvider)) {
        newDefault = providers[0];
      }
      
      await api.updateConfig({ 
        provider: { 
          default: newDefault,
          enabled: providers 
        } 
      });
      setEnabledProviders(providers);
      setDefaultProvider(newDefault);
      message.success(`已启用 ${providers.length} 个 Provider，配置已生效`);
      // Reload models to reflect provider changes
      loadModels();
    } catch (error) {
      console.error('Failed to update providers:', error);
      message.error('更新 Provider 失败');
    } finally {
      setProviderLoading(false);
    }
  };

  const handleDefaultProviderChange = async (provider: string) => {
    if (!enabledProviders.includes(provider)) {
      message.warning('只能将已启用的 Provider 设为默认');
      return;
    }
    
    setProviderLoading(true);
    try {
      await api.updateConfig({ provider: { default: provider } });
      setDefaultProvider(provider);
      message.success(`默认 Provider 已切换为 ${provider}`);
    } catch (error) {
      console.error('Failed to update default provider:', error);
      message.error('更新默认 Provider 失败');
    } finally {
      setProviderLoading(false);
    }
  };

  const loadServiceStatus = async () => {
    try {
      const status = await api.getStatus();
      setServiceStatus(status);
    } catch (error) {
      setServiceStatus({
        running: false,
        port: 18788,
        version: 'unknown',
        uptime: 0,
        error: String(error),
      });
    }
  };

  const loadModels = async () => {
    try {
      const response = await api.getModels();
      setModels(response.models || []);
      if (response.providers) {
        setProviderStatuses(response.providers);
      }
    } catch (error) {
      console.error('Failed to load models:', error);
    }
  };

  // Helper: Get provider display label
  const getProviderLabel = (provider: string): string => {
    switch (provider) {
      case 'copilot': return 'Copilot API (免费模型)';
      case 'copilot-acp': return 'Copilot ACP (付费模型)';
      case 'ollama': return 'Ollama (本地)';
      default: return provider;
    }
  };

  // Helper: Get provider icon component
  const getProviderIcon = (provider: string, props?: Record<string, unknown>) => {
    if (provider === 'ollama') {
      return <OllamaIcon {...props} />;
    }
    if (provider === 'copilot-acp') {
      return <RocketOutlined {...props} />;
    }
    return <GithubOutlined {...props} />;
  };

  // Helper: Group models by provider for OptGroup display
  const getModelsByProvider = useCallback(() => {
    const grouped: Record<string, Model[]> = {};
    models.forEach(model => {
      const provider = model.provider || 'copilot';
      if (!grouped[provider]) {
        grouped[provider] = [];
      }
      grouped[provider].push(model);
    });
    return grouped;
  }, [models]);

  // Helper: Render model option with provider icon and tags
  const renderModelOption = (model: Model) => (
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
      <Space>
        {model.provider === 'ollama' ? <OllamaIcon size={12} style={{ opacity: 0.6 }} /> : <GithubOutlined style={{ fontSize: 12, opacity: 0.6 }} />}
        <span>{model.display_name}</span>
      </Space>
      <Space size={4}>
        {model.is_free && <Tag color="green" style={{ marginRight: 0 }}>免费</Tag>}
        {model.available === false && <Tag color="red" style={{ marginRight: 0 }}>不可用</Tag>}
      </Space>
    </div>
  );

  // Helper: Render model selector with OptGroup by provider
  const renderModelSelector = (
    value: string,
    onChange: (value: string) => void,
    loading?: boolean
  ) => {
    const grouped = getModelsByProvider();
    const providers = Object.keys(grouped);
    
    // If only one provider, don't use OptGroup
    if (providers.length <= 1) {
      return (
        <Select
          value={value}
          onChange={onChange}
          loading={loading}
          style={{ width: 320 }}
          placeholder="选择默认模型"
        >
          {models.map((model) => (
            <Select.Option key={model.id} value={model.id} disabled={model.available === false}>
              {renderModelOption(model)}
            </Select.Option>
          ))}
        </Select>
      );
    }
    
    // Multiple providers: use OptGroup
    return (
      <Select
        value={value}
        onChange={onChange}
        loading={loading}
        style={{ width: 320 }}
        placeholder="选择默认模型"
      >
        {providers.map(provider => (
          <Select.OptGroup 
            key={provider} 
            label={
              <Space>
                {getProviderIcon(provider)}
                {getProviderLabel(provider)}
              </Space>
            }
          >
            {grouped[provider].map(model => (
              <Select.Option key={model.id} value={model.id} disabled={model.available === false}>
                {renderModelOption(model)}
              </Select.Option>
            ))}
          </Select.OptGroup>
        ))}
      </Select>
    );
  };

  const loadScenarioModels = async () => {
    if (!api.getScenarioModels) return;
    try {
      const scenarios = await api.getScenarioModels();
      setScenarioModels(scenarios);
    } catch (error) {
      console.error('Failed to load scenario models:', error);
    }
  };

  const handleScenarioModelChange = async (scenario: 'chat' | 'cron' | 'channel', modelId: string) => {
    if (!api.setScenarioModel) return;
    
    setScenarioLoading(prev => ({ ...prev, [scenario]: true }));
    try {
      await api.setScenarioModel(scenario, modelId);
      setScenarioModels(prev => ({ ...prev, [scenario]: modelId }));
      message.success(`${scenario} 默认模型已更新`);
    } catch (error) {
      message.error('更新默认模型失败');
    } finally {
      setScenarioLoading(prev => ({ ...prev, [scenario]: false }));
    }
  };

  const loadAuthStatus = async () => {
    if (!api.getAuthStatus) return;
    
    try {
      const status = await api.getAuthStatus();
      setAuthStatus(status);
    } catch (error) {
      console.error('Failed to load auth status:', error);
    }
  };

  // ================================================================
  // Channel Functions
  // ================================================================
  const loadChannels = async () => {
    if (!api.getChannels) return;

    try {
      const list = await api.getChannels();
      setChannels(list);
    } catch (error) {
      console.error('Failed to load channels:', error);
    }
  };

  const handleConfigureChannel = async (channelType: string) => {
    if (!api.getChannelConfig) return;

    try {
      const config = await api.getChannelConfig(channelType);
      setCurrentChannelType(channelType);
      setCurrentConfig(config as ChannelConfig);
      setConfigModalVisible(true);
    } catch (error) {
      message.error('获取渠道配置失败');
    }
  };

  const handleToggleChannel = async (channelType: string, enabled: boolean) => {
    setChannelLoading(prev => ({ ...prev, [channelType]: true }));

    try {
      if (enabled) {
        if (api.startChannel) {
          await api.startChannel(channelType);
          message.success('渠道已启动');
        }
      } else {
        if (api.stopChannel) {
          await api.stopChannel(channelType);
          message.success('渠道已停止');
        }
      }
      // Wait a short moment for the channel to fully start/stop
      await new Promise(resolve => setTimeout(resolve, 500));
      await loadChannels();
    } catch (error) {
      message.error(enabled ? '启动渠道失败' : '停止渠道失败');
    } finally {
      setChannelLoading(prev => ({ ...prev, [channelType]: false }));
    }
  };

  const handleSaveChannelConfig = async (config: ChannelConfig) => {
    if (!api.updateChannelConfig || !currentChannelType) return;

    await api.updateChannelConfig(currentChannelType, config);
    loadChannels();
  };

  const handleRestartService = async () => {
    if (!api.restartService) {
      message.error('此功能仅在桌面应用中可用');
      return;
    }
    
    try {
      setLoading(true);
      await api.restartService();
      message.success('服务已重启');
      setTimeout(loadServiceStatus, 2000);
    } catch (error) {
      message.error('重启服务失败');
    } finally {
      setLoading(false);
    }
  };

  const handleStartDeviceLogin = async () => {
    if (!api.startDeviceLogin) {
      message.error('请在桌面应用中使用此功能');
      return;
    }

    try {
      setLoading(true);
      setLoginStep('waiting');
      
      const response = await api.startDeviceLogin();
      setDeviceCode(response);
      setLoginStep('polling');
      
      // Start polling
      startPolling(response.device_code, response.interval);
    } catch (error) {
      message.error('启动登录失败: ' + String(error));
      setLoginStep('idle');
    } finally {
      setLoading(false);
    }
  };

  const startPolling = useCallback((code: string, interval: number) => {
    if (!api.pollDeviceLogin) return;

    let currentInterval = Math.max(interval, 5) * 1000;
    
    const poll = async () => {
      try {
        const result = await api.pollDeviceLogin!(code);
        
        if (result.success) {
          // Success!
          setLoginStep('idle');
          setDeviceCode(null);
          message.success('登录成功！');
          loadAuthStatus();
          loadServiceStatus();
          return; // Stop polling
        } else if (result.error && result.error !== 'pending') {
          // Error other than pending
          setLoginStep('idle');
          setDeviceCode(null);
          message.error('登录失败: ' + result.error);
          return; // Stop polling
        }
        
        // If pending, check if we need to slow down
        if (result.interval && result.interval > 0) {
          currentInterval = result.interval * 1000;
          console.log('Adjusting poll interval to', result.interval, 'seconds');
        }
        
        // Schedule next poll
        pollingRef.current = window.setTimeout(poll, currentInterval);
      } catch (error) {
        console.error('Polling error:', error);
        // Continue polling on network errors
        pollingRef.current = window.setTimeout(poll, currentInterval);
      }
    };
    
    // Start first poll after initial interval
    pollingRef.current = window.setTimeout(poll, currentInterval);
  }, [api]);

  const handleCancelLogin = () => {
    if (pollingRef.current) {
      clearTimeout(pollingRef.current);
      pollingRef.current = null;
    }
    setLoginStep('idle');
    setDeviceCode(null);
  };

  const handleLogout = () => {
    Modal.confirm({
      title: '确认退出登录',
      icon: <ExclamationCircleOutlined />,
      content: '退出登录后将无法使用 AI 功能，确定要退出吗？',
      okText: '确认退出',
      cancelText: '取消',
      onOk: async () => {
        if (!api.logout) return;

        try {
          await api.logout();
          message.success('已退出登录');
          loadAuthStatus();
        } catch (error) {
          message.error('退出失败: ' + String(error));
        }
      },
    });
  };

  const handleQuitApp = () => {
    Modal.confirm({
      title: '确认退出应用',
      icon: <PoweroffOutlined />,
      content: '确定要退出 Mote 应用吗？',
      okText: '退出',
      cancelText: '取消',
      okType: 'danger',
      onOk: async () => {
        if (!api.quit) return;
        try {
          await api.quit();
        } catch (error) {
          console.error('Quit failed:', error);
        }
      },
    });
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    message.success('已复制到剪贴板');
  };

  const formatUptime = (seconds: number): string => {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);
    return `${hours}小时 ${minutes}分钟 ${secs}秒`;
  };

  return (
    <div className="settings-page" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Fixed Header */}
      <div style={{ padding: '12px 24px', borderBottom: `1px solid ${token.colorBorderSecondary}`, background: token.colorBgContainer, flexShrink: 0 }}>
        <Title level={4} style={{ margin: 0 }}>设置</Title>
      </div>
      
      {/* Scrollable Content */}
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ maxWidth: 900 }}>
        {/* Service Status */}
        {!hideStatusCard && (
        <Card style={{ marginBottom: 24 }}>
          <Title level={5}>服务状态</Title>
          {serviceStatus && (
            <>
              <Descriptions column={2}>
                <Descriptions.Item label="状态">
                  <Badge
                    status={serviceStatus.running ? 'success' : 'error'}
                    text={serviceStatus.running ? '运行中' : '已停止'}
                  />
                </Descriptions.Item>
                <Descriptions.Item label="端口">
                  {serviceStatus.port}
                </Descriptions.Item>
                <Descriptions.Item label="版本">
                  {serviceStatus.version}
                </Descriptions.Item>
                <Descriptions.Item label="运行时间">
                  {serviceStatus.running ? formatUptime(serviceStatus.uptime || 0) : '-'}
                </Descriptions.Item>
              </Descriptions>
              {/* Show service control buttons if supported */}
              {(hasRestartSupport || hasQuitSupport) && (
                <div style={{ marginTop: 16 }}>
                  <Space>
                    {hasRestartSupport && (
                      <Button
                        icon={<ReloadOutlined />}
                        onClick={handleRestartService}
                        loading={loading}
                      >
                        重启服务
                      </Button>
                    )}
                    {hasQuitSupport && (
                      <Button
                        icon={<PoweroffOutlined />}
                        onClick={handleQuitApp}
                        danger
                      >
                        退出应用
                      </Button>
                    )}
                  </Space>
                </div>
              )}
            </>
          )}
        </Card>
        )}

        {/* Provider Selection - Multi-select */}
        <Card style={{ marginBottom: 24 }}>
          <Title level={5}>
            <SettingOutlined style={{ marginRight: 8 }} />
            AI Provider
          </Title>
          <Paragraph type="secondary">
            选择要启用的 AI 服务提供商。可同时启用多个 Provider，模型列表将合并显示。配置更改将立即生效。
          </Paragraph>
          
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            {/* Provider Checkboxes */}
            <div>
              <Text strong style={{ marginBottom: 8, display: 'block' }}>启用的 Provider:</Text>
              <Checkbox.Group
                value={enabledProviders}
                onChange={(values) => handleProvidersChange(values as string[])}
                disabled={providerLoading}
              >
                <Space direction="vertical">
                  <Checkbox value="copilot">
                    <Space>
                      <GithubOutlined />
                      Copilot API (免费模型)
                      {defaultProvider === 'copilot' && <Tag color="blue" style={{ marginLeft: 4 }}>默认</Tag>}
                    </Space>
                  </Checkbox>
                  <Checkbox value="copilot-acp">
                    <Space>
                      <RocketOutlined />
                      Copilot ACP (付费模型)
                      {defaultProvider === 'copilot-acp' && <Tag color="blue" style={{ marginLeft: 4 }}>默认</Tag>}
                    </Space>
                  </Checkbox>
                  <Checkbox value="ollama">
                    <Space>
                      <OllamaIcon />
                      Ollama (本地)
                      {defaultProvider === 'ollama' && <Tag color="blue" style={{ marginLeft: 4 }}>默认</Tag>}
                    </Space>
                  </Checkbox>
                </Space>
              </Checkbox.Group>
            </div>

            {/* Default Provider Selection */}
            {enabledProviders.length > 1 && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <Text strong>默认 Provider:</Text>
                <Select
                  value={defaultProvider}
                  onChange={handleDefaultProviderChange}
                  loading={providerLoading}
                  style={{ width: 200 }}
                >
                  {enabledProviders.map(p => (
                    <Select.Option key={p} value={p}>
                      <Space>
                        {getProviderIcon(p)}
                        {getProviderLabel(p)}
                      </Space>
                    </Select.Option>
                  ))}
                </Select>
              </div>
            )}

            {/* Provider Status Cards */}
            {providerStatuses.length > 0 && (
              <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginTop: 8 }}>
                {providerStatuses.map(status => (
                  <Card 
                    key={status.name} 
                    size="small" 
                    style={{ minWidth: 200 }}
                  >
                    <Space direction="vertical" size="small" style={{ width: '100%' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Space>
                          {getProviderIcon(status.name)}
                          <Text strong>{getProviderLabel(status.name)}</Text>
                        </Space>
                        <Badge 
                          status={status.available ? 'success' : 'error'} 
                          text={status.available ? '已连接' : '未连接'} 
                        />
                      </div>
                      <Text type="secondary">模型数量: {status.model_count}</Text>
                      {status.error && (
                        <Text type="danger" style={{ fontSize: 12 }}>{status.error}</Text>
                      )}
                    </Space>
                  </Card>
                ))}
              </div>
            )}
            
            {enabledProviders.includes('copilot') && (
              <Alert
                type="info"
                message="GitHub Copilot API 需要有效的订阅和认证"
                showIcon
              />
            )}

            {enabledProviders.includes('copilot-acp') && (
              <Alert
                type="info"
                message="Copilot ACP 需要本地安装 GitHub Copilot CLI 并完成认证"
                showIcon
              />
            )}
            
            {enabledProviders.includes('ollama') && (
              <div>
                <Alert
                  type="info"
                  message="Ollama 需要在本地运行 Ollama 服务"
                  showIcon
                  style={{ marginBottom: 12 }}
                />
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Text strong style={{ minWidth: 100 }}>服务地址:</Text>
                  <Input
                    value={ollamaEndpoint}
                    onChange={(e) => setOllamaEndpoint(e.target.value)}
                    placeholder="http://localhost:11434"
                    style={{ width: 300 }}
                  />
                  <Button 
                    type="primary" 
                    onClick={handleOllamaEndpointSave}
                    loading={ollamaLoading}
                  >
                    保存
                  </Button>
                </div>
              </div>
            )}
          </Space>
        </Card>

        {/* Scenario Default Models */}
        {hasScenarioModelsSupport && (
          <Card style={{ marginBottom: 24 }}>
            <Title level={5}>
              <SettingOutlined style={{ marginRight: 8 }} />
              场景默认模型
            </Title>
            <Paragraph type="secondary">
              为不同场景设置默认模型。新建会话时将自动使用对应场景的默认模型。
              {enabledProviders.length > 1 && ' 模型已按 Provider 分组显示。'}
            </Paragraph>
            
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="对话 (Chat)">
                {renderModelSelector(
                  scenarioModels.chat,
                  (value) => handleScenarioModelChange('chat', value),
                  scenarioLoading.chat
                )}
              </Descriptions.Item>
              <Descriptions.Item label="定时任务 (Cron)">
                {renderModelSelector(
                  scenarioModels.cron,
                  (value) => handleScenarioModelChange('cron', value),
                  scenarioLoading.cron
                )}
              </Descriptions.Item>
              <Descriptions.Item label="消息渠道 (Channel)">
                {renderModelSelector(
                  scenarioModels.channel,
                  (value) => handleScenarioModelChange('channel', value),
                  scenarioLoading.channel
                )}
              </Descriptions.Item>
            </Descriptions>
            <Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
              <Text type="secondary">提示：在会话中切换模型不会影响这里的默认设置。</Text>
            </Paragraph>
          </Card>
        )}

        {/* GitHub Copilot Authentication - Show if auth API is supported */}
        {hasAuthSupport && (
          <Card style={{ marginBottom: 24 }}>
            <Title level={5}>
              <GithubOutlined style={{ marginRight: 8 }} />
              GitHub Copilot 认证
            </Title>
            
            <Paragraph type="secondary">
              Mote 使用 GitHub Copilot 作为 AI 提供商。您需要拥有有效的 GitHub Copilot 订阅才能使用 AI 功能。
            </Paragraph>

            {authStatus?.authenticated ? (
              // Logged in state
              <div>
                <Alert
                  message="已认证"
                  description={
                    <div>
                      <Text>Token: {authStatus.token_masked}</Text>
                    </div>
                  }
                  type="success"
                  showIcon
                  icon={<CheckCircleOutlined />}
                  style={{ marginBottom: 16 }}
                />
                {api.logout ? (
                  <Button
                    icon={<LogoutOutlined />}
                    onClick={handleLogout}
                    danger
                  >
                    退出登录
                  </Button>
                ) : (
                  <Text type="secondary">
                    使用 <code>mote auth logout</code> 命令退出登录
                  </Text>
                )}
              </div>
            ) : loginStep !== 'idle' && deviceCode ? (
              // Login in progress
              <div>
                <Steps
                  current={loginStep === 'waiting' ? 0 : 1}
                  items={[
                    { title: '获取验证码' },
                    { title: '等待授权' },
                    { title: '完成' },
                  ]}
                  style={{ marginBottom: 24 }}
                />
                
                <Card
                  style={{ 
                    textAlign: 'center', 
                    background: token.colorBgLayout,
                    marginBottom: 16,
                  }}
                >
                  <Text type="secondary">请访问以下链接并输入验证码：</Text>
                  <div style={{ margin: '16px 0' }}>
                    <a 
                      href={deviceCode.verification_uri} 
                      target="_blank" 
                      rel="noopener noreferrer"
                      style={{ fontSize: 16 }}
                    >
                      {deviceCode.verification_uri}
                    </a>
                  </div>
                  <div style={{ marginBottom: 16 }}>
                    <Text strong style={{ fontSize: 32, letterSpacing: 4 }}>
                      {deviceCode.user_code}
                    </Text>
                    <Button
                      type="text"
                      icon={<CopyOutlined />}
                      onClick={() => copyToClipboard(deviceCode.user_code)}
                      style={{ marginLeft: 8 }}
                    />
                  </div>
                  <div>
                    <Spin indicator={<LoadingOutlined spin />} />
                    <Text type="secondary" style={{ marginLeft: 8 }}>
                      等待您在浏览器中完成授权...
                    </Text>
                  </div>
                </Card>
                
                <Button onClick={handleCancelLogin}>
                  取消登录
                </Button>
              </div>
            ) : (
              // Not logged in
              <div>
                <Alert
                  message="未认证"
                  description={authStatus?.error || "请登录 GitHub 以使用 Copilot AI 功能"}
                  type="warning"
                  showIcon
                  style={{ marginBottom: 16 }}
                />
                {api.startDeviceLogin ? (
                  <Button
                    type="primary"
                    icon={<GithubOutlined />}
                    onClick={handleStartDeviceLogin}
                    loading={loading}
                    size="large"
                  >
                    使用 GitHub 登录
                  </Button>
                ) : (
                  <Alert
                    message="命令行认证"
                    description={
                      <div>
                        <Paragraph style={{ marginBottom: 8 }}>
                          Web 模式下请使用命令行进行认证：
                        </Paragraph>
                        <code style={{ 
                          display: 'block', 
                          background: token.colorBgLayout, 
                          padding: '8px 12px',
                          borderRadius: 4,
                          fontFamily: 'monospace'
                        }}>
                          mote auth device-login
                        </code>
                        <Paragraph style={{ marginTop: 8, marginBottom: 0 }} type="secondary">
                          或者直接设置 Token: <code>mote auth login --token YOUR_TOKEN</code>
                        </Paragraph>
                      </div>
                    }
                    type="info"
                    showIcon={false}
                  />
                )}
              </div>
            )}
          </Card>
        )}

        {/* Message Channels - Show if channel API is supported */}
        {hasChannelSupport && (
          <Card style={{ marginBottom: 24 }}>
            <Title level={5}>消息渠道</Title>
            <Paragraph type="secondary">
              配置和管理消息渠道，连接不同的通讯平台。
            </Paragraph>
            
            <Space direction="vertical" style={{ width: '100%' }}>
              {channels.map(channel => (
                <ChannelCard
                  key={channel.type}
                  channel={channel}
                  onConfigure={() => handleConfigureChannel(channel.type)}
                  onToggle={(enabled) => handleToggleChannel(channel.type, enabled)}
                  loading={channelLoading[channel.type]}
                />
              ))}
              {channels.length === 0 && (
                <Text type="secondary">暂无可用渠道</Text>
              )}
            </Space>
          </Card>
        )}

        {/* Channel Config Modals */}
        <IMessageConfig
          visible={configModalVisible && currentChannelType === 'imessage'}
          config={currentConfig as IMessageChannelConfig}
          onSave={handleSaveChannelConfig}
          onCancel={() => {
            setConfigModalVisible(false);
            setCurrentChannelType(null);
            setCurrentConfig(null);
          }}
        />
        <AppleNotesConfig
          visible={configModalVisible && currentChannelType === 'apple-notes'}
          config={currentConfig as AppleNotesChannelConfig}
          onSave={handleSaveChannelConfig}
          onCancel={() => {
            setConfigModalVisible(false);
            setCurrentChannelType(null);
            setCurrentConfig(null);
          }}
        />
        <AppleRemindersConfig
          visible={configModalVisible && currentChannelType === 'apple-reminders'}
          config={currentConfig as AppleRemindersChannelConfig}
          onSave={handleSaveChannelConfig}
          onCancel={() => {
            setConfigModalVisible(false);
            setCurrentChannelType(null);
            setCurrentConfig(null);
          }}
        />

        {/* Help Section */}
        {!hideHelpCard && (
        <Card>
          <Title level={5}>帮助</Title>
          <Descriptions column={1}>
            <Descriptions.Item label="如何获取 GitHub Copilot">
              <a href="https://github.com/features/copilot" target="_blank" rel="noopener noreferrer">
                访问 GitHub Copilot 官网
              </a>
            </Descriptions.Item>
            <Descriptions.Item label="问题反馈">
              <a href="https://github.com/nicholaslyang/mote/issues" target="_blank" rel="noopener noreferrer">
                GitHub Issues
              </a>
            </Descriptions.Item>
          </Descriptions>
        </Card>
        )}
      </div>
      </div>
    </div>
  );
};
