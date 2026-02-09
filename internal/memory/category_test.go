package memory

import (
	"testing"
)

func TestNewCategoryDetector(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("failed to create detector: %v", err)
	}
	if detector == nil {
		t.Fatal("detector is nil")
	}
	if len(detector.patterns) != len(DefaultCategoryPatterns) { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Errorf("expected %d patterns, got %d", len(DefaultCategoryPatterns), len(detector.patterns))
	}
}

func TestCategoryDetector_Detect_Preference(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		// English preference patterns
		{"prefer dark theme", "I prefer dark theme for coding", CategoryPreference},
		{"like coffee", "I like drinking coffee in the morning", CategoryPreference},
		{"love programming", "I love programming in Go", CategoryPreference},
		{"hate bugs", "I hate dealing with memory bugs", CategoryPreference},
		{"want feature", "I want this feature implemented", CategoryPreference},
		{"favorite editor", "VS Code is my favorite editor", CategoryPreference}, // Avoid "my X is" entity pattern
		{"enjoy learning", "I enjoy learning new languages", CategoryPreference},
		{"dislike meetings", "I dislike long meetings", CategoryPreference},

		// Chinese preference patterns
		{"喜欢深色", "我喜欢深色主题", CategoryPreference},
		{"讨厌广告", "我讨厌弹窗广告", CategoryPreference},
		{"偏好简洁", "用户偏好简洁的界面", CategoryPreference},
		{"最爱Go", "他最爱用Go编程", CategoryPreference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.text)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestCategoryDetector_Detect_Decision(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		// English decision patterns
		{"decided to use", "We decided to use PostgreSQL", CategoryDecision},
		{"will use Go", "The team will use Go for backend", CategoryDecision},
		{"chose React", "We chose React for the frontend", CategoryDecision},
		{"going to deploy", "We are going to deploy on AWS", CategoryDecision},
		{"plan to refactor", "I plan to refactor this module", CategoryDecision},

		// Chinese decision patterns
		{"我们决定", "我们决定使用微服务架构", CategoryDecision},
		{"确定使用", "确定使用Docker部署", CategoryDecision},
		{"选择了", "团队选择了敏捷开发方法", CategoryDecision},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.text)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestCategoryDetector_Detect_Entity(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		// Phone numbers
		{"phone number", "My phone is +8613812345678", CategoryEntity},
		{"international phone", "Contact me at +14155551234", CategoryEntity},

		// Email addresses
		{"email address", "Email: john.doe@example.com", CategoryEntity},
		{"work email", "Send to team@company.org", CategoryEntity},

		// English name declarations
		{"is called", "The project is called Phoenix", CategoryEntity},
		{"named", "The variable named configPath", CategoryEntity},
		{"my name is", "My name is John", CategoryEntity},

		// Chinese entity patterns
		{"叫做", "这个项目叫做凤凰计划", CategoryEntity},
		{"名字是", "我的名字是张三", CategoryEntity},
		{"我的邮箱是", "我的邮箱是test@example.com", CategoryEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.text)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestCategoryDetector_Detect_Fact(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		// English fact patterns
		{"is a language", "Go is a statically typed language", CategoryFact},
		{"are used for", "Channels are used for communication", CategoryFact},
		{"has methods", "The struct has several methods", CategoryFact},
		{"have been updated", "The docs have been updated recently", CategoryFact},

		// Chinese fact patterns
		{"是一个", "Python是一个动态语言", CategoryFact},
		{"有很多", "这个库有很多功能", CategoryFact},
		{"存在于", "这个文件存在于src目录", CategoryFact},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.text)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestCategoryDetector_Detect_Other(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	tests := []struct {
		name string
		text string
	}{
		{"empty string", ""},
		{"random text", "xyz abc 123"},
		{"just numbers", "12345"},
		{"chinese noise", "啊啊啊"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.text)
			if got != CategoryOther {
				t.Errorf("Detect(%q) = %q, want %q", tt.text, got, CategoryOther)
			}
		})
	}
}

func TestCategoryDetector_DetectWithConfidence(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	t.Run("returns category and positive confidence", func(t *testing.T) {
		category, confidence := detector.DetectWithConfidence("I really like and love this feature")
		if category != CategoryPreference {
			t.Errorf("expected %s, got %s", CategoryPreference, category)
		}
		if confidence <= 0 {
			t.Errorf("expected positive confidence, got %f", confidence)
		}
	})

	t.Run("returns other with zero confidence for no match", func(t *testing.T) {
		category, confidence := detector.DetectWithConfidence("xyz 123")
		if category != CategoryOther {
			t.Errorf("expected %s, got %s", CategoryOther, category)
		}
		if confidence != 0 {
			t.Errorf("expected zero confidence, got %f", confidence)
		}
	})

	t.Run("empty text returns other with zero confidence", func(t *testing.T) {
		category, confidence := detector.DetectWithConfidence("")
		if category != CategoryOther {
			t.Errorf("expected %s, got %s", CategoryOther, category)
		}
		if confidence != 0 {
			t.Errorf("expected zero confidence, got %f", confidence)
		}
	})
}

func TestCategoryDetector_Priority(t *testing.T) {
	detector, err := NewCategoryDetector()
	if err != nil {
		t.Fatalf("create detector: %v", err)
	}

	// Entity should take priority over fact when both patterns match
	t.Run("entity takes priority over fact", func(t *testing.T) {
		// "is" is a fact pattern, but "叫做" is an entity pattern
		text := "这个项目叫做Phoenix，它是一个好项目"
		got := detector.Detect(text)
		if got != CategoryEntity {
			t.Errorf("Detect(%q) = %q, want %q (entity should have priority)", text, got, CategoryEntity)
		}
	})

	// Email with "is" - entity should win
	t.Run("email entity over fact", func(t *testing.T) {
		text := "My email is test@example.com"
		got := detector.Detect(text)
		if got != CategoryEntity {
			t.Errorf("Detect(%q) = %q, want %q", text, got, CategoryEntity)
		}
	})
}

func TestNewCategoryDetectorWithPatterns(t *testing.T) {
	t.Run("custom patterns", func(t *testing.T) {
		patterns := map[string]string{
			"custom": `\bcustom\b`,
		}
		order := []string{"custom"}

		detector, err := NewCategoryDetectorWithPatterns(patterns, order)
		if err != nil {
			t.Fatalf("create detector: %v", err)
		}

		got := detector.Detect("This is a custom pattern")
		if got != "custom" {
			t.Errorf("expected custom, got %s", got)
		}
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		patterns := map[string]string{
			"invalid": `[unclosed`,
		}
		_, err := NewCategoryDetectorWithPatterns(patterns, nil)
		if err == nil {
			t.Error("expected error for invalid regex")
		}
	})
}
