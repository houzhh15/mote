package memory

import (
	"testing"
)

func TestBM25Scorer_Score(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Setup index with some entries
	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()
	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := t.Context()

	// Add entries
	entries := []MemoryEntry{
		{Content: "Go is a compiled programming language", Source: SourceConversation},
		{Content: "Python is an interpreted language", Source: SourceConversation},
		{Content: "Rust programming language is fast", Source: SourceConversation},
	}
	for _, e := range entries {
		if err := idx.Add(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	bm25 := NewBM25Scorer(db, DefaultBM25Config())

	// Test scoring
	results, err := bm25.Score("programming language", 10)
	if err != nil {
		t.Fatalf("score: %v", err)
	}

	// Should return results for entries containing "programming" and "language"
	if len(results) == 0 {
		t.Error("expected results, got 0")
	}

	// All scores should be >= 0
	for _, r := range results {
		if r.Score < 0 {
			t.Errorf("negative score: %f for %s", r.Score, r.ID)
		}
	}
}

func TestBM25Scorer_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	bm25 := NewBM25Scorer(db, DefaultBM25Config())

	results, err := bm25.Score("", 10)
	if err != nil {
		t.Fatalf("score: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestBM25_Tokenize(t *testing.T) {
	tests := []struct {
		input    string
		minWords int
	}{
		{"hello world", 2},
		{"Go programming", 2},
		{"测试中文分词", 1},       // CJK block treated as single token
		{"混合English和中文", 3}, // CJK/ASCII segments split
		{"", 0},
	}

	for _, tt := range tests {
		tokens := tokenize(tt.input)
		if len(tokens) < tt.minWords {
			t.Errorf("tokenize(%q): got %d tokens, want at least %d", tt.input, len(tokens), tt.minWords)
		}
	}
}

func TestBM25_IsCJKChar(t *testing.T) {
	tests := []struct {
		ch   rune
		want bool
	}{
		{'A', false},
		{'z', false},
		{'1', false},
		{'中', true},
		{'日', true},
		{'가', false}, // Korean not in CJK range
	}

	for _, tt := range tests {
		got := isCJKChar(tt.ch)
		if got != tt.want {
			t.Errorf("isCJKChar(%c): got %v, want %v", tt.ch, got, tt.want)
		}
	}
}
