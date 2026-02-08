package channel

import "time"

// InboundMessage 入站消息
type InboundMessage struct {
	ID          string         `json:"id"`
	ChannelType ChannelType    `json:"channelType"`
	MessageType MessageType    `json:"messageType"`
	ChatID      string         `json:"chatId"`
	SenderID    string         `json:"senderId"`
	SenderName  string         `json:"senderName"`
	Content     string         `json:"content"`
	RawContent  string         `json:"rawContent"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`

	// 触发相关
	WasMentioned bool `json:"wasMentioned"`
	IsSelfSend   bool `json:"isSelfSend"`
}

// OutboundMessage 出站消息
type OutboundMessage struct {
	ChannelType ChannelType    `json:"channelType"`
	ChatID      string         `json:"chatId"`
	Content     string         `json:"content"`
	ReplyToID   string         `json:"replyToId,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
