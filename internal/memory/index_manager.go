package memory

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// IndexManager manages memory indexing and search operations.
// It wraps a MemoryIndex internally and delegates core SQLite operations,
// while adding BM25 search and unified search capabilities.
type IndexManager struct {
	db       *sql.DB
	embedder Embedder
	bm25     *BM25Scorer
	config   IndexConfig
	hybrid   HybridConfig
	legacy   *MemoryIndex // delegate to existing MemoryIndex for backward compat
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// IndexManagerOptions holds options for creating an IndexManager.
type IndexManagerOptions struct {
	DB           *sql.DB
	Embedder     Embedder
	Config       IndexConfig
	HybridConfig HybridConfig
	BM25Config   BM25Config
	Logger       zerolog.Logger
}

// NewIndexManager creates a new IndexManager.
func NewIndexManager(opts IndexManagerOptions) (*IndexManager, error) {
	if opts.DB == nil {
		return nil, fmt.Errorf("index_manager: DB is required")
	}

	// Create underlying MemoryIndex for delegation
	legacy, err := NewMemoryIndexWithOptions(MemoryIndexOptions{
		DB:           opts.DB,
		Embedder:     opts.Embedder,
		Config:       opts.Config,
		HybridConfig: opts.HybridConfig,
		Logger:       opts.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("index_manager: create legacy index: %w", err)
	}

	// Create BM25 scorer
	bm25Config := opts.BM25Config
	if bm25Config.K1 == 0 {
		bm25Config = DefaultBM25Config()
	}
	bm25 := NewBM25Scorer(opts.DB, bm25Config)

	im := &IndexManager{
		db:       opts.DB,
		embedder: opts.Embedder,
		bm25:     bm25,
		config:   opts.Config,
		hybrid:   opts.HybridConfig,
		legacy:   legacy,
		logger:   opts.Logger,
	}

	return im, nil
}

// Index adds a single memory entry to the index.
func (im *IndexManager) Index(ctx context.Context, entry MemoryEntry) error {
	if err := im.legacy.Add(ctx, entry); err != nil {
		return fmt.Errorf("index_manager.Index: %w", err)
	}
	// Update BM25 stats asynchronously
	go func() {
		if err := im.bm25.UpdateStats(); err != nil {
			im.logger.Warn().Err(err).Msg("index_manager: failed to update BM25 stats")
		}
	}()
	return nil
}

// IndexBatch adds multiple memory entries to the index in a batch.
func (im *IndexManager) IndexBatch(ctx context.Context, entries []MemoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Use transaction for batch insert
	tx, err := im.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index_manager.IndexBatch: begin tx: %w", err)
	}
	defer tx.Rollback()

	var errs []error
	for i, entry := range entries {
		if err := im.legacy.Add(ctx, entry); err != nil {
			errs = append(errs, fmt.Errorf("entry %d (%s): %w", i, entry.ID, err))
			im.logger.Warn().Err(err).Str("id", entry.ID).Msg("index_manager: batch index failed for entry")
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("index_manager.IndexBatch: commit: %w", err)
	}

	// Update BM25 stats
	if err := im.bm25.UpdateStats(); err != nil {
		im.logger.Warn().Err(err).Msg("index_manager: failed to update BM25 stats after batch")
	}

	if len(errs) > 0 {
		return fmt.Errorf("index_manager.IndexBatch: %d/%d entries failed: %w", len(errs), len(entries), errs[0])
	}

	im.logger.Info().Int("count", len(entries)).Msg("index_manager: batch indexed")
	return nil
}

// Remove removes a memory entry from the index by ID.
func (im *IndexManager) Remove(ctx context.Context, id string) error {
	if err := im.legacy.Delete(ctx, id); err != nil {
		return fmt.Errorf("index_manager.Remove: %w", err)
	}
	return nil
}

// Search performs a search using the specified options.
// Supports vector, FTS, BM25, and hybrid modes.
func (im *IndexManager) Search(ctx context.Context, query string, embedding []float32, opts ExtendedSearchOptions) ([]SearchResult, error) {
	switch opts.Mode {
	case SearchModeVector:
		return im.searchVectorOnly(ctx, embedding, opts.TopK)
	case SearchModeText:
		return im.searchTextOnly(ctx, query, opts.TopK)
	case SearchModeHybrid:
		return im.searchHybrid(ctx, query, embedding, opts)
	case SearchModeAuto:
		return im.searchAuto(ctx, query, embedding, opts)
	default:
		return im.searchAuto(ctx, query, embedding, opts)
	}
}

// SearchVector performs vector-only search (delegates to legacy).
func (im *IndexManager) SearchVector(ctx context.Context, embedding []float32, topK int) ([]ScoredResult, error) {
	return im.legacy.SearchVector(ctx, embedding, topK)
}

// SearchFTS performs FTS-only search (delegates to legacy).
func (im *IndexManager) SearchFTS(ctx context.Context, query string, topK int) ([]ScoredResult, error) {
	results, err := im.legacy.SearchFTS(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	// Convert SearchResult to ScoredResult
	scored := make([]ScoredResult, len(results))
	for i, r := range results {
		scored[i] = ScoredResult{
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
			Source:  r.Source,
		}
	}
	return scored, nil
}

// SearchBM25 performs BM25 text search.
func (im *IndexManager) SearchBM25(ctx context.Context, query string, topK int) ([]ScoredResult, error) {
	return im.bm25.Score(query, topK)
}

// Count returns the number of indexed entries.
func (im *IndexManager) Count(ctx context.Context) (int, error) {
	return im.legacy.Count(ctx)
}

// List returns entries with pagination.
func (im *IndexManager) List(ctx context.Context, limit, offset int) ([]SearchResult, error) {
	return im.legacy.List(ctx, limit, offset)
}

// GetByID retrieves a memory entry by ID.
func (im *IndexManager) GetByID(ctx context.Context, id string) (*MemoryEntry, error) {
	return im.legacy.GetByID(ctx, id)
}

// GetLegacyIndex returns the underlying MemoryIndex for backward compatibility.
// Deprecated: Use IndexManager methods directly.
func (im *IndexManager) GetLegacyIndex() *MemoryIndex {
	return im.legacy
}

// --- internal search methods ---

func (im *IndexManager) searchVectorOnly(ctx context.Context, embedding []float32, topK int) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("index_manager: embedding is required for vector search")
	}
	scored, err := im.legacy.SearchVector(ctx, embedding, topK)
	if err != nil {
		return nil, err
	}
	return scoredToSearchResults(scored), nil
}

func (im *IndexManager) searchTextOnly(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Combine FTS and BM25
	ftsResults, ftsErr := im.SearchFTS(ctx, query, topK)
	bm25Results, bm25Err := im.SearchBM25(ctx, query, topK)

	if ftsErr != nil && bm25Err != nil {
		return nil, fmt.Errorf("index_manager: both FTS and BM25 failed: fts=%w", ftsErr)
	}

	// Merge with RRF
	merged := mergeWithRRF(ftsResults, bm25Results, 60, 0.5, 0.5)
	if len(merged) > topK {
		merged = merged[:topK]
	}
	return scoredToSearchResults(merged), nil
}

func (im *IndexManager) searchHybrid(ctx context.Context, query string, embedding []float32, opts ExtendedSearchOptions) ([]SearchResult, error) {
	fetchK := opts.TopK * 4
	if fetchK < 40 {
		fetchK = 40
	}

	var vecResults, ftsResults, bm25Results []ScoredResult
	var vecErr, ftsErr, bm25Err error
	var wg sync.WaitGroup

	// Run all three searches in parallel
	if len(embedding) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vecResults, vecErr = im.SearchVector(ctx, embedding, fetchK)
		}()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = im.SearchFTS(ctx, query, fetchK)
	}()
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = im.SearchBM25(ctx, query, fetchK)
	}()

	wg.Wait()

	// Log errors but don't fail if at least one source succeeded
	if vecErr != nil {
		im.logger.Warn().Err(vecErr).Msg("index_manager: vector search failed in hybrid")
	}
	if ftsErr != nil {
		im.logger.Warn().Err(ftsErr).Msg("index_manager: FTS search failed in hybrid")
	}
	if bm25Err != nil {
		im.logger.Warn().Err(bm25Err).Msg("index_manager: BM25 search failed in hybrid")
	}

	if vecErr != nil && ftsErr != nil && bm25Err != nil {
		return nil, fmt.Errorf("index_manager: all search methods failed")
	}

	// Three-way RRF fusion
	merged := mergeThreeWayRRF(vecResults, ftsResults, bm25Results, 60,
		opts.VectorWeight, getFTSWeight(opts), opts.BM25Weight)

	// Apply minimum score filter
	filtered := filterByMinScore(merged, opts.MinScore)

	// Limit to TopK
	if len(filtered) > opts.TopK {
		filtered = filtered[:opts.TopK]
	}

	return scoredToSearchResults(filtered), nil
}

