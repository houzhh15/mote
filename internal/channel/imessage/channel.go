// Package imessage implements the iMessage channel plugin.
package imessage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mote/pkg/channel"
)

// Config iMessage 渠道配置
type Config struct {
	Trigger   channel.TriggerConfig `json:"trigger"`
	Reply     channel.ReplyConfig   `json:"reply"`
	SelfID    string                `json:"selfId"`
	AllowFrom []string              `json:"allowFrom"` // 允许的发信人白名单（为空则允许所有）
}

// RPCRequest JSON-RPC 请求结构
type RPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// RPCResponse JSON-RPC 响应结构
type RPCResponse struct {
	JSONRPC string                 `json:"jsonrpc,omitempty"`
	ID      *int                   `json:"id,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *RPCError              `json:"error,omitempty"`
	Method  string                 `json:"method,omitempty"`
	Params  json.RawMessage        `json:"params,omitempty"`
}

// RPCError JSON-RPC 错误结构
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MessageNotification 消息通知的 params 结构
type MessageNotification struct {
	Message *WatchMessage `json:"message"`
}

// WatchMessage imsg watch 输出的消息结构
type WatchMessage struct {
	ChatID      int      `json:"chat_id"`
	Sender      string   `json:"sender"`
	Text        string   `json:"text"`
	CreatedAt   string   `json:"created_at"`
	IsFromMe    bool     `json:"is_from_me"`
	GUID        string   `json:"guid"`
	ID          int      `json:"id"`
	Reactions   []string `json:"reactions"`
	Attachments []string `json:"attachments"`
}

// iMessageChannel iMessage 渠道实现
type iMessageChannel struct {
	config    Config
	handler   channel.MessageHandler
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser // 保持 stdin pipe 打开
	stdin     *bufio.Writer
	stopCh    chan struct{}
	stopped   atomic.Bool
	mu        sync.RWMutex
	nextID    int

	// 去重：记录已处理的消息 GUID
	processedMsgs map[string]time.Time
	processedMu   sync.RWMutex

	// 用于测试的命令构造器
	cmdBuilder func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// New 创建新的 iMessage 渠道
func New(cfg Config) *iMessageChannel {
	return &iMessageChannel{
		config:        cfg,
		stopCh:        make(chan struct{}),
		cmdBuilder:    exec.CommandContext,
		nextID:        1,
		processedMsgs: make(map[string]time.Time),
	}
}

// ID 返回渠道唯一标识
func (c *iMessageChannel) ID() channel.ChannelType {
	return channel.ChannelTypeIMessage
}

// Name 返回渠道显示名称
func (c *iMessageChannel) Name() string {
	return "iMessage"
}

// Capabilities 返回渠道能力
func (c *iMessageChannel) Capabilities() channel.ChannelCapabilities {
	return channel.ChannelCapabilities{
		CanSendText:      true,
		CanSendMedia:     true,
		CanDetectMention: false,
		CanWatch:         true,
		MaxMessageLength: 0,
	}
}

// Start 启动渠道监听（JSON-RPC 模式）
func (c *iMessageChannel) Start(ctx context.Context) error {
	c.cmd = c.cmdBuilder(ctx, "imsg", "rpc")

	stdinPipe, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	c.stdinPipe = stdinPipe // 保持引用防止被 GC
	c.stdin = bufio.NewWriter(stdinPipe)

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start imsg rpc: %w", err)
	}

	slog.Info("imsg rpc process started", "pid", c.cmd.Process.Pid)

	// 读取 stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Debug("imsg rpc stderr", "line", scanner.Text())
		}
	}()

	// 读取 stdout（JSON-RPC 响应和通知）
	go func() {
		slog.Info("imsg rpc reader goroutine started")
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if c.stopped.Load() {
				slog.Info("imsg rpc stopped flag set, exiting")
				return
			}

			select {
			case <-c.stopCh:
				slog.Info("imsg rpc stop channel closed, exiting")
				return
			default:
			}

			line := scanner.Text()
			if line == "" {
				continue
			}

			slog.Debug("imsg rpc received line", "line", line)

			var resp RPCResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				slog.Warn("imsg rpc parse error", "error", err, "line", line)
				continue
			}

			// 处理通知（没有 ID 的消息）
			if resp.ID == nil && resp.Method == "message" {
				var notification MessageNotification
				if err := json.Unmarshal(resp.Params, &notification); err != nil {
					slog.Warn("imsg rpc notification parse error", "error", err)
					continue
				}
				if notification.Message != nil {
					slog.Info("imsg received message",
						"chatID", notification.Message.ChatID,
						"sender", notification.Message.Sender,
						"text", notification.Message.Text,
						"isFromMe", notification.Message.IsFromMe,
					)
					c.handleMessage(ctx, *notification.Message)
				}
			} else if resp.ID != nil {
				// 响应消息（目前只是日志记录）
				if resp.Error != nil {
					slog.Warn("imsg rpc error response", "id", *resp.ID, "error", resp.Error.Message)
				} else {
					slog.Debug("imsg rpc response", "id", *resp.ID, "result", resp.Result)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Error("imsg rpc scanner error", "error", err)
		}
		slog.Info("imsg rpc reader goroutine exiting")
	}()

	// 发送 watch.subscribe 请求
	if err := c.sendRPCRequest("watch.subscribe", map[string]interface{}{
		"attachments": false,
	}); err != nil {
		return fmt.Errorf("send watch.subscribe: %w", err)
	}

	slog.Info("imsg rpc watch.subscribe sent")

	return nil
}

// Stop 停止渠道监听
func (c *iMessageChannel) Stop(ctx context.Context) error {
	if c.stopped.Swap(true) {
		return nil
	}

	close(c.stopCh)

	// 关闭 stdin pipe
	if c.stdinPipe != nil {
		_ = c.stdinPipe.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// handleMessage 处理收到的消息
func (c *iMessageChannel) handleMessage(ctx context.Context, msg WatchMessage) {
	text := strings.TrimSpace(msg.Text)

	// 0. 去重检查：基于消息内容和 chatID 生成 key（因为给自己发消息会产生两条 GUID 不同但内容相同的消息）
	dedupeKey := fmt.Sprintf("%d:%s", msg.ChatID, text)
	c.processedMu.Lock()
	if lastTime, exists := c.processedMsgs[dedupeKey]; exists {
		// 5秒内的重复消息跳过
		if time.Since(lastTime) < 5*time.Second {
			c.processedMu.Unlock()
			slog.Debug("imsg skipping duplicate message", "guid", msg.GUID, "dedupeKey", dedupeKey)
			return
		}
	}
	c.processedMsgs[dedupeKey] = time.Now()
	c.processedMu.Unlock()

	// 清理过期的去重记录（超过 1 分钟的）
	go c.cleanupProcessedMsgs()

	// 1. 忽略以回复前缀开头的消息（避免处理自己的回复，防止无限循环）
	replyPrefix := c.config.Reply.Prefix
	if replyPrefix == "" {
		replyPrefix = "[Mote]" // 默认前缀
	}
	if strings.HasPrefix(text, replyPrefix) {
		slog.Debug("imsg skipping reply message", "guid", msg.GUID, "prefix", replyPrefix)
		return
	}

	// 2. 跳过自己发送的消息（除非启用了 SelfTrigger）
	if msg.IsFromMe && !c.config.Trigger.SelfTrigger {
		slog.Debug("imsg skipping self message (SelfTrigger disabled)", "guid", msg.GUID)
		return
	}

	// 3. 检查发信人白名单（如果配置了的话）
	if len(c.config.AllowFrom) > 0 && !msg.IsFromMe {
		allowed := false
		sender := strings.ToLower(strings.TrimSpace(msg.Sender))
		for _, allowedSender := range c.config.AllowFrom {
			if strings.ToLower(strings.TrimSpace(allowedSender)) == sender {
				allowed = true
				break
			}
			// 支持 chat_id 匹配
			if allowedSender == fmt.Sprintf("%d", msg.ChatID) {
				allowed = true
				break
			}
		}
		if !allowed {
			slog.Debug("imsg skipping message from unauthorized sender", "guid", msg.GUID, "sender", msg.Sender)
			return
		}
	}

	// 4. 必须有触发前缀才处理（无论是自己还是他人的消息）
	triggerPrefix := c.config.Trigger.Prefix
	if triggerPrefix != "" {
		hasPrefix := false
		if c.config.Trigger.CaseSensitive {
			hasPrefix = strings.HasPrefix(text, triggerPrefix)
		} else {
			hasPrefix = strings.HasPrefix(strings.ToLower(text), strings.ToLower(triggerPrefix))
		}
		if !hasPrefix {
			slog.Debug("imsg skipping message without trigger prefix", "guid", msg.GUID, "prefix", triggerPrefix)
			return
		}
	}

	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler == nil {
		return
	}

	var timestamp time.Time
	if msg.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, msg.CreatedAt); err == nil {
			timestamp = t
		}
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	chatID := fmt.Sprintf("%d", msg.ChatID)
	inbound := channel.InboundMessage{
		ID:          msg.GUID,
		ChannelType: channel.ChannelTypeIMessage,
		MessageType: channel.MessageTypeDM,
		ChatID:      chatID,
		SenderID:    msg.Sender,
		SenderName:  msg.Sender,
		Content:     msg.Text,
		RawContent:  msg.Text,
		Timestamp:   timestamp,
		IsSelfSend:  msg.IsFromMe,
	}

	result := channel.CheckTrigger(inbound, c.config.Trigger)
	slog.Info("imsg trigger check",
		"shouldProcess", result.ShouldProcess,
		"skipReason", result.SkipReason,
		"strippedContent", result.StrippedContent,
	)
	if !result.ShouldProcess {
		return
	}

	inbound.Content = result.StrippedContent
	inbound.WasMentioned = true

	_ = handler(ctx, inbound)
}

// SendMessage 发送消息
func (c *iMessageChannel) SendMessage(ctx context.Context, msg channel.OutboundMessage) error {
	content := channel.InjectReplyPrefix(msg.Content, c.config.Reply)

	// imsg send --chat-id <id> --text <content>
	cmd := c.cmdBuilder(ctx, "imsg", "send", "--chat-id", msg.ChatID, "--text", content)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	return nil
}

// OnMessage 注册消息回调
func (c *iMessageChannel) OnMessage(handler channel.MessageHandler) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

// SetCmdBuilder 设置命令构造器（用于测试）
func (c *iMessageChannel) SetCmdBuilder(builder func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	c.cmdBuilder = builder
}

// sendRPCRequest 发送 JSON-RPC 请求
func (c *iMessageChannel) sendRPCRequest(method string, params map[string]interface{}) error {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	if c.stdin == nil {
		return fmt.Errorf("stdin not initialized")
	}

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	if _, err := c.stdin.WriteString("\n"); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	if err := c.stdin.Flush(); err != nil {
		return fmt.Errorf("flush request: %w", err)
	}

	slog.Debug("imsg rpc request sent", "method", method, "id", id)
	return nil
}

// cleanupProcessedMsgs 清理过期的去重记录
func (c *iMessageChannel) cleanupProcessedMsgs() {
	c.processedMu.Lock()
	defer c.processedMu.Unlock()

	now := time.Now()
	for key, t := range c.processedMsgs {
		if now.Sub(t) > time.Minute {
			delete(c.processedMsgs, key)
		}
	}
}
