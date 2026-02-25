package memory

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// RecallConfig holds configuration for the auto-recall engine.
type RecallConfig struct {
	Enabled      bool    `json:"enabled"`        // Enable auto-recall
	Limit        int     `json:"limit"`          // Maximum memories to recall
	Threshold    float64 `json:"threshold"`      // Minimum similarity threshold
	MinPromptLen int     `json:"min_prompt_len"` // Minimum prompt length to trigger recall
}

// DefaultRecallConfig returns a RecallConfig with default values.
func DefaultRecallConfig() RecallConfig {
	return RecallConfig{
		Enabled:      true,
		Limit:        3,
		Threshold:    0.3,
		MinPromptLen: 5,
	}
}

// RecallEngine handles automatic memory recall for context injection.
type RecallEngine struct {
	memory      *MemoryIndex
	config      RecallConfig
	logger      zerolog.Logger
	recallCount atomic.Int64 // Daily recall counter
	recallDate  string       // Date string for counter reset (YYYY-MM-DD)
}

// RecallEngineOptions holds options for creating a RecallEngine.
type RecallEngineOptions struct {
	Memory *MemoryIndex
	Config RecallConfig
	Logger zerolog.Logger
}

// NewRecallEngine creates a new RecallEngine.
func NewRecallEngine(opts RecallEngineOptions) *RecallEngine {
	config := opts.Config
	if config.Limit == 0 {
		config = DefaultRecallConfig()
		config.Enabled = opts.Config.Enabled // Preserve enabled flag
	}

	return &RecallEngine{
		memory: opts.Memory,
		config: config,
		logger: opts.Logger,
	}
}

// Recall searches for relevant memories and returns formatted context.
// Returns empty string if no relevant memories are found or conditions aren't met.
func (e *RecallEngine) Recall(ctx context.Context, prompt string) (string, error) {
	// Check if enabled
	if !e.config.Enabled {
		return "", nil
	}

	// Check minimum prompt length
	if len(prompt) < e.config.MinPromptLen {
		e.logger.Debug().
			Int("prompt_len", len(prompt)).
			Int("min_len", e.config.MinPromptLen).
			Msg("prompt too short for recall")
		return "", nil
	}

	// Check if memory index is available
	if e.memory == nil {
		return "", nil
	}

	// Search for relevant memories
	results, err := e.memory.Search(ctx, prompt, e.config.Limit)
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}

	// Filter by threshold and build context
	var relevant []SearchResult
	for _, r := range results {
		if r.Score >= e.config.Threshold {
			relevant = append(relevant, r)
		}
	}

	if len(relevant) == 0 {
		e.logger.Debug().
			Str("prompt", truncate(prompt, 50)).
			Msg("no relevant memories found")
		return "", nil
	}

	// Format memories as XML context
	context := e.formatMemories(relevant)

	e.logger.Info().
		Int("count", len(relevant)).
		Str("prompt", truncate(prompt, 50)).
		Msg("recalled memories")

	// Track daily recall count
	e.incrementRecallCount()

	return context, nil
}

// incrementRecallCount increments the daily recall counter, resetting on date change.
func (e *RecallEngine) incrementRecallCount() {
	today := time.Now().Format("2006-01-02")
	if e.recallDate != today {
		e.recallCount.Store(1)
		e.recallDate = today
	} else {
		e.recallCount.Add(1)
	}
}

// TodayRecallCount returns the number of recalls that happened today.
func (e *RecallEngine) TodayRecallCount() int64 {
	today := time.Now().Format("2006-01-02")
	if e.recallDate != today {
		return 0
	}
	return e.recallCount.Load()
}

// formatMemories formats search results as XML context for injection.
func (e *RecallEngine) formatMemories(memories []SearchResult) string {
	var sb strings.Builder

	sb.WriteString("<relevant-memories>\n")
	sb.WriteString("The following memories may be relevant to the current conversation:\n")

	for _, m := range memories {
		category := m.Category
		if category == "" {
			category = CategoryOther
		}
		fmt.Fprintf(&sb, "- [%s] %s\n", category, m.Content)
	}

	sb.WriteString("</relevant-memories>")

	return sb.String()
}

// RecallWithFilter recalls memories filtered by category.
func (e *RecallEngine) RecallWithFilter(ctx context.Context, prompt string, categories []string) (string, error) {
	if !e.config.Enabled || e.memory == nil {
		return "", nil
	}

	if len(prompt) < e.config.MinPromptLen {
		return "", nil
	}

	// Search with higher limit to allow for filtering
	results, err := e.memory.Search(ctx, prompt, e.config.Limit*2)
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}

	// Filter by categories
	categorySet := make(map[string]bool)
	for _, c := range categories {
		categorySet[c] = true
	}

	var relevant []SearchResult
	for _, r := range results {
		if r.Score < e.config.Threshold {
			continue
		}
		if len(categories) > 0 && !categorySet[r.Category] {
			continue
		}
		relevant = append(relevant, r)
		if len(relevant) >= e.config.Limit {
			break
		}
	}

	if len(relevant) == 0 {
		return "", nil
	}

	return e.formatMemories(relevant), nil
}

// GetConfig returns the current recall configuration.
func (e *RecallEngine) GetConfig() RecallConfig {
	return e.config
}

// SetEnabled enables or disables the recall engine.
func (e *RecallEngine) SetEnabled(enabled bool) {
	e.config.Enabled = enabled
}
