package skills

import (
	"testing"
)

func TestParseSkillMDContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
		wantErr  bool
	}{
		{
			name: "basic skill",
			content: `---
name: Test Skill
description: A test skill for testing
---

# Test Skill

This is the body.
`,
			wantName: "Test Skill",
			wantDesc: "A test skill for testing",
			wantErr:  false,
		},
		{
			name: "skill with homepage",
			content: `---
name: GitHub Skill
description: GitHub integration
homepage: https://github.com
---

# GitHub Skill
`,
			wantName: "GitHub Skill",
			wantDesc: "GitHub integration",
			wantErr:  false,
		},
		{
			name:    "no frontmatter",
			content: "# Just a markdown file\n\nNo frontmatter here.",
			wantErr: true,
		},
		{
			name: "empty name",
			content: `---
description: No name provided
---

# Missing Name
`,
			wantErr: true,
		},
		{
			name: "skill with metadata json",
			content: `---
name: MCP Config
description: Configure MCP servers
metadata: '{"openclaw": {"emoji": "🔧", "os": ["mac", "linux"]}}'
---

# MCP Configuration
`,
			wantName: "🔧 MCP Config", // Emoji prepended
			wantDesc: "Configure MCP servers",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := ParseSkillMDContent(tt.content, "/test/SKILL.md")
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSkillMDContent() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSkillMDContent() error = %v", err)
				return
			}
			if entry.Name != tt.wantName {
				t.Errorf("ParseSkillMDContent() name = %v, want %v", entry.Name, tt.wantName)
			}
			if entry.Description != tt.wantDesc {
				t.Errorf("ParseSkillMDContent() description = %v, want %v", entry.Description, tt.wantDesc)
			}
			if entry.Source != SourceSkillMD {
				t.Errorf("ParseSkillMDContent() source = %v, want %v", entry.Source, SourceSkillMD)
			}
		})
	}
}

func TestGetSkillMDBody(t *testing.T) {
	content := `---
name: Test
description: Test skill
---

# Body Content

This is the body.
`
	body := GetSkillMDBody(content)
	if body == "" {
		t.Error("GetSkillMDBody() returned empty string")
	}
	if body[0:1] == "-" {
		t.Error("GetSkillMDBody() should not include frontmatter")
	}
	if !containsStr(body, "Body Content") {
		t.Error("GetSkillMDBody() should include body content")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrImpl(s, substr))
}

func containsStrImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
