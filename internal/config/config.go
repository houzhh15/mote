package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"mote/internal/cli/defaults"
	"mote/internal/runner/delegate/cfg"
)

// Config 是应用配置的根结构体
type Config struct {
	Version  string                 `mapstructure:"version" yaml:"version"`
	Gateway  GatewayConfig          `mapstructure:"gateway" yaml:"gateway"`
	Provider ProviderConfig         `mapstructure:"provider" yaml:"provider"` // 新增: Provider 选择
	Copilot  CopilotConfig          `mapstructure:"copilot" yaml:"copilot"`
	Ollama   OllamaConfig           `mapstructure:"ollama" yaml:"ollama"`   // 新增: Ollama 配置
	Minimax  MinimaxConfig          `mapstructure:"minimax" yaml:"minimax"` // 新增: MiniMax 配置
	GLM      GLMConfig              `mapstructure:"glm" yaml:"glm"`         // 新增: GLM (智谱AI) 配置
	VLLM     VLLMConfig             `mapstructure:"vllm" yaml:"vllm"`       // 新增: vLLM 配置
	Log      LogConfig              `mapstructure:"log" yaml:"log"`
	Storage  StorageConfig          `mapstructure:"storage" yaml:"storage"`
	Memory   MemoryConfig           `mapstructure:"memory" yaml:"memory"`
	JSVM     JSVMConfig             `mapstructure:"jsvm" yaml:"jsvm"`
	Cron     CronConfig             `mapstructure:"cron" yaml:"cron"`
	MCP      MCPConfig              `mapstructure:"mcp" yaml:"mcp"`
	Channels ChannelsConfig         `mapstructure:"channels" yaml:"channels"`
	Agents   map[string]AgentConfig `mapstructure:"agents" yaml:"agents,omitempty"`
	Delegate DelegateConfig         `mapstructure:"delegate" yaml:"delegate,omitempty"`
}

// AgentConfig 子代理配置
type AgentConfig struct {
	Enabled       *bool    `json:"enabled,omitempty" mapstructure:"enabled" yaml:"enabled,omitempty"`
	Stealth       bool     `json:"stealth,omitempty" mapstructure:"stealth" yaml:"stealth,omitempty"`             // 隐身：不注入到系统提示词
	EntryPoint    bool     `json:"entry_point,omitempty" mapstructure:"entry_point" yaml:"entry_point,omitempty"` // 入口：在 @ 引用中优先展示
	Description   string   `json:"description" mapstructure:"description" yaml:"description,omitempty"`
	Provider      string   `json:"provider" mapstructure:"provider" yaml:"provider,omitempty"`
	Model         string   `json:"model" mapstructure:"model" yaml:"model,omitempty"`
	SystemPrompt  string   `json:"system_prompt" mapstructure:"system_prompt" yaml:"system_prompt,omitempty"`
	Tools         []string `json:"tools" mapstructure:"tools" yaml:"tools,omitempty"`
	Tags          []string `json:"tags,omitempty" mapstructure:"tags" yaml:"tags,omitempty"`
	MaxDepth      int      `json:"max_depth" mapstructure:"max_depth" yaml:"max_depth,omitempty"`
	Timeout       string   `json:"timeout" mapstructure:"timeout" yaml:"timeout,omitempty"`
	MaxIterations int      `json:"max_iterations" mapstructure:"max_iterations" yaml:"max_iterations,omitempty"`
	MaxTokens     int      `json:"max_tokens" mapstructure:"max_tokens" yaml:"max_tokens,omitempty"` // 最大输出 token 数，0 表示继承主 runner
	Temperature   float64  `json:"temperature" mapstructure:"temperature" yaml:"temperature,omitempty"`

	// 结构化编排
	Steps        []cfg.Step  `json:"steps,omitempty" mapstructure:"steps" yaml:"steps,omitempty"`
	MaxRecursion int         `json:"max_recursion,omitempty" mapstructure:"max_recursion" yaml:"max_recursion,omitempty"`
	Draft        *AgentDraft `json:"draft,omitempty" mapstructure:"draft" yaml:"draft,omitempty"`
}

// AgentDraft 编排草稿
type AgentDraft struct {
	Steps   []cfg.Step `json:"steps,omitempty" yaml:"steps,omitempty"`
	SavedAt time.Time  `json:"saved_at" yaml:"saved_at"`
}

