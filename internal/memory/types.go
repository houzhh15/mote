package memory

import "time"

// MemoryEntry represents a memory entry to be stored.
type MemoryEntry struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Source    string         `json:"source"`
	SessionID string         `json:"session_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`

	// P0: Vector embedding fields
	Embedding      []float32 `json:"-"`                         // Vector embedding (not serialized to JSON)
	EmbeddingModel string    `json:"embedding_model,omitempty"` // Model used for embedding

	// P1: Chunk metadata fields
	ChunkIndex      int    `json:"chunk_index,omitempty"`
	ChunkTotal      int    `json:"chunk_total,omitempty"`
	SourceFile      string `json:"source_file,omitempty"`
	SourceLineStart int    `json:"source_line_start,omitempty"`
	SourceLineEnd   int    `json:"source_line_end,omitempty"`

	// P2: Auto-capture/recall classification fields
	Category      string  `json:"category,omitempty"`       // preference|fact|decision|entity|other
	Importance    float64 `json:"importance,omitempty"`     // 0.0-1.0, default 0.7
	CaptureMethod string  `json:"capture_method,omitempty"` // manual|auto|import
}

// SearchResult represents a memory search result.
type SearchResult struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Score     float64        `json:"score"`
	Source    string         `json:"source"`
	CreatedAt time.Time      `json:"created_at"`
	Highlight string         `json:"highlight,omitempty"` // P0: Highlighted snippet
	Metadata  map[string]any `json:"metadata,omitempty"`  // P0: Additional metadata

	// P2: Classification fields
	Category      string  `json:"category,omitempty"`       // preference|fact|decision|entity|other
	Importance    float64 `json:"importance,omitempty"`     // 0.0-1.0
	CaptureMethod string  `json:"capture_method,omitempty"` // manual|auto|import

	// P1: Chunk metadata fields
	ChunkIndex int    `json:"chunk_index,omitempty"`
	ChunkTotal int    `json:"chunk_total,omitempty"`
	SourceFile string `json:"source_file,omitempty"`
}

// SearchOptions holds options for memory search.
type SearchOptions struct {
	TopK         int        `json:"top_k"`         // Maximum number of results
	MinScore     float64    `json:"min_score"`     // Minimum score threshold
	Hybrid       bool       `json:"hybrid"`        // Enable hybrid search (vector + FTS)
	VectorWeight float64    `json:"vector_weight"` // Weight for vector search (0.0-1.0)
	Source       string     `json:"source"`        // Filter by source
	DateFrom     *time.Time `json:"-"`             // Filter by start date
	DateTo       *time.Time `json:"-"`             // Filter by end date
}

// DefaultSearchOptions returns SearchOptions with default values.
// Note: MinScore is set to filter out low-relevance results from RRF.
// With RRF k=60, the max score for a single result is 1/(60+1) ≈ 0.016.
// Setting MinScore to 0.005 keeps only results in top ~20% of theoretical max.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		TopK:         10,
		MinScore:     0.005, // Higher threshold to filter irrelevant results
		Hybrid:       true,
		VectorWeight: 0.7,
	}
}

// ScoredResult represents a search result with a score (used internally).
type ScoredResult struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
}

// IndexConfig holds configuration for the memory index.
type IndexConfig struct {
	Dimensions     int  `json:"dimensions"`
	EnableFTS      bool `json:"enable_fts"`
	EnableVec      bool `json:"enable_vec"`
	ChunkThreshold int  `json:"chunk_threshold"` // Auto-chunk content exceeding this length (chars). 0 disables.
}

// DefaultIndexConfig returns an IndexConfig with default values.
func DefaultIndexConfig() IndexConfig {
	return IndexConfig{
		Dimensions:     384,
		EnableFTS:      true,
		EnableVec:      true,
		ChunkThreshold: 2000, // Auto-chunk content > 2000 chars
	}
}

// Memory source constants.
const (
	SourceConversation = "conversation"
	SourceDocument     = "document"
	SourceTool         = "tool"
)

// P2: Memory category constants.
const (
	CategoryPreference = "preference" // User preferences: like, hate, prefer
	CategoryFact       = "fact"       // Factual information: is, are, has
	CategoryDecision   = "decision"   // Decisions made: decided, will use
	CategoryEntity     = "entity"     // Named entities: phone, email, name
	CategoryOther      = "other"      // Default/unclassified
)

// P2: Capture method constants.
const (
	CaptureMethodManual = "manual" // User explicitly added
	CaptureMethodAuto   = "auto"   // Automatically captured by engine
	CaptureMethodImport = "import" // Imported from external source
)

// P2: Default importance value.
const DefaultImportance = 0.7
