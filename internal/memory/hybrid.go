package memory

import (
	"context"
	"sort"
	"sync"
)

// HybridConfig holds configuration for hybrid search.
type HybridConfig struct {
	VectorWeight float64 // Weight for vector search results (default 0.7)
	TextWeight   float64 // Weight for text/FTS search results (default 0.3)
	BM25Weight   float64 // Weight for BM25 search results (default 0.0, set >0 to enable)
	MinScore     float64 // Minimum score threshold (default 0.35)
	RRFConstant  int     // RRF constant k (default 60)
}

// DefaultHybridConfig returns a HybridConfig with default values.
// Note: MinScore is set to filter out low-relevance results from RRF.
// With k=60, typical scores range from 0.001 to 0.016.
// Setting MinScore to 0.005 keeps only more relevant results.
func DefaultHybridConfig() HybridConfig {
	return HybridConfig{
		VectorWeight: 0.7,
		TextWeight:   0.3,
		MinScore:     0.005, // Higher threshold to improve precision
		RRFConstant:  60,
	}
}

// VectorSearcher defines the interface for vector-based search.
type VectorSearcher interface {
	SearchVector(ctx context.Context, embedding []float32, topK int) ([]ScoredResult, error)
}

// FTSSearcher defines the interface for full-text search.
type FTSSearcher interface {
	SearchFTS(ctx context.Context, query string, topK int) ([]ScoredResult, error)
}

// HybridSearcher performs hybrid search combining vector and text search results.
type HybridSearcher struct {
	vectorSearcher VectorSearcher
	ftsSearcher    FTSSearcher
	bm25Searcher   BM25Searcher // Optional BM25 searcher (nil = disabled)
	embedder       Embedder
	config         HybridConfig
}

// HybridSearcherOptions holds options for creating a HybridSearcher.
type HybridSearcherOptions struct {
	VectorSearcher VectorSearcher
	FTSSearcher    FTSSearcher
	BM25Searcher   BM25Searcher // Optional
	Embedder       Embedder
	Config         HybridConfig
}

// NewHybridSearcher creates a new HybridSearcher with the given options.
func NewHybridSearcher(opts HybridSearcherOptions) *HybridSearcher {
	config := opts.Config
	if config.VectorWeight == 0 && config.TextWeight == 0 {
		config = DefaultHybridConfig()
	}
	if config.RRFConstant == 0 {
		config.RRFConstant = 60
	}

	return &HybridSearcher{
		vectorSearcher: opts.VectorSearcher,
		ftsSearcher:    opts.FTSSearcher,
		bm25Searcher:   opts.BM25Searcher,
		embedder:       opts.Embedder,
		config:         config,
	}
}

// Search performs hybrid search using both vector and text search.
// It combines results using Reciprocal Rank Fusion (RRF).
func (h *HybridSearcher) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Get query embedding
	embedding, err := h.embedder.Embed(ctx, query)
	if err != nil {
		// Fallback to FTS-only search if embedding fails
		ftsResults, ftsErr := h.ftsSearcher.SearchFTS(ctx, query, topK)
		if ftsErr != nil {
			return nil, ftsErr
		}
		return h.convertToSearchResults(ftsResults), nil
	}

	return h.SearchWithEmbedding(ctx, query, embedding, topK)
}

// SearchWithEmbedding performs hybrid search with a pre-computed embedding.
func (h *HybridSearcher) SearchWithEmbedding(ctx context.Context, query string, embedding []float32, topK int) ([]SearchResult, error) {
	// Fetch more results than needed for better fusion
	fetchK := topK * 4
	if fetchK < 40 {
		fetchK = 40
	}

	var vecResults, ftsResults []ScoredResult
	var vecErr, ftsErr error
	var bm25Results []ScoredResult
	var bm25Err error
	var wg sync.WaitGroup

	// Determine how many parallel searches to run
	hasBM25 := h.bm25Searcher != nil && h.config.BM25Weight > 0
	searchCount := 2
	if hasBM25 {
		searchCount = 3
	}

	// Run searches in parallel
	wg.Add(searchCount)

	go func() {
		defer wg.Done()
		vecResults, vecErr = h.vectorSearcher.SearchVector(ctx, embedding, fetchK)
	}()

	go func() {
		defer wg.Done()
		ftsResults, ftsErr = h.ftsSearcher.SearchFTS(ctx, query, fetchK)
	}()

	if hasBM25 {
		go func() {
			defer wg.Done()
			bm25Results, bm25Err = h.bm25Searcher.SearchBM25(ctx, query, fetchK)
		}()
	}

	wg.Wait()

	// Handle errors - prefer partial results over complete failure
	allFailed := vecErr != nil && ftsErr != nil && (!hasBM25 || bm25Err != nil)
	if allFailed {
		return nil, vecErr
	}
	if vecErr != nil {
		vecResults = nil
	}
	if ftsErr != nil {
		ftsResults = nil
	}
	if bm25Err != nil {
		bm25Results = nil
	}

	// If BM25 is enabled, use three-way merge
	if hasBM25 {
		return h.mergeThreeWayResults(vecResults, ftsResults, bm25Results, topK), nil
	}

	// Legacy two-way merge
	if vecResults == nil {
		return h.convertToSearchResults(ftsResults[:min(len(ftsResults), topK)]), nil
	}
	if ftsResults == nil {
		return h.convertToSearchResults(vecResults[:min(len(vecResults), topK)]), nil
	}

	// Merge results using RRF
	return h.mergeResults(vecResults, ftsResults, topK), nil
}

