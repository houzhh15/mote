package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// MemoryManager is the top-level coordinator for the new memory architecture.
// It aggregates ContentStore, SyncEngine, IndexManager, UnifiedSearch,
// SummaryCapture, and FileWatcher into a single entry point.
type MemoryManager struct {
	config         ManagerConfig
	contentStore   *ContentStore
	indexMgr       *IndexManager
	syncEngine     *SyncEngine
	unifiedSearch  *UnifiedSearch
	summaryCapture *SummaryCapture
	batchEmbedder  *BatchEmbedder
	fileWatcher    *FileWatcher
	logger         zerolog.Logger
}

// NewMemoryManager creates and initializes a MemoryManager.
func NewMemoryManager(
	db *sql.DB,
	embedder Embedder,
	config ManagerConfig,
	logger zerolog.Logger,
) (*MemoryManager, error) {
	if config.BaseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("memory_manager: get home dir: %w", err)
		}
		config.BaseDir = filepath.Join(home, ".mote")
	}

	mm := &MemoryManager{
		config: config,
		logger: logger,
	}

	// Initialize MarkdownStore
	mdStore, err := NewMarkdownStore(MarkdownStoreOptions{
		BaseDir: config.BaseDir,
		Logger:  logger,
	})
	if err != nil {
		return nil, fmt.Errorf("memory_manager: create markdown store: %w", err)
	}

	// Initialize ContentStore
	cs, err := NewContentStore(ContentStoreOptions{
		Store:  mdStore,
		Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("memory_manager: create content store: %w", err)
	}
	mm.contentStore = cs

	// Initialize IndexManager (wraps MemoryIndex + BM25)
	indexMgr, err := NewIndexManager(IndexManagerOptions{
		DB:           db,
		Embedder:     embedder,
		Config:       config.IndexConfig,
		HybridConfig: config.HybridConfig,
		BM25Config:   config.BM25Config,
		Logger:       logger,
	})
	if err != nil {
		return nil, fmt.Errorf("memory_manager: create index manager: %w", err)
	}
	mm.indexMgr = indexMgr

	// Attach MarkdownStore to the legacy MemoryIndex for daily log and sync operations
	indexMgr.GetLegacyIndex().SetMarkdownStore(mdStore)

	// Initialize BatchEmbedder
	mm.batchEmbedder = NewBatchEmbedder(embedder, config.BatchConfig, logger)

	// Initialize SyncEngine
	mm.syncEngine = NewSyncEngine(mm.contentStore, mm.indexMgr, mm.batchEmbedder, logger)

	// Initialize UnifiedSearch
	mm.unifiedSearch = NewUnifiedSearch(mm.indexMgr, embedder, logger)

	logger.Info().
		Str("baseDir", config.BaseDir).
		Bool("watch", config.EnableWatch).
		Bool("capture", config.EnableCapture).
		Msg("memory_manager: initialized")

	return mm, nil
}

// Init performs initial sync and starts file watcher if enabled.
func (mm *MemoryManager) Init(ctx context.Context) error {
	// Perform initial full sync
	result, err := mm.syncEngine.FullSync(ctx)
	if err != nil {
		mm.logger.Warn().Err(err).Msg("memory_manager: initial sync failed")
	} else {
		mm.logger.Info().
			Int("created", result.Created).
			Dur("duration", result.Duration).
			Msg("memory_manager: initial sync completed")
	}

	// Start file watcher if enabled
	if mm.config.EnableWatch {
		if err := mm.startWatcher(); err != nil {
			mm.logger.Warn().Err(err).Msg("memory_manager: failed to start file watcher")
		}
	}

	return nil
}

// Search performs a unified search.
func (mm *MemoryManager) Search(ctx context.Context, query string, opts ExtendedSearchOptions) ([]SearchResult, error) {
	return mm.unifiedSearch.Search(ctx, query, opts)
}

// QuickSearch performs a quick search with auto mode.
func (mm *MemoryManager) QuickSearch(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	return mm.unifiedSearch.QuickSearch(ctx, query, topK)
}

// Add adds a single memory entry to the index.
func (mm *MemoryManager) Add(ctx context.Context, entry MemoryEntry) error {
	return mm.indexMgr.Index(ctx, entry)
}

// AddBatch adds multiple memory entries using batch embedding.
func (mm *MemoryManager) AddBatch(ctx context.Context, entries []MemoryEntry) (*BatchResult, error) {
	batchResult, err := mm.batchEmbedder.EmbedBatch(ctx, entries)
	if err != nil {
		return nil, fmt.Errorf("memory_manager: batch embed: %w", err)
	}

	// Index each entry
	for _, entry := range entries {
		if err := mm.indexMgr.Index(ctx, entry); err != nil {
			mm.logger.Warn().Err(err).Str("id", entry.ID).Msg("memory_manager: index failed")
		}
	}

	return batchResult, nil
}

// OnSessionEnd processes session end for summary capture.
// It extracts key memories via LLM summary, indexes them, and writes to daily log.
func (mm *MemoryManager) OnSessionEnd(ctx context.Context, sessionID string, messages []Message) error {
	if mm.summaryCapture == nil {
		return nil
	}
	entries, err := mm.summaryCapture.OnSessionEnd(ctx, sessionID, messages)
	if err != nil {
		return err
	}

	// Index captured entries so they appear in search and stats
	for _, entry := range entries {
		if indexErr := mm.indexMgr.Index(ctx, entry); indexErr != nil {
			mm.logger.Warn().Err(indexErr).Str("id", entry.ID).Msg("memory_manager: failed to index captured entry")
		}
	}

	// Append captured entries to daily log
	if len(entries) > 0 {
		var summary string
		for _, e := range entries {
			cat := e.Category
			if cat == "" {
				cat = CategoryOther
			}
			summary += fmt.Sprintf("- [%s] %s\n", cat, e.Content)
		}
		section := "自动捕获"
		if appendErr := mm.AppendDailyLog(ctx, summary, section); appendErr != nil {
			mm.logger.Warn().Err(appendErr).Msg("memory_manager: failed to append to daily log")
		}
	}

	return nil
}

