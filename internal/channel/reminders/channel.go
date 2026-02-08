// Package reminders implements the Apple Reminders channel plugin.
package reminders

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mote/pkg/channel"
)

// Config Apple Reminders 渠道配置
type Config struct {
	Trigger      channel.TriggerConfig `json:"trigger"`
	Reply        channel.ReplyConfig   `json:"reply"`
	WatchList    string                `json:"watchList"`    // 监控列表，如 "Mote Tasks"
	PollInterval time.Duration         `json:"pollInterval"` // 轮询间隔
}

// Reminder 提醒事项结构（remindctl CLI 输出格式）
type Reminder struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Completed bool   `json:"completed"`
	List      string `json:"list"`
	DueDate   string `json:"due_date,omitempty"`
	Priority  int    `json:"priority,omitempty"`
}

// remindersChannel Apple Reminders 渠道实现
type remindersChannel struct {
	config    Config
	handler   channel.MessageHandler
	stopCh    chan struct{}
	stopped   atomic.Bool
	processed map[string]bool
	mu        sync.RWMutex

	// 用于测试的命令构造器
	cmdBuilder func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// New 创建新的 Apple Reminders 渠道
func New(cfg Config) *remindersChannel {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	return &remindersChannel{
		config:     cfg,
		stopCh:     make(chan struct{}),
		processed:  make(map[string]bool),
		cmdBuilder: exec.CommandContext,
	}
}

// ID 返回渠道唯一标识
func (c *remindersChannel) ID() channel.ChannelType {
	return channel.ChannelTypeReminders
}

// Name 返回渠道显示名称
func (c *remindersChannel) Name() string {
	return "Apple Reminders"
}

// Capabilities 返回渠道能力
func (c *remindersChannel) Capabilities() channel.ChannelCapabilities {
	return channel.ChannelCapabilities{
		CanSendText:      true,
		CanSendMedia:     false,
		CanDetectMention: true,
		CanWatch:         true,
		MaxMessageLength: 0, // 无限制
	}
}

// Start 启动渠道监听（轮询模式）
func (c *remindersChannel) Start(ctx context.Context) error {
	if c.stopped.Load() {
		return fmt.Errorf("channel already stopped")
	}

	go func() {
		ticker := time.NewTicker(c.config.PollInterval)
		defer ticker.Stop()

		// 立即执行一次
		c.pollReminders(ctx)

		for {
			select {
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.pollReminders(ctx)
			}
		}
	}()

	return nil
}

// Stop 停止渠道监听
func (c *remindersChannel) Stop(ctx context.Context) error {
	if c.stopped.Swap(true) {
		return nil // 已经停止
	}
	close(c.stopCh)
	return nil
}

// pollReminders 轮询获取提醒列表
func (c *remindersChannel) pollReminders(ctx context.Context) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler == nil {
		return
	}

	// 获取指定列表的提醒
	// remindctl show all --list "Mote Tasks" --json
	// 注意：remindctl show 命令用于查看提醒，list 命令用于管理列表
	args := []string{"show", "all", "--json"}
	if c.config.WatchList != "" {
		args = []string{"show", "all", "--list", c.config.WatchList, "--json"}
	}

	cmd := c.cmdBuilder(ctx, "remindctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return // 静默处理错误
	}

	var reminders []Reminder
	if err := json.Unmarshal(output, &reminders); err != nil {
		// 尝试逐行解析 JSON Lines 格式
		reminders = c.parseJSONLines(output)
		if len(reminders) == 0 {
			return
		}
	}

	for _, reminder := range reminders {
		// 跳过已完成的提醒
		if reminder.Completed {
			continue
		}

		c.mu.Lock()
		if c.processed[reminder.ID] {
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()

		// 检查标题是否以触发前缀开头
		if !c.checkTrigger(reminder.Title) {
			continue
		}

		c.mu.Lock()
		c.processed[reminder.ID] = true
		c.mu.Unlock()

		// 构建消息
		query := c.stripPrefix(reminder.Title)

		content := query
		if reminder.Notes != "" {
			content = query + "\n\n附加信息：\n" + reminder.Notes
		}

		msg := channel.InboundMessage{
			ID:           reminder.ID,
			ChannelType:  channel.ChannelTypeReminders,
			MessageType:  channel.MessageTypeDM,
			ChatID:       reminder.ID,
			Content:      content,
			RawContent:   reminder.Title,
			WasMentioned: true,
			Metadata: map[string]any{
				"reminderTitle": reminder.Title,
				"reminderList":  reminder.List,
				"reminderNotes": reminder.Notes,
			},
		}

		if reminder.DueDate != "" {
			msg.Metadata["reminderDueDate"] = reminder.DueDate
		}
		if reminder.Priority > 0 {
			msg.Metadata["reminderPriority"] = reminder.Priority
		}

		_ = handler(ctx, msg)
	}
}

// parseJSONLines 解析 JSON Lines 格式
func (c *remindersChannel) parseJSONLines(data []byte) []Reminder {
	var reminders []Reminder
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var reminder Reminder
		if err := json.Unmarshal([]byte(line), &reminder); err == nil {
			reminders = append(reminders, reminder)
		}
	}
	return reminders
}

// checkTrigger 检查标题是否匹配触发前缀
func (c *remindersChannel) checkTrigger(title string) bool {
	prefix := c.config.Trigger.Prefix
	if prefix == "" {
		return false
	}

	if c.config.Trigger.CaseSensitive {
		return strings.HasPrefix(title, prefix)
	}
	return strings.HasPrefix(strings.ToLower(title), strings.ToLower(prefix))
}

// stripPrefix 去除触发前缀
func (c *remindersChannel) stripPrefix(title string) string {
	prefix := c.config.Trigger.Prefix
	if prefix == "" {
		return title
	}

	if c.config.Trigger.CaseSensitive {
		return strings.TrimSpace(strings.TrimPrefix(title, prefix))
	}

	// 不区分大小写：找到匹配的部分并移除
	lowerTitle := strings.ToLower(title)
	lowerPrefix := strings.ToLower(prefix)
	if strings.HasPrefix(lowerTitle, lowerPrefix) {
		return strings.TrimSpace(title[len(prefix):])
	}
	return title
}

// SendMessage 发送消息（写入备注并标记完成）
func (c *remindersChannel) SendMessage(ctx context.Context, msg channel.OutboundMessage) error {
	reminderID := msg.ChatID
	content := channel.InjectReplyPrefix(msg.Content, c.config.Reply)

	// 在备注中写入回复
	// remindctl edit <id> --notes "<content>"
	cmd := c.cmdBuilder(ctx, "remindctl", "edit", reminderID, "--notes", content)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update reminder notes: %w", err)
	}

	// 标记为已完成
	// remindctl complete <id>
	cmd = c.cmdBuilder(ctx, "remindctl", "complete", reminderID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("complete reminder: %w", err)
	}

	return nil
}

// OnMessage 注册消息回调
func (c *remindersChannel) OnMessage(handler channel.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// SetCmdBuilder 设置命令构造器（用于测试）
func (c *remindersChannel) SetCmdBuilder(builder func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	c.cmdBuilder = builder
}

// ResetProcessed 重置已处理记录（用于测试）
func (c *remindersChannel) ResetProcessed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processed = make(map[string]bool)
}
