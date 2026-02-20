package memory

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

// UnifiedSearch provides a single search entry point that dispatches
// to the appropriate search strategy based on SearchMode.
type UnifiedSearch struct {
	indexMgr *IndexManager
	embedder Embedder
	logger   zerolog.Logger
}

// NewUnifiedSearch creates a new UnifiedSearch.
func NewUnifiedSearch(indexMgr *IndexManager, embedder Embedder, logger zerolog.Logger) *UnifiedSearch {
	return &UnifiedSearch{
		indexMgr: indexMgr,
		embedder: embedder,
		logger:   logger,
	}
}

// Search performs a search using the given extended options.
// It dispatches to vector, text, hybrid, or auto mode based on options.Mode.
func (us *UnifiedSearch) Search(ctx context.Context, query string, opts ExtendedSearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("unified_search: empty query")
	}

	us.logger.Debug().
		Str("query", truncateQuery(query)).
		Str("mode", string(opts.Mode)).
		Float64("bm25Weight", opts.BM25Weight).
		Int("topK", opts.TopK).
		Msg("unified_search: searching")

	// Generate embedding for the query
	var embedding []float32
	if opts.Mode != SearchModeText {
		var err error
		embedding, err = us.embedder.Embed(ctx, query)
		if err != nil {
			us.logger.Warn().Err(err).Msg("unified_search: embedding failed, falling back to text")
			opts.Mode = SearchModeText
		}
	}

	results, err := us.indexMgr.Search(ctx, query, embedding, opts)
	if err != nil {
		return nil, fmt.Errorf("unified_search: search: %w", err)
	}

	// Apply post-search filters
	results = us.applyFilters(results, opts)

	us.logger.Debug().
		Int("results", len(results)).
		Msg("unified_search: completed")

	return results, nil
}

// QuickSearch performs a search with default settings and auto mode.
func (us *UnifiedSearch) QuickSearch(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	opts := ExtendedSearchOptions{
		SearchOptions: SearchOptions{
			TopK: topK,
		},
		Mode: SearchModeAuto,
	}
	return us.Search(ctx, query, opts)
}

// VectorSearch performs a vector-only search.
func (us *UnifiedSearch) VectorSearch(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	opts := ExtendedSearchOptions{
		SearchOptions: SearchOptions{
			TopK: topK,
		},
		Mode: SearchModeVector,
	}
	return us.Search(ctx, query, opts)
}

// TextSearch performs a text-only search (FTS + BM25).
func (us *UnifiedSearch) TextSearch(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	opts := ExtendedSearchOptions{
		SearchOptions: SearchOptions{
			TopK: topK,
		},
		Mode: SearchModeText,
	}
	return us.Search(ctx, query, opts)
}

// HybridSearch performs a hybrid search with configurable BM25 weight.
func (us *UnifiedSearch) HybridSearch(ctx context.Context, query string, topK int, bm25Weight float64) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	opts := ExtendedSearchOptions{
		SearchOptions: SearchOptions{
			TopK: topK,
		},
		Mode:       SearchModeHybrid,
		BM25Weight: bm25Weight,
	}
	return us.Search(ctx, query, opts)
}

// applyFilters applies post-search filtering based on options.
func (us *UnifiedSearch) applyFilters(results []SearchResult, opts ExtendedSearchOptions) []SearchResult {
	if opts.CategoryFilter == "" && opts.ImportanceMin <= 0 {
		return results
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if opts.CategoryFilter != "" && r.Category != opts.CategoryFilter {
			continue
		}
		if opts.ImportanceMin > 0 && r.Importance < opts.ImportanceMin {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// truncateQuery truncates a query string for logging.
func truncateQuery(q string) string {
	const maxLen = 80
	runes := []rune(q)
	if len(runes) <= maxLen {
		return q
	}
	return string(runes[:maxLen]) + "..."
}