// SetLLMProvider configures the LLM provider for summary capture.
func (mm *MemoryManager) SetLLMProvider(provider LLMProvider) {
	captureConfig := DefaultSummaryCaptureConfig()
	captureConfig.Enabled = mm.config.EnableCapture
	captureConfig.Mode = mm.config.CaptureMode

	sc, err := NewSummaryCapture(SummaryCaptureOptions{
		Provider: provider,
		Content:  mm.contentStore,
		Config:   captureConfig,
		Logger:   mm.logger,
	})
	if err != nil {
		mm.logger.Error().Err(err).Msg("memory_manager: failed to create summary capture")
		return
	}
	mm.summaryCapture = sc
	mm.logger.Info().Msg("memory_manager: LLM provider configured for summary capture")
}

// FullSync triggers a full synchronization.
func (mm *MemoryManager) FullSync(ctx context.Context) (*SyncResult, error) {
	return mm.syncEngine.FullSync(ctx)
}

// IncrementalSync triggers an incremental synchronization.
func (mm *MemoryManager) IncrementalSync(ctx context.Context) (*SyncResult, error) {
	return mm.syncEngine.IncrementalSync(ctx)
}

// GetIndexManager returns the underlying IndexManager for direct access.
func (mm *MemoryManager) GetIndexManager() *IndexManager {
	return mm.indexMgr
}

// GetContentStore returns the underlying ContentStore.
func (mm *MemoryManager) GetContentStore() *ContentStore {
	return mm.contentStore
}

// Close shuts down the MemoryManager and releases resources.
func (mm *MemoryManager) Close() error {
	mm.logger.Info().Msg("memory_manager: shutting down")
	if mm.fileWatcher != nil {
		if err := mm.fileWatcher.Close(); err != nil {
			mm.logger.Warn().Err(err).Msg("memory_manager: failed to close file watcher")
		}
	}
	return nil
}

// startWatcher starts the file watcher on memory directories.
func (mm *MemoryManager) startWatcher() error {
	memoryDir := filepath.Join(mm.config.BaseDir, "memory")
	memoryFile := filepath.Join(mm.config.BaseDir, "MEMORY.md")

	// Ensure memory directory exists
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return fmt.Errorf("memory_manager: create memory dir: %w", err)
	}

	watchPaths := []string{memoryDir}
	// Only add MEMORY.md parent if the file exists
	if _, err := os.Stat(memoryFile); err == nil {
		watchPaths = append(watchPaths, mm.config.BaseDir)
	}

	watcher, err := NewFileWatcher(watchPaths, mm.syncEngine.OnFilesChanged, mm.logger)
	if err != nil {
		return fmt.Errorf("memory_manager: create file watcher: %w", err)
	}
	mm.fileWatcher = watcher

	mm.logger.Info().
		Strs("paths", watchPaths).
		Msg("memory_manager: file watcher started")

	return nil
}

// Stats returns memory system statistics.
func (mm *MemoryManager) Stats(ctx context.Context) (map[string]any, error) {
	count, err := mm.indexMgr.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory_manager: get count: %w", err)
	}

	sections, err := mm.contentStore.ListSections()
	sectionCount := 0
	if err == nil {
		sectionCount = len(sections)
	}

	stats := map[string]any{
		"index_entries":    count,
		"content_sections": sectionCount,
		"watch_enabled":    mm.config.EnableWatch,
		"capture_enabled":  mm.config.EnableCapture,
		"capture_mode":     string(mm.config.CaptureMode),
		"timestamp":        time.Now().Format(time.RFC3339),
	}

	return stats, nil
}

// Delete removes a memory entry by ID.
func (mm *MemoryManager) Delete(ctx context.Context, id string) error {
	return mm.indexMgr.Remove(ctx, id)
}

// GetByID retrieves a memory entry by ID.
func (mm *MemoryManager) GetByID(ctx context.Context, id string) (*MemoryEntry, error) {
	return mm.indexMgr.GetByID(ctx, id)
}

// List returns entries with pagination.
func (mm *MemoryManager) List(ctx context.Context, limit, offset int) ([]SearchResult, error) {
	return mm.indexMgr.List(ctx, limit, offset)
}

// ListFiltered returns entries with pagination and optional filtering.
func (mm *MemoryManager) ListFiltered(ctx context.Context, limit, offset int, filter ListFilter) ([]SearchResult, error) {
	return mm.indexMgr.ListFiltered(ctx, limit, offset, filter)
}

// CountFiltered returns the number of entries matching the filter.
func (mm *MemoryManager) CountFiltered(ctx context.Context, filter ListFilter) (int, error) {
	return mm.indexMgr.CountFiltered(ctx, filter)
}

// Count returns the number of indexed entries.
func (mm *MemoryManager) Count(ctx context.Context) (int, error) {
	return mm.indexMgr.Count(ctx)
}

// GetDailyLog retrieves the daily log for a given date.
func (mm *MemoryManager) GetDailyLog(ctx context.Context, date time.Time) (string, error) {
	return mm.indexMgr.GetLegacyIndex().GetDailyLog(ctx, date)
}

// AppendDailyLog appends content to the daily log.
func (mm *MemoryManager) AppendDailyLog(ctx context.Context, content, section string) error {
	return mm.indexMgr.GetLegacyIndex().AppendDailyLog(ctx, content, section)
}
