// Package channel defines the core interfaces and types for channel plugins.
package channel

import "context"

// ChannelType 渠道类型
type ChannelType string

const (
	ChannelTypeIMessage  ChannelType = "imessage"
	ChannelTypeNotes     ChannelType = "apple-notes"
	ChannelTypeReminders ChannelType = "apple-reminders"
)

// MessageType 消息类型
type MessageType string

const (
	MessageTypeDM    MessageType = "dm"
	MessageTypeGroup MessageType = "group"
)

// ChannelCapabilities 渠道能力
type ChannelCapabilities struct {
	CanSendText      bool `json:"canSendText"`
	CanSendMedia     bool `json:"canSendMedia"`
	CanDetectMention bool `json:"canDetectMention"`
	CanWatch         bool `json:"canWatch"`
	MaxMessageLength int  `json:"maxMessageLength,omitempty"`
}

// ChannelPlugin 渠道插件接口
type ChannelPlugin interface {
	// ID 返回渠道唯一标识
	ID() ChannelType

	// Name 返回渠道显示名称
	Name() string

	// Capabilities 返回渠道能力
	Capabilities() ChannelCapabilities

	// Start 启动渠道监听
	Start(ctx context.Context) error

	// Stop 停止渠道监听
	Stop(ctx context.Context) error

	// SendMessage 发送消息
	SendMessage(ctx context.Context, msg OutboundMessage) error

	// OnMessage 注册消息回调
	OnMessage(handler MessageHandler)
}

// MessageHandler 消息处理回调
type MessageHandler func(ctx context.Context, msg InboundMessage) error
