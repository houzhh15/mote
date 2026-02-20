package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// SyncEngine handles synchronization between Markdown content and the search index.
// It supports both full sync and incremental sync with content hashing.
type SyncEngine struct {
	contentStore  *ContentStore
	indexMgr      *IndexManager
	batchEmbedder *BatchEmbedder
	logger        zerolog.Logger

	mu        sync.Mutex
	hashCache map[string]string // sectionID -> content hash
}

// NewSyncEngine creates a new SyncEngine.
func NewSyncEngine(
	contentStore *ContentStore,
	indexMgr *IndexManager,
	batchEmbedder *BatchEmbedder,
	logger zerolog.Logger,
) *SyncEngine {
	return &SyncEngine{
		contentStore:  contentStore,
		indexMgr:      indexMgr,
		batchEmbedder: batchEmbedder,
		logger:        logger,
		hashCache:     make(map[string]string),
	}
}

// FullSync performs a full synchronization from content store to index.
// It re-reads all sections and re-indexes them.
func (se *SyncEngine) FullSync(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	se.logger.Info().Msg("sync_engine: starting full sync")

	// Refresh content store cache
	if err := se.contentStore.Refresh(); err != nil {
		return nil, fmt.Errorf("sync_engine: refresh content: %w", err)
	}

	sections, err := se.contentStore.ListSections()
	if err != nil {
		return nil, fmt.Errorf("sync_engine: list sections: %w", err)
	}
	if len(sections) == 0 {
		se.logger.Info().Msg("sync_engine: no sections found")
		return &SyncResult{Duration: time.Since(start)}, nil
	}

	// Convert sections to memory entries
	entries := sectionsToEntries(sections)

	// Batch embed
	batchResult, err := se.batchEmbedder.EmbedBatch(ctx, entries)
	if err != nil {
		return nil, fmt.Errorf("sync_engine: batch embed: %w", err)
	}

	// Index all entries
	indexed := 0
	for _, entry := range entries {
		if err := se.indexMgr.Index(ctx, entry); err != nil {
			se.logger.Warn().Err(err).Str("id", entry.ID).Msg("sync_engine: index failed")
			continue
		}
		indexed++
	}

	// Update hash cache
	se.mu.Lock()
	se.hashCache = make(map[string]string)
	for _, s := range sections {
		se.hashCache[s.ID] = contentHash(s.Content)
	}
	se.mu.Unlock()

	duration := time.Since(start)
	se.logger.Info().
		Int("sections", len(sections)).
		Int("indexed", indexed).
		Int("embedFailed", len(batchResult.Failed)).
		Dur("duration", duration).
		Msg("sync_engine: full sync completed")

	return &SyncResult{
		Created:  indexed,
		Duration: duration,
	}, nil
}

// IncrementalSync checks for changed sections and re-indexes only those.
func (se *SyncEngine) IncrementalSync(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	se.logger.Debug().Msg("sync_engine: starting incremental sync")

	// Refresh content store cache
	if err := se.contentStore.Refresh(); err != nil {
		return nil, fmt.Errorf("sync_engine: refresh content: %w", err)
	}

	sections, err := se.contentStore.ListSections()
	if err != nil {
		return nil, fmt.Errorf("sync_engine: list sections: %w", err)
	}

	se.mu.Lock()
	// Detect changes
	var created, updated, deleted int
	var changedSections []Section
	currentIDs := make(map[string]struct{})

	for _, s := range sections {
		currentIDs[s.ID] = struct{}{}
		hash := contentHash(s.Content)
		oldHash, exists := se.hashCache[s.ID]

		if !exists {
			// New section
			changedSections = append(changedSections, s)
			se.hashCache[s.ID] = hash
			created++
		} else if hash != oldHash {
			// Updated section
			changedSections = append(changedSections, s)
			se.hashCache[s.ID] = hash
			updated++
		}
	}

	// Find deleted sections
	var removedIDs []string
	for id := range se.hashCache {
		if _, exists := currentIDs[id]; !exists {
			removedIDs = append(removedIDs, id)
			deleted++
		}
	}
	for _, id := range removedIDs {
		delete(se.hashCache, id)
	}
	se.mu.Unlock()

	// Remove deleted entries from index
	for _, id := range removedIDs {
		if err := se.indexMgr.Remove(ctx, id); err != nil {
			se.logger.Warn().Err(err).Str("id", id).Msg("sync_engine: remove failed")
		}
	}

	// Index changed sections
	if len(changedSections) > 0 {
		entries := sectionsToEntries(changedSections)

		// Batch embed
		if _, err := se.batchEmbedder.EmbedBatch(ctx, entries); err != nil {
			se.logger.Warn().Err(err).Msg("sync_engine: batch embed failed for incremental")
		}

		for _, entry := range entries {
			if err := se.indexMgr.Index(ctx, entry); err != nil {
				se.logger.Warn().Err(err).Str("id", entry.ID).Msg("sync_engine: index failed")
			}
		}
	}

	duration := time.Since(start)
	if created > 0 || updated > 0 || deleted > 0 {
		se.logger.Info().
			Int("created", created).
			Int("updated", updated).
			Int("deleted", deleted).
			Dur("duration", duration).
			Msg("sync_engine: incremental sync completed")
	}

	return &SyncResult{
		Created:  created,
		Updated:  updated,
		Deleted:  deleted,
		Duration: duration,
	}, nil
}

// OnFilesChanged handles file change notifications from FileWatcher.
func (se *SyncEngine) OnFilesChanged(changedFiles []string) {
	se.logger.Info().
		Int("files", len(changedFiles)).
		Msg("sync_engine: files changed, running incremental sync")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := se.IncrementalSync(ctx); err != nil {
		se.logger.Error().Err(err).Msg("sync_engine: incremental sync failed on file change")
	}
}

// sectionsToEntries converts sections to memory entries.
func sectionsToEntries(sections []Section) []MemoryEntry {
	entries := make([]MemoryEntry, 0, len(sections))
	for _, s := range sections {
		entry := MemoryEntry{
			ID:         s.ID,
			Content:    s.Content,
			Source:     SourceDocument,
			CreatedAt:  time.Now(),
			Category:   s.Metadata.Category,
			SourceFile: s.FilePath,
		}
		if s.Metadata.Importance > 0 {
			entry.Importance = s.Metadata.Importance
		} else {
			entry.Importance = DefaultImportance
		}
		entries = append(entries, entry)
	}
	return entries
}

// contentHash returns a SHA256 hash of content for change detection.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}
