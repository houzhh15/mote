package memory

import (
	"context"
	"math"
	"testing"
)

// mockVectorSearcher implements VectorSearcher for testing.
type mockVectorSearcher struct {
	results []ScoredResult
	err     error
}

func (m *mockVectorSearcher) SearchVector(ctx context.Context, embedding []float32, topK int) ([]ScoredResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.results) > topK {
		return m.results[:topK], nil
	}
	return m.results, nil
}

// mockFTSSearcher implements FTSSearcher for testing.
type mockFTSSearcher struct {
	results []ScoredResult
	err     error
}

func (m *mockFTSSearcher) SearchFTS(ctx context.Context, query string, topK int) ([]ScoredResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.results) > topK {
		return m.results[:topK], nil
	}
	return m.results, nil
}

// mockEmbedder implements Embedder for testing.
type mockEmbedder struct {
	embedding []float32
	err       error
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embedding, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		results[i] = m.embedding
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return len(m.embedding)
}

func TestDefaultHybridConfig(t *testing.T) {
	config := DefaultHybridConfig()

	if config.VectorWeight != 0.7 {
		t.Errorf("expected VectorWeight 0.7, got %f", config.VectorWeight)
	}
	if config.TextWeight != 0.3 {
		t.Errorf("expected TextWeight 0.3, got %f", config.TextWeight)
	}
	// MinScore should filter low-relevance results; DefaultHybridConfig uses 0.005
	if config.MinScore != 0.005 {
		t.Errorf("expected MinScore 0.005, got %f", config.MinScore)
	}
	if config.RRFConstant != 60 {
		t.Errorf("expected RRFConstant 60, got %d", config.RRFConstant)
	}
}

func TestHybridSearcher_MergeResults(t *testing.T) {
	t.Run("basic RRF fusion", func(t *testing.T) {
		h := NewHybridSearcher(HybridSearcherOptions{
			Config: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.0, // No min score filter for this test
				RRFConstant:  60,
			},
		})

		// Same ID appears in both results - should have combined score
		vecResults := []ScoredResult{
			{ID: "a", Content: "doc a", Score: 0.9},
			{ID: "b", Content: "doc b", Score: 0.8},
		}
		ftsResults := []ScoredResult{
			{ID: "a", Content: "doc a", Score: 0.85},
			{ID: "c", Content: "doc c", Score: 0.7},
		}

		results := h.mergeResults(vecResults, ftsResults, 10)

		// "a" should be first (appears in both)
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if results[0].ID != "a" {
			t.Errorf("expected ID 'a' first, got '%s'", results[0].ID)
		}

		// Calculate expected RRF score for "a"
		// Vector: 0.7 * 1/(60+0+1) = 0.7/61 ≈ 0.01148
		// FTS:    0.3 * 1/(60+0+1) = 0.3/61 ≈ 0.00492
		// Total:  ≈ 0.0164
		expectedScore := 0.7/61.0 + 0.3/61.0
		if math.Abs(results[0].Score-expectedScore) > 0.0001 {
			t.Errorf("expected score %f, got %f", expectedScore, results[0].Score)
		}
	})

	t.Run("respects MinScore threshold", func(t *testing.T) {
		h := NewHybridSearcher(HybridSearcherOptions{
			Config: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.02, // High threshold
				RRFConstant:  60,
			},
		})

		// Single result with low RRF score
		vecResults := []ScoredResult{
			{ID: "low", Content: "low score doc", Score: 0.5},
		}
		ftsResults := []ScoredResult{}

		results := h.mergeResults(vecResults, ftsResults, 10)

		// RRF score for rank 0: 0.7 * 1/61 ≈ 0.0115 < 0.02
		if len(results) != 0 {
			t.Errorf("expected 0 results due to MinScore filter, got %d", len(results))
		}
	})

	t.Run("respects topK limit", func(t *testing.T) {
		h := NewHybridSearcher(HybridSearcherOptions{
			Config: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.0,
				RRFConstant:  60,
			},
		})

		vecResults := []ScoredResult{
			{ID: "1", Content: "doc 1"},
			{ID: "2", Content: "doc 2"},
			{ID: "3", Content: "doc 3"},
			{ID: "4", Content: "doc 4"},
			{ID: "5", Content: "doc 5"},
		}
		ftsResults := []ScoredResult{}

		results := h.mergeResults(vecResults, ftsResults, 3)

		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("orders by score descending", func(t *testing.T) {
		h := NewHybridSearcher(HybridSearcherOptions{
			Config: HybridConfig{
				VectorWeight: 0.5,
				TextWeight:   0.5,
				MinScore:     0.0,
				RRFConstant:  60,
			},
		})

		// "both" appears in both lists at rank 0
		// "vec_only" appears only in vector at rank 1
		// "fts_only" appears only in FTS at rank 1
		vecResults := []ScoredResult{
			{ID: "both", Content: "both"},
			{ID: "vec_only", Content: "vec only"},
		}
		ftsResults := []ScoredResult{
			{ID: "both", Content: "both"},
			{ID: "fts_only", Content: "fts only"},
		}

		results := h.mergeResults(vecResults, ftsResults, 10)

		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}

		// "both" should be first (highest combined score)
		if results[0].ID != "both" {
			t.Errorf("expected 'both' first, got '%s'", results[0].ID)
		}

		// Scores should be descending
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("scores not descending: %f > %f", results[i].Score, results[i-1].Score)
			}
		}
	})
}

