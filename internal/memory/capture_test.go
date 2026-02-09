package memory

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func TestDefaultCaptureConfig(t *testing.T) {
	config := DefaultCaptureConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}
	if config.MinLength != 10 {
		t.Errorf("expected MinLength 10, got %d", config.MinLength)
	}
	if config.MaxLength != 500 {
		t.Errorf("expected MaxLength 500, got %d", config.MaxLength)
	}
	if config.DupThreshold != 0.95 {
		t.Errorf("expected DupThreshold 0.95, got %f", config.DupThreshold)
	}
	if config.MaxPerSession != 3 {
		t.Errorf("expected MaxPerSession 3, got %d", config.MaxPerSession)
	}
}

func TestNewCaptureEngine(t *testing.T) {
	t.Run("creates with defaults", func(t *testing.T) {
		engine, err := NewCaptureEngine(CaptureEngineOptions{
			Config: DefaultCaptureConfig(),
		})
		if err != nil {
			t.Fatalf("create engine: %v", err)
		}
		if engine == nil {
			t.Fatal("engine is nil")
		}
		if len(engine.triggers) != len(DefaultCapturePatterns) { //nolint:staticcheck // SA5011: Check above ensures non-nil
			t.Errorf("expected %d triggers, got %d", len(DefaultCapturePatterns), len(engine.triggers))
		}
	})

	t.Run("creates with custom patterns", func(t *testing.T) {
		engine, err := NewCaptureEngine(CaptureEngineOptions{
			Config:   DefaultCaptureConfig(),
			Patterns: []string{`\bcustom\b`},
		})
		if err != nil {
			t.Fatalf("create engine: %v", err)
		}
		if len(engine.triggers) != 1 {
			t.Errorf("expected 1 trigger, got %d", len(engine.triggers))
		}
	})

	t.Run("returns error for invalid regex", func(t *testing.T) {
		_, err := NewCaptureEngine(CaptureEngineOptions{
			Config:   DefaultCaptureConfig(),
			Patterns: []string{`[invalid`},
		})
		if err == nil {
			t.Error("expected error for invalid regex")
		}
	})
}

func TestCaptureEngine_ShouldCapture(t *testing.T) {
	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: DefaultCaptureConfig(),
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Should capture - explicit commands
		{"remember command", "Please remember that I prefer dark mode", true},
		{"don't forget", "Don't forget to use tabs for indentation", true},
		{"记住", "记住我喜欢简洁的代码风格", true},
		{"别忘了", "别忘了这个API需要认证", true},

		// Should capture - preferences
		{"prefer", "I prefer using Go for backend services", true},
		{"like", "I like functional programming patterns", true},
		{"hate", "I hate unnecessary complexity", true},
		{"喜欢", "我喜欢类型安全的语言", true},

		// Should capture - decisions
		{"decided", "We decided to use PostgreSQL", true},
		{"will use", "The team will use Docker for deployment", true},
		{"决定", "我们决定采用微服务架构", true},

		// Should capture - entities
		{"phone number", "My phone is +8613812345678", true},
		{"email", "Contact me at john@example.com", true},
		{"name declaration", "The project is called Phoenix", true},
		{"叫做", "这个服务叫做UserService", true},

		// Should capture - emphasis
		{"important", "This is an important configuration", true},
		{"must", "You must always validate input", true},
		{"必须", "必须保证数据一致性", true},

		// Should NOT capture - too short
		{"too short", "Hi", false},
		{"short", "OK", false},

		// Should NOT capture - too long (over 500 chars)
		{"too long", string(make([]byte, 501)), false},

		// Should NOT capture - XML content
		{"xml tag", "<tool>Some tool output here</tool>", false},
		{"relevant-memories", "<relevant-memories>Previous context</relevant-memories>", false},

		// Should NOT capture - heavily formatted markdown
		{"markdown", "## Title\n- Item 1\n- Item 2\n\n**Bold text**", false},
		{"code block", "```go\nfunc main() {}\n```\n- List item", false},

		// Should NOT capture - no trigger patterns
		{"no pattern", "The weather is nice today", false},
		{"random", "Random text without any triggers", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.ShouldCapture(tt.text)
			if got != tt.expected {
				t.Errorf("ShouldCapture(%q) = %v, want %v", truncate(tt.text, 50), got, tt.expected)
			}
		})
	}
}

