package reminders

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"testing"
	"time"

	"mote/pkg/channel"
)

func TestRemindersChannel_ID(t *testing.T) {
	ch := New(Config{})
	if ch.ID() != channel.ChannelTypeReminders {
		t.Errorf("expected ID %s, got %s", channel.ChannelTypeReminders, ch.ID())
	}
}

func TestRemindersChannel_Name(t *testing.T) {
	ch := New(Config{})
	if ch.Name() != "Apple Reminders" {
		t.Errorf("expected name 'Apple Reminders', got '%s'", ch.Name())
	}
}

func TestRemindersChannel_Capabilities(t *testing.T) {
	ch := New(Config{})
	caps := ch.Capabilities()

	if !caps.CanSendText {
		t.Error("expected CanSendText to be true")
	}
	if caps.CanSendMedia {
		t.Error("expected CanSendMedia to be false")
	}
	if !caps.CanDetectMention {
		t.Error("expected CanDetectMention to be true")
	}
	if !caps.CanWatch {
		t.Error("expected CanWatch to be true")
	}
}

func TestRemindersChannel_DefaultPollInterval(t *testing.T) {
	ch := New(Config{})
	if ch.config.PollInterval != 5*time.Second {
		t.Errorf("expected default poll interval 5s, got %v", ch.config.PollInterval)
	}
}

func TestRemindersChannel_CustomPollInterval(t *testing.T) {
	ch := New(Config{PollInterval: 10 * time.Second})
	if ch.config.PollInterval != 10*time.Second {
		t.Errorf("expected poll interval 10s, got %v", ch.config.PollInterval)
	}
}

func TestRemindersChannel_CheckTrigger(t *testing.T) {
	tests := []struct {
		name          string
		prefix        string
		caseSensitive bool
		title         string
		want          bool
	}{
		{
			name:   "match prefix",
			prefix: "@mote:",
			title:  "@mote: Buy groceries",
			want:   true,
		},
		{
			name:   "no match",
			prefix: "@mote:",
			title:  "Buy groceries",
			want:   false,
		},
		{
			name:          "case insensitive match",
			prefix:        "@mote:",
			caseSensitive: false,
			title:         "@MOTE: Buy groceries",
			want:          true,
		},
		{
			name:          "case sensitive no match",
			prefix:        "@mote:",
			caseSensitive: true,
			title:         "@MOTE: Buy groceries",
			want:          false,
		},
		{
			name:   "empty prefix",
			prefix: "",
			title:  "Buy groceries",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := New(Config{
				Trigger: channel.TriggerConfig{
					Prefix:        tt.prefix,
					CaseSensitive: tt.caseSensitive,
				},
			})
			got := ch.checkTrigger(tt.title)
			if got != tt.want {
				t.Errorf("checkTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemindersChannel_StripPrefix(t *testing.T) {
	tests := []struct {
		name          string
		prefix        string
		caseSensitive bool
		title         string
		want          string
	}{
		{
			name:   "strip prefix",
			prefix: "@mote:",
			title:  "@mote: Buy groceries",
			want:   "Buy groceries",
		},
		{
			name:          "strip prefix case insensitive",
			prefix:        "@mote:",
			caseSensitive: false,
			title:         "@MOTE: Buy groceries",
			want:          "Buy groceries",
		},
		{
			name:   "no prefix to strip",
			prefix: "@mote:",
			title:  "Buy groceries",
			want:   "Buy groceries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := New(Config{
				Trigger: channel.TriggerConfig{
					Prefix:        tt.prefix,
					CaseSensitive: tt.caseSensitive,
				},
			})
			got := ch.stripPrefix(tt.title)
			if got != tt.want {
				t.Errorf("stripPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemindersChannel_ParseJSONLines(t *testing.T) {
	ch := New(Config{})

	input := `{"id":"1","title":"Task 1","notes":"Note 1","completed":false}
{"id":"2","title":"Task 2","notes":"Note 2","completed":true}`

	reminders := ch.parseJSONLines([]byte(input))
	if len(reminders) != 2 {
		t.Fatalf("expected 2 reminders, got %d", len(reminders))
	}

	if reminders[0].ID != "1" || reminders[0].Title != "Task 1" {
		t.Errorf("unexpected first reminder: %+v", reminders[0])
	}
	if reminders[1].ID != "2" || reminders[1].Completed != true {
		t.Errorf("unexpected second reminder: %+v", reminders[1])
	}
}

func TestRemindersChannel_PollReminders(t *testing.T) {
	reminders := []Reminder{
		{ID: "rem-1", Title: "@mote: What's the weather?", Notes: "", Completed: false},
		{ID: "rem-2", Title: "Regular task", Notes: "", Completed: false},
		{ID: "rem-3", Title: "@mote: Already done", Notes: "", Completed: true},
	}
	output, _ := json.Marshal(reminders)

	var receivedMsgs []channel.InboundMessage
	var mu sync.Mutex

	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
		WatchList: "Mote Tasks",
	})
	ch.SetCmdBuilder(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", string(output))
	})
	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		receivedMsgs = append(receivedMsgs, msg)
		mu.Unlock()
		return nil
	})

	ctx := context.Background()
	ch.pollReminders(ctx)

	mu.Lock()
	defer mu.Unlock()

	// 只有 rem-1 应该被处理（rem-2 没有前缀，rem-3 已完成）
	if len(receivedMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(receivedMsgs))
	}

	msg := receivedMsgs[0]
	if msg.ID != "rem-1" {
		t.Errorf("expected ID 'rem-1', got '%s'", msg.ID)
	}
	if msg.ChannelType != channel.ChannelTypeReminders {
		t.Errorf("expected channel type %s, got %s", channel.ChannelTypeReminders, msg.ChannelType)
	}
	if msg.Content != "What's the weather?" {
		t.Errorf("unexpected content: %q", msg.Content)
	}
}

