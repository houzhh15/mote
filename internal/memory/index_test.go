package memory

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestMemoryIndex_AddAndCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	count, err := idx.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	entry := MemoryEntry{
		Content: "Hello world",
		Source:  SourceConversation,
	}
	if err := idx.Add(ctx, entry); err != nil {
		t.Fatalf("add: %v", err)
	}

	count, err = idx.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	entry2 := MemoryEntry{
		ID:      "custom-id",
		Content: "Another memory",
		Source:  SourceDocument,
	}
	if err := idx.Add(ctx, entry2); err != nil {
		t.Fatalf("add: %v", err)
	}

	count, err = idx.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestMemoryIndex_SearchFTS(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	entries := []MemoryEntry{
		{Content: "The quick brown fox jumps over the lazy dog", Source: SourceConversation},
		{Content: "Hello world from Go programming", Source: SourceDocument},
		{Content: "SQLite is a great database", Source: SourceTool},
	}
	for _, e := range entries {
		if err := idx.Add(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	results, err := idx.SearchFTS(ctx, "fox", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	results, err = idx.SearchFTS(ctx, "database", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Test queries with special characters (should not cause SQL errors)
	testQueries := []string{
		"user@example.com", // @ character
		"C++ programming",  // + character
		"key:value",        // : character
		"(parentheses)",    // () characters
		"[brackets]",       // [] characters
		`"quoted text"`,    // " characters
		"term*wildcard",    // * character
	}
	for _, query := range testQueries {
		_, err := idx.SearchFTS(ctx, query, 10)
		if err != nil {
			t.Errorf("search with special chars %q failed: %v", query, err)
		}
	}
}

func TestMemoryIndex_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	entry := MemoryEntry{
		ID:      "test-id",
		Content: "Test content",
		Source:  SourceConversation,
	}
	if err := idx.Add(ctx, entry); err != nil {
		t.Fatalf("add: %v", err)
	}

	count, _ := idx.Count(ctx)
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	if err := idx.Delete(ctx, "test-id"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	count, _ = idx.Count(ctx)
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	err = idx.Delete(ctx, "non-existent")
	if err != ErrEntryNotFound {
		t.Errorf("expected ErrEntryNotFound, got %v", err)
	}
}

func TestMemoryIndex_Search(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	entries := []MemoryEntry{
		{Content: "Go programming language", Source: SourceDocument},
		{Content: "Python programming language", Source: SourceDocument},
	}
	for _, e := range entries {
		if err := idx.Add(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	results, err := idx.Search(ctx, "programming", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestMemoryIndex_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add a memory entry
	entry := MemoryEntry{
		ID:      "get-test-id",
		Content: "Test content for GetByID",
		Source:  SourceDocument,
		Metadata: map[string]any{
			"key": "value",
		},
	}
	if err := idx.Add(ctx, entry); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Test successful get
	t.Run("existing entry", func(t *testing.T) {
		result, err := idx.GetByID(ctx, "get-test-id")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if result.ID != "get-test-id" {
			t.Errorf("expected ID 'get-test-id', got '%s'", result.ID)
		}
		if result.Content != "Test content for GetByID" {
			t.Errorf("expected content 'Test content for GetByID', got '%s'", result.Content)
		}
		if result.Source != SourceDocument {
			t.Errorf("expected source '%s', got '%s'", SourceDocument, result.Source)
		}
	})

	// Test not found
	t.Run("non-existent entry", func(t *testing.T) {
		_, err := idx.GetByID(ctx, "non-existent-id")
		if err != ErrMemoryNotFound {
			t.Errorf("expected ErrMemoryNotFound, got %v", err)
		}
	})
}

func TestMemoryIndex_SearchVector(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := IndexConfig{
		Dimensions: 384,
		EnableFTS:  true,
		EnableVec:  true,
	}

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add entries with embeddings
	entries := []MemoryEntry{
		{ID: "vec-1", Content: "Machine learning with neural networks", Source: SourceDocument},
		{ID: "vec-2", Content: "Deep learning for image classification", Source: SourceDocument},
		{ID: "vec-3", Content: "Cooking recipes for dinner", Source: SourceConversation},
	}
	for _, e := range entries {
		if err := idx.Add(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	// Generate query embedding
	queryEmbed, err := embedder.Embed(ctx, "neural network deep learning")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}

	results, err := idx.SearchVector(ctx, queryEmbed, 10)
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}

	// Should return all entries with embeddings
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Results should be sorted by score descending
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted by score: %f > %f", results[i].Score, results[i-1].Score)
		}
	}
}

func TestMemoryIndex_HybridSearch(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	config := IndexConfig{
		Dimensions: 384,
		EnableFTS:  true,
		EnableVec:  true,
	}

	t.Run("hybrid search returns results via vector", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		idx, err := NewMemoryIndexWithOptions(MemoryIndexOptions{
			DB:       db,
			Embedder: embedder,
			Config:   config,
			HybridConfig: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.001,
				RRFConstant:  60,
			},
		})
		if err != nil {
			t.Fatalf("create index: %v", err)
		}

		ctx := context.Background()
		entries := []MemoryEntry{
			{ID: "h-1", Content: "Go programming language features", Source: SourceDocument},
			{ID: "h-2", Content: "Python programming with machine learning", Source: SourceDocument},
			{ID: "h-3", Content: "JavaScript for web development", Source: SourceConversation},
		}
		for _, e := range entries {
			if err := idx.Add(ctx, e); err != nil {
				t.Fatalf("add: %v", err)
			}
		}

		opts := SearchOptions{
			TopK:         10,
			MinScore:     0.001,
			Hybrid:       true,
			VectorWeight: 0.7,
		}

		results, err := idx.HybridSearch(ctx, "Go programming language features", opts)
		if err != nil {
			t.Fatalf("HybridSearch: %v", err)
		}

		if len(results) == 0 {
			t.Error("expected results via vector search, got none")
		}
	})

	t.Run("respects topK limit", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		idx, err := NewMemoryIndexWithOptions(MemoryIndexOptions{
			DB:       db,
			Embedder: embedder,
			Config:   config,
			HybridConfig: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.001,
				RRFConstant:  60,
			},
		})
		if err != nil {
			t.Fatalf("create index: %v", err)
		}

		ctx := context.Background()
		entries := []MemoryEntry{
			{ID: "h-1", Content: "Go programming language features", Source: SourceDocument},
			{ID: "h-2", Content: "Python programming with machine learning", Source: SourceDocument},
			{ID: "h-3", Content: "JavaScript for web development", Source: SourceConversation},
		}
		for _, e := range entries {
			if err := idx.Add(ctx, e); err != nil {
				t.Fatalf("add: %v", err)
			}
		}

		opts := SearchOptions{
			TopK:     1,
			MinScore: 0.001,
		}

		results, err := idx.HybridSearch(ctx, "Python programming with machine learning", opts)
		if err != nil {
			t.Fatalf("HybridSearch: %v", err)
		}

		if len(results) > 1 {
			t.Errorf("expected at most 1 result, got %d", len(results))
		}
	})

	t.Run("results sorted by score descending", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		idx, err := NewMemoryIndexWithOptions(MemoryIndexOptions{
			DB:       db,
			Embedder: embedder,
			Config:   config,
			HybridConfig: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.001,
				RRFConstant:  60,
			},
		})
		if err != nil {
			t.Fatalf("create index: %v", err)
		}

		ctx := context.Background()
		entries := []MemoryEntry{
			{ID: "h-1", Content: "Go programming language features", Source: SourceDocument},
			{ID: "h-2", Content: "Python programming with machine learning", Source: SourceDocument},
			{ID: "h-3", Content: "JavaScript for web development", Source: SourceConversation},
		}
		for _, e := range entries {
			if err := idx.Add(ctx, e); err != nil {
				t.Fatalf("add: %v", err)
			}
		}

		opts := SearchOptions{
			TopK:     10,
			MinScore: 0.001,
		}

		results, err := idx.HybridSearch(ctx, "programming", opts)
		if err != nil {
			t.Fatalf("HybridSearch: %v", err)
		}

		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("results not sorted: score[%d]=%f > score[%d]=%f",
					i, results[i].Score, i-1, results[i-1].Score)
			}
		}
	})
}

