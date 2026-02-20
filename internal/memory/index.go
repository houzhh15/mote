package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// parseTimeFlexible parses time strings in various formats used by SQLite and Go.
// Returns a zero time if parsing fails.
func parseTimeFlexible(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try common formats in order of likelihood
	formats := []string{
		"2006-01-02 15:04:05",           // SQLite DATETIME default
		time.RFC3339,                    // Go default (2006-01-02T15:04:05Z07:00)
		time.RFC3339Nano,                // Go with nanoseconds
		"2006-01-02T15:04:05",           // ISO without timezone
		"2006-01-02 15:04:05.999999999", // SQLite with fractional seconds
		"2006-01-02T15:04:05.999999999", // ISO with fractional seconds
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// escapeFTS5Query escapes special characters in FTS5 queries and formats for better matching.
// FTS5 special characters: " * ( ) [ ] { } : @ + - = < > ! ^ .
// Strategy: Remove/escape problematic characters, split into tokens, and join with OR for broader matching.
func escapeFTS5Query(query string) string {
	// List of FTS5 special characters that can cause syntax errors
	specialChars := `"*()[]{}:@+-=<>!^.`

	var result strings.Builder
	for _, char := range query {
		if strings.ContainsRune(specialChars, char) {
			// Replace special characters with space to preserve word boundaries
			result.WriteRune(' ')
		} else {
			result.WriteRune(char)
		}
	}

	// Clean up and normalize whitespace
	tokens := strings.Fields(result.String())

	// If no tokens, return a safe wildcard
	if len(tokens) == 0 {
		return "*"
	}

	// Join tokens with OR for broader matching (helps with Chinese text
	// where tokenization may not work well with unicode61)
	return strings.Join(tokens, " OR ")
}

// MemoryIndex manages memory storage and retrieval.
type MemoryIndex struct {
	db            *sql.DB
	embedder      Embedder
	config        IndexConfig
	hybridConfig  HybridConfig
	vecReady      bool
	logger        zerolog.Logger
	markdownStore *MarkdownStore // P1: Markdown file storage
}

// MemoryIndexOptions holds options for creating a MemoryIndex.
type MemoryIndexOptions struct {
	DB           *sql.DB
	Embedder     Embedder
	Config       IndexConfig
	HybridConfig HybridConfig
	Logger       zerolog.Logger
}

// NewMemoryIndex creates a new MemoryIndex.
func NewMemoryIndex(db *sql.DB, embedder Embedder, config IndexConfig) (*MemoryIndex, error) {
	return NewMemoryIndexWithOptions(MemoryIndexOptions{
		DB:       db,
		Embedder: embedder,
		Config:   config,
	})
}

// NewMemoryIndexWithOptions creates a new MemoryIndex with full options.
func NewMemoryIndexWithOptions(opts MemoryIndexOptions) (*MemoryIndex, error) {
	if opts.Config.Dimensions == 0 {
		opts.Config.Dimensions = 384
	}

	// Apply default hybrid config
	if opts.HybridConfig.VectorWeight == 0 && opts.HybridConfig.TextWeight == 0 {
		opts.HybridConfig = DefaultHybridConfig()
	}

	// Create tables
	if err := CreateMemoryTables(opts.DB, opts.Config.EnableFTS); err != nil {
		return nil, fmt.Errorf("create tables: %w", err)
	}

	// Run migrations for P0/P1 columns
	if err := MigrateMemorySchema(opts.DB); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	m := &MemoryIndex{
		db:           opts.DB,
		embedder:     opts.Embedder,
		config:       opts.Config,
		hybridConfig: opts.HybridConfig,
		vecReady:     opts.Config.EnableVec,
		logger:       opts.Logger,
	}

	return m, nil
}

// Add adds a memory entry to the index.
// If ChunkThreshold is configured and content exceeds it, the entry is automatically chunked.
func (m *MemoryIndex) Add(ctx context.Context, entry MemoryEntry) error {
	// Auto-chunk if threshold is set and content exceeds it
	if m.config.ChunkThreshold > 0 && len(entry.Content) > m.config.ChunkThreshold {
		m.logger.Debug().
			Int("content_len", len(entry.Content)).
			Int("threshold", m.config.ChunkThreshold).
			Msg("auto-chunking memory entry")
		return m.AddWithChunking(ctx, entry)
	}

	// Otherwise, add as single entry
	return m.addSingleEntry(ctx, entry)
}

// AddWithChunking adds a memory entry with automatic chunking for long content.
// This method splits the content using Chunker and stores each chunk as a separate entry.
// Each chunk maintains a reference to the original via ChunkIndex and ChunkTotal fields.
func (m *MemoryIndex) AddWithChunking(ctx context.Context, entry MemoryEntry) error {
	chunker := NewChunker(DefaultChunkerOptions())
	chunks := chunker.Split(entry.Content, entry.SourceFile)

	if len(chunks) == 0 {
		// No chunks produced, store as single entry
		return m.addSingleEntry(ctx, entry)
	}

	// Generate a parent ID for all chunks to share
	parentID := entry.ID
	if parentID == "" {
		parentID = uuid.New().String()
	}

	for _, chunk := range chunks {
		chunkEntry := MemoryEntry{
			ID:              fmt.Sprintf("%s-chunk-%d", parentID, chunk.Index),
			Content:         chunk.Content,
			Source:          entry.Source,
			SessionID:       entry.SessionID,
			Metadata:        entry.Metadata,
			Category:        entry.Category,
			Importance:      entry.Importance,
			CaptureMethod:   entry.CaptureMethod,
			CreatedAt:       entry.CreatedAt,
			ChunkIndex:      chunk.Index,
			ChunkTotal:      len(chunks),
			SourceFile:      chunk.SourceFile,
			SourceLineStart: chunk.StartLine,
			SourceLineEnd:   chunk.EndLine,
		}
		if err := m.addSingleEntry(ctx, chunkEntry); err != nil {
			return fmt.Errorf("add chunk %d: %w", chunk.Index, err)
		}
	}

	m.logger.Debug().
		Str("parent_id", parentID).
		Int("chunks", len(chunks)).
		Msg("added memory with chunking")

	return nil
}

// addSingleEntry adds a single memory entry without chunking logic.
// This is the internal implementation used by both Add() and AddWithChunking().
func (m *MemoryIndex) addSingleEntry(ctx context.Context, entry MemoryEntry) error {
	// Reject empty content to prevent blank memory entries
	if strings.TrimSpace(entry.Content) == "" {
		return fmt.Errorf("memory content is empty")
	}

	// Generate ID if not provided
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	// Set CreatedAt if not provided
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// P2: Set default values for classification fields
	if entry.Category == "" {
		entry.Category = CategoryOther
	}
	if entry.Importance == 0 {
		entry.Importance = DefaultImportance
	}
	if entry.CaptureMethod == "" {
		entry.CaptureMethod = CaptureMethodManual
	}

	// Serialize metadata
	var metadataJSON []byte
	if entry.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	// Generate embedding if embedder is available and entry doesn't have one
	var embeddingBlob []byte
	var embeddingModel string
	if m.embedder != nil && len(entry.Embedding) == 0 && m.config.EnableVec {
		embedding, err := m.embedder.Embed(ctx, entry.Content)
		if err != nil {
			// Log but don't fail - embedding is optional
			m.logger.Warn().Err(err).Str("id", entry.ID).Msg("failed to generate embedding")
		} else {
			entry.Embedding = embedding
			entry.EmbeddingModel = "auto"
		}
	}

	// Serialize embedding to blob
	if len(entry.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(entry.Embedding)
		embeddingModel = entry.EmbeddingModel
	}

	// Insert into main table (including P2 fields and chunk fields)
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO memories (id, content, metadata, source, session_id, created_at, embedding, embedding_model, 
		                      category, importance, capture_method, chunk_index, chunk_total, source_file, source_line_start, source_line_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Content, string(metadataJSON), entry.Source, entry.SessionID, entry.CreatedAt,
		embeddingBlob, embeddingModel, entry.Category, entry.Importance, entry.CaptureMethod,
		entry.ChunkIndex, entry.ChunkTotal, entry.SourceFile, entry.SourceLineStart, entry.SourceLineEnd)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	// Insert into FTS table if enabled
	if m.config.EnableFTS {
		_, err = m.db.ExecContext(ctx, `
			INSERT INTO memory_fts (id, content) VALUES (?, ?)
		`, entry.ID, entry.Content)
		if err != nil {
			return fmt.Errorf("insert fts: %w", err)
		}
	}

	return nil
}

// Search searches for memories similar to the query.
// Uses hybrid search (vector + FTS) when available, with LIKE fallback for non-tokenizable queries.
func (m *MemoryIndex) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	m.logger.Debug().Str("query", query).Int("topK", topK).Msg("Search: starting")

	// Try hybrid search first (uses both vector similarity and FTS)
	results, err := m.HybridSearch(ctx, query, SearchOptions{TopK: topK})
	if err != nil {
		m.logger.Warn().Err(err).Msg("hybrid search failed, falling back to LIKE")
	}
	m.logger.Debug().Int("hybridResultsCount", len(results)).Msg("Search: hybrid search completed")

	// If no results from hybrid search, try LIKE search as fallback
	// This helps with non-tokenizable queries (e.g., Chinese without proper tokenizer)
	if len(results) == 0 {
		m.logger.Debug().Msg("Search: no hybrid results, trying LIKE search")
		likeResults, likeErr := m.searchLike(ctx, query, topK)
		if likeErr != nil {
			m.logger.Warn().Err(likeErr).Msg("LIKE search also failed")
			if err != nil {
				return nil, err // Return original error
			}
			return nil, likeErr
		}
		m.logger.Debug().Int("likeResultsCount", len(likeResults)).Msg("Search: LIKE search completed")
		return likeResults, nil
	}

	return results, nil
}

// SearchFTS performs full-text search on memories.
func (m *MemoryIndex) SearchFTS(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if !m.config.EnableFTS {
		return nil, fmt.Errorf("FTS not enabled")
	}

	// Escape FTS5 special characters to prevent syntax errors
	escapedQuery := escapeFTS5Query(query)

	rows, err := m.db.QueryContext(ctx, `
		SELECT m.id, m.content, m.source, m.created_at, m.category, m.importance, m.capture_method,
		       m.chunk_index, m.chunk_total, m.source_file
		FROM memory_fts f
		JOIN memories m ON f.id = m.id
		WHERE memory_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, escapedQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("search fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var createdAt string
		var category, captureMethod sql.NullString
		var importance sql.NullFloat64
		var chunkIndex, chunkTotal sql.NullInt64
		var sourceFile sql.NullString
		if err := rows.Scan(&r.ID, &r.Content, &r.Source, &createdAt, &category, &importance, &captureMethod,
			&chunkIndex, &chunkTotal, &sourceFile); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.CreatedAt = parseTimeFlexible(createdAt)
		r.Score = 1.0 // FTS doesn't provide similarity score
		// P2: Set classification fields
		if category.Valid {
			r.Category = category.String
		}
		if importance.Valid {
			r.Importance = importance.Float64
		}
		if captureMethod.Valid {
			r.CaptureMethod = captureMethod.String
		}
		// P1: Set chunk metadata fields
		if chunkIndex.Valid {
			r.ChunkIndex = int(chunkIndex.Int64)
		}
		if chunkTotal.Valid {
			r.ChunkTotal = int(chunkTotal.Int64)
		}
		if sourceFile.Valid {
			r.SourceFile = sourceFile.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// searchLike performs a LIKE-based search as a fallback for non-tokenizable queries (e.g., Chinese).
// This is less efficient than FTS but works for any text.
func (m *MemoryIndex) searchLike(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Split query into tokens - for Chinese, also try character-level splits
	tokens := strings.Fields(query)

	// If no space-separated tokens and query looks like it might contain non-ASCII (e.g., Chinese),
	// also add the original query as a token and try splitting by common patterns
	if len(tokens) == 0 || (len(tokens) == 1 && len(query) > len(tokens[0])*3) {
		// For queries without spaces, add the whole query as one token
		if len(tokens) == 0 {
			tokens = []string{query}
		}

		// Also try common keyword patterns - extract potential keywords
		// This helps with queries like "NIEP功能" -> search for both "NIEP" and "功能"
		var additionalTokens []string
		currentToken := ""
		lastWasASCII := false

		for _, r := range query {
			isASCII := r < 128 && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))

			if isASCII != lastWasASCII && currentToken != "" {
				additionalTokens = append(additionalTokens, currentToken)
				currentToken = ""
			}
			currentToken += string(r)
			lastWasASCII = isASCII
		}
		if currentToken != "" {
			additionalTokens = append(additionalTokens, currentToken)
		}

		// Add extracted tokens that are at least 2 chars
		for _, t := range additionalTokens {
			if len(t) >= 2 && t != query {
				tokens = append(tokens, t)
			}
		}
	}

	m.logger.Debug().Strs("tokens", tokens).Msg("searchLike: extracted tokens")

	// Build OR conditions for each token
	var conditions []string
	var args []interface{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		conditions = append(conditions, "content LIKE ?")
		args = append(args, "%"+token+"%")
	}
	if len(conditions) == 0 {
		return nil, nil
	}

	whereClause := strings.Join(conditions, " OR ")
	args = append(args, topK)

	sqlQuery := fmt.Sprintf(`
		SELECT id, content, source, created_at, category, importance, capture_method,
		       chunk_index, chunk_total, source_file
		FROM memories
		WHERE %s
		ORDER BY created_at DESC
		LIMIT ?
	`, whereClause)

	rows, err := m.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search like: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var createdAt string
		var category, captureMethod sql.NullString
		var importance sql.NullFloat64
		var chunkIndex, chunkTotal sql.NullInt64
		var sourceFile sql.NullString
		if err := rows.Scan(&r.ID, &r.Content, &r.Source, &createdAt, &category, &importance, &captureMethod,
			&chunkIndex, &chunkTotal, &sourceFile); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.CreatedAt = parseTimeFlexible(createdAt)
		r.Score = 0.5 // Fixed score for LIKE matches
		if category.Valid {
			r.Category = category.String
		}
		if importance.Valid {
			r.Importance = importance.Float64
		}
		if captureMethod.Valid {
			r.CaptureMethod = captureMethod.String
		}
		if chunkIndex.Valid {
			r.ChunkIndex = int(chunkIndex.Int64)
		}
		if chunkTotal.Valid {
			r.ChunkTotal = int(chunkTotal.Int64)
		}
		if sourceFile.Valid {
			r.SourceFile = sourceFile.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// Delete removes a memory entry by ID.
func (m *MemoryIndex) Delete(ctx context.Context, id string) error {
	// Delete from main table
	result, err := m.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrEntryNotFound
	}

	// Delete from FTS table if enabled
	if m.config.EnableFTS {
		_, err = m.db.ExecContext(ctx, `DELETE FROM memory_fts WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete fts: %w", err)
		}
	}

	return nil
}

