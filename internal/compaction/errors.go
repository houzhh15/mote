// Package compaction provides history compression for conversation context.
package compaction

import "errors"

// Compaction errors.
var (
	// ErrSummaryFailed indicates that summary generation failed.
	ErrSummaryFailed = errors.New("compaction: summary generation failed")

	// ErrNoProvider indicates that no provider is configured for summarization.
	ErrNoProvider = errors.New("compaction: provider not configured")

	// ErrMessagesTooShort indicates that there are not enough messages to compact.
	ErrMessagesTooShort = errors.New("compaction: not enough messages to compact")
)
