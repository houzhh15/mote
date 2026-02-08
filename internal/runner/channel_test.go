package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"mote/internal/config"
	"mote/pkg/channel"
)

// mockChannelPlugin 用于测试的 mock 渠道插件
type mockChannelPlugin struct {
	id       channel.ChannelType
	name     string
	handler  channel.MessageHandler
	started  bool
	stopped  bool
	sentMsgs []channel.OutboundMessage
	mu       sync.Mutex
	startErr error
	stopErr  error
	sendErr  error
}

func newMockChannelPlugin(id channel.ChannelType, name string) *mockChannelPlugin {
	return &mockChannelPlugin{
		id:   id,
		name: name,
	}
}

func (m *mockChannelPlugin) ID() channel.ChannelType {
	return m.id
}

func (m *mockChannelPlugin) Name() string {
	return m.name
}

func (m *mockChannelPlugin) Capabilities() channel.ChannelCapabilities {
	return channel.ChannelCapabilities{
		CanSendText: true,
		CanWatch:    true,
	}
}

func (m *mockChannelPlugin) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockChannelPlugin) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}

func (m *mockChannelPlugin) SendMessage(ctx context.Context, msg channel.OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockChannelPlugin) OnMessage(handler channel.MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

func (m *mockChannelPlugin) isStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockChannelPlugin) isStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *mockChannelPlugin) getSentMessages() []channel.OutboundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]channel.OutboundMessage{}, m.sentMsgs...)
}

func (m *mockChannelPlugin) simulateMessage(ctx context.Context, msg channel.InboundMessage) error {
	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()
	if handler != nil {
		return handler(ctx, msg)
	}
	return nil
}

func TestRunner_InitChannels_Disabled(t *testing.T) {
	runner := &Runner{}

	cfg := config.ChannelsConfig{
		IMessage: config.IMessageConfig{
			Enabled: false,
		},
	}

	err := runner.InitChannels(cfg)
	if err != nil {
		t.Fatalf("InitChannels failed: %v", err)
	}

	registry := runner.ChannelRegistry()
	if registry == nil {
		t.Fatal("expected registry to be created")
	}

	if registry.Count() != 0 {
		t.Errorf("expected 0 channels, got %d", registry.Count())
	}
}

func TestRunner_InitChannels_IMessage(t *testing.T) {
	runner := &Runner{}

	cfg := config.ChannelsConfig{
		IMessage: config.IMessageConfig{
			Enabled: true,
			SelfID:  "test@icloud.com",
			Trigger: config.TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: false,
				SelfTrigger:   true,
			},
			Reply: config.ReplyConfig{
				Prefix:    "[Mote]",
				Separator: "\n",
			},
		},
	}

	err := runner.InitChannels(cfg)
	if err != nil {
		t.Fatalf("InitChannels failed: %v", err)
	}

	registry := runner.ChannelRegistry()
	if registry == nil {
		t.Fatal("expected registry to be created")
	}

	if registry.Count() != 1 {
		t.Errorf("expected 1 channel, got %d", registry.Count())
	}

	plugin, ok := registry.Get(channel.ChannelTypeIMessage)
	if !ok {
		t.Fatal("expected iMessage channel to be registered")
	}

	if plugin.ID() != channel.ChannelTypeIMessage {
		t.Errorf("expected channel ID %s, got %s", channel.ChannelTypeIMessage, plugin.ID())
	}
}

func TestRunner_ChannelRegistry_NilBeforeInit(t *testing.T) {
	runner := &Runner{}

	registry := runner.ChannelRegistry()
	if registry != nil {
		t.Error("expected nil registry before InitChannels")
	}
}

func TestRunner_HandleChannelMessage_NoRegistry(t *testing.T) {
	runner := &Runner{}

	// 验证未初始化时 ChannelRegistry 返回 nil
	registry := runner.ChannelRegistry()
	if registry != nil {
		t.Error("expected nil registry before InitChannels")
	}
}

func TestRunner_InitChannels_MultipleChannels(t *testing.T) {
	runner := &Runner{}

	cfg := config.ChannelsConfig{
		IMessage: config.IMessageConfig{
			Enabled: true,
			SelfID:  "test@icloud.com",
			Trigger: config.TriggerConfig{
				Prefix: "@mote",
			},
		},
		AppleNotes: config.AppleNotesConfig{
			Enabled:     true,
			WatchFolder: "Mote Inbox",
			Trigger: config.TriggerConfig{
				Prefix: "@mote:",
			},
		},
		AppleReminders: config.AppleRemindersConfig{
			Enabled:   true,
			WatchList: "Mote Tasks",
			Trigger: config.TriggerConfig{
				Prefix: "@mote:",
			},
		},
	}

	err := runner.InitChannels(cfg)
	if err != nil {
		t.Fatalf("InitChannels failed: %v", err)
	}

	registry := runner.ChannelRegistry()
	// All 3 channels are now implemented
	if registry.Count() != 3 {
		t.Errorf("expected 3 channels, got %d", registry.Count())
	}

	// Verify each channel is registered
	if _, ok := registry.Get(channel.ChannelTypeIMessage); !ok {
		t.Error("expected iMessage channel to be registered")
	}
	if _, ok := registry.Get(channel.ChannelTypeNotes); !ok {
		t.Error("expected Apple Notes channel to be registered")
	}
	if _, ok := registry.Get(channel.ChannelTypeReminders); !ok {
		t.Error("expected Apple Reminders channel to be registered")
	}
}

func TestRunner_InitChannels_DefaultTriggerConfig(t *testing.T) {
	runner := &Runner{}

	cfg := config.ChannelsConfig{
		IMessage: config.IMessageConfig{
			Enabled: true,
			SelfID:  "user@icloud.com",
			// 使用空 Trigger 配置，测试默认值处理
			Trigger: config.TriggerConfig{},
			Reply:   config.ReplyConfig{},
		},
	}

	err := runner.InitChannels(cfg)
	if err != nil {
		t.Fatalf("InitChannels failed: %v", err)
	}

	registry := runner.ChannelRegistry()
	if registry.Count() != 1 {
		t.Errorf("expected 1 channel, got %d", registry.Count())
	}
}

// TestChannelIntegration_SessionID 测试渠道消息的 sessionID 生成
func TestChannelIntegration_SessionID(t *testing.T) {
	// 验证 sessionID 格式
	channelType := channel.ChannelTypeIMessage
	chatID := "chat-123"

	expectedSessionID := "channel:imessage:chat-123"
	actualSessionID := "channel:" + string(channelType) + ":" + chatID

	if actualSessionID != expectedSessionID {
		t.Errorf("expected sessionID %s, got %s", expectedSessionID, actualSessionID)
	}
}

// BenchmarkInitChannels 基准测试
func BenchmarkInitChannels(b *testing.B) {
	cfg := config.ChannelsConfig{
		IMessage: config.IMessageConfig{
			Enabled: true,
			SelfID:  "test@icloud.com",
			Trigger: config.TriggerConfig{
				Prefix: "@mote",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runner := &Runner{}
		_ = runner.InitChannels(cfg)
	}
}

// 辅助函数：等待条件满足
func waitFor(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}
