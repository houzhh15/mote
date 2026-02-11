package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config 是应用配置的根结构体
type Config struct {
	Version  string         `mapstructure:"version" yaml:"version"`
	Gateway  GatewayConfig  `mapstructure:"gateway" yaml:"gateway"`
	Provider ProviderConfig `mapstructure:"provider" yaml:"provider"` // 新增: Provider 选择
	Copilot  CopilotConfig  `mapstructure:"copilot" yaml:"copilot"`
	Ollama   OllamaConfig   `mapstructure:"ollama" yaml:"ollama"` // 新增: Ollama 配置
	Log      LogConfig      `mapstructure:"log" yaml:"log"`
	Storage  StorageConfig  `mapstructure:"storage" yaml:"storage"`
	Memory   MemoryConfig   `mapstructure:"memory" yaml:"memory"`
	JSVM     JSVMConfig     `mapstructure:"jsvm" yaml:"jsvm"`
	Cron     CronConfig     `mapstructure:"cron" yaml:"cron"`
	MCP      MCPConfig      `mapstructure:"mcp" yaml:"mcp"`
	Channels ChannelsConfig `mapstructure:"channels" yaml:"channels"`
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
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型（向后兼容）
	ChatModel string `mapstructure:"chat_model" yaml:"chat_model"` // Chat场景默认模型
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
// 1. 如果 Enabled 非空，直接返回
// 2. 如果 Enabled 为空但 Default 非空，返回 [Default]
// 3. 如果都为空，默认返回 ["copilot"]
func (c *ProviderConfig) GetEnabledProviders() []string {
	if len(c.Enabled) > 0 {
		return c.Enabled
	}
	if c.Default != "" {
		return []string{c.Default}
	}
	return []string{"copilot"}
}

// OllamaConfig Ollama 本地 LLM 配置
type OllamaConfig struct {
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`     // API 地址
	Model     string `mapstructure:"model" yaml:"model"`           // 默认模型
	Timeout   string `mapstructure:"timeout" yaml:"timeout"`       // 超时时间
	KeepAlive string `mapstructure:"keep_alive" yaml:"keep_alive"` // 模型保持时间
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
	SelfID    string        `mapstructure:"self_id" yaml:"self_id"`
	Trigger   TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply     ReplyConfig   `mapstructure:"reply" yaml:"reply"`
	AllowFrom []string      `mapstructure:"allow_from" yaml:"allow_from"` // 允许的发信人白名单
}

// AppleNotesConfig Apple Notes 渠道配置
type AppleNotesConfig struct {
	Enabled       bool          `mapstructure:"enabled" yaml:"enabled"`
	WatchFolder   string        `mapstructure:"watch_folder" yaml:"watch_folder"`
	ArchiveFolder string        `mapstructure:"archive_folder" yaml:"archive_folder"`
	PollInterval  time.Duration `mapstructure:"poll_interval" yaml:"poll_interval"`
	Trigger       TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply         ReplyConfig   `mapstructure:"reply" yaml:"reply"`
}

// AppleRemindersConfig Apple Reminders 渠道配置
type AppleRemindersConfig struct {
	Enabled      bool          `mapstructure:"enabled" yaml:"enabled"`
	WatchList    string        `mapstructure:"watch_list" yaml:"watch_list"`
	PollInterval time.Duration `mapstructure:"poll_interval" yaml:"poll_interval"`
	Trigger      TriggerConfig `mapstructure:"trigger" yaml:"trigger"`
	Reply        ReplyConfig   `mapstructure:"reply" yaml:"reply"`
}

var (
	globalConfig *Config
	configPath   string
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

// Save 保存配置到文件
func Save() error {
	mu.Lock()
	defer mu.Unlock()
	return save()
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

	// 写入文件
	return os.WriteFile(configPath, data, 0644)
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

// Reset 重置配置（主要用于测试）
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	globalConfig = nil
	configPath = ""
	viper.Reset()
}