// Count returns the number of memory entries.
func (m *MemoryIndex) Count(ctx context.Context) (int, error) {
	var count int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// List returns all memory entries with pagination.
func (m *MemoryIndex) List(ctx context.Context, limit, offset int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, content, source, created_at, category, importance, capture_method,
		       chunk_index, chunk_total, source_file
		FROM memories
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var createdAt string
		var category, captureMethod sql.NullString
		var importance sql.NullFloat64
		var chunkIndex, chunkTotal sql.NullInt64
		var sourceFile sql.NullString
		if err := rows.Scan(&r.ID, &r.Content, &r.Source, &createdAt, &category, &importance, &captureMethod,
			&chunkIndex, &chunkTotal, &sourceFile); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.CreatedAt = parseTimeFlexible(createdAt)
		r.Score = 1.0
		// P2: Set classification fields
		if category.Valid {
			r.Category = category.String
		}
		if importance.Valid {
			r.Importance = importance.Float64
		}
		if captureMethod.Valid {
			r.CaptureMethod = captureMethod.String
		}
		// P1: Set chunk metadata fields
		if chunkIndex.Valid {
			r.ChunkIndex = int(chunkIndex.Int64)
		}
		if chunkTotal.Valid {
			r.ChunkTotal = int(chunkTotal.Int64)
		}
		if sourceFile.Valid {
			r.SourceFile = sourceFile.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// GetByID retrieves a memory entry by its ID.
func (m *MemoryIndex) GetByID(ctx context.Context, id string) (*MemoryEntry, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, content, source, session_id, created_at, metadata, embedding, embedding_model, 
		       category, importance, capture_method, chunk_index, chunk_total, source_file, source_line_start, source_line_end
		FROM memories WHERE id = ?
	`, id)

	var entry MemoryEntry
	var metadataJSON sql.NullString
	var createdAt string
	var embeddingBlob []byte
	var embeddingModel sql.NullString
	var sessionID sql.NullString
	var category, captureMethod sql.NullString
	var importance sql.NullFloat64
	var chunkIndex, chunkTotal sql.NullInt64
	var sourceFile sql.NullString
	var sourceLineStart, sourceLineEnd sql.NullInt64

	err := row.Scan(&entry.ID, &entry.Content, &entry.Source, &sessionID, &createdAt, &metadataJSON,
		&embeddingBlob, &embeddingModel, &category, &importance, &captureMethod,
		&chunkIndex, &chunkTotal, &sourceFile, &sourceLineStart, &sourceLineEnd)
	if err == sql.ErrNoRows {
		return nil, ErrMemoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}

	entry.CreatedAt = parseTimeFlexible(createdAt)
	if sessionID.Valid {
		entry.SessionID = sessionID.String
	}
	if metadataJSON.Valid {
		_ = json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
	}
	if len(embeddingBlob) > 0 {
		entry.Embedding = decodeEmbedding(embeddingBlob)
	}
	if embeddingModel.Valid {
		entry.EmbeddingModel = embeddingModel.String
	}
	// P2: Set classification fields
	if category.Valid {
		entry.Category = category.String
	}
	if importance.Valid {
		entry.Importance = importance.Float64
	}
	if captureMethod.Valid {
		entry.CaptureMethod = captureMethod.String
	}
	// Chunk fields
	if chunkIndex.Valid {
		entry.ChunkIndex = int(chunkIndex.Int64)
	}
	if chunkTotal.Valid {
		entry.ChunkTotal = int(chunkTotal.Int64)
	}
	if sourceFile.Valid {
		entry.SourceFile = sourceFile.String
	}
	if sourceLineStart.Valid {
		entry.SourceLineStart = int(sourceLineStart.Int64)
	}
	if sourceLineEnd.Valid {
		entry.SourceLineEnd = int(sourceLineEnd.Int64)
	}

	return &entry, nil
}

// SearchVector performs vector similarity search.
// Returns results sorted by cosine similarity (descending).
func (m *MemoryIndex) SearchVector(ctx context.Context, embedding []float32, topK int) ([]ScoredResult, error) {
	// Fetch all embeddings and compute similarity in-memory
	// (For production, consider sqlite-vec extension or external vector DB)
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, content, source, embedding
		FROM memories
		WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("query embeddings: %w", err)
	}
	defer rows.Close()

	var results []ScoredResult
	for rows.Next() {
		var id, content, source string
		var embeddingBlob []byte

		if err := rows.Scan(&id, &content, &source, &embeddingBlob); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if len(embeddingBlob) == 0 {
			continue
		}

		storedEmbedding := decodeEmbedding(embeddingBlob)
		similarity := cosineSimilarity(embedding, storedEmbedding)

		results = append(results, ScoredResult{
			ID:      id,
			Content: content,
			Source:  source,
			Score:   similarity,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Sort by similarity (descending)
	sortByScore(results)

	// Limit to topK
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// HybridSearch performs combined vector + FTS search with RRF fusion.
func (m *MemoryIndex) HybridSearch(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	// Apply defaults
	if opts.TopK <= 0 {
		opts.TopK = DefaultSearchOptions().TopK
	}
	if opts.VectorWeight == 0 {
		opts.VectorWeight = m.hybridConfig.VectorWeight
	}
	if opts.MinScore == 0 {
		opts.MinScore = m.hybridConfig.MinScore
	}

	// Short query optimization: use FTS-only for queries with <= 5 characters
	// to ensure exact keyword matching (e.g., "NIEP", "AI", "云安全")
	queryLen := len([]rune(query))
	if queryLen > 0 && queryLen <= 5 {
		m.logger.Debug().
			Str("query", query).
			Int("queryLen", queryLen).
			Msg("HybridSearch: short query, using FTS-only")
		ftsResults, err := m.searchFTSInternal(ctx, query, opts.TopK)
		if err != nil {
			return nil, err
		}
		return m.scoredToSearchResults(ftsResults), nil
	}

	m.logger.Debug().
		Str("query", query).
		Bool("hasEmbedder", m.embedder != nil).
		Bool("enableVec", m.config.EnableVec).
		Msg("HybridSearch: starting")

	// Get query embedding
	var queryEmbedding []float32
	var embedErr error
	if m.embedder != nil && m.config.EnableVec {
		queryEmbedding, embedErr = m.embedder.Embed(ctx, query)
		if embedErr != nil {
			m.logger.Warn().Err(embedErr).Msg("embedding failed, fallback to FTS-only")
		} else {
			m.logger.Debug().Int("embeddingLen", len(queryEmbedding)).Msg("HybridSearch: got embedding")
		}
	}

	// If no embedding, fall back to pure FTS
	if len(queryEmbedding) == 0 {
		m.logger.Debug().Msg("HybridSearch: no embedding, fallback to FTS-only")
		ftsResults, err := m.searchFTSInternal(ctx, query, opts.TopK)
		if err != nil {
			return nil, err
		}
		m.logger.Debug().Int("ftsResultsCount", len(ftsResults)).Msg("HybridSearch: FTS-only results")
		return m.scoredToSearchResults(ftsResults), nil
	}

	// Fetch more results for better RRF fusion
	fetchK := opts.TopK * 4
	if fetchK < 40 {
		fetchK = 40
	}

	var vecResults, ftsResults []ScoredResult
	var vecErr, ftsErr error
	var wg sync.WaitGroup

	// Run vector and FTS search in parallel
	wg.Add(2)

	go func() {
		defer wg.Done()
		vecResults, vecErr = m.SearchVector(ctx, queryEmbedding, fetchK)
	}()

	go func() {
		defer wg.Done()
		ftsResults, ftsErr = m.searchFTSInternal(ctx, query, fetchK)
	}()

	wg.Wait()

	// Handle errors gracefully
	if vecErr != nil && ftsErr != nil {
		return nil, vecErr
	}
	if vecErr != nil {
		m.logger.Warn().Err(vecErr).Msg("vector search failed, using FTS only")
		vecResults = nil
	}
	if ftsErr != nil {
		m.logger.Warn().Err(ftsErr).Msg("FTS search failed, using vector only")
		ftsResults = nil
	}

	// Merge using RRF
	results := m.mergeRRF(vecResults, ftsResults, opts)

	// Enrich results with additional fields (chunk info, etc.)
	return m.enrichSearchResults(ctx, results), nil
}

// searchFTSInternal performs FTS search and returns ScoredResult.
func (m *MemoryIndex) searchFTSInternal(ctx context.Context, query string, topK int) ([]ScoredResult, error) {
	if !m.config.EnableFTS {
		return nil, nil
	}

	// Escape FTS5 special characters to prevent syntax errors
	escapedQuery := escapeFTS5Query(query)

	rows, err := m.db.QueryContext(ctx, `
		SELECT m.id, m.content, m.source
		FROM memory_fts f
		JOIN memories m ON f.id = m.id
		WHERE memory_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, escapedQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("search fts: %w", err)
	}
	defer rows.Close()

	var results []ScoredResult
	rank := 0
	for rows.Next() {
		var r ScoredResult
		if err := rows.Scan(&r.ID, &r.Content, &r.Source); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		// Use inverse rank as score (higher rank = better)
		r.Score = 1.0 / float64(rank+1)
		rank++
		results = append(results, r)
	}

	return results, rows.Err()
}

// mergeRRF merges vector and FTS results using Reciprocal Rank Fusion.
func (m *MemoryIndex) mergeRRF(vecResults, ftsResults []ScoredResult, opts SearchOptions) []SearchResult {
	rrfScores := make(map[string]float64)
	idToResult := make(map[string]ScoredResult)

	k := m.hybridConfig.RRFConstant
	if k == 0 {
		k = 60
	}
	vecWeight := opts.VectorWeight
	textWeight := 1.0 - vecWeight

	// Calculate RRF scores for vector results
	for rank, r := range vecResults {
		rrfScore := 1.0 / float64(k+rank+1)
		rrfScores[r.ID] += vecWeight * rrfScore
		idToResult[r.ID] = r
	}

	// Calculate RRF scores for FTS results
	for rank, r := range ftsResults {
		rrfScore := 1.0 / float64(k+rank+1)
		rrfScores[r.ID] += textWeight * rrfScore
		if _, exists := idToResult[r.ID]; !exists {
			idToResult[r.ID] = r
		}
	}

	// Build result list and filter by MinScore
	var results []SearchResult
	for id, score := range rrfScores {
		if score < opts.MinScore {
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
	sortSearchResults(results)

	// Limit to TopK
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	return results
}

// enrichSearchResults enriches search results with additional fields from the database.
// This is used after mergeRRF to add chunk metadata and other fields.
func (m *MemoryIndex) enrichSearchResults(ctx context.Context, results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// Build ID list for batch query
	ids := make([]interface{}, len(results))
	idToIdx := make(map[string]int)
	for i, r := range results {
		ids[i] = r.ID
		idToIdx[r.ID] = i
	}

	// Build placeholders
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT id, created_at, category, importance, capture_method, chunk_index, chunk_total, source_file
		FROM memories
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := m.db.QueryContext(ctx, query, ids...)
	if err != nil {
		m.logger.Warn().Err(err).Msg("failed to enrich search results")
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var createdAt string
		var category, captureMethod sql.NullString
		var importance sql.NullFloat64
		var chunkIndex, chunkTotal sql.NullInt64
		var sourceFile sql.NullString

		if err := rows.Scan(&id, &createdAt, &category, &importance, &captureMethod,
			&chunkIndex, &chunkTotal, &sourceFile); err != nil {
			m.logger.Warn().Err(err).Msg("failed to scan enrichment row")
			continue
		}

		if idx, ok := idToIdx[id]; ok {
			results[idx].CreatedAt = parseTimeFlexible(createdAt)
			if category.Valid {
				results[idx].Category = category.String
			}
			if importance.Valid {
				results[idx].Importance = importance.Float64
			}
			if captureMethod.Valid {
				results[idx].CaptureMethod = captureMethod.String
			}
			if chunkIndex.Valid {
				results[idx].ChunkIndex = int(chunkIndex.Int64)
			}
			if chunkTotal.Valid {
				results[idx].ChunkTotal = int(chunkTotal.Int64)
			}
			if sourceFile.Valid {
				results[idx].SourceFile = sourceFile.String
			}
		}
	}

	return results
}

// scoredToSearchResults converts ScoredResult slice to SearchResult slice.
func (m *MemoryIndex) scoredToSearchResults(scored []ScoredResult) []SearchResult {
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

// Helper functions for embedding serialization

// encodeEmbedding serializes a float32 slice to bytes.
func encodeEmbedding(embedding []float32) []byte {
	buf := make([]byte, len(embedding)*4)
	for i, v := range embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// decodeEmbedding deserializes bytes to a float32 slice.
func decodeEmbedding(data []byte) []float32 {
	if len(data)%4 != 0 {
		return nil
	}
	embedding := make([]float32, len(data)/4)
	for i := range embedding {
		embedding[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return embedding
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// sortByScore sorts ScoredResult slice by score descending.
func sortByScore(results []ScoredResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// sortSearchResults sorts SearchResult slice by score descending.
func sortSearchResults(results []SearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// =============================================================================
// P1 Methods: Markdown Store Integration
// =============================================================================

// SetMarkdownStore sets the markdown store for the memory index.
func (m *MemoryIndex) SetMarkdownStore(store *MarkdownStore) {
	m.markdownStore = store
}

// SyncFromMarkdown scans markdown files and rebuilds the index.
func (m *MemoryIndex) SyncFromMarkdown(ctx context.Context, force bool) (int, error) {
	if m.markdownStore == nil {
		return 0, fmt.Errorf("markdown store not configured")
	}

	// Get daily files
	files, err := m.markdownStore.ListDailyFiles()
	if err != nil {
		return 0, fmt.Errorf("list daily files: %w", err)
	}

	chunker := NewChunker(DefaultChunkerOptions())
	synced := 0

	// Process daily files
	for _, f := range files {
		content, err := m.markdownStore.GetDaily(f.Date)
		if err != nil {
			m.logger.Warn().Err(err).Str("file", f.Filename).Msg("failed to read daily file")
			continue
		}

		chunks := chunker.SplitBySection(content, f.Filename)
		for _, chunk := range chunks {
			entry := MemoryEntry{
				Content:         chunk.Content,
				Source:          "daily:" + f.Date.Format("2006-01-02"),
				SourceFile:      chunk.SourceFile,
				SourceLineStart: chunk.StartLine,
				SourceLineEnd:   chunk.EndLine,
				ChunkIndex:      chunk.Index,
				ChunkTotal:      len(chunks),
			}
			if err := m.Add(ctx, entry); err != nil {
				m.logger.Warn().Err(err).Str("file", f.Filename).Int("chunk", chunk.Index).Msg("failed to add chunk")
				continue
			}
			synced++
		}
	}

	// Process MEMORY.md
	memoryContent, err := m.markdownStore.GetMemory()
	if err == nil && memoryContent != "" {
		chunks := chunker.SplitBySection(memoryContent, "MEMORY.md")
		for _, chunk := range chunks {
			entry := MemoryEntry{
				Content:         chunk.Content,
				Source:          "memory",
				SourceFile:      "MEMORY.md",
				SourceLineStart: chunk.StartLine,
				SourceLineEnd:   chunk.EndLine,
				ChunkIndex:      chunk.Index,
				ChunkTotal:      len(chunks),
			}
			if err := m.Add(ctx, entry); err != nil {
				m.logger.Warn().Err(err).Int("chunk", chunk.Index).Msg("failed to add MEMORY.md chunk")
				continue
			}
			synced++
		}
	}

	m.logger.Info().Int("synced", synced).Msg("synced from markdown")
	return synced, nil
}

// GetDailyLog gets the daily log content for a specific date.
func (m *MemoryIndex) GetDailyLog(ctx context.Context, date time.Time) (string, error) {
	if m.markdownStore == nil {
		return "", fmt.Errorf("markdown store not configured")
	}
	return m.markdownStore.GetDaily(date)
}

// AppendDailyLog appends content to today's daily log.
func (m *MemoryIndex) AppendDailyLog(ctx context.Context, content, section string) error {
	if m.markdownStore == nil {
		return fmt.Errorf("markdown store not configured")
	}
	return m.markdownStore.AppendDaily(content, section)
}