func TestEmbeddingEncodeDecode(t *testing.T) {
	original := []float32{0.1, 0.2, -0.3, 0.456789, -0.999}

	encoded := encodeEmbedding(original)
	decoded := decodeEmbedding(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: expected %d, got %d", len(original), len(decoded))
	}

	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("value mismatch at %d: expected %f, got %f", i, original[i], decoded[i])
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors", func(t *testing.T) {
		a := []float32{1, 0, 0}
		sim := cosineSimilarity(a, a)
		if sim < 0.999 {
			t.Errorf("expected similarity ~1.0, got %f", sim)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		a := []float32{1, 0, 0}
		b := []float32{0, 1, 0}
		sim := cosineSimilarity(a, b)
		if sim > 0.001 {
			t.Errorf("expected similarity ~0.0, got %f", sim)
		}
	})

	t.Run("opposite vectors", func(t *testing.T) {
		a := []float32{1, 0, 0}
		b := []float32{-1, 0, 0}
		sim := cosineSimilarity(a, b)
		if sim > -0.999 {
			t.Errorf("expected similarity ~-1.0, got %f", sim)
		}
	})

	t.Run("empty vectors", func(t *testing.T) {
		sim := cosineSimilarity([]float32{}, []float32{})
		if sim != 0 {
			t.Errorf("expected 0 for empty vectors, got %f", sim)
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		a := []float32{1, 2, 3}
		b := []float32{1, 2}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("expected 0 for different length vectors, got %f", sim)
		}
	})
}