func TestHybridSearcher_Search(t *testing.T) {
	t.Run("combines vector and FTS results", func(t *testing.T) {
		vecSearcher := &mockVectorSearcher{
			results: []ScoredResult{
				{ID: "1", Content: "vector result", Score: 0.9},
			},
		}
		ftsSearcher := &mockFTSSearcher{
			results: []ScoredResult{
				{ID: "2", Content: "fts result", Score: 0.8},
			},
		}
		embedder := &mockEmbedder{
			embedding: []float32{0.1, 0.2, 0.3},
		}

		h := NewHybridSearcher(HybridSearcherOptions{
			VectorSearcher: vecSearcher,
			FTSSearcher:    ftsSearcher,
			Embedder:       embedder,
			Config: HybridConfig{
				VectorWeight: 0.7,
				TextWeight:   0.3,
				MinScore:     0.0,
				RRFConstant:  60,
			},
		})

		results, err := h.Search(context.Background(), "test query", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("falls back to FTS when embedding fails", func(t *testing.T) {
		vecSearcher := &mockVectorSearcher{
			results: []ScoredResult{
				{ID: "1", Content: "vector result", Score: 0.9},
			},
		}
		ftsSearcher := &mockFTSSearcher{
			results: []ScoredResult{
				{ID: "2", Content: "fts result", Score: 0.8},
				{ID: "3", Content: "fts result 2", Score: 0.7},
			},
		}
		embedder := &mockEmbedder{
			err: ErrEmbeddingFailed,
		}

		h := NewHybridSearcher(HybridSearcherOptions{
			VectorSearcher: vecSearcher,
			FTSSearcher:    ftsSearcher,
			Embedder:       embedder,
		})

		results, err := h.Search(context.Background(), "test query", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Should only have FTS results
		if len(results) != 2 {
			t.Errorf("expected 2 FTS results, got %d", len(results))
		}
		if results[0].ID != "2" {
			t.Errorf("expected ID '2', got '%s'", results[0].ID)
		}
	})

	t.Run("handles vector search failure gracefully", func(t *testing.T) {
		vecSearcher := &mockVectorSearcher{
			err: ErrIndexCorrupted,
		}
		ftsSearcher := &mockFTSSearcher{
			results: []ScoredResult{
				{ID: "fts1", Content: "fts result", Score: 0.8},
			},
		}
		embedder := &mockEmbedder{
			embedding: []float32{0.1, 0.2},
		}

		h := NewHybridSearcher(HybridSearcherOptions{
			VectorSearcher: vecSearcher,
			FTSSearcher:    ftsSearcher,
			Embedder:       embedder,
		})

		results, err := h.Search(context.Background(), "test", 10)
		if err != nil {
			t.Fatalf("expected graceful degradation, got error: %v", err)
		}

		// Should fall back to FTS results
		if len(results) != 1 {
			t.Errorf("expected 1 FTS result, got %d", len(results))
		}
	})
}

func TestHybridSearcher_Config(t *testing.T) {
	config := HybridConfig{
		VectorWeight: 0.8,
		TextWeight:   0.2,
		MinScore:     0.5,
		RRFConstant:  100,
	}

	h := NewHybridSearcher(HybridSearcherOptions{
		Config: config,
	})

	got := h.Config()
	if got.VectorWeight != 0.8 {
		t.Errorf("expected VectorWeight 0.8, got %f", got.VectorWeight)
	}
	if got.TextWeight != 0.2 {
		t.Errorf("expected TextWeight 0.2, got %f", got.TextWeight)
	}
	if got.MinScore != 0.5 {
		t.Errorf("expected MinScore 0.5, got %f", got.MinScore)
	}
	if got.RRFConstant != 100 {
		t.Errorf("expected RRFConstant 100, got %d", got.RRFConstant)
	}
}
