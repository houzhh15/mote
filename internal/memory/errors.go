// Package memory provides vector-based memory storage and retrieval.
package memory

import (
	"errors"
	"fmt"
)

// Memory errors.
var (
	// ErrEmbedFailed indicates that embedding generation failed.
	ErrEmbedFailed = errors.New("memory: embedding failed")

	// ErrVecNotLoaded indicates that sqlite-vec extension is not loaded.
	ErrVecNotLoaded = errors.New("memory: sqlite-vec extension not loaded")

	// ErrEntryNotFound indicates that the memory entry was not found.
	ErrEntryNotFound = errors.New("memory: entry not found")

	// ErrInvalidDims indicates that vector dimensions don't match.
	ErrInvalidDims = errors.New("memory: vector dimensions mismatch")

	// P0: Additional error types for memory operations
	// ErrMemoryNotFound indicates that the requested memory entry was not found.
	ErrMemoryNotFound = errors.New("memory: memory not found")

	// ErrEmbeddingFailed indicates that the embedding API call failed.
	ErrEmbeddingFailed = errors.New("memory: embedding API failed")

	// ErrTokenExpired indicates that the authentication token has expired.
	ErrTokenExpired = errors.New("memory: token expired")

	// ErrRateLimited indicates that the API rate limit was exceeded.
	ErrRateLimited = errors.New("memory: rate limited")

	// ErrInvalidChunk indicates that the chunk is invalid.
	ErrInvalidChunk = errors.New("memory: invalid chunk")

	// ErrIndexCorrupted indicates that the memory index is corrupted.
	ErrIndexCorrupted = errors.New("memory: index corrupted")

	// ErrInvalidRule indicates that the gating rule is invalid.
	ErrInvalidRule = errors.New("memory: invalid gating rule")

	// ErrRuleNotFound indicates that the gating rule was not found.
	ErrRuleNotFound = errors.New("memory: gating rule not found")

	// ErrWriteDenied indicates that the memory write was denied by a rule.
	ErrWriteDenied = errors.New("memory: write denied by gating rule")
)

// MemoryError represents an error with context about the memory operation.
type MemoryError struct {
	Op  string // Operation name (e.g., "search", "add", "get")
	ID  string // Related memory ID (if applicable)
	Err error  // Underlying error
}

// Error implements the error interface.
func (e *MemoryError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s [%s]: %v", e.Op, e.ID, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error.
func (e *MemoryError) Unwrap() error {
	return e.Err
}