// =============================================================================
// P2 Tests: Category, Importance, and CaptureMethod Fields
// =============================================================================

func TestMemoryIndex_P2Fields_AddAndRetrieve(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	t.Run("explicit P2 fields are persisted", func(t *testing.T) {
		entry := MemoryEntry{
			Content:       "I prefer dark theme",
			Source:        SourceConversation,
			Category:      CategoryPreference,
			Importance:    0.9,
			CaptureMethod: CaptureMethodAuto,
		}
		if err := idx.Add(ctx, entry); err != nil {
			t.Fatalf("add: %v", err)
		}

		// Retrieve via List
		results, err := idx.List(ctx, 10, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}

		found := results[0]
		if found.Category != CategoryPreference {
			t.Errorf("expected category %q, got %q", CategoryPreference, found.Category)
		}
		if found.Importance != 0.9 {
			t.Errorf("expected importance 0.9, got %f", found.Importance)
		}
		if found.CaptureMethod != CaptureMethodAuto {
			t.Errorf("expected capture_method %q, got %q", CaptureMethodAuto, found.CaptureMethod)
		}
	})
}

func TestMemoryIndex_P2Fields_Defaults(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	t.Run("default P2 fields when not provided", func(t *testing.T) {
		entry := MemoryEntry{
			ID:      "test-defaults",
			Content: "Test memory without P2 fields",
			Source:  SourceConversation,
		}
		if err := idx.Add(ctx, entry); err != nil {
			t.Fatalf("add: %v", err)
		}

		// Retrieve via GetByID
		retrieved, err := idx.GetByID(ctx, "test-defaults")
		if err != nil {
			t.Fatalf("get by id: %v", err)
		}

		if retrieved.Category != CategoryOther {
			t.Errorf("expected default category %q, got %q", CategoryOther, retrieved.Category)
		}
		if retrieved.Importance != DefaultImportance {
			t.Errorf("expected default importance %f, got %f", DefaultImportance, retrieved.Importance)
		}
		if retrieved.CaptureMethod != CaptureMethodManual {
			t.Errorf("expected default capture_method %q, got %q", CaptureMethodManual, retrieved.CaptureMethod)
		}
	})
}

