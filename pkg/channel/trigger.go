package channel

import "strings"

// TriggerConfig 触发配置
type TriggerConfig struct {
	Prefix        string   `json:"prefix"`        // 触发前缀 e.g. "@mote"
	CaseSensitive bool     `json:"caseSensitive"` // 是否区分大小写
	SelfTrigger   bool     `json:"selfTrigger"`   // 允许自触发
	AllowList     []string `json:"allowList"`     // 发送者白名单
}

// ReplyConfig 回复配置
type ReplyConfig struct {
	Prefix    string `json:"prefix"`    // 回复前缀 e.g. "[Mote]"
	Separator string `json:"separator"` // 分隔符 e.g. "\n"
}

// TriggerResult 触发检测结果
type TriggerResult struct {
	ShouldProcess   bool   // 是否应该处理
	SkipReason      string // 跳过原因
	StrippedContent string // 去除前缀后的内容
}

// CheckTrigger 检查消息是否触发
func CheckTrigger(msg InboundMessage, cfg TriggerConfig) TriggerResult {
	content := msg.Content
	prefix := cfg.Prefix

	// 大小写处理
	checkContent := content
	checkPrefix := prefix
	if !cfg.CaseSensitive {
		checkContent = strings.ToLower(content)
		checkPrefix = strings.ToLower(prefix)
	}

	// 检查前缀
	trimmedCheck := strings.TrimSpace(checkContent)
	if !strings.HasPrefix(trimmedCheck, checkPrefix) {
		// 自触发模式：自己发给自己的消息不需要前缀
		if cfg.SelfTrigger && msg.IsSelfSend {
			return TriggerResult{
				ShouldProcess:   true,
				StrippedContent: msg.Content,
			}
		}
		return TriggerResult{
			ShouldProcess: false,
			SkipReason:    "no trigger prefix",
		}
	}

	// 白名单检查
	if len(cfg.AllowList) > 0 {
		allowed := false
		for _, id := range cfg.AllowList {
			if msg.SenderID == id {
				allowed = true
				break
			}
		}
		if !allowed {
			return TriggerResult{
				ShouldProcess: false,
				SkipReason:    "sender not in allowlist",
			}
		}
	}

	// 去除前缀（使用原始内容，保持大小写）
	trimmed := strings.TrimSpace(content)
	// 找到前缀在原始内容中的位置并去除
	lowerTrimmed := strings.ToLower(trimmed)
	lowerPrefix := strings.ToLower(prefix)
	if strings.HasPrefix(lowerTrimmed, lowerPrefix) {
		stripped := trimmed[len(prefix):]
		stripped = strings.TrimSpace(stripped)
		return TriggerResult{
			ShouldProcess:   true,
			StrippedContent: stripped,
		}
	}

	// 如果大小写敏感且前缀匹配
	stripped := strings.TrimPrefix(trimmed, prefix)
	stripped = strings.TrimSpace(stripped)

	return TriggerResult{
		ShouldProcess:   true,
		StrippedContent: stripped,
	}
}

// InjectReplyPrefix 注入回复前缀
func InjectReplyPrefix(content string, cfg ReplyConfig) string {
	if cfg.Prefix == "" {
		return content
	}
	sep := cfg.Separator
	if sep == "" {
		sep = "\n"
	}
	return cfg.Prefix + sep + content
}
