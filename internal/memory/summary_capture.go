package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// SummaryCapture handles automatic memory capture via LLM session summaries.
// It replaces the regex-based CaptureEngine with a more intelligent approach.
type SummaryCapture struct {
	provider LLMProvider
	content  *ContentStore
	config   SummaryCaptureConfig
	logger   zerolog.Logger
}

// SummaryCaptureOptions holds options for creating a SummaryCapture.
type SummaryCaptureOptions struct {
	Provider LLMProvider
	Content  *ContentStore
	Config   SummaryCaptureConfig
	Logger   zerolog.Logger
}

// NewSummaryCapture creates a new SummaryCapture.
func NewSummaryCapture(opts SummaryCaptureOptions) (*SummaryCapture, error) {
	if opts.Provider == nil {
		return nil, fmt.Errorf("summary_capture: LLMProvider is required")
	}

	config := opts.Config
	if config.MinMessages == 0 {
		config = DefaultSummaryCaptureConfig()
		config.Enabled = opts.Config.Enabled
	}

	return &SummaryCapture{
		provider: opts.Provider,
		content:  opts.Content,
		config:   config,
		logger:   opts.Logger,
	}, nil
}

// OnSessionEnd processes a completed session and extracts memories.
// Returns the extracted memory entries (already written to ContentStore if available).
func (sc *SummaryCapture) OnSessionEnd(ctx context.Context, sessionID string, messages []Message) ([]MemoryEntry, error) {
	if !sc.config.Enabled {
		return nil, nil
	}

	// Check minimum message count
	if len(messages) < sc.config.MinMessages {
		sc.logger.Debug().
			Int("messages", len(messages)).
			Int("minRequired", sc.config.MinMessages).
			Msg("summary_capture: not enough messages, skipping")
		return nil, nil
	}

	// Generate summary via LLM
	result, err := sc.generateSummary(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("summary_capture: generate summary: %w", err)
	}

	if result == nil || len(result.Entries) == 0 {
		return nil, nil
	}

	// Limit entries per session
	entries := result.Entries
	if len(entries) > sc.config.MaxPerSession {
		entries = entries[:sc.config.MaxPerSession]
	}

	// Convert to MemoryEntry and optionally write to ContentStore
	var memEntries []MemoryEntry
	for _, se := range entries {
		entry := MemoryEntry{
			ID:            uuid.New().String(),
			Content:       se.Content,
			Source:        SourceConversation,
			SessionID:     sessionID,
			Category:      se.Category,
			Importance:    se.Importance,
			CaptureMethod: CaptureMethodAuto,
			CreatedAt:     time.Now(),
		}

		// Validate category
		if !isValidCategory(entry.Category) {
			entry.Category = CategoryOther
		}

		// Validate importance
		if entry.Importance <= 0 || entry.Importance > 1.0 {
			entry.Importance = DefaultImportance
		}

		memEntries = append(memEntries, entry)

		// Write to ContentStore if available
		if sc.content != nil {
			section := Section{
				ID:       sectionID("MEMORY.md", entry.Content[:min(40, len(entry.Content))]),
				FilePath: "MEMORY.md",
				Heading:  categoryHeading(entry.Category),
				Content:  entry.Content,
				Metadata: SectionMetadata{
					Category:   entry.Category,
					Importance: entry.Importance,
				},
			}
			if err := sc.content.UpsertSection(section); err != nil {
				sc.logger.Warn().Err(err).Str("id", entry.ID).Msg("summary_capture: failed to write to content store")
			}
		}
	}

	sc.logger.Info().
		Str("sessionID", sessionID).
		Int("captured", len(memEntries)).
		Msg("summary_capture: session summary captured")

	return memEntries, nil
}

// generateSummary calls the LLM to extract key memories from conversation.
func (sc *SummaryCapture) generateSummary(ctx context.Context, messages []Message) (*SummaryResult, error) {
	// Build conversation text
	var conv strings.Builder
	for _, m := range messages {
		conv.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	prompt := fmt.Sprintf(`Please analyze the following conversation and extract key information worth remembering long-term.
For each piece of information, provide:
- content: The key information (concise, 1-2 sentences)
- category: One of "preference", "fact", "decision", "entity", "other"
- importance: A float between 0.0 and 1.0 (1.0 = very important)

Return ONLY a JSON array of objects. Example:
[{"content": "User prefers TypeScript", "category": "preference", "importance": 0.8}]

If no important information is found, return an empty array: []

Conversation:
%s`, conv.String())

	response, err := sc.provider.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	// Parse JSON response
	response = extractJSON(response)
	var entries []SummaryEntry
	if err := json.Unmarshal([]byte(response), &entries); err != nil {
		sc.logger.Warn().
			Err(err).
			Str("response", response[:min(200, len(response))]).
			Msg("summary_capture: failed to parse LLM response")
		return &SummaryResult{}, nil
	}

	return &SummaryResult{Entries: entries}, nil
}

// extractJSON extracts a JSON array from a potentially markdown-wrapped response.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Try to find JSON array
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return s
}

// isValidCategory checks if a category string is valid.
func isValidCategory(cat string) bool {
	switch cat {
	case CategoryPreference, CategoryFact, CategoryDecision, CategoryEntity, CategoryOther:
		return true
	}
	return false
}

// categoryHeading returns the Markdown heading for a memory category.
func categoryHeading(category string) string {
	switch category {
	case CategoryPreference:
		return "User Preferences"
	case CategoryFact:
		return "Facts"
	case CategoryDecision:
		return "Decisions"
	case CategoryEntity:
		return "Entities"
	default:
		return "Other"
	}
}