func (im *IndexManager) searchAuto(ctx context.Context, query string, embedding []float32, opts ExtendedSearchOptions) ([]SearchResult, error) {
	queryLen := len([]rune(query))

	// Short queries: favor text search
	if queryLen <= 10 {
		opts.VectorWeight = 0.2
		opts.BM25Weight = 0.5
	} else if queryLen > 50 {
		// Long queries: favor vector search
		opts.VectorWeight = 0.6
		opts.BM25Weight = 0.2
	} else {
		// Default balanced weights
		opts.VectorWeight = 0.4
		opts.BM25Weight = 0.3
	}

	return im.searchHybrid(ctx, query, embedding, opts)
}

// --- utility functions ---

func getFTSWeight(opts ExtendedSearchOptions) float64 {
	// FTS weight is the remainder after vector and BM25
	w := 1.0 - opts.VectorWeight - opts.BM25Weight
	if w < 0 {
		w = 0.1
	}
	return w
}

// mergeThreeWayRRF merges results from three search sources using Reciprocal Rank Fusion.
func mergeThreeWayRRF(vecResults, ftsResults, bm25Results []ScoredResult, k int, vecWeight, ftsWeight, bm25Weight float64) []ScoredResult {
	scores := make(map[string]float64)
	contents := make(map[string]ScoredResult)

	// Assign RRF scores for vector results
	for rank, r := range vecResults {
		rrfScore := vecWeight / float64(k+rank+1)
		scores[r.ID] += rrfScore
		if _, ok := contents[r.ID]; !ok {
			contents[r.ID] = r
		}
	}

	// Assign RRF scores for FTS results
	for rank, r := range ftsResults {
		rrfScore := ftsWeight / float64(k+rank+1)
		scores[r.ID] += rrfScore
		if _, ok := contents[r.ID]; !ok {
			contents[r.ID] = r
		}
	}

	// Assign RRF scores for BM25 results
	for rank, r := range bm25Results {
		rrfScore := bm25Weight / float64(k+rank+1)
		scores[r.ID] += rrfScore
		if _, ok := contents[r.ID]; !ok {
			contents[r.ID] = r
		}
	}

	// Build result list
	var merged []ScoredResult
	for id, score := range scores {
		r := contents[id]
		r.Score = score
		merged = append(merged, r)
	}

	// Sort by score descending
	sortByScore(merged)
	return merged
}

// mergeWithRRF merges two result lists using RRF.
func mergeWithRRF(list1, list2 []ScoredResult, k int, weight1, weight2 float64) []ScoredResult {
	return mergeThreeWayRRF(list1, list2, nil, k, weight1, weight2, 0)
}

// filterByMinScore filters results by minimum score.
func filterByMinScore(results []ScoredResult, minScore float64) []ScoredResult {
	if minScore <= 0 {
		return results
	}
	var filtered []ScoredResult
	for _, r := range results {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// scoredToSearchResults converts ScoredResult slice to SearchResult slice.
func scoredToSearchResults(scored []ScoredResult) []SearchResult {
	results := make([]SearchResult, len(scored))
	for i, s := range scored {
		results[i] = SearchResult{
			ID:      s.ID,
			Content: s.Content,
			Score:   s.Score,
			Source:  s.Source,
		}
	}
	return results
}
