package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// BatchEmbedder performs batch embedding with worker pool and retry support.
type BatchEmbedder struct {
	embedder  Embedder
	batchSize int
	workers   int
	logger    zerolog.Logger
}

// NewBatchEmbedder creates a new BatchEmbedder.
func NewBatchEmbedder(embedder Embedder, config BatchConfig, logger zerolog.Logger) *BatchEmbedder {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.Workers <= 0 {
		config.Workers = 4
	}

	return &BatchEmbedder{
		embedder:  embedder,
		batchSize: config.BatchSize,
		workers:   config.Workers,
		logger:    logger,
	}
}

// EmbedBatch generates embeddings for multiple entries using worker pool.
func (be *BatchEmbedder) EmbedBatch(ctx context.Context, entries []MemoryEntry) (*BatchResult, error) {
	if len(entries) == 0 {
		return &BatchResult{}, nil
	}

	start := time.Now()

	// Split into batches
	batches := splitIntoBatches(entries, be.batchSize)

	// Process batches with worker pool
	type batchJob struct {
		index   int
		entries []MemoryEntry
	}

	type batchResultItem struct {
		index   int
		entries []MemoryEntry
		failed  []FailedEntry
	}

	jobs := make(chan batchJob, len(batches))
	results := make(chan batchResultItem, len(batches))

	// Start workers
	var wg sync.WaitGroup
	workerCount := be.workers
	if workerCount > len(batches) {
		workerCount = len(batches)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				result := be.processBatch(ctx, job.entries)
				results <- batchResultItem{
					index:   job.index,
					entries: result.entries,
					failed:  result.failed,
				}
			}
		}()
	}

	// Send jobs
	for i, batch := range batches {
		jobs <- batchJob{index: i, entries: batch}
	}
	close(jobs)

	// Wait for workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	allEntries := make([][]MemoryEntry, len(batches))
	var allFailed []FailedEntry
	succeeded := 0

	for result := range results {
		allEntries[result.index] = result.entries
		allFailed = append(allFailed, result.failed...)
		succeeded += len(result.entries)
	}

	// Flatten entries back into original order
	var embeddedEntries []MemoryEntry
	for _, batch := range allEntries {
		embeddedEntries = append(embeddedEntries, batch...)
	}

	// Copy embeddings back to original entries
	embeddedIdx := 0
	for i := range entries {
		if embeddedIdx < len(embeddedEntries) && entries[i].ID == embeddedEntries[embeddedIdx].ID {
			entries[i].Embedding = embeddedEntries[embeddedIdx].Embedding
			entries[i].EmbeddingModel = embeddedEntries[embeddedIdx].EmbeddingModel
			embeddedIdx++
		}
	}

	duration := time.Since(start)
	be.logger.Info().
		Int("total", len(entries)).
		Int("succeeded", succeeded).
		Int("failed", len(allFailed)).
		Dur("duration", duration).
		Msg("batch_embedder: completed")

	return &BatchResult{
		Succeeded: succeeded,
		Failed:    allFailed,
		Duration:  duration,
	}, nil
}

type processBatchResult struct {
	entries []MemoryEntry
	failed  []FailedEntry
}

// processBatch processes a single batch of entries.
func (be *BatchEmbedder) processBatch(ctx context.Context, entries []MemoryEntry) processBatchResult {
	// Extract texts for batch embedding
	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = e.Content
	}

	// Try batch embedding
	embeddings, err := be.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		be.logger.Warn().Err(err).Int("batchSize", len(entries)).Msg("batch_embedder: batch failed, trying individual")
		// Fallback to individual embedding with retry
		return be.processIndividually(ctx, entries)
	}

	// Assign embeddings
	var succeeded []MemoryEntry
	for i, entry := range entries {
		if i < len(embeddings) && len(embeddings[i]) > 0 {
			entry.Embedding = embeddings[i]
			entry.EmbeddingModel = "auto"
			succeeded = append(succeeded, entry)
		}
	}

	return processBatchResult{entries: succeeded}
}

// processIndividually embeds entries one by one, with retry for failures.
func (be *BatchEmbedder) processIndividually(ctx context.Context, entries []MemoryEntry) processBatchResult {
	var succeeded []MemoryEntry
	var failed []FailedEntry

	for _, entry := range entries {
		embedding, err := be.embedder.Embed(ctx, entry.Content)
		if err != nil {
			// Retry once
			embedding, err = be.embedder.Embed(ctx, entry.Content)
			if err != nil {
				failed = append(failed, FailedEntry{Entry: entry, Error: err})
				continue
			}
		}
		entry.Embedding = embedding
		entry.EmbeddingModel = "auto"
		succeeded = append(succeeded, entry)
	}

	return processBatchResult{entries: succeeded, failed: failed}
}

// splitIntoBatches splits a slice of entries into batches of the given size.
func splitIntoBatches(entries []MemoryEntry, batchSize int) [][]MemoryEntry {
	var batches [][]MemoryEntry
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}
	return batches
}

// EmbedEntries is a convenience function that embeds entries and returns them with embeddings set.
func (be *BatchEmbedder) EmbedEntries(ctx context.Context, entries []MemoryEntry) ([]MemoryEntry, error) {
	result, err := be.EmbedBatch(ctx, entries)
	if err != nil {
		return nil, fmt.Errorf("batch_embedder: embed entries: %w", err)
	}

	if len(result.Failed) > 0 {
		be.logger.Warn().Int("failed", len(result.Failed)).Msg("batch_embedder: some entries failed to embed")
	}

	return entries, nil
}
