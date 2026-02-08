package notes

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"mote/pkg/channel"
)

// mockCmdBuilder 用于测试的命令构造器
type mockCmdBuilder struct {
	output []byte
	err    error
	calls  [][]string
	mu     sync.Mutex
}

func (m *mockCmdBuilder) build(ctx context.Context, name string, args ...string) *exec.Cmd {
	m.mu.Lock()
	m.calls = append(m.calls, append([]string{name}, args...))
	m.mu.Unlock()

	// 返回一个会输出预设内容的命令
	if m.err != nil {
		return exec.Command("false")
	}
	return exec.Command("echo", string(m.output))
}

func (m *mockCmdBuilder) getCalls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]string{}, m.calls...)
}

func TestNotesChannel_ID(t *testing.T) {
	ch := New(Config{})
	if ch.ID() != channel.ChannelTypeNotes {
		t.Errorf("expected ID %s, got %s", channel.ChannelTypeNotes, ch.ID())
	}
}

func TestNotesChannel_Name(t *testing.T) {
	ch := New(Config{})
	if ch.Name() != "Apple Notes" {
		t.Errorf("expected name 'Apple Notes', got '%s'", ch.Name())
	}
}

func TestNotesChannel_Capabilities(t *testing.T) {
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

func TestNotesChannel_DefaultPollInterval(t *testing.T) {
	ch := New(Config{})
	if ch.config.PollInterval != 5*time.Second {
		t.Errorf("expected default poll interval 5s, got %v", ch.config.PollInterval)
	}
}

func TestNotesChannel_CustomPollInterval(t *testing.T) {
	ch := New(Config{PollInterval: 10 * time.Second})
	if ch.config.PollInterval != 10*time.Second {
		t.Errorf("expected poll interval 10s, got %v", ch.config.PollInterval)
	}
}

func TestNotesChannel_CheckTrigger(t *testing.T) {
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
			title:  "@mote: Hello",
			want:   true,
		},
		{
			name:   "no match",
			prefix: "@mote:",
			title:  "Hello World",
			want:   false,
		},
		{
			name:          "case insensitive match",
			prefix:        "@mote:",
			caseSensitive: false,
			title:         "@MOTE: Hello",
			want:          true,
		},
		{
			name:          "case sensitive no match",
			prefix:        "@mote:",
			caseSensitive: true,
			title:         "@MOTE: Hello",
			want:          false,
		},
		{
			name:   "empty prefix",
			prefix: "",
			title:  "Hello",
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

func TestNotesChannel_StripPrefix(t *testing.T) {
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
			title:  "@mote: Hello World",
			want:   "Hello World",
		},
		{
			name:          "strip prefix case insensitive",
			prefix:        "@mote:",
			caseSensitive: false,
			title:         "@MOTE: Hello World",
			want:          "Hello World",
		},
		{
			name:   "no prefix to strip",
			prefix: "@mote:",
			title:  "Hello World",
			want:   "Hello World",
		},
		{
			name:   "empty prefix",
			prefix: "",
			title:  "Hello World",
			want:   "Hello World",
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

func TestNotesChannel_ParseAppleScriptOutput(t *testing.T) {
	ch := New(Config{})

	// 模拟 AppleScript 输出格式：ID|||Title|||Folder|||Body<<<END>>>
	input := `x-coredata://123/Note/p1|||Note 1|||Notes|||Content 1<<<END>>>
x-coredata://123/Note/p2|||Note 2|||Notes|||Content 2<<<END>>>`

	notes := ch.parseAppleScriptOutput(input)
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}

	if notes[0].Title != "Note 1" || notes[0].Content != "Content 1" {
		t.Errorf("unexpected first note: %+v", notes[0])
	}
	if notes[1].Title != "Note 2" || notes[1].Content != "Content 2" {
		t.Errorf("unexpected second note: %+v", notes[1])
	}

	// 验证 ID 映射已保存
	ch.mu.RLock()
	if len(ch.noteIDMap) != 2 {
		t.Errorf("expected 2 ID mappings, got %d", len(ch.noteIDMap))
	}
	ch.mu.RUnlock()
}

func TestNotesChannel_PollNotes(t *testing.T) {
	// 模拟 AppleScript 输出格式
	appleScriptOutput := `x-coredata://123/Note/p1|||@mote: Test question|||Mote Inbox|||Details here<<<END>>>
x-coredata://123/Note/p2|||Regular note|||Mote Inbox|||Not triggered<<<END>>>`

	var receivedMsgs []channel.InboundMessage
	var mu sync.Mutex

	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
		WatchFolder: "Mote Inbox",
	})
	ch.SetCmdBuilder(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", appleScriptOutput)
	})
	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		receivedMsgs = append(receivedMsgs, msg)
		mu.Unlock()
		return nil
	})

	ctx := context.Background()
	ch.pollNotes(ctx)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(receivedMsgs))
	}

	msg := receivedMsgs[0]
	// ID 是哈希后的短 ID，不再是原始 ID
	if msg.ID == "" {
		t.Error("expected non-empty ID")
	}
	if msg.ChannelType != channel.ChannelTypeNotes {
		t.Errorf("expected channel type %s, got %s", channel.ChannelTypeNotes, msg.ChannelType)
	}
	if msg.Content != "Test question\n\nDetails here" {
		t.Errorf("unexpected content: %q", msg.Content)
	}
}

func TestNotesChannel_PollNotes_NoHandler(t *testing.T) {
	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
	})
	// 不设置 handler

	ctx := context.Background()
	ch.pollNotes(ctx) // 不应该 panic
}

func TestNotesChannel_PollNotes_Dedup(t *testing.T) {
	// 模拟 AppleScript 输出格式
	appleScriptOutput := `x-coredata://123/Note/p1|||@mote: Test|||Notes|||Content<<<END>>>`

	callCount := 0
	var mu sync.Mutex

	ch := New(Config{
		Trigger: channel.TriggerConfig{
			Prefix: "@mote:",
		},
	})
	ch.SetCmdBuilder(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", appleScriptOutput)
	})
	ch.OnMessage(func(ctx context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	ctx := context.Background()

	// 第一次轮询
	ch.pollNotes(ctx)
	// 第二次轮询（同一个笔记应该被去重）
	ch.pollNotes(ctx)

	mu.Lock()
	defer mu.Unlock()

	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
}

func TestNotesChannel_StartStop(t *testing.T) {
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

func TestNotesChannel_OnMessage(t *testing.T) {
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
