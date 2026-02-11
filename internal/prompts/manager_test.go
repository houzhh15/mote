package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDirectory(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	
	// Create a test prompt file
	testPromptContent := `---
name: test-prompt
description: A test prompt
type: system
priority: 10
enabled: true
arguments:
  - name: input
    description: Test input
    required: true
---

This is a test prompt with {{input}}.
`
	
	promptFile := filepath.Join(tmpDir, "test-prompt.md")
	if err := os.WriteFile(promptFile, []byte(testPromptContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Create manager and load
	manager := NewManagerWithConfig(ManagerConfig{
		PromptsDirs: []string{tmpDir},
	})
	
	// Check that prompt was loaded
	prompts := manager.ListPrompts()
	if len(prompts) != 1 {
		t.Fatalf("Expected 1 prompt, got %d", len(prompts))
	}
	
	prompt := prompts[0]
	if prompt.Name != "test-prompt" {
		t.Errorf("Expected name 'test-prompt', got '%s'", prompt.Name)
	}
	if prompt.Description != "A test prompt" {
		t.Errorf("Expected description 'A test prompt', got '%s'", prompt.Description)
	}
	if len(prompt.Arguments) != 1 {
		t.Fatalf("Expected 1 argument, got %d", len(prompt.Arguments))
	}
	if prompt.Arguments[0].Name != "input" {
		t.Errorf("Expected argument name 'input', got '%s'", prompt.Arguments[0].Name)
	}
	if prompt.Source != PromptSourceFile {
		t.Errorf("Expected source 'file', got '%s'", prompt.Source)
	}
	if !prompt.Enabled {
		t.Error("Expected prompt to be enabled")
	}
}

func TestSavePromptToFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	manager := NewManagerWithConfig(ManagerConfig{
		PromptsDirs:    []string{tmpDir},
		EnableAutoSave: true,
	})
	
	// Add a prompt
	prompt, err := manager.AddPrompt(PromptConfig{
		Name:        "auto-saved",
		Description: "Auto-saved prompt",
		Type:        PromptTypeSystem,
		Content:     "Test content with {{variable}}",
		Arguments: []PromptArgument{
			{Name: "variable", Description: "A test variable", Required: true},
		},
		Priority: 5,
		Enabled:  true,
	})
	
	if err != nil {
		t.Fatalf("Failed to add prompt: %v", err)
	}
	
	// Check that file was created
	expectedFile := filepath.Join(tmpDir, "auto-saved.md")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Expected file %s to be created", expectedFile)
	}
	
	// Read file and verify content
	content, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	
	contentStr := string(content)
	if contentStr == "" {
		t.Error("File content is empty")
	}
	
	// Verify the prompt has file path set
	if prompt.FilePath == "" {
		t.Error("Expected prompt to have file path set")
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantContent string
		wantErr     bool
	}{
		{
			name: "valid frontmatter",
			input: `---
name: test
description: Test prompt
---

Content here`,
			wantName:    "test",
			wantContent: "Content here",
			wantErr:     false,
		},
		{
			name:        "no frontmatter",
			input:       "Just content",
			wantName:    "",
			wantContent: "Just content",
			wantErr:     false,
		},
		{
			name: "invalid frontmatter - missing closing",
			input: `---
name: test
`,
			wantName:    "",
			wantContent: "",
			wantErr:     true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, content, err := parseFrontmatter(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFrontmatter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if fm.Name != tt.wantName {
					t.Errorf("parseFrontmatter() name = %v, want %v", fm.Name, tt.wantName)
				}
				if content != tt.wantContent {
					t.Errorf("parseFrontmatter() content = %v, want %v", content, tt.wantContent)
				}
			}
		})
	}
}

func TestReloadFromFiles(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create initial prompt file
	initialContent := `---
name: reload-test
description: Initial content
---

Initial prompt content`
	
	promptFile := filepath.Join(tmpDir, "reload-test.md")
	if err := os.WriteFile(promptFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}
	
	// Load prompts
	manager := NewManagerWithConfig(ManagerConfig{
		PromptsDirs: []string{tmpDir},
	})
	
	prompts := manager.ListPrompts()
	if len(prompts) != 1 {
		t.Fatalf("Expected 1 prompt, got %d", len(prompts))
	}
	if prompts[0].Description != "Initial content" {
		t.Errorf("Expected 'Initial content', got '%s'", prompts[0].Description)
	}
	
	// Update file
	updatedContent := `---
name: reload-test
description: Updated content
---

Updated prompt content`
	
	if err := os.WriteFile(promptFile, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}
	
	// Reload
	if err := manager.ReloadFromFiles(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}
	
	// Verify updated content
	prompts = manager.ListPrompts()
	if len(prompts) != 1 {
		t.Fatalf("Expected 1 prompt after reload, got %d", len(prompts))
	}
	if prompts[0].Description != "Updated content" {
		t.Errorf("Expected 'Updated content', got '%s'", prompts[0].Description)
	}
}
