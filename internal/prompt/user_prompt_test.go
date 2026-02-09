package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileUserPromptStore_GetSet(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "prompts.json")
	store := NewFileUserPromptStore(storePath)

	// Test Get on empty store
	prompt, err := store.Get("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != nil {
		t.Error("expected nil prompt for nonexistent name")
	}

	// Test Set
	testPrompt := &UserPrompt{
		Name:        "test",
		Description: "A test prompt",
		Content:     "You are helpful.",
		Slot:        SlotIdentity,
		Tags:        []string{"test", "identity"},
	}
	if err := store.Set(testPrompt); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get after Set
	prompt, err = store.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if prompt == nil {
		t.Fatal("expected prompt to exist")
	}
	if prompt.Name != "test" { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Errorf("expected name 'test', got '%s'", prompt.Name)
	}
	if prompt.Content != "You are helpful." {
		t.Errorf("expected content, got '%s'", prompt.Content)
	}
	if prompt.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestFileUserPromptStore_Update(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "prompts.json")
	store := NewFileUserPromptStore(storePath)

	// Create initial prompt
	_ = store.Set(&UserPrompt{Name: "test", Content: "original"})

	// Update
	_ = store.Set(&UserPrompt{Name: "test", Content: "updated"})

	// Verify update
	prompt, _ := store.Get("test")
	if prompt.Content != "updated" {
		t.Errorf("expected updated content, got '%s'", prompt.Content)
	}

	// Verify only one prompt exists
	prompts, _ := store.List()
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt after update, got %d", len(prompts))
	}
}

func TestFileUserPromptStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "prompts.json")
	store := NewFileUserPromptStore(storePath)

	_ = store.Set(&UserPrompt{Name: "test", Content: "content"})

	// Delete
	if err := store.Delete("test"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	prompt, _ := store.Get("test")
	if prompt != nil {
		t.Error("expected prompt to be deleted")
	}
}

func TestFileUserPromptStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "prompts.json")
	store := NewFileUserPromptStore(storePath)

	// List empty
	prompts, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(prompts))
	}

	// Add some prompts
	_ = store.Set(&UserPrompt{Name: "a", Content: "content a"})
	_ = store.Set(&UserPrompt{Name: "b", Content: "content b"})
	_ = store.Set(&UserPrompt{Name: "c", Content: "content c"})

	// List again
	prompts, err = store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(prompts) != 3 {
		t.Errorf("expected 3 prompts, got %d", len(prompts))
	}
}

func TestFileUserPromptStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "prompts.json")

	// Create store and add prompt
	store1 := NewFileUserPromptStore(storePath)
	_ = store1.Set(&UserPrompt{Name: "persist", Content: "test"})

	// Create new store instance
	store2 := NewFileUserPromptStore(storePath)
	prompt, err := store2.Get("persist")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if prompt == nil || prompt.Content != "test" {
		t.Error("persistence failed")
	}
}

func TestFileUserPromptStore_DefaultPath(t *testing.T) {
	store := NewFileUserPromptStore("")
	if store.path == "" {
		t.Error("expected default path to be set")
	}

	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".mote", "user-prompts.json")
	if store.path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, store.path)
	}
}

func TestSlashCommandParser_Parse(t *testing.T) {
	parser := NewSlashCommandParser()
	parser.RegisterCommand("prompt", []string{"list", "use", "show"})

	tests := []struct {
		input    string
		expected *SlashCommand
	}{
		{"/prompt list", &SlashCommand{Name: "prompt", Action: "list"}},
		{"/prompt use my-prompt", &SlashCommand{Name: "prompt", Action: "use", Args: []string{"my-prompt"}}},
		{"/prompt show test", &SlashCommand{Name: "prompt", Action: "show", Args: []string{"test"}}},
		{"/prompt", &SlashCommand{Name: "prompt"}},
		{"/unknown", nil},
		{"not a command", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd := parser.Parse(tt.input)
			if tt.expected == nil {
				if cmd != nil {
					t.Errorf("expected nil, got %v", cmd)
				}
				return
			}
			if cmd == nil {
				t.Fatalf("expected command, got nil")
			}
			if cmd.Name != tt.expected.Name { //nolint:staticcheck // SA5011: Check above ensures non-nil
				t.Errorf("expected name '%s', got '%s'", tt.expected.Name, cmd.Name)
			}
			if cmd.Action != tt.expected.Action {
				t.Errorf("expected action '%s', got '%s'", tt.expected.Action, cmd.Action)
			}
			if len(cmd.Args) != len(tt.expected.Args) {
				t.Errorf("expected %d args, got %d", len(tt.expected.Args), len(cmd.Args))
			}
		})
	}
}

