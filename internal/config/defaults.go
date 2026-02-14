package config

import (
	"time"

	"github.com/spf13/viper"
)

// SetDefaults 设置所有配置项的默认值
func SetDefaults() {
	// Gateway 配置
	viper.SetDefault("gateway.port", 8080)
	viper.SetDefault("gateway.host", "127.0.0.1")

	// Log 配置
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("log.file", "")

	// Memory 配置
	viper.SetDefault("memory.enabled", true)
	// P2: Auto capture defaults
	viper.SetDefault("memory.auto_capture.enabled", true)
	viper.SetDefault("memory.auto_capture.min_length", 10)
	viper.SetDefault("memory.auto_capture.max_length", 500)
	viper.SetDefault("memory.auto_capture.dup_threshold", 0.95)
	viper.SetDefault("memory.auto_capture.max_per_session", 3)
	// P2: Auto recall defaults
	viper.SetDefault("memory.auto_recall.enabled", true)
	viper.SetDefault("memory.auto_recall.limit", 3)
	viper.SetDefault("memory.auto_recall.threshold", 0.3)
	viper.SetDefault("memory.auto_recall.min_prompt_len", 5)

	// JSVM 配置
	viper.SetDefault("jsvm.enabled", true)
	viper.SetDefault("jsvm.pool_size", 5)
	viper.SetDefault("jsvm.idle_timeout", 5*time.Minute)
	viper.SetDefault("jsvm.acquire_timeout", 5*time.Second)
	viper.SetDefault("jsvm.timeout", 30*time.Second)
	viper.SetDefault("jsvm.memory_limit", "64MB")
	viper.SetDefault("jsvm.allowed_paths", []string{"~/.mote/", "/tmp"})
	viper.SetDefault("jsvm.http_allowlist", []string{})

	// Cron 配置
	viper.SetDefault("cron.enabled", true)
	viper.SetDefault("cron.model", "gpt-4o-mini") // Cron 场景默认模型（低成本）
	viper.SetDefault("cron.history_limit", 100)
	viper.SetDefault("cron.retry.max_attempts", 3)
	viper.SetDefault("cron.retry.initial_delay", 1*time.Second)
	viper.SetDefault("cron.retry.max_delay", 1*time.Minute)

	// MCP Server 配置
	viper.SetDefault("mcp.server.enabled", true)
	viper.SetDefault("mcp.server.transport", "stdio")

	// MCP Client 配置
	viper.SetDefault("mcp.client.enabled", false)

	// Copilot 配置
	// NOTE: Default model must be compatible with provider.default (copilot-acp).
	// ACP CLI does not support API-only models like grok-code-fast-1.
	viper.SetDefault("copilot.model", "claude-sonnet-4.5") // ACP 兼容的默认模型
	viper.SetDefault("copilot.max_tokens", 4096)

	// Provider 配置
	viper.SetDefault("provider.default", "copilot-acp")
	viper.SetDefault("provider.enabled", []string{"copilot-acp"}) // 默认仅启用 ACP (CLI 模式)，copilot REST API 已暂时禁用

	// MiniMax 配置
	viper.SetDefault("minimax.endpoint", "https://api.minimaxi.com/v1")
	viper.SetDefault("minimax.model", "MiniMax-M2.5")
	viper.SetDefault("minimax.max_tokens", 16384)

	// Storage 配置
	viper.SetDefault("storage.driver", "sqlite")

	// Channels 配置
	viper.SetDefault("channels.model", "gpt-4o-mini") // Channels 场景默认模型（低成本）
	// iMessage
	viper.SetDefault("channels.imessage.enabled", false)
	viper.SetDefault("channels.imessage.self_id", "")
	viper.SetDefault("channels.imessage.trigger.prefix", "@mote")
	viper.SetDefault("channels.imessage.trigger.case_sensitive", false)
	viper.SetDefault("channels.imessage.trigger.self_trigger", true)
	viper.SetDefault("channels.imessage.trigger.allow_list", []string{})
	viper.SetDefault("channels.imessage.reply.prefix", "[Mote]")
	viper.SetDefault("channels.imessage.reply.separator", "\n")

	// Apple Notes
	viper.SetDefault("channels.apple_notes.enabled", false)
	viper.SetDefault("channels.apple_notes.watch_folder", "Mote Inbox")
	viper.SetDefault("channels.apple_notes.archive_folder", "Mote Archive")
	viper.SetDefault("channels.apple_notes.poll_interval", 5*time.Second)
	viper.SetDefault("channels.apple_notes.trigger.prefix", "@mote:")
	viper.SetDefault("channels.apple_notes.trigger.case_sensitive", false)
	viper.SetDefault("channels.apple_notes.reply.prefix", "[Mote 回复]")
	viper.SetDefault("channels.apple_notes.reply.separator", "\n")

	// Apple Reminders
	viper.SetDefault("channels.apple_reminders.enabled", false)
	viper.SetDefault("channels.apple_reminders.watch_list", "Mote Tasks")
	viper.SetDefault("channels.apple_reminders.poll_interval", 5*time.Second)
	viper.SetDefault("channels.apple_reminders.trigger.prefix", "@mote:")
	viper.SetDefault("channels.apple_reminders.trigger.case_sensitive", false)
	viper.SetDefault("channels.apple_reminders.reply.prefix", "[Mote]")
	viper.SetDefault("channels.apple_reminders.reply.separator", "\n")
}
