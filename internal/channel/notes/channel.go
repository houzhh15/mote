// Package notes implements the Apple Notes channel plugin.
// Uses AppleScript to interact with Notes.app directly (memo CLI doesn't support JSON output).
package notes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mote/pkg/channel"
)

// Config Apple Notes 渠道配置
type Config struct {
	Trigger       channel.TriggerConfig `json:"trigger"`
	Reply         channel.ReplyConfig   `json:"reply"`
	WatchFolder   string                `json:"watchFolder"`   // 监控文件夹，如 "Mote Inbox"
	ArchiveFolder string                `json:"archiveFolder"` // 归档文件夹，如 "Mote Archive"
	PollInterval  time.Duration         `json:"pollInterval"`  // 轮询间隔
}

// Note 笔记结构
type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Folder    string `json:"folder"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// notesChannel Apple Notes 渠道实现
type notesChannel struct {
	config    Config
	handler   channel.MessageHandler
	stopCh    chan struct{}
	stopped   atomic.Bool
	processed map[string]bool   // 已处理的笔记 ID
	noteIDMap map[string]string // 短 ID -> 原始 Notes ID 的映射
	mu        sync.RWMutex

	// 用于测试的命令构造器
	cmdBuilder func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// New 创建新的 Apple Notes 渠道
func New(cfg Config) *notesChannel {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	return &notesChannel{
		config:     cfg,
		stopCh:     make(chan struct{}),
		processed:  make(map[string]bool),
		noteIDMap:  make(map[string]string),
		cmdBuilder: exec.CommandContext,
	}
}

// ID 返回渠道唯一标识
func (c *notesChannel) ID() channel.ChannelType {
	return channel.ChannelTypeNotes
}

// Name 返回渠道显示名称
func (c *notesChannel) Name() string {
	return "Apple Notes"
}

// Capabilities 返回渠道能力
func (c *notesChannel) Capabilities() channel.ChannelCapabilities {
	return channel.ChannelCapabilities{
		CanSendText:      true,
		CanSendMedia:     false,
		CanDetectMention: true,
		CanWatch:         true,
		MaxMessageLength: 0, // 无限制
	}
}

// Start 启动渠道监听（轮询模式）
func (c *notesChannel) Start(ctx context.Context) error {
	if c.stopped.Load() {
		return fmt.Errorf("channel already stopped")
	}

	go func() {
		ticker := time.NewTicker(c.config.PollInterval)
		defer ticker.Stop()

		// 立即执行一次
		c.pollNotes(ctx)

		for {
			select {
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.pollNotes(ctx)
			}
		}
	}()

	return nil
}

// Stop 停止渠道监听
func (c *notesChannel) Stop(ctx context.Context) error {
	if c.stopped.Swap(true) {
		return nil // 已经停止
	}
	close(c.stopCh)
	return nil
}

// pollNotes 轮询获取笔记列表
// 使用 AppleScript 直接与 Notes.app 通信（memo CLI 不支持 JSON 输出）
func (c *notesChannel) pollNotes(ctx context.Context) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler == nil {
		return
	}

	// 使用 AppleScript 获取笔记列表
	notes, err := c.fetchNotesViaAppleScript(ctx)
	if err != nil {
		return // 静默处理错误
	}

	for _, note := range notes {
		c.mu.Lock()
		if c.processed[note.ID] {
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()

		// 检查标题是否以触发前缀开头
		if !c.checkTrigger(note.Title) {
			continue
		}

		c.mu.Lock()
		c.processed[note.ID] = true
		c.mu.Unlock()

		// 构建消息
		query := c.stripPrefix(note.Title)

		content := query
		if note.Content != "" {
			content = query + "\n\n" + note.Content
		}

		msg := channel.InboundMessage{
			ID:           note.ID,
			ChannelType:  channel.ChannelTypeNotes,
			MessageType:  channel.MessageTypeDM,
			ChatID:       note.ID,
			Content:      content,
			RawContent:   note.Title + "\n\n" + note.Content,
			WasMentioned: true,
			Metadata: map[string]any{
				"noteTitle":  note.Title,
				"noteFolder": note.Folder,
			},
		}

		if note.CreatedAt != "" {
			if ts, err := time.Parse(time.RFC3339, note.CreatedAt); err == nil {
				msg.Timestamp = ts
			}
		}

		_ = handler(ctx, msg)
	}
}

// fetchNotesViaAppleScript 使用 AppleScript 获取笔记列表
func (c *notesChannel) fetchNotesViaAppleScript(ctx context.Context) ([]Note, error) {
	// AppleScript 获取指定文件夹的笔记（标题和正文）
	// 输出格式：每行 "ID|||TITLE|||FOLDER|||BODY" 用 ||| 分隔，BODY 中的换行替换为 <<<BR>>>
	// 每条记录以 <<<END>>> 结尾
	folderFilter := ""
	if c.config.WatchFolder != "" {
		folderFilter = fmt.Sprintf(`whose name is "%s"`, c.config.WatchFolder)
	}

	script := fmt.Sprintf(`
