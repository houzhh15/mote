package memory

import (
	"context"
	"time"
)

// SearchMode defines the search strategy to use.
type SearchMode string

const (
	SearchModeAuto   SearchMode = "auto"   // Automatically select the best strategy
	SearchModeVector SearchMode = "vector" // Vector search only
	SearchModeText   SearchMode = "text"   // Text search only (FTS + BM25)
	SearchModeHybrid SearchMode = "hybrid" // Full hybrid (vector + FTS + BM25)
)

// CaptureMode defines the auto-capture strategy.
type CaptureMode string

const (
	CaptureModeRegex      CaptureMode = "regex"       // Legacy regex-based capture
	CaptureModeLLMSummary CaptureMode = "llm_summary" // LLM-based session summary
	CaptureModeHybrid     CaptureMode = "hybrid"      // Both regex and LLM summary
)

// Section represents a logical section within a Markdown memory file.
type Section struct {
	ID       string          `json:"id"`        // Unique ID based on file path + heading
	FilePath string          `json:"file_path"` // Source file path
	Heading  string          `json:"heading"`   // Markdown heading text
	Content  string          `json:"content"`   // Section body (without heading)
	Hash     string          `json:"hash"`      // SHA256 hash of content
	Metadata SectionMetadata `json:"metadata"`  // Parsed from Frontmatter
}

// SectionMetadata holds metadata parsed from Markdown Frontmatter.
type SectionMetadata struct {
	Category   string   `json:"category"`   // preference/fact/decision/entity/other
	Importance float64  `json:"importance"` // 0.0-1.0, default 0.7
	Tags       []string `json:"tags"`       // Optional tags
}

// DefaultSectionMetadata returns a SectionMetadata with default values.
func DefaultSectionMetadata() SectionMetadata {
	return SectionMetadata{
		Category:   CategoryOther,
		Importance: DefaultImportance,
	}
}

// ManagerConfig holds configuration for the MemoryManager.
type ManagerConfig struct {
	BaseDir       string       `json:"base_dir"`       // Base directory (~/.mote/)
	EnableWatch   bool         `json:"enable_watch"`   // Enable file watching
	EnableCapture bool         `json:"enable_capture"` // Enable auto-capture
	CaptureMode   CaptureMode  `json:"capture_mode"`   // Capture strategy
	IndexConfig   IndexConfig  `json:"index_config"`   // Index configuration
	HybridConfig  HybridConfig `json:"hybrid_config"`  // Hybrid search configuration
	BatchConfig   BatchConfig  `json:"batch_config"`   // Batch embedding configuration
	BM25Config    BM25Config   `json:"bm25_config"`    // BM25 configuration
}

// DefaultManagerConfig returns a ManagerConfig with default values.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		EnableWatch:   true,
		EnableCapture: true,
		CaptureMode:   CaptureModeLLMSummary,
		IndexConfig:   DefaultIndexConfig(),
		HybridConfig:  DefaultHybridConfig(),
		BatchConfig:   DefaultBatchConfig(),
		BM25Config:    DefaultBM25Config(),
	}
}

// BatchConfig holds configuration for batch embedding.
type BatchConfig struct {
	BatchSize int `json:"batch_size"` // Number of entries per batch (default 50)
	Workers   int `json:"workers"`    // Number of concurrent workers (default 4)
}

// DefaultBatchConfig returns a BatchConfig with default values.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		BatchSize: 50,
		Workers:   4,
	}
}

// BM25Config holds configuration for BM25 scoring.
type BM25Config struct {
	K1 float64 `json:"k1"` // Term frequency saturation parameter (default 1.2)
	B  float64 `json:"b"`  // Document length normalization parameter (default 0.75)
}

// DefaultBM25Config returns a BM25Config with default values.
func DefaultBM25Config() BM25Config {
	return BM25Config{
		K1: 1.2,
		B:  0.75,
	}
}

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	Created  int           `json:"created"`  // Number of new sections indexed
	Updated  int           `json:"updated"`  // Number of sections re-indexed
	Deleted  int           `json:"deleted"`  // Number of sections removed from index
	Errors   []error       `json:"-"`        // Errors encountered during sync
	Duration time.Duration `json:"duration"` // Total sync duration
}