func TestCaptureEngine_ShouldCapture_Disabled(t *testing.T) {
	config := DefaultCaptureConfig()
	config.Enabled = false

	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: config,
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// Should return false when disabled
	if engine.ShouldCapture("Please remember this important fact") {
		t.Error("expected false when capture is disabled")
	}
}

func TestCaptureEngine_Capture(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Memory: idx,
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	ctx := context.Background()

	t.Run("captures matching messages", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Please remember that I prefer dark mode for coding"},
			{Role: "user", Content: "My phone number is +8613812345678"},
			{Role: "user", Content: "We decided to use PostgreSQL for the database"},
		}

		captured, err := engine.Capture(ctx, messages)
		if err != nil {
			t.Fatalf("capture: %v", err)
		}

		// Should capture all 3 messages (each matches different patterns)
		if captured != 3 {
			t.Errorf("expected 3 captured, got %d", captured)
		}

		// Verify in database
		count, _ := idx.Count(ctx)
		if count != 3 {
			t.Errorf("expected 3 entries in db, got %d", count)
		}
	})
}

func TestCaptureEngine_Capture_SessionLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	captureConfig := DefaultCaptureConfig()
	captureConfig.MaxPerSession = 2 // Limit to 2

	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Memory: idx,
		Config: captureConfig,
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	ctx := context.Background()

	messages := []Message{
		{Role: "user", Content: "Remember my preference for dark mode"},
		{Role: "user", Content: "Remember my phone is +8613812345678"},
		{Role: "user", Content: "Remember my email is test@example.com"},
		{Role: "user", Content: "Remember I like Go language"},
	}

	captured, err := engine.Capture(ctx, messages)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Should only capture 2 due to session limit
	if captured != 2 {
		t.Errorf("expected 2 captured (session limit), got %d", captured)
	}
}

func TestCaptureEngine_Capture_SkipsSystemMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Memory: idx,
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	ctx := context.Background()

	messages := []Message{
		{Role: "system", Content: "Remember to be helpful and accurate"},
		{Role: "user", Content: "Hello"},
	}

	captured, err := engine.Capture(ctx, messages)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Should skip system message even though it matches "remember"
	if captured != 0 {
		t.Errorf("expected 0 captured (system messages skipped), got %d", captured)
	}
}

func TestCaptureEngine_ResetSession(t *testing.T) {
	engine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: DefaultCaptureConfig(),
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// Simulate some captures
	engine.captured = 3

	engine.ResetSession()

	if engine.captured != 0 {
		t.Errorf("expected captured to be 0 after reset, got %d", engine.captured)
	}
}

func TestIsXMLContent(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"<tool>output</tool>", true},
		{"<result>data</result>", true},
		{"<relevant-memories>context</relevant-memories>", true},
		{"Plain text", false},
		{"<short>", false}, // Too short
		{"Some text with < and >", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := isXMLContent(tt.text)
			if got != tt.expected {
				t.Errorf("isXMLContent(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestIsFormattedMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"plain text", "Just some plain text", false},
		{"one pattern", "Some **bold** text", false},
		{"two patterns", "## Title\n- List item", true},
		{"three patterns", "## Title\n- Item\n**Bold**", true},
		{"code block with list", "```\ncode\n```\n- Item", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFormattedMarkdown(tt.text)
			if got != tt.expected {
				t.Errorf("isFormattedMarkdown(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestStringSimilarity(t *testing.T) {
	tests := []struct {
		a, b   string
		minSim float64
		maxSim float64
	}{
		{"hello world", "hello world", 1.0, 1.0},
		{"", "", 1.0, 1.0}, // Identical strings (even empty) have similarity 1.0
		{"hello", "", 0.0, 0.0},
		{"", "world", 0.0, 0.0},
		{"hello world", "hello there", 0.2, 0.5},
		{"completely different", "nothing alike", 0.0, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := stringSimilarity(tt.a, tt.b)
			if got < tt.minSim || got > tt.maxSim {
				t.Errorf("stringSimilarity(%q, %q) = %f, want between %f and %f",
					tt.a, tt.b, got, tt.minSim, tt.maxSim)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s        string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is a ..."},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.expected)
			}
		})
	}
}
