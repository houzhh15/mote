package channel
package channel

import (
	"testing"
)

func TestCheckTrigger_WithPrefix(t *testing.T) {
	tests := []struct {
		name    string
		msg     InboundMessage
		cfg     TriggerConfig
		want    bool
		wantStr string
	}{
		{
			name: "exact prefix match",
			msg: InboundMessage{
				Content:  "@mote hello world",
				SenderID: "user1",
			},
			cfg: TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: false,
			},
			want:    true,
			wantStr: "hello world",
		},
		{
			name: "prefix with extra spaces",
			msg: InboundMessage{
				Content:  "  @mote   query text  ",
				SenderID: "user1",
			},
			cfg: TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: false,
			},
			want:    true,
			wantStr: "query text",
		},
		{
			name: "case insensitive match",
			msg: InboundMessage{
				Content:  "@MOTE uppercase",
				SenderID: "user1",
			},
			cfg: TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: false,
			},
			want:    true,
			wantStr: "uppercase",
		},
		{
			name: "case sensitive no match",
			msg: InboundMessage{
				Content:  "@MOTE uppercase",
				SenderID: "user1",
			},
			cfg: TriggerConfig{
				Prefix:        "@mote",
				CaseSensitive: true,
			},
			want:    false,
			wantStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckTrigger(tt.msg, tt.cfg)
			if result.ShouldProcess != tt.want {
				t.Errorf("ShouldProcess = %v, want %v", result.ShouldProcess, tt.want)
			}
			if tt.want && result.StrippedContent != tt.wantStr {
				t.Errorf("StrippedContent = %q, want %q", result.StrippedContent, tt.wantStr)
			}
		})
	}
}

func TestCheckTrigger_WithoutPrefix(t *testing.T) {
	msg := InboundMessage{
		Content:    "hello world",
		SenderID:   "user1",
		IsSelfSend: false,
	}
	cfg := TriggerConfig{
		Prefix:      "@mote",
		SelfTrigger: false,
	}

	result := CheckTrigger(msg, cfg)

	if result.ShouldProcess {
		t.Error("expected ShouldProcess = false for message without prefix")
	}
	if result.SkipReason != "no trigger prefix" {
		t.Errorf("SkipReason = %q, want %q", result.SkipReason, "no trigger prefix")
	}
}

func TestCheckTrigger_SelfTrigger(t *testing.T) {
	tests := []struct {
		name       string
		isSelfSend bool
		selfTrig   bool
		hasPrefix  bool
		want       bool
	}{
		{
			name:       "self send with self trigger enabled, no prefix",
			isSelfSend: true,
			selfTrig:   true,
			hasPrefix:  false,
			want:       true,
		},
		{
			name:       "self send with self trigger disabled, no prefix",
			isSelfSend: true,
			selfTrig:   false,
			hasPrefix:  false,
			want:       false,
		},
		{
			name:       "not self send with self trigger enabled, no prefix",
			isSelfSend: false,
			selfTrig:   true,
			hasPrefix:  false,
			want:       false,
		},
		{
			name:       "self send with prefix",
			isSelfSend: true,
			selfTrig:   true,
			hasPrefix:  true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "hello world"
			if tt.hasPrefix {
				content = "@mote hello world"
			}
			msg := InboundMessage{
				Content:    content,
				SenderID:   "user1",
				IsSelfSend: tt.isSelfSend,
			}
			cfg := TriggerConfig{
				Prefix:      "@mote",
				SelfTrigger: tt.selfTrig,
			}

			result := CheckTrigger(msg, cfg)

			if result.ShouldProcess != tt.want {
				t.Errorf("ShouldProcess = %v, want %v", result.ShouldProcess, tt.want)
			}
		})
	}
}

func TestCheckTrigger_AllowList(t *testing.T) {
	tests := []struct {
		name      string
		senderID  string
		allowList []string
		want      bool
		wantSkip  string
	}{
		{
			name:      "sender in allowlist",
			senderID:  "allowed-user",
			allowList: []string{"allowed-user", "another-user"},
			want:      true,
			wantSkip:  "",
		},
		{
			name:      "sender not in allowlist",
			senderID:  "blocked-user",
			allowList: []string{"allowed-user"},
			want:      false,
			wantSkip:  "sender not in allowlist",
		},
		{
			name:      "empty allowlist allows all",
			senderID:  "any-user",
			allowList: []string{},
			want:      true,
			wantSkip:  "",
		},
		{
			name:      "nil allowlist allows all",
			senderID:  "any-user",
			allowList: nil,
			want:      true,
			wantSkip:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := InboundMessage{
				Content:  "@mote test message",
				SenderID: tt.senderID,
			}
			cfg := TriggerConfig{
				Prefix:    "@mote",
				AllowList: tt.allowList,
			}

			result := CheckTrigger(msg, cfg)

			if result.ShouldProcess != tt.want {
				t.Errorf("ShouldProcess = %v, want %v", result.ShouldProcess, tt.want)
			}
			if !tt.want && result.SkipReason != tt.wantSkip {
				t.Errorf("SkipReason = %q, want %q", result.SkipReason, tt.wantSkip)
			}
		})
	}
}

func TestInjectReplyPrefix(t *testing.T) {
	tests := []struct {
		name    string
		content string
		cfg     ReplyConfig
		want    string
	}{
		{
			name:    "with prefix and default separator",
			content: "Hello, this is a reply",
			cfg: ReplyConfig{
				Prefix: "[Mote]",
			},
			want: "[Mote]\nHello, this is a reply",
		},
		{
			name:    "with prefix and custom separator",
			content: "Hello, this is a reply",
			cfg: ReplyConfig{
				Prefix:    "[Mote]",
				Separator: " ",
			},
			want: "[Mote] Hello, this is a reply",
		},
		{
			name:    "empty prefix returns content unchanged",
			content: "Hello, this is a reply",
			cfg: ReplyConfig{
				Prefix: "",
			},
			want: "Hello, this is a reply",
		},
		{
			name:    "multiline content",
			content: "Line 1\nLine 2\nLine 3",
			cfg: ReplyConfig{
				Prefix:    "[Mote]",
				Separator: "\n",
			},
			want: "[Mote]\nLine 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectReplyPrefix(tt.content, tt.cfg)
			if got != tt.want {
				t.Errorf("InjectReplyPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
