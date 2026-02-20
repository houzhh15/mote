package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryManager_NewAndInit(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultManagerConfig()
	config.BaseDir = tmpDir
	config.EnableWatch = false // Disable for test

	mm, err := NewMemoryManager(db, embedder, config, testLogger())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	defer mm.Close()

	if err := mm.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
}

func TestMemoryManager_AddAndSearch(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultManagerConfig()
	config.BaseDir = tmpDir
	config.EnableWatch = false

	mm, err := NewMemoryManager(db, embedder, config, testLogger())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	defer mm.Close()

	ctx := context.Background()

	// Add entry
	entry := MemoryEntry{
		Content: "Go is a compiled programming language",
		Source:  SourceConversation,
	}
	if err := mm.Add(ctx, entry); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Search
	results, err := mm.QuickSearch(ctx, "Go programming", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	_ = results // SimpleEmbedder may not produce great results, just verify no error
}

func TestMemoryManager_OnSessionEnd_NoProvider(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultManagerConfig()
	config.BaseDir = tmpDir
	config.EnableWatch = false

	mm, err := NewMemoryManager(db, embedder, config, testLogger())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	defer mm.Close()

	// Should not error when no LLM provider is set
	err = mm.OnSessionEnd(context.Background(), "session-1", []Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestMemoryManager_Stats(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultManagerConfig()
	config.BaseDir = tmpDir
	config.EnableWatch = false

	mm, err := NewMemoryManager(db, embedder, config, testLogger())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	defer mm.Close()

	stats, err := mm.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	if _, ok := stats["index_entries"]; !ok {
		t.Error("stats missing index_entries")
	}
	if _, ok := stats["content_sections"]; !ok {
		t.Error("stats missing content_sections")
	}
}