// BatchResult holds the result of a batch embedding operation.
type BatchResult struct {
	Succeeded int           `json:"succeeded"` // Number of successfully embedded entries
	Failed    []FailedEntry `json:"failed"`    // Entries that failed to embed
	Duration  time.Duration `json:"duration"`  // Total batch duration
}

// FailedEntry represents an entry that failed during batch processing.
type FailedEntry struct {
	Entry MemoryEntry `json:"entry"` // The entry that failed
	Error error       `json:"-"`     // The error encountered
}

// SummaryCaptureConfig holds configuration for LLM-based session summary capture.
type SummaryCaptureConfig struct {
	Enabled       bool        `json:"enabled"`         // Enable summary capture
	Mode          CaptureMode `json:"mode"`            // Capture mode
	MinMessages   int         `json:"min_messages"`    // Minimum messages to trigger summary (default 5)
	MaxTokens     int         `json:"max_tokens"`      // Maximum tokens for summary (default 200)
	DupThreshold  float64     `json:"dup_threshold"`   // Deduplication similarity threshold (default 0.95)
	MaxPerSession int         `json:"max_per_session"` // Maximum captures per session (default 3)
}

// DefaultSummaryCaptureConfig returns a SummaryCaptureConfig with default values.
func DefaultSummaryCaptureConfig() SummaryCaptureConfig {
	return SummaryCaptureConfig{
		Enabled:       true,
		Mode:          CaptureModeLLMSummary,
		MinMessages:   5,
		MaxTokens:     200,
		DupThreshold:  0.95,
		MaxPerSession: 3,
	}
}

// Note: Message type is defined in capture.go.

// SummaryResult holds the result of LLM summary generation.
type SummaryResult struct {
	Entries []SummaryEntry `json:"entries"` // Extracted memory entries
}

// SummaryEntry represents a single extracted memory from a session summary.
type SummaryEntry struct {
	Content    string  `json:"content"`    // Extracted content
	Category   string  `json:"category"`   // preference/fact/decision/entity/other
	Importance float64 `json:"importance"` // 0.0-1.0
}

// --- Interfaces for modular design ---

// ContentStorer defines the interface for content storage operations.
type ContentStorer interface {
	ListSections() ([]Section, error)
	GetSection(id string) (*Section, error)
	UpsertSection(section Section) error
	DeleteSection(id string) error
}

// Indexer defines the interface for index management operations.
type Indexer interface {
	Index(ctx context.Context, entry MemoryEntry) error
	IndexBatch(ctx context.Context, entries []MemoryEntry) error
	Remove(ctx context.Context, id string) error
	Search(ctx context.Context, query string, embedding []float32, opts SearchOptions) ([]SearchResult, error)
}

// Syncer defines the interface for sync engine operations.
type Syncer interface {
	FullSync(ctx context.Context) (*SyncResult, error)
	IncrementalSync(ctx context.Context, changedFiles []string) (*SyncResult, error)
	Start(ctx context.Context) error
	Stop() error
}

// Capturer defines the interface for auto-capture operations.
type Capturer interface {
	OnSessionEnd(ctx context.Context, sessionID string, messages []Message) ([]MemoryEntry, error)
}

// LLMProvider defines the interface for LLM completion calls.
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// BM25Searcher defines the interface for BM25 text search.
type BM25Searcher interface {
	SearchBM25(ctx context.Context, query string, topK int) ([]ScoredResult, error)
}

// ExtendedSearchOptions extends SearchOptions with v2 fields.
// This is kept separate to maintain backward compatibility with existing SearchOptions.
type ExtendedSearchOptions struct {
	SearchOptions             // Embed original options
	Mode           SearchMode `json:"mode"`            // Search mode (auto/vector/text/hybrid)
	BM25Weight     float64    `json:"bm25_weight"`     // Weight for BM25 search (0.0-1.0)
	CategoryFilter string     `json:"category_filter"` // Filter by category
	ImportanceMin  float64    `json:"importance_min"`  // Minimum importance threshold
}

// DefaultExtendedSearchOptions returns ExtendedSearchOptions with default values.
func DefaultExtendedSearchOptions() ExtendedSearchOptions {
	return ExtendedSearchOptions{
		SearchOptions:  DefaultSearchOptions(),
		Mode:           SearchModeAuto,
		BM25Weight:     0.3,
		CategoryFilter: "",
		ImportanceMin:  0.0,
	}
}