tell application "Notes"
	set output to ""
	set targetFolders to every folder %s
	repeat with f in targetFolders
		set folderName to name of f
		repeat with n in notes of f
			set noteId to id of n
			set noteTitle to name of n
			set noteBody to plaintext of n
			-- 替换所有类型的换行符为占位符
			set AppleScript's text item delimiters to {return & linefeed, return, linefeed, character id 10, character id 13}
			set bodyParts to text items of noteBody
			set AppleScript's text item delimiters to "<<<BR>>>"
			set noteBody to bodyParts as text
			set AppleScript's text item delimiters to ""
			set output to output & noteId & "|||" & noteTitle & "|||" & folderName & "|||" & noteBody & "<<<END>>>" & linefeed
		end repeat
	end repeat
	return output
end tell
`, folderFilter)

	cmd := c.cmdBuilder(ctx, "osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("applescript error: %w", err)
	}

	return c.parseAppleScriptOutput(string(output)), nil
}

// parseAppleScriptOutput 解析 AppleScript 输出
func (c *notesChannel) parseAppleScriptOutput(output string) []Note {
	var notes []Note
	// 使用 <<<END>>> 作为记录分隔符
	records := strings.Split(output, "<<<END>>>")

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "|||", 4)
		if len(parts) < 4 {
			continue
		}

		originalID := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		folder := strings.TrimSpace(parts[2])
		body := strings.ReplaceAll(parts[3], "<<<BR>>>", "\n")
		body = strings.TrimSpace(body)

		// 生成稳定的短 ID（基于原始 ID 的哈希）
		hash := sha256.Sum256([]byte(originalID))
		shortID := hex.EncodeToString(hash[:8])

		// 保存短 ID 到原始 ID 的映射
		c.mu.Lock()
		c.noteIDMap[shortID] = originalID
		c.mu.Unlock()

		notes = append(notes, Note{
			ID:      shortID,
			Title:   title,
			Content: body,
			Folder:  folder,
		})
	}

	return notes
}

// checkTrigger 检查标题是否匹配触发前缀
func (c *notesChannel) checkTrigger(title string) bool {
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
func (c *notesChannel) stripPrefix(title string) string {
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

// SendMessage 发送消息（追加到笔记）
// 使用 AppleScript 直接追加内容到笔记
func (c *notesChannel) SendMessage(ctx context.Context, msg channel.OutboundMessage) error {
	shortID := msg.ChatID
	content := channel.InjectReplyPrefix(msg.Content, c.config.Reply)

	// 获取原始 Notes ID
	c.mu.RLock()
	originalID, ok := c.noteIDMap[shortID]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("note ID not found: %s", shortID)
	}

	// 格式化追加内容
	appendContent := "\n\n---\n\n" + content

	// 使用 AppleScript 追加内容到笔记
	// 转义双引号
	escapedContent := strings.ReplaceAll(appendContent, "\\", "\\\\")
	escapedContent = strings.ReplaceAll(escapedContent, "\"", "\\\"")

	script := fmt.Sprintf(`
tell application "Notes"
	set targetNote to first note whose id is "%s"
	set currentBody to body of targetNote
	set body of targetNote to currentBody & "%s"
end tell
`, originalID, escapedContent)

	cmd := c.cmdBuilder(ctx, "osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("append to note: %w", err)
	}

	// 可选：移动到归档文件夹
	if c.config.ArchiveFolder != "" {
		moveScript := fmt.Sprintf(`
tell application "Notes"
	set targetNote to first note whose id is "%s"
	set targetFolder to first folder whose name is "%s"
	move targetNote to targetFolder
end tell
`, originalID, c.config.ArchiveFolder)

		cmd = c.cmdBuilder(ctx, "osascript", "-e", moveScript)
		_ = cmd.Run() // 静默处理归档错误
	}

	return nil
}

// OnMessage 注册消息回调
func (c *notesChannel) OnMessage(handler channel.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// SetCmdBuilder 设置命令构造器（用于测试）
func (c *notesChannel) SetCmdBuilder(builder func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	c.cmdBuilder = builder
}

// ResetProcessed 重置已处理记录（用于测试）
func (c *notesChannel) ResetProcessed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processed = make(map[string]bool)
	c.noteIDMap = make(map[string]string)
}
