package imessage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mote/pkg/channel"
)

func TestIMessageChannel_ID(t *testing.T) {
	ch := New(Config{})
	if ch.ID() != channel.ChannelTypeIMessage {
		t.Errorf("ID() = %v, want %v", ch.ID(), channel.ChannelTypeIMessage)
	}
}

func TestIMessageChannel_Name(t *testing.T) {
	ch := New(Config{})
	if ch.Name() != "iMessage" {
		t.Errorf("Name() = %v, want %v", ch.Name(), "iMessage")
	}
}

func TestIMessageChannel_Capabilities(t *testing.T) {
	ch := New(Config{})
	caps := ch.Capabilities()

	if !caps.CanSendText {
		t.Error("CanSendText should be true")
	}
	if !caps.CanSendMedia {
		t.Error("CanSendMedia should be true")
	}
	if caps.CanDetectMention {
		t.Error("CanDetectMention should be false")
	}
	if !caps.CanWatch {
		t.Error("CanWatch should be true")
	}
}

func TestIMessageChannel_HandleMessage(t *testing.T) {
	tests := []struct {
		name        string
		msg         WatchMessage
		cfg         Config
		wantProcess bool
		wantContent string
	}{
		{
			name: "message with trigger prefix",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "user1@icloud.com",
				Text:      "@mote hello world",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-1",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix:        "@mote",
					CaseSensitive: false,
				},
			},
			wantProcess: true,
			wantContent: "hello world",
		},
		{
			name: "message without trigger prefix",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "user1@icloud.com",
				Text:      "hello world",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-2",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix:      "@mote",
					SelfTrigger: false,
				},
			},
			wantProcess: false,
		},
		{
			name: "self message with self trigger enabled",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "me@icloud.com",
				Text:      "@mote hello world",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  true,
				GUID:      "test-guid-3",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix:      "@mote",
					SelfTrigger: true,
				},
			},
			wantProcess: true,
			wantContent: "hello world",
		},
		{
			name: "self message without trigger prefix should be skipped",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "me@icloud.com",
				Text:      "hello world",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  true,
				GUID:      "test-guid-4",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix:      "@mote",
					SelfTrigger: true,
				},
			},
			wantProcess: false,
		},
		{
			name: "reply message should be skipped to prevent loop",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "me@icloud.com",
				Text:      "[Mote] Hello! How can I help?",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  true,
				GUID:      "test-guid-5",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix:      "@mote",
					SelfTrigger: true,
				},
				Reply: channel.ReplyConfig{
					Prefix: "[Mote]",
				},
			},
			wantProcess: false,
		},
		{
			name: "message from whitelisted sender should be processed",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "friend@icloud.com",
				Text:      "@mote hello",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-6",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix: "@mote",
				},
				AllowFrom: []string{"friend@icloud.com", "family@icloud.com"},
			},
			wantProcess: true,
			wantContent: "hello",
		},
		{
			name: "message from non-whitelisted sender should be skipped",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "stranger@icloud.com",
				Text:      "@mote hello",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-7",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix: "@mote",
				},
				AllowFrom: []string{"friend@icloud.com", "family@icloud.com"},
			},
			wantProcess: false,
		},
		{
			name: "whitelist check should be case insensitive",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "Friend@iCloud.com",
				Text:      "@mote hello",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-8",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix: "@mote",
				},
				AllowFrom: []string{"friend@icloud.com"},
			},
			wantProcess: true,
			wantContent: "hello",
		},
		{
			name: "empty whitelist allows all senders",
			msg: WatchMessage{
				ChatID:    1,
				Sender:    "anyone@icloud.com",
				Text:      "@mote hello",
				CreatedAt: time.Now().Format(time.RFC3339),
				IsFromMe:  false,
				GUID:      "test-guid-9",
			},
			cfg: Config{
				Trigger: channel.TriggerConfig{
					Prefix: "@mote",
				},
				AllowFrom: []string{}, // empty = allow all
			},
			wantProcess: true,
			wantContent: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := New(tt.cfg)

			var received *channel.InboundMessage
			ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
				received = &msg
				return nil
			})

			ch.handleMessage(context.Background(), tt.msg)

			if tt.wantProcess {
				if received == nil {
					t.Error("expected message to be processed, but handler was not called")
					return
				}
				if received.Content != tt.wantContent {
					t.Errorf("Content = %q, want %q", received.Content, tt.wantContent)
				}
				if !received.WasMentioned {
					t.Error("WasMentioned should be true")
				}
			} else {
				if received != nil {
					t.Error("expected message to be skipped, but handler was called")
				}
			}
		})
	}
}

func TestIMessageChannel_HandleMessage_NoHandler(t *testing.T) {
	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote",
		},
	})

	msg := WatchMessage{
		ChatID: 1,
		Sender: "user1@icloud.com",
		Text:   "@mote test",
		GUID:   "test-guid",
	}

	// 应该不会 panic
	ch.handleMessage(context.Background(), msg)
}

func TestIMessageChannel_OnMessage(t *testing.T) {
	ch := New(Config{})

	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		return nil
	})

	ch.mu.RLock()
	hasHandler := ch.handler != nil
	ch.mu.RUnlock()

	if !hasHandler {
		t.Error("handler should be set")
	}
}

func TestWatchMessage_Parse(t *testing.T) {
	jsonStr := `{"chat_id":123,"sender":"user456@icloud.com","text":"Hello","created_at":"2024-01-01T12:00:00Z","is_from_me":false,"guid":"test-guid","id":1}`

	var msg WatchMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if msg.ChatID != 123 {
		t.Errorf("ChatID = %d, want %d", msg.ChatID, 123)
	}
	if msg.Sender != "user456@icloud.com" {
		t.Errorf("Sender = %q, want %q", msg.Sender, "user456@icloud.com")
	}
	if msg.Text != "Hello" {
		t.Errorf("Text = %q, want %q", msg.Text, "Hello")
	}
	if msg.IsFromMe {
		t.Error("IsFromMe should be false")
	}
	if msg.GUID != "test-guid" {
		t.Errorf("GUID = %q, want %q", msg.GUID, "test-guid")
	}
}
