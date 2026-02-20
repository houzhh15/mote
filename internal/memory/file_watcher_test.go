package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWatcher_IsMarkdownFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"MEMORY.md", true},
		{"memory/2024-01-01.md", true},
		{"file.txt", false},
		{"readme.MD", true},
		{"data.json", false},
	}

	for _, tt := range tests {
		got := isMarkdownFile(tt.path)
		if got != tt.want {
			t.Errorf("isMarkdownFile(%q): got %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestFileWatcher_DetectsChange(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a markdown file
	mdFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(mdFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed := make(chan []string, 1)
	callback := func(files []string) {
		changed <- files
	}

	watcher, err := NewFileWatcher([]string{tmpDir}, callback, testLogger())
	if err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	watcher.SetDebounceDelay(100 * time.Millisecond)
	defer watcher.Close()

	// Modify the file
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(mdFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for callback
	select {
	case files := <-changed:
		if len(files) == 0 {
			t.Error("expected changed files, got empty")
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for file change notification")
	}
}

func TestFileWatcher_IgnoresNonMarkdown(t *testing.T) {
	tmpDir := t.TempDir()

	changed := make(chan []string, 1)
	callback := func(files []string) {
		changed <- files
	}

	watcher, err := NewFileWatcher([]string{tmpDir}, callback, testLogger())
	if err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	watcher.SetDebounceDelay(100 * time.Millisecond)
	defer watcher.Close()

	// Create a non-markdown file
	txtFile := filepath.Join(tmpDir, "test.txt")
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(txtFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Should NOT trigger callback
	select {
	case <-changed:
		t.Error("should not trigger for non-markdown files")
	case <-time.After(500 * time.Millisecond):
		// Expected: no notification
	}
}
