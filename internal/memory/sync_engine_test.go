package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncEngine_FullSync(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MEMORY.md
	memoryContent := "## Test Entry\n\nSome memory content.\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(memoryContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	logger := testLogger()
	embedder := NewSimpleEmbedder(384)

	mdStore, err := NewMarkdownStore(MarkdownStoreOptions{BaseDir: tmpDir, Logger: logger})
	if err != nil {
		t.Fatalf("create markdown store: %v", err)
	}

	cs, err := NewContentStore(ContentStoreOptions{Store: mdStore, Logger: logger})
	if err != nil {
		t.Fatalf("create content store: %v", err)
	}

	indexMgr, err := NewIndexManager(IndexManagerOptions{
		DB:       db,
		Embedder: embedder,
		Config:   DefaultIndexConfig(),
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("create index manager: %v", err)
	}

	be := NewBatchEmbedder(embedder, DefaultBatchConfig(), logger)
	se := NewSyncEngine(cs, indexMgr, be, logger)

	result, err := se.FullSync(context.Background())
	if err != nil {
		t.Fatalf("full sync: %v", err)
	}

	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestSyncEngine_IncrementalSync_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := setupTestDB(t)
	defer db.Close()

	logger := testLogger()
	embedder := NewSimpleEmbedder(384)

	mdStore, err := NewMarkdownStore(MarkdownStoreOptions{BaseDir: tmpDir, Logger: logger})
	if err != nil {
		t.Fatalf("create markdown store: %v", err)
	}

	cs, err := NewContentStore(ContentStoreOptions{Store: mdStore, Logger: logger})
	if err != nil {
		t.Fatalf("create content store: %v", err)
	}

	indexMgr, err := NewIndexManager(IndexManagerOptions{
		DB:       db,
		Embedder: embedder,
		Config:   DefaultIndexConfig(),
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("create index manager: %v", err)
	}

	be := NewBatchEmbedder(embedder, DefaultBatchConfig(), logger)
	se := NewSyncEngine(cs, indexMgr, be, logger)

	// Double sync should show no changes on second run
	if _, err := se.IncrementalSync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	result, err := se.IncrementalSync(context.Background())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if result.Created != 0 || result.Updated != 0 || result.Deleted != 0 {
		t.Errorf("expected no changes, got created=%d updated=%d deleted=%d",
			result.Created, result.Updated, result.Deleted)
	}
}

func TestContentHash(t *testing.T) {
	h1 := contentHash("hello")
	h2 := contentHash("hello")
	h3 := contentHash("world")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}