func TestSlashCommandParser_IsCommand(t *testing.T) {
	parser := NewSlashCommandParser()
	parser.RegisterCommand("prompt", []string{"list"})

	if !parser.IsCommand("/prompt") {
		t.Error("expected '/prompt' to be a command")
	}
	if !parser.IsCommand("/prompt list") {
		t.Error("expected '/prompt list' to be a command")
	}
	if parser.IsCommand("/unknown") {
		t.Error("expected '/unknown' not to be a command")
	}
	if parser.IsCommand("not a command") {
		t.Error("expected regular text not to be a command")
	}
}

func TestSlashCommand_String(t *testing.T) {
	tests := []struct {
		cmd      *SlashCommand
		expected string
	}{
		{&SlashCommand{Name: "prompt"}, "/prompt"},
		{&SlashCommand{Name: "prompt", Action: "list"}, "/prompt list"},
		{&SlashCommand{Name: "prompt", Action: "use", Args: []string{"my-prompt"}}, "/prompt use my-prompt"},
	}

	for _, tt := range tests {
		result := tt.cmd.String()
		if result != tt.expected {
			t.Errorf("expected '%s', got '%s'", tt.expected, result)
		}
	}
}

func TestRegisterBuiltinCommands(t *testing.T) {
	parser := NewSlashCommandParser()
	RegisterBuiltinCommands(parser)

	// Check prompt command is registered
	if !parser.IsCommand("/prompt") {
		t.Error("expected '/prompt' to be registered")
	}
	if !parser.IsCommand("/memory") {
		t.Error("expected '/memory' to be registered")
	}
	if !parser.IsCommand("/help") {
		t.Error("expected '/help' to be registered")
	}
	if !parser.IsCommand("/clear") {
		t.Error("expected '/clear' to be registered")
	}
	if !parser.IsCommand("/status") {
		t.Error("expected '/status' to be registered")
	}
}

func TestExecutePromptCommand_List(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileUserPromptStore(filepath.Join(tmpDir, "prompts.json"))

	// Empty list
	cmd := &SlashCommand{Name: "prompt", Action: "list"}
	msg, handled := ExecutePromptCommand(cmd, store)
	if !handled {
		t.Error("expected command to be handled")
	}
	if msg != "No user prompts defined." {
		t.Errorf("expected empty message, got '%s'", msg)
	}

	// Add prompts and list again
	_ = store.Set(&UserPrompt{Name: "test", Description: "Test prompt"})
	msg, _ = ExecutePromptCommand(cmd, store)
	if msg == "No user prompts defined." {
		t.Error("expected prompts to be listed")
	}
}

func TestExecutePromptCommand_Show(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileUserPromptStore(filepath.Join(tmpDir, "prompts.json"))
	_ = store.Set(&UserPrompt{Name: "test", Description: "Test", Content: "Test content"})

	// Show existing
	cmd := &SlashCommand{Name: "prompt", Action: "show", Args: []string{"test"}}
	msg, handled := ExecutePromptCommand(cmd, store)
	if !handled {
		t.Error("expected command to be handled")
	}
	if msg == "" {
		t.Error("expected show output")
	}

	// Show missing
	cmd.Args = []string{"nonexistent"}
	msg, _ = ExecutePromptCommand(cmd, store)
	if msg != "Prompt 'nonexistent' not found." {
		t.Errorf("expected not found message, got '%s'", msg)
	}

	// Show without args
	cmd.Args = nil
	msg, _ = ExecutePromptCommand(cmd, store)
	if msg != "Usage: /prompt show <name>" {
		t.Errorf("expected usage message, got '%s'", msg)
	}
}

func TestExecutePromptCommand_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileUserPromptStore(filepath.Join(tmpDir, "prompts.json"))
	_ = store.Set(&UserPrompt{Name: "test", Content: "content"})

	cmd := &SlashCommand{Name: "prompt", Action: "delete", Args: []string{"test"}}
	msg, handled := ExecutePromptCommand(cmd, store)
	if !handled {
		t.Error("expected command to be handled")
	}
	if msg != "Prompt 'test' deleted." {
		t.Errorf("expected delete message, got '%s'", msg)
	}

	// Verify deleted
	prompt, _ := store.Get("test")
	if prompt != nil {
		t.Error("expected prompt to be deleted")
	}
}

func TestExecutePromptCommand_NotPrompt(t *testing.T) {
	cmd := &SlashCommand{Name: "memory", Action: "list"}
	_, handled := ExecutePromptCommand(cmd, nil)
	if handled {
		t.Error("expected non-prompt command not to be handled")
	}

	_, handled = ExecutePromptCommand(nil, nil)
	if handled {
		t.Error("expected nil command not to be handled")
	}
}
