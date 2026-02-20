package memory

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// BM25Scorer implements BM25 scoring for text search.
// It reuses SQLite FTS5 tokenization data to compute BM25 relevance scores.
type BM25Scorer struct {
	k1        float64 // Term frequency saturation parameter
	b         float64 // Document length normalization parameter
	db        *sql.DB
	avgDocLen float64 // Average document length (cached)
	docCount  int     // Total document count (cached)
	mu        sync.RWMutex
}

// NewBM25Scorer creates a new BM25Scorer with the given configuration.
func NewBM25Scorer(db *sql.DB, config BM25Config) *BM25Scorer {
	if config.K1 == 0 {
		config.K1 = 1.2
	}
	if config.B == 0 {
		config.B = 0.75
	}

	scorer := &BM25Scorer{
		k1: config.K1,
		b:  config.B,
		db: db,
	}

	// Initialize stats (best effort)
	_ = scorer.UpdateStats()

	return scorer
}

// Score computes BM25 scores for the given query and returns top-K results.
func (s *BM25Scorer) Score(query string, topK int) ([]ScoredResult, error) {
	s.mu.RLock()
	avgDocLen := s.avgDocLen
	docCount := s.docCount
	s.mu.RUnlock()

	if docCount == 0 {
		return nil, nil
	}

	// Tokenize query
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}

	// Get document frequencies for each term
	dfMap, err := s.getDocumentFrequencies(terms)
	if err != nil {
		return nil, fmt.Errorf("bm25: get document frequencies: %w", err)
	}

	// Get all documents with their content lengths
	docs, err := s.getDocuments()
	if err != nil {
		return nil, fmt.Errorf("bm25: get documents: %w", err)
	}

	// Compute BM25 score for each document
	var results []ScoredResult
	for _, doc := range docs {
		score := s.computeScore(terms, doc, dfMap, avgDocLen, docCount)
		if score > 0 {
			results = append(results, ScoredResult{
				ID:      doc.id,
				Content: doc.content,
				Source:  doc.source,
				Score:   score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to topK
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// UpdateStats refreshes the cached document statistics (doc count and avg doc length).
func (s *BM25Scorer) UpdateStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	var totalLen sql.NullFloat64

	err := s.db.QueryRow(`SELECT COUNT(*), AVG(LENGTH(content)) FROM memories`).Scan(&count, &totalLen)
	if err != nil {
		return fmt.Errorf("bm25: update stats: %w", err)
	}

	s.docCount = count
	if totalLen.Valid {
		s.avgDocLen = totalLen.Float64
	} else {
		s.avgDocLen = 0
	}

	return nil
}

// --- internal types ---

type bm25Doc struct {
	id      string
	content string
	source  string
	docLen  int
}

// --- internal methods ---

// computeScore calculates BM25 score for a single document.
func (s *BM25Scorer) computeScore(terms []string, doc bm25Doc, dfMap map[string]int, avgDocLen float64, n int) float64 {
	score := 0.0

	for _, term := range terms {
		df := dfMap[term]
		if df == 0 {
			continue
		}

		// IDF component
		idf := math.Log(1 + (float64(n)-float64(df)+0.5)/(float64(df)+0.5))

		// Term frequency in document
		tf := countTermOccurrences(doc.content, term)
		if tf == 0 {
			continue
		}

		// BM25 formula
		tfNorm := (float64(tf) * (s.k1 + 1)) /
			(float64(tf) + s.k1*(1-s.b+s.b*float64(doc.docLen)/avgDocLen))

		score += idf * tfNorm
	}

	return score
}

// getDocumentFrequencies returns the number of documents containing each term.
func (s *BM25Scorer) getDocumentFrequencies(terms []string) (map[string]int, error) {
	dfMap := make(map[string]int)

	for _, term := range terms {
		var count int
		// Use LIKE for term matching (works with any text, including Chinese)
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM memories WHERE content LIKE ?`,
			"%"+term+"%",
		).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("count term %q: %w", term, err)
		}
		dfMap[term] = count
	}

	return dfMap, nil
}

// getDocuments retrieves all documents for BM25 scoring.
func (s *BM25Scorer) getDocuments() ([]bm25Doc, error) {
	rows, err := s.db.Query(`SELECT id, content, source FROM memories`)
	if err != nil {
		return nil, fmt.Errorf("query documents: %w", err)
	}
	defer rows.Close()

	var docs []bm25Doc
	for rows.Next() {
		var doc bm25Doc
		if err := rows.Scan(&doc.id, &doc.content, &doc.source); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		doc.docLen = len(doc.content)
		docs = append(docs, doc)
	}

	return docs, rows.Err()
}

// tokenize splits text into searchable tokens.
// Handles both ASCII and CJK text by splitting on whitespace and
// treating runs of ASCII/CJK characters as separate tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)

	// Split by whitespace first
	words := strings.Fields(text)

	var tokens []string
	for _, word := range words {
		// For mixed ASCII/CJK words, split into segments
		var current strings.Builder
		var lastIsCJK bool

		for _, r := range word {
			isCJK := isCJKChar(r)

			if current.Len() > 0 && isCJK != lastIsCJK {
				tokens = append(tokens, current.String())
				current.Reset()
			}

			current.WriteRune(r)
			lastIsCJK = isCJK
		}

		if current.Len() > 0 {
			tokens = append(tokens, current.String())
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, t := range tokens {
		if !seen[t] && len(t) > 0 {
			seen[t] = true
			unique = append(unique, t)
		}
	}

	return unique
}

// isCJKChar checks if a rune is a CJK character.
func isCJKChar(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) // Katakana
}

// countTermOccurrences counts how many times a term appears in text (case-insensitive).
func countTermOccurrences(text, term string) int {
	text = strings.ToLower(text)
	term = strings.ToLower(term)
	return strings.Count(text, term)
}