func TestRemindersChannel_PollReminders_WithNotes(t *testing.T) {
	reminders := []Reminder{
		{ID: "rem-1", Title: "@mote: Research topic", Notes: "Focus on AI", Completed: false},
	}
	output, _ := json.Marshal(reminders)

	var receivedMsg channel.InboundMessage
	var mu sync.Mutex

	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
	})
	ch.SetCmdBuilder(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", string(output))
	})
	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		receivedMsg = msg
		mu.Unlock()
		return nil
	})

	ctx := context.Background()
	ch.pollReminders(ctx)

	mu.Lock()
	defer mu.Unlock()

	expectedContent := "Research topic\n\n附加信息：\nFocus on AI"
	if receivedMsg.Content != expectedContent {
		t.Errorf("unexpected content: %q, want %q", receivedMsg.Content, expectedContent)
	}
}

func TestRemindersChannel_PollReminders_NoHandler(t *testing.T) {
	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
	})
	// 不设置 handler

	ctx := context.Background()
	ch.pollReminders(ctx) // 不应该 panic
}

func TestRemindersChannel_PollReminders_Dedup(t *testing.T) {
	reminders := []Reminder{
		{ID: "rem-1", Title: "@mote: Test", Completed: false},
	}
	output, _ := json.Marshal(reminders)

	callCount := 0
	var mu sync.Mutex

	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
	})
	ch.SetCmdBuilder(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", string(output))
	})
	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	ctx := context.Background()

	// 第一次轮询
	ch.pollReminders(ctx)
	// 第二次轮询（同一个提醒应该被去重）
	ch.pollReminders(ctx)

	mu.Lock()
	defer mu.Unlock()

	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
}

func TestRemindersChannel_StartStop(t *testing.T) {
	ch := New(Config{
		PollInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()

	err := ch.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 给一点时间让 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	err = ch.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 第二次 Stop 应该是 no-op
	err = ch.Stop(ctx)
	if err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestRemindersChannel_OnMessage(t *testing.T) {
	ch := New(Config{})

	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		return nil
	})

	// 验证 handler 已注册
	ch.mu.RLock()
	hasHandler := ch.handler != nil
	ch.mu.RUnlock()

	if !hasHandler {
		t.Error("expected handler to be registered")
	}
}
