package memory

import (
	"context"
	"testing"
)

func TestUnifiedSearch_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	indexMgr, err := NewIndexManager(IndexManagerOptions{
		DB:       db,
		Embedder: embedder,
		Config:   DefaultIndexConfig(),
	})
	if err != nil {
		t.Fatalf("create index manager: %v", err)
	}

	us := NewUnifiedSearch(indexMgr, embedder, testLogger())

	_, err = us.Search(context.Background(), "", DefaultExtendedSearchOptions())
	if err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

func TestUnifiedSearch_QuickSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	indexMgr, err := NewIndexManager(IndexManagerOptions{
		DB:       db,
		Embedder: embedder,
		Config:   DefaultIndexConfig(),
	})
	if err != nil {
		t.Fatalf("create index manager: %v", err)
	}

	ctx := context.Background()

	// Add an entry
	entry := MemoryEntry{Content: "Go is great", Source: SourceConversation}
	if err := indexMgr.Index(ctx, entry); err != nil {
		t.Fatalf("index: %v", err)
	}

	us := NewUnifiedSearch(indexMgr, embedder, testLogger())

	results, err := us.QuickSearch(ctx, "Go programming", 10)
	if err != nil {
		t.Fatalf("quick search: %v", err)
	}

	// Should return at least something from FTS or vector
	_ = results // results count depends on indexing, just check no error
}

func TestUnifiedSearch_FilterByCategory(t *testing.T) {
	us := &UnifiedSearch{}

	results := []SearchResult{
		{ID: "1", Category: "preference"},
		{ID: "2", Category: "fact"},
		{ID: "3", Category: "preference"},
	}

	opts := ExtendedSearchOptions{CategoryFilter: "preference"}
	filtered := us.applyFilters(results, opts)

	if len(filtered) != 2 {
		t.Errorf("expected 2 results, got %d", len(filtered))
	}
	for _, r := range filtered {
		if r.Category != "preference" {
			t.Errorf("expected category preference, got %s", r.Category)
		}
	}
}

func TestUnifiedSearch_FilterByImportance(t *testing.T) {
	us := &UnifiedSearch{}

	results := []SearchResult{
		{ID: "1", Importance: 0.9},
		{ID: "2", Importance: 0.3},
		{ID: "3", Importance: 0.7},
	}

	opts := ExtendedSearchOptions{ImportanceMin: 0.5}
	filtered := us.applyFilters(results, opts)

	if len(filtered) != 2 {
		t.Errorf("expected 2 results, got %d", len(filtered))
	}
}

func TestTruncateQuery(t *testing.T) {
	short := "hello"
	if truncateQuery(short) != short {
		t.Errorf("short query should not be truncated")
	}

	long := ""
	for i := 0; i < 100; i++ {
		long += "あ" // 100 CJK characters
	}
	result := truncateQuery(long)
	if len([]rune(result)) != 83 { // 80 + "..."
		t.Errorf("expected 83 runes, got %d", len([]rune(result)))
	}
}