// IsEnabled returns true if the agent is enabled.
// A nil Enabled pointer defaults to true (backward compatible).
func (c *AgentConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// HasSteps 返回该 Agent 是否配置了结构化编排步骤
func (c *AgentConfig) HasSteps() bool {
	return len(c.Steps) > 0
}

// GetTimeout 解析 Timeout 字段为 time.Duration。
// 未设置（空字符串）或特殊值 "0"、"none"、"infinite" 返回 0 表示无超时。
func (c *AgentConfig) GetTimeout() time.Duration {
	if c.Timeout == "" || c.Timeout == "0" || c.Timeout == "none" || c.Timeout == "infinite" {
		return 0
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 0
	}
	return d
}

// GetMaxDepth 返回该 agent 还能继续向下委派的层数。
// 0 表示未设置（继承全局限制），>0 表示从当前位置可再委派 N 层。
func (c *AgentConfig) GetMaxDepth() int {
	return c.MaxDepth
}

// DelegateConfig 全局委托默认配置
type DelegateConfig struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled"`
	MaxStackDepth  int    `mapstructure:"max_stack_depth" yaml:"max_stack_depth,omitempty"` // 替代 MaxDepth，0 = 无限制
	MaxDepth       int    `mapstructure:"max_depth" yaml:"max_depth,omitempty"`             // deprecated: use max_stack_depth
	DefaultTimeout string `mapstructure:"default_timeout" yaml:"default_timeout,omitempty"`
}