// mergeResults combines vector and FTS results using Reciprocal Rank Fusion.
func (h *HybridSearcher) mergeResults(vecResults, ftsResults []ScoredResult, topK int) []SearchResult {
	rrfScores := make(map[string]float64)
	idToResult := make(map[string]ScoredResult)

	// Calculate RRF scores for vector search results
	for rank, r := range vecResults {
		rrfScore := 1.0 / float64(h.config.RRFConstant+rank+1)
		rrfScores[r.ID] += h.config.VectorWeight * rrfScore
		idToResult[r.ID] = r
	}

	// Calculate RRF scores for FTS results
	for rank, r := range ftsResults {
		rrfScore := 1.0 / float64(h.config.RRFConstant+rank+1)
		rrfScores[r.ID] += h.config.TextWeight * rrfScore
		if _, exists := idToResult[r.ID]; !exists {
			idToResult[r.ID] = r
		}
	}

	// Build result list and filter by minimum score
	var results []SearchResult
	for id, score := range rrfScores {
		if score < h.config.MinScore {
			continue
		}
		r := idToResult[id]
		results = append(results, SearchResult{
			ID:      id,
			Content: r.Content,
			Score:   score,
			Source:  r.Source,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to topK
	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// convertToSearchResults converts ScoredResult slice to SearchResult slice.
func (h *HybridSearcher) convertToSearchResults(scored []ScoredResult) []SearchResult {
	results := make([]SearchResult, len(scored))
	for i, r := range scored {
		results[i] = SearchResult{
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
			Source:  r.Source,
		}
	}
	return results
}

// mergeThreeWayResults combines vector, FTS and BM25 results using three-way RRF.
func (h *HybridSearcher) mergeThreeWayResults(vecResults, ftsResults, bm25Results []ScoredResult, topK int) []SearchResult {
	rrfScores := make(map[string]float64)
	idToResult := make(map[string]ScoredResult)

	// Normalize weights
	totalWeight := h.config.VectorWeight + h.config.TextWeight + h.config.BM25Weight
	vecW := h.config.VectorWeight / totalWeight
	ftsW := h.config.TextWeight / totalWeight
	bm25W := h.config.BM25Weight / totalWeight

	// Vector results
	for rank, r := range vecResults {
		rrfScore := vecW / float64(h.config.RRFConstant+rank+1)
		rrfScores[r.ID] += rrfScore
		if _, exists := idToResult[r.ID]; !exists {
			idToResult[r.ID] = r
		}
	}

	// FTS results
	for rank, r := range ftsResults {
		rrfScore := ftsW / float64(h.config.RRFConstant+rank+1)
		rrfScores[r.ID] += rrfScore
		if _, exists := idToResult[r.ID]; !exists {
			idToResult[r.ID] = r
		}
	}

	// BM25 results
	for rank, r := range bm25Results {
		rrfScore := bm25W / float64(h.config.RRFConstant+rank+1)
		rrfScores[r.ID] += rrfScore
		if _, exists := idToResult[r.ID]; !exists {
			idToResult[r.ID] = r
		}
	}

	// Build result list with minimum score filtering
	var results []SearchResult
	for id, score := range rrfScores {
		if score < h.config.MinScore {
			continue
		}
		r := idToResult[id]
		results = append(results, SearchResult{
			ID:      id,
			Content: r.Content,
			Score:   score,
			Source:  r.Source,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// Config returns the current hybrid search configuration.
func (h *HybridSearcher) Config() HybridConfig {
	return h.config
}