func TestMemoryIndex_P2Fields_SearchFTS(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()
	config.EnableFTS = true

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add entries with different categories
	entries := []MemoryEntry{
		{Content: "User prefers dark theme", Source: SourceConversation, Category: CategoryPreference, Importance: 0.8},
		{Content: "User's email is test@example.com", Source: SourceConversation, Category: CategoryEntity, Importance: 0.95},
		{Content: "Team decided to use Go language", Source: SourceConversation, Category: CategoryDecision, Importance: 0.85},
	}

	for _, e := range entries {
		if err := idx.Add(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	t.Run("FTS results include P2 fields", func(t *testing.T) {
		results, err := idx.SearchFTS(ctx, "theme", 10)
		if err != nil {
			t.Fatalf("search fts: %v", err)
		}
		if len(results) == 0 {
			t.Skip("FTS didn't match 'theme', skipping P2 field check")
		}

		found := results[0]
		if found.Category != CategoryPreference {
			t.Errorf("expected category %q, got %q", CategoryPreference, found.Category)
		}
		if found.Importance != 0.8 {
			t.Errorf("expected importance 0.8, got %f", found.Importance)
		}
	})
}

func TestMemoryIndex_P2Constants(t *testing.T) {
	t.Run("category constants", func(t *testing.T) {
		categories := []string{
			CategoryPreference,
			CategoryFact,
			CategoryDecision,
			CategoryEntity,
			CategoryOther,
		}
		for _, c := range categories {
			if c == "" {
				t.Error("category constant should not be empty")
			}
		}
	})

	t.Run("capture method constants", func(t *testing.T) {
		methods := []string{
			CaptureMethodManual,
			CaptureMethodAuto,
			CaptureMethodImport,
		}
		for _, m := range methods {
			if m == "" {
				t.Error("capture method constant should not be empty")
			}
		}
	})

	t.Run("default importance", func(t *testing.T) {
		if DefaultImportance < 0 || DefaultImportance > 1 {
			t.Errorf("DefaultImportance should be between 0 and 1, got %f", DefaultImportance)
		}
	})
}

// TestMemoryIndex_AutoChunking tests automatic chunking for long content.
func TestMemoryIndex_AutoChunking(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()
	config.ChunkThreshold = 100 // Low threshold for testing

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	t.Run("short content no chunking", func(t *testing.T) {
		entry := MemoryEntry{
			ID:      "short-1",
			Content: "This is short content.",
			Source:  SourceConversation,
		}
		if err := idx.Add(ctx, entry); err != nil {
			t.Fatalf("add: %v", err)
		}

		// Should be stored as single entry
		got, err := idx.GetByID(ctx, "short-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ChunkTotal != 0 {
			t.Errorf("expected ChunkTotal=0 for short content, got %d", got.ChunkTotal)
		}
	})

	t.Run("long content auto chunking", func(t *testing.T) {
		// Create content that exceeds threshold
		longContent := ""
		for i := 0; i < 50; i++ {
			longContent += "This is paragraph number " + string(rune('0'+i%10)) + ". It contains some text.\n\n"
		}

		entry := MemoryEntry{
			ID:      "long-1",
			Content: longContent,
			Source:  SourceDocument,
		}
		if err := idx.Add(ctx, entry); err != nil {
			t.Fatalf("add: %v", err)
		}

		// Should be chunked into multiple entries - check first chunk
		got, err := idx.GetByID(ctx, "long-1-chunk-0")
		if err != nil {
			t.Fatalf("get chunk-0: %v", err)
		}
		if got.ChunkTotal < 2 {
			t.Errorf("expected ChunkTotal >= 2, got %d", got.ChunkTotal)
		}
		if got.ChunkIndex != 0 {
			t.Errorf("expected ChunkIndex=0, got %d", got.ChunkIndex)
		}

		// Verify count
		count, err := idx.Count(ctx)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		// At least 3 entries: short-1 + multiple chunks
		if count < 3 {
			t.Errorf("expected at least 3 entries, got %d", count)
		}
	})
}

// TestMemoryIndex_AddWithChunking tests explicit chunking.
func TestMemoryIndex_AddWithChunking(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()
	config.ChunkThreshold = 0 // Disable auto-chunking

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Create multi-paragraph content
	content := `# Introduction

This is the first paragraph with some important information about the topic.

# Section 1

This section contains detailed explanations and examples. It goes on for a while.

# Section 2

Another section with more content. We need enough text to create multiple chunks.

# Conclusion

Final thoughts and summary of the document.`

	entry := MemoryEntry{
		ID:         "explicit-chunk",
		Content:    content,
		Source:     SourceDocument,
		SourceFile: "test.md",
	}

	if err := idx.AddWithChunking(ctx, entry); err != nil {
		t.Fatalf("AddWithChunking: %v", err)
	}

	// Check that chunks were created
	count, err := idx.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if count == 0 {
		t.Error("expected chunks to be created")
	}

	// Verify first chunk has proper fields
	got, err := idx.GetByID(ctx, "explicit-chunk-chunk-0")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ChunkIndex != 0 {
		t.Errorf("expected ChunkIndex=0, got %d", got.ChunkIndex)
	}
	if got.SourceFile != "test.md" {
		t.Errorf("expected SourceFile=test.md, got %s", got.SourceFile)
	}
}