// GetDefaultTimeout 解析 DefaultTimeout 字段为 time.Duration，默认返回 5 分钟
func (c *DelegateConfig) GetDefaultTimeout() time.Duration {
	if c.DefaultTimeout == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(c.DefaultTimeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// GetMaxDepth 返回全局最大递归深度，默认 3
// Deprecated: 优先使用 GetMaxStackDepth
func (c *DelegateConfig) GetMaxDepth() int {
	if c.MaxDepth <= 0 {
		return 3
	}
	return c.MaxDepth
}

// GetMaxStackDepth 返回最大 PDA 栈深度。
// 优先读 MaxStackDepth，兼容旧 MaxDepth，0 表示无限制。
func (c *DelegateConfig) GetMaxStackDepth() int {
	if c.MaxStackDepth > 0 {
		return c.MaxStackDepth
	}
	if c.MaxDepth > 0 {
		return c.MaxDepth
	}
	return 0
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	Port      int             `mapstructure:"port" yaml:"port"`
	Host      string          `mapstructure:"host" yaml:"host"`
	StaticDir string          `mapstructure:"static_dir" yaml:"static_dir"`
	UIDir     string          `mapstructure:"ui_dir" yaml:"ui_dir"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit" yaml:"rate_limit"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled           bool          `mapstructure:"enabled" yaml:"enabled"`
	RequestsPerMinute int           `mapstructure:"requests_per_minute" yaml:"requests_per_minute"`
	Burst             int           `mapstructure:"burst" yaml:"burst"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval" yaml:"cleanup_interval"`
}

// CopilotConfig AI 助手配置
// Copilot 支持两种模式，由选择的模型自动决定：
//   - API 模式：使用免费模型 (gpt-4.1, gpt-4o 等)，通过 GitHub Token + REST API 认证
//   - ACP 模式：使用付费模型 (claude-sonnet-4.5 等)，通过 Copilot CLI 认证，按 prompt 计费
type CopilotConfig struct {
	Token     string `mapstructure:"token" yaml:"token"`
	Model     string `mapstructure:"model" yaml:"model"` // 默认模型（向后兼容）
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"`

	// ACP mode configuration
	Mode          string `mapstructure:"mode" yaml:"mode"`                       // Deprecated: 模型选择自动决定模式，保留字段仅为向后兼容
	AllowAllTools bool   `mapstructure:"allow_all_tools" yaml:"allow_all_tools"` // Auto-approve all tool calls in ACP mode
}

// ProviderConfig Provider 选择配置
type ProviderConfig struct {
	Default string   `mapstructure:"default" yaml:"default"` // 默认 Provider: copilot, ollama
	Enabled []string `mapstructure:"enabled" yaml:"enabled"` // 启用的 Provider 列表
}

// GetEnabledProviders 返回启用的 Provider 列表
// 向后兼容逻辑：
// 1. 如果 Enabled 非空，检查旧格式并迁移
// 2. 如果 Enabled 为空但 Default 非空，返回 [Default]
// 3. 如果都为空，默认返回 ["copilot-acp"]
//
// NOTE: copilot REST API 模式已暂时禁用，仅保留 copilot-acp (CLI 模式)。
// 旧配置中 ["copilot"] 将自动迁移为 ["copilot-acp"]。
func (c *ProviderConfig) GetEnabledProviders() []string {
	if len(c.Enabled) > 0 {
		// 向后兼容：旧配置 ["copilot"] 迁移为 ["copilot-acp"]
		// copilot REST API 已暂时禁用
		result := make([]string, 0, len(c.Enabled))
		hasCopilotACP := false
		for _, p := range c.Enabled {
			if p == "copilot-acp" {
				hasCopilotACP = true
			}
		}
		for _, p := range c.Enabled {
			if p == "copilot" {
				// 将旧的 "copilot" 替换为 "copilot-acp"（如果列表中还没有的话）
				if !hasCopilotACP {
					result = append(result, "copilot-acp")
					hasCopilotACP = true
				}
				// 跳过 "copilot"，不再添加到列表
				continue
			}
			result = append(result, p)
		}
		if len(result) == 0 {
			return []string{"copilot-acp"}
		}
		return result
	}
	if c.Default != "" {
		if c.Default == "copilot" {
			return []string{"copilot-acp"}
		}
		return []string{c.Default}
	}
	return []string{"copilot-acp"}
}

// OllamaConfig Ollama 本地 LLM 配置
type OllamaConfig struct {
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`     // API 地址
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型
	Timeout   string `mapstructure:"timeout" yaml:"timeout"`       // 超时时间
	KeepAlive string `mapstructure:"keep_alive" yaml:"keep_alive"` // 模型保持时间
}

// MinimaxConfig MiniMax 云端 LLM 配置
type MinimaxConfig struct {
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`       // API Key
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`     // API 地址
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"` // 最大输出 token 数
	Timeout   string `mapstructure:"timeout" yaml:"timeout"`       // 超时时间
}

// GLMConfig GLM (智谱AI) 云端 LLM 配置
type GLMConfig struct {
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`       // API Key
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`     // API 地址
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"` // 最大输出 token 数
	Timeout   string `mapstructure:"timeout" yaml:"timeout"`       // 超时时间
}

// VLLMConfig vLLM 本地高性能推理服务配置
type VLLMConfig struct {
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`       // API Key (可选，vLLM --api-key 启动时需要)
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`     // API 地址 (默认: http://localhost:8000)
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型 (留空自动从 /v1/models 获取)
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"` // 最大输出 token 数
	Timeout   string `mapstructure:"timeout" yaml:"timeout"`       // 超时时间
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
	File   string `mapstructure:"file" yaml:"file"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Driver string `mapstructure:"driver" yaml:"driver"`
	Path   string `mapstructure:"path" yaml:"path"`
}

// MemoryConfig 记忆系统配置
type MemoryConfig struct {
	Enabled     bool              `mapstructure:"enabled" yaml:"enabled"`
	AutoCapture AutoCaptureConfig `mapstructure:"auto_capture" yaml:"auto_capture"`
	AutoRecall  AutoRecallConfig  `mapstructure:"auto_recall" yaml:"auto_recall"`
}

// AutoCaptureConfig P2 自动捕获配置
type AutoCaptureConfig struct {
	Enabled       bool    `mapstructure:"enabled" yaml:"enabled"`
	MinLength     int     `mapstructure:"min_length" yaml:"min_length"`           // 最小内容长度
	MaxLength     int     `mapstructure:"max_length" yaml:"max_length"`           // 最大内容长度
	DupThreshold  float64 `mapstructure:"dup_threshold" yaml:"dup_threshold"`     // 重复检测阈值
	MaxPerSession int     `mapstructure:"max_per_session" yaml:"max_per_session"` // 每会话最大捕获数
}

// AutoRecallConfig P2 自动召回配置
type AutoRecallConfig struct {
	Enabled      bool    `mapstructure:"enabled" yaml:"enabled"`
	Limit        int     `mapstructure:"limit" yaml:"limit"`                   // 最大召回数量
	Threshold    float64 `mapstructure:"threshold" yaml:"threshold"`           // 相关性阈值
	MinPromptLen int     `mapstructure:"min_prompt_len" yaml:"min_prompt_len"` // 最小提示词长度
}

// JSVMConfig JavaScript VM 配置
type JSVMConfig struct {
	Enabled        bool          `mapstructure:"enabled" yaml:"enabled"`
	PoolSize       int           `mapstructure:"pool_size" yaml:"pool_size"`
	IdleTimeout    time.Duration `mapstructure:"idle_timeout" yaml:"idle_timeout"`
	AcquireTimeout time.Duration `mapstructure:"acquire_timeout" yaml:"acquire_timeout"`
	Timeout        time.Duration `mapstructure:"timeout" yaml:"timeout"`
	MemoryLimit    string        `mapstructure:"memory_limit" yaml:"memory_limit"`
	AllowedPaths   []string      `mapstructure:"allowed_paths" yaml:"allowed_paths"`
	HTTPAllowlist  []string      `mapstructure:"http_allowlist" yaml:"http_allowlist"`
}

// RetryConfig 重试策略配置
type RetryConfig struct {
	MaxAttempts  int           `mapstructure:"max_attempts" yaml:"max_attempts"`
	InitialDelay time.Duration `mapstructure:"initial_delay" yaml:"initial_delay"`
	MaxDelay     time.Duration `mapstructure:"max_delay" yaml:"max_delay"`
}

// CronConfig 定时任务配置
type CronConfig struct {
	Enabled      bool        `mapstructure:"enabled" yaml:"enabled"`
	Model        string      `mapstructure:"model" yaml:"model"` // Cron场景默认模型
	HistoryLimit int         `mapstructure:"history_limit" yaml:"history_limit"`
	Retry        RetryConfig `mapstructure:"retry" yaml:"retry"`
}

// MCPConfig MCP 配置
type MCPConfig struct {
	Server MCPServerConfig `mapstructure:"server" yaml:"server"`
	Client MCPClientConfig `mapstructure:"client" yaml:"client"`
}

// MCPServerConfig MCP 服务端配置
type MCPServerConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	Transport string `mapstructure:"transport" yaml:"transport"`
}

// MCPClientConfig MCP 客户端配置
type MCPClientConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

// ChannelsConfig 渠道配置
type ChannelsConfig struct {
	Model          string               `mapstructure:"model" yaml:"model"` // Channels场景默认模型
	IMessage       IMessageConfig       `mapstructure:"imessage" yaml:"imessage"`
	AppleNotes     AppleNotesConfig     `mapstructure:"apple_notes" yaml:"apple_notes"`
	AppleReminders AppleRemindersConfig `mapstructure:"apple_reminders" yaml:"apple_reminders"`
}

// TriggerConfig 触发配置
type TriggerConfig struct {
	Prefix        string   `mapstructure:"prefix" yaml:"prefix"`
	CaseSensitive bool     `mapstructure:"case_sensitive" yaml:"case_sensitive"`
	SelfTrigger   bool     `mapstructure:"self_trigger" yaml:"self_trigger"`
	AllowList     []string `mapstructure:"allow_list" yaml:"allow_list"`
}

// ReplyConfig 回复配置
type ReplyConfig struct {
	Prefix    string `mapstructure:"prefix" yaml:"prefix"`
	Separator string `mapstructure:"separator" yaml:"separator"`
}

// IMessageConfig iMessage 渠道配置
type IMessageConfig struct {
	Enabled   bool          `mapstructure:"enabled" yaml:"enabled"`
	Model     string        `mapstructure:"model" yaml:"model,omitempty"` // 渠道专属模型（空=使用默认）
	SelfID    string        `mapstructure:"self_id" yaml:"self_id"`
	Trigger   TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply     ReplyConfig   `mapstructure:"reply" yaml:"reply"`
	AllowFrom []string      `mapstructure:"allow_from" yaml:"allow_from"` // 允许的发信人白名单
}

// AppleNotesConfig Apple Notes 渠道配置
type AppleNotesConfig struct {
	Enabled       bool          `mapstructure:"enabled" yaml:"enabled"`
	Model         string        `mapstructure:"model" yaml:"model,omitempty"` // 渠道专属模型（空=使用默认）
	WatchFolder   string        `mapstructure:"watch_folder" yaml:"watch_folder"`
	ArchiveFolder string        `mapstructure:"archive_folder" yaml:"archive_folder"`
	PollInterval  time.Duration `mapstructure:"poll_interval" yaml:"poll_interval"`
	Trigger       TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply         ReplyConfig   `mapstructure:"reply" yaml:"reply"`
}

// AppleRemindersConfig Apple Reminders 渠道配置
type AppleRemindersConfig struct {
	Enabled      bool          `mapstructure:"enabled" yaml:"enabled"`
	Model        string        `mapstructure:"model" yaml:"model,omitempty"` // 渠道专属模型（空=使用默认）
	WatchList    string        `mapstructure:"watch_list" yaml:"watch_list"`
	PollInterval time.Duration `mapstructure:"poll_interval" yaml:"poll_interval"`
	Trigger      TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply        ReplyConfig   `mapstructure:"reply" yaml:"reply"`
}

var (
	globalConfig     *Config
	configPath       string
	agentsConfigPath string // 独立的 agents 配置文件路径 (agents.yaml)
	agentsDirPath    string // agents 目录路径 (agents/)
	// agentSources 记录每个 agent 的来源文件路径。
	// 值为 agents/ 目录下的绝对路径表示来自该文件；空字符串表示来自 agents.yaml。
	agentSources map[string]string
	mu           sync.RWMutex
)

// Load 加载配置文件
// 优先级: ENV > 配置文件 > 默认值
func Load(path string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	// 设置默认值
	SetDefaults()

	// 设置环境变量前缀
	viper.SetEnvPrefix("MOTE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 如果提供了配置路径，则加载配置文件
	if path != "" {
		expandedPath, err := ExpandPath(path)
		if err != nil {
			return nil, err
		}
		configPath = expandedPath

		viper.SetConfigFile(expandedPath)
		if err := viper.ReadInConfig(); err != nil {
			// 忽略文件不存在错误
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) && !os.IsNotExist(err) {
				// 如果是配置文件解析错误，则返回
				if _, ok := err.(viper.ConfigParseError); ok {
					return nil, err
				}
			}
		}
	}

	// 反序列化到结构体
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// 尝试从独立的 agents.yaml 加载 agents（优先级高于 config.yaml）
	if configPath != "" {
		agentsConfigPath = filepath.Join(filepath.Dir(configPath), "agents.yaml")
		agentSources = make(map[string]string) // 初始化来源追踪

		// 如果 agents.yaml 不存在，安装内嵌的默认版本
		if _, statErr := os.Stat(agentsConfigPath); os.IsNotExist(statErr) {
			if defaultData := defaults.GetDefaultAgentsYAML(); len(defaultData) > 0 {
				if writeErr := os.WriteFile(agentsConfigPath, defaultData, 0600); writeErr == nil {
					slog.Info("installed default agents.yaml",
						"path", agentsConfigPath)
				}
			}
		}

		if agentsData, err := os.ReadFile(agentsConfigPath); err == nil {
			var agentsFile struct {
				Agents map[string]AgentConfig `yaml:"agents"`
			}
			if err := yaml.Unmarshal(agentsData, &agentsFile); err == nil && agentsFile.Agents != nil {
				cfg.Agents = agentsFile.Agents
				// agents.yaml 中的 agent 来源标记为空字符串
				for name := range agentsFile.Agents {
					agentSources[name] = ""
				}
			}
		}

		// 尝试从 agents/ 目录加载额外的 agent 配置文件（优先级高于 agents.yaml）
		agentsDirPath = filepath.Join(filepath.Dir(configPath), "agents")

		// 安装内嵌的默认 agent 目录文件（仅当文件不存在时）
		if defaultFiles := defaults.GetDefaultAgentDirFiles(); len(defaultFiles) > 0 {
			if mkErr := os.MkdirAll(agentsDirPath, 0755); mkErr == nil {
				for name, data := range defaultFiles {
					targetPath := filepath.Join(agentsDirPath, name)
					if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
						if writeErr := os.WriteFile(targetPath, data, 0600); writeErr == nil {
							slog.Info("installed default agent file",
								"path", targetPath)
						}
					}
				}
			}
		}

		if dirAgents, err := loadAgentsFromDir(agentsDirPath); err == nil && len(dirAgents) > 0 {
			if cfg.Agents == nil {
				cfg.Agents = make(map[string]AgentConfig)
			}
			for name, agent := range dirAgents {
				cfg.Agents[name] = agent
				// agentSources 已在 loadAgentsFromDir 中设置
			}
			slog.Info("loaded agents from directory",
				"path", agentsDirPath,
				"count", len(dirAgents))
		}
	}

	globalConfig = &cfg
	return &cfg, nil
}

// GetConfig 获取当前配置
func GetConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return globalConfig
}

// Get 获取任意配置键值
func Get(key string) any {
	return viper.Get(key)
}

// GetString 获取字符串配置值
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt 获取整数配置值
func GetInt(key string) int {
	return viper.GetInt(key)
}

// GetBool 获取布尔配置值
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// Set 设置配置值并持久化
func Set(key string, value any) error {
	mu.Lock()
	defer mu.Unlock()

	viper.Set(key, value)

	// 如果有配置文件路径，则持久化
	if configPath != "" {
		return save()
	}
	return nil
}

// AddAgent 添加一个新的代理配置并持久化
func AddAgent(name string, agent AgentConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if globalConfig == nil {
		return errors.New("config not loaded")
	}
	if globalConfig.Agents == nil {
		globalConfig.Agents = make(map[string]AgentConfig)
	}
	if _, exists := globalConfig.Agents[name]; exists {
		return fmt.Errorf("agent already exists: %s", name)
	}
	globalConfig.Agents[name] = agent
	// 新添加的 agent 默认归属 agents.yaml（来源为空）
	if agentSources != nil {
		agentSources[name] = ""
	}
	viper.Set("agents", globalConfig.Agents)
	return saveAgents()
}

// UpdateAgent 更新已有代理配置并持久化
func UpdateAgent(name string, agent AgentConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if globalConfig == nil {
		return errors.New("config not loaded")
	}
	if globalConfig.Agents == nil || len(globalConfig.Agents) == 0 {
		return fmt.Errorf("agent not found: %s", name)
	}
	if _, exists := globalConfig.Agents[name]; !exists {
		return fmt.Errorf("agent not found: %s", name)
	}
	globalConfig.Agents[name] = agent
	// 来源保持不变（agentSources 中已记录）
	viper.Set("agents", globalConfig.Agents)
	return saveAgents()
}

// RemoveAgent 移除代理配置并持久化
func RemoveAgent(name string) error {
	mu.Lock()
	defer mu.Unlock()

	if globalConfig == nil {
		return errors.New("config not loaded")
	}
	if globalConfig.Agents == nil {
		return fmt.Errorf("agent not found: %s", name)
	}
	if _, exists := globalConfig.Agents[name]; !exists {
		return fmt.Errorf("agent not found: %s", name)
	}
	delete(globalConfig.Agents, name)
	// 清理来源追踪
	if agentSources != nil {
		delete(agentSources, name)
	}
	viper.Set("agents", globalConfig.Agents)
	return saveAgents()
}

// Save 保存配置到文件
func Save() error {
	mu.Lock()
	defer mu.Unlock()
	return save()
}

// saveAgents 将 agents 按来源分别保存到对应文件。
// 来自 agents/ 目录的 agent 写回其源文件；其余写入 agents.yaml。
// 如果 agentsConfigPath 未设置（例如测试环境），回退到 save()。
func saveAgents() error {
	if agentsConfigPath == "" {
		// 没有独立文件路径，回退到保存整个 config
		if configPath != "" {
			return save()
		}
		return nil
	}

	dir := filepath.Dir(agentsConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 按来源文件分组：空字符串 → agents.yaml，其他 → agents/ 目录文件
	yamlAgents := make(map[string]AgentConfig)               // 属于 agents.yaml 的
	dirFileAgents := make(map[string]map[string]AgentConfig) // filePath → { name → config }
	// 记录单文件格式的 agent（文件名即 agent 名）
	singleFileAgents := make(map[string]string) // filePath → agentName

	for name, cfg := range globalConfig.Agents {
		source, tracked := "", false
		if agentSources != nil {
			source, tracked = agentSources[name]
		}

		if !tracked || source == "" {
			// 未追踪或来自 agents.yaml
			yamlAgents[name] = cfg
		} else {
			// 来自 agents/ 目录
			if dirFileAgents[source] == nil {
				dirFileAgents[source] = make(map[string]AgentConfig)
			}
			dirFileAgents[source][name] = cfg
		}
	}

	// 1. 保存 agents.yaml（仅包含属于它的 agents）
	agentsFileData := struct {
		Agents map[string]AgentConfig `yaml:"agents"`
	}{
		Agents: yamlAgents,
	}
	data, err := yaml.Marshal(agentsFileData)
	if err != nil {
		return fmt.Errorf("marshal agents.yaml: %w", err)
	}
	if err := os.WriteFile(agentsConfigPath, data, 0600); err != nil {
		return fmt.Errorf("write agents.yaml: %w", err)
	}

	// 2. 保存 agents/ 目录中的各文件
	for filePath, agents := range dirFileAgents {
		// 检测原始文件格式：如果该文件只有一个 agent 且名称等于文件名（去掉扩展名），
		// 则使用单 agent 格式保存
		isSingle := false
		if len(agents) == 1 {
			baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			for agentName := range agents {
				if agentName == baseName {
					isSingle = true
					singleFileAgents[filePath] = agentName
				}
			}
		}

		var fileData []byte
		if isSingle {
			agentName := singleFileAgents[filePath]
			fileData, err = yaml.Marshal(agents[agentName])
		} else {
			wrapper := struct {
				Agents map[string]AgentConfig `yaml:"agents"`
			}{Agents: agents}
			fileData, err = yaml.Marshal(wrapper)
		}
		if err != nil {
			slog.Warn("failed to marshal agent file", "path", filePath, "error", err)
			continue
		}
		if writeErr := os.WriteFile(filePath, fileData, 0600); writeErr != nil {
			slog.Warn("failed to write agent file", "path", filePath, "error", writeErr)
		}
	}

	return nil
}

// GetAgentsConfigPath 返回独立 agents 配置文件的路径
func GetAgentsConfigPath() string {
	mu.RLock()
	defer mu.RUnlock()
	return agentsConfigPath
}

// save 内部保存函数，调用者需要持有锁
func save() error {
	if configPath == "" {
		return errors.New("config path not set")
	}

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 获取所有配置
	allSettings := viper.AllSettings()

	// 序列化为 YAML
	data, err := yaml.Marshal(allSettings)
	if err != nil {
		return err
	}

	// 写入文件 (M08B: 使用 0600 保护含 API Key 的配置文件)
	return os.WriteFile(configPath, data, 0600)
}

// SaveTo 保存配置到指定路径
func SaveTo(cfg *Config, path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(path, data, 0600) // 0600 for security (contains tokens)
}

// loadAgentsFromDir 从指定目录加载所有 .yaml/.yml 格式的 agent 配置文件。
// 每个文件支持两种格式：
//  1. 标准格式（与 agents.yaml 相同）：agents: { name: {...}, ... }
//  2. 单 agent 格式：文件内容直接是 AgentConfig 字段，文件名（不含扩展名）作为 agent 名称
//
// 如果目录不存在，返回空 map 和 nil error。
// 同时会更新 agentSources 记录每个 agent 的来源文件路径。
func loadAgentsFromDir(dir string) (map[string]AgentConfig, error) {
	result := make(map[string]AgentConfig)

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return result, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		filePath := filepath.Join(dir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Warn("failed to read agent file", "path", filePath, "error", err)
			continue
		}

		// 尝试标准格式：agents: { name: {...} }
		var multiFile struct {
			Agents map[string]AgentConfig `yaml:"agents"`
		}
		if err := yaml.Unmarshal(data, &multiFile); err == nil && multiFile.Agents != nil && len(multiFile.Agents) > 0 {
			for agentName, agentCfg := range multiFile.Agents {
				result[agentName] = agentCfg
				if agentSources != nil {
					agentSources[agentName] = filePath
				}
			}
			continue
		}

		// 尝试单 agent 格式：文件名作为 agent 名称
		var singleAgent AgentConfig
		if err := yaml.Unmarshal(data, &singleAgent); err == nil && singleAgent.Description != "" {
			agentName := strings.TrimSuffix(name, ext)
			result[agentName] = singleAgent
			if agentSources != nil {
				agentSources[agentName] = filePath
			}
			continue
		}

		slog.Warn("skipped agent file: unrecognized format", "path", filePath)
	}

	return result, nil
}

// GetAgentsDirPath 返回 agents 目录的路径
func GetAgentsDirPath() string {
	mu.RLock()
	defer mu.RUnlock()
	return agentsDirPath
}

// ReloadAgents 从 agents.yaml 和 agents/ 目录重新加载 agent 配置（热加载）。
// 加载优先级：agents.yaml < agents/ 目录文件
func ReloadAgents() (int, error) {
	mu.Lock()
	defer mu.Unlock()

	if globalConfig == nil {
		return 0, errors.New("config not loaded")
	}

	newAgents := make(map[string]AgentConfig)
	agentSources = make(map[string]string) // 重新初始化来源追踪

	// Step 1: 从 agents.yaml 加载
	if agentsConfigPath != "" {
		if agentsData, err := os.ReadFile(agentsConfigPath); err == nil {
			var agentsFile struct {
				Agents map[string]AgentConfig `yaml:"agents"`
			}
			if err := yaml.Unmarshal(agentsData, &agentsFile); err == nil && agentsFile.Agents != nil {
				for k, v := range agentsFile.Agents {
					newAgents[k] = v
					agentSources[k] = "" // agents.yaml 来源
				}
			}
		}
	}

	// Step 2: 从 agents/ 目录加载（优先级更高，覆盖同名 agent）
	// agentSources 会在 loadAgentsFromDir 中被更新
	if agentsDirPath != "" {
		if dirAgents, err := loadAgentsFromDir(agentsDirPath); err == nil && len(dirAgents) > 0 {
			for k, v := range dirAgents {
				newAgents[k] = v
			}
		}
	}

	globalConfig.Agents = newAgents
	viper.Set("agents", newAgents)

	slog.Info("agents reloaded",
		"total", len(newAgents))

	return len(newAgents), nil
}

// AgentsDirValidationResult 表示单个文件的验证结果
type AgentsDirValidationResult struct {
	File   string   `json:"file"`
	Valid  bool     `json:"valid"`
	Agents []string `json:"agents,omitempty"` // 该文件中成功解析的 agent 名称列表
	Error  string   `json:"error,omitempty"`  // 解析错误信息
	Format string   `json:"format,omitempty"` // "multi" (标准格式) 或 "single" (单 agent 格式)
}

// AgentsDirValidationSummary 表示 agents/ 目录的验证概要
type AgentsDirValidationSummary struct {
	DirPath     string                      `json:"dir_path"`
	Exists      bool                        `json:"exists"`
	TotalFiles  int                         `json:"total_files"`  // YAML 文件总数
	ValidFiles  int                         `json:"valid_files"`  // 有效文件数
	TotalAgents int                         `json:"total_agents"` // 总 agent 数
	Results     []AgentsDirValidationResult `json:"results"`
}

// ValidateAgentsDir 验证 agents/ 目录下所有 YAML 文件的语法和格式。
// 返回详细的验证结果供 LLM 或者用户检查。
func ValidateAgentsDir() AgentsDirValidationSummary {
	mu.RLock()
	dir := agentsDirPath
	mu.RUnlock()

	summary := AgentsDirValidationSummary{
		DirPath: dir,
		Results: make([]AgentsDirValidationResult, 0),
	}

	if dir == "" {
		return summary
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return summary
	}
	summary.Exists = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		return summary
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		summary.TotalFiles++
		result := AgentsDirValidationResult{File: name}

		filePath := filepath.Join(dir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			result.Error = fmt.Sprintf("read error: %v", err)
			summary.Results = append(summary.Results, result)
			continue
		}

		// 尝试标准格式
		var multiFile struct {
			Agents map[string]AgentConfig `yaml:"agents"`
		}
		if err := yaml.Unmarshal(data, &multiFile); err == nil && multiFile.Agents != nil && len(multiFile.Agents) > 0 {
			result.Valid = true
			result.Format = "multi"
			result.Agents = make([]string, 0, len(multiFile.Agents))
			for agentName := range multiFile.Agents {
				result.Agents = append(result.Agents, agentName)
			}
			summary.ValidFiles++
			summary.TotalAgents += len(result.Agents)
			summary.Results = append(summary.Results, result)
			continue
		}

		// 尝试单 agent 格式
		var singleAgent AgentConfig
		if yamlErr := yaml.Unmarshal(data, &singleAgent); yamlErr != nil {
			result.Error = fmt.Sprintf("YAML syntax error: %v", yamlErr)
			summary.Results = append(summary.Results, result)
			continue
		}
		if singleAgent.Description != "" {
			agentName := strings.TrimSuffix(name, ext)
			result.Valid = true
			result.Format = "single"
			result.Agents = []string{agentName}
			summary.ValidFiles++
			summary.TotalAgents++
			summary.Results = append(summary.Results, result)
			continue
		}

		// 可解析但不符合任何格式
		result.Error = "unrecognized format: no 'agents' map and no 'description' field for single-agent format"
		summary.Results = append(summary.Results, result)
	}

	return summary
}

// Reset 重置配置（主要用于测试）
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	globalConfig = nil
	configPath = ""
	agentsConfigPath = ""
	agentsDirPath = ""
	viper.Reset()
}

// SetTestConfig 设置全局配置（仅用于测试）
func SetTestConfig(cfg *Config) {
	mu.Lock()
	defer mu.Unlock()
	globalConfig = cfg
}
