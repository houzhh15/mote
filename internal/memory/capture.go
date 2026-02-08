package memory

import (
	"context"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
)

// CaptureConfig holds configuration for the auto-capture engine.
type CaptureConfig struct {
	Enabled       bool    `json:"enabled"`         // Enable auto-capture
	MinLength     int     `json:"min_length"`      // Minimum text length to consider
	MaxLength     int     `json:"max_length"`      // Maximum text length to consider
	DupThreshold  float64 `json:"dup_threshold"`   // Similarity threshold for deduplication
	MaxPerSession int     `json:"max_per_session"` // Maximum captures per session
}

// DefaultCaptureConfig returns a CaptureConfig with default values.
func DefaultCaptureConfig() CaptureConfig {
	return CaptureConfig{
		Enabled:       true,
		MinLength:     10,
		MaxLength:     500,
		DupThreshold:  0.95,
		MaxPerSession: 3,
	}
}

// CaptureEngine handles automatic memory capture from conversations.
type CaptureEngine struct {
	triggers  []*regexp.Regexp
	memory    *MemoryIndex
	detector  *CategoryDetector
	config    CaptureConfig
	logger    zerolog.Logger
	sessionID string
	captured  int // Count of captures in current session
}

// DefaultCapturePatterns defines regex patterns that trigger auto-capture.
var DefaultCapturePatterns = []string{
	// Explicit memory commands
	`(?i)\b(remember|don't forget|note that)\b|记住|别忘了|记下来`,
	// Preference expressions
	`(?i)\b(prefer|like|love|hate|want|need|favorite)\b|喜欢|讨厌|偏好|最爱`,
	// Decision statements
	`(?i)\b(decided|will use|chose|going to)\b|决定|确定使用|选择了`,
	// Contact information
	`\+\d{10,}`,            // Phone numbers
	`[\w.-]+@[\w.-]+\.\w+`, // Email addresses
	// Entity declarations
	`(?i)\b(is called|named|my .+ is)\b|叫做|名字是|我的.+是`,
	// Emphasis words indicating importance
	`(?i)\b(always|never|important|must|crucial)\b|必须|绝对|重要|一定`,
}

// CaptureEngineOptions holds options for creating a CaptureEngine.
type CaptureEngineOptions struct {
	Memory   *MemoryIndex
	Detector *CategoryDetector
	Config   CaptureConfig
	Logger   zerolog.Logger
	Patterns []string // Custom patterns (optional, uses defaults if empty)
}

// NewCaptureEngine creates a new CaptureEngine.
func NewCaptureEngine(opts CaptureEngineOptions) (*CaptureEngine, error) {
	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = DefaultCapturePatterns
	}

	triggers := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, re)
	}

	// Create default detector if not provided
	detector := opts.Detector
	if detector == nil {
		var err error
		detector, err = NewCategoryDetector()
		if err != nil {
			return nil, err
		}
	}

	// Apply default config if not set
	config := opts.Config
	if config.MinLength == 0 {
		config = DefaultCaptureConfig()
		config.Enabled = opts.Config.Enabled // Preserve enabled flag
	}

	return &CaptureEngine{
		triggers: triggers,
		memory:   opts.Memory,
		detector: detector,
		config:   config,
		logger:   opts.Logger,
	}, nil
}

// ShouldCapture determines if the text should be auto-captured.
func (e *CaptureEngine) ShouldCapture(text string) bool {
	if !e.config.Enabled {
		return false
	}

	// Length check
	textLen := len(text)
	if textLen < e.config.MinLength || textLen > e.config.MaxLength {
		return false
	}

	// Exclude recalled memory context (prevent re-capturing injected memories)
	if strings.Contains(text, "<relevant-memories>") {
		return false
	}

	// Exclude XML-wrapped content (system/tool responses)
	if isXMLContent(text) {
		return false
	}

	// Exclude heavily formatted markdown (likely agent responses, not user input)
	if isFormattedMarkdown(text) {
		return false
	}

	// Check trigger patterns
	for _, pattern := range e.triggers {
		if pattern.MatchString(text) {
			return true
		}
	}

	return false
}

// Message represents a conversation message for capture processing.
type Message struct {
	Role    string `json:"role"`    // "user", "assistant", "system"
	Content string `json:"content"` // Message content
}

// Capture processes messages and captures relevant memories.
// Returns the number of memories captured.
func (e *CaptureEngine) Capture(ctx context.Context, messages []Message) (int, error) {
	if !e.config.Enabled || e.memory == nil {
		return 0, nil
	}

	captured := 0
	for _, msg := range messages {
		// Only process user and assistant messages
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}

		// Check session limit
		if e.captured+captured >= e.config.MaxPerSession {
			e.logger.Debug().
				Int("captured", captured).
				Int("limit", e.config.MaxPerSession).
				Msg("reached session capture limit")
			break
		}

		// Check if should capture
		if !e.ShouldCapture(msg.Content) {
			continue
		}

		// Detect category
		category := e.detector.Detect(msg.Content)

		// Check for duplicates
		isDup, err := e.isDuplicate(ctx, msg.Content)
		if err != nil {
			e.logger.Warn().Err(err).Msg("duplicate check failed")
			continue
		}
		if isDup {
			e.logger.Debug().Str("content", truncate(msg.Content, 50)).Msg("skipping duplicate")
			continue
		}

		// Create and store memory entry
		entry := MemoryEntry{
			Content:       msg.Content,
			Source:        "auto:" + msg.Role,
			Category:      category,
			Importance:    DefaultImportance,
			CaptureMethod: CaptureMethodAuto,
		}

		if err := e.memory.Add(ctx, entry); err != nil {
			e.logger.Warn().Err(err).Msg("failed to capture memory")
			continue
		}

		captured++
		e.logger.Info().
			Str("category", category).
			Str("content", truncate(msg.Content, 50)).
			Msg("auto-captured memory")
	}

	e.captured += captured
	return captured, nil
}

// ResetSession resets the session capture counter.
func (e *CaptureEngine) ResetSession() {
	e.captured = 0
}

// SetSessionID sets the current session ID.
func (e *CaptureEngine) SetSessionID(id string) {
	e.sessionID = id
	e.captured = 0
}

// GetMemoryIndex returns the underlying memory index for direct access.
func (e *CaptureEngine) GetMemoryIndex() *MemoryIndex {
	return e.memory
}

// isDuplicate checks if the content is similar to existing memories.
func (e *CaptureEngine) isDuplicate(ctx context.Context, content string) (bool, error) {
	// Use search to find similar content
	results, err := e.memory.Search(ctx, content, 1)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, nil
	}

	// For now, use simple string similarity
	// TODO: Use embedding similarity when available
	similarity := stringSimilarity(content, results[0].Content)
	return similarity >= e.config.DupThreshold, nil
}

// isXMLContent checks if text appears to be XML-wrapped content.
func isXMLContent(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 10 {
		return false
	}
	// Check for opening and closing XML-style tags
	if strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, "</") {
		return true
	}
	return false
}

// isFormattedMarkdown checks if text is heavily formatted markdown.
func isFormattedMarkdown(text string) bool {
	// Check for common markdown formatting patterns
	markdownPatterns := []string{
		"**",  // Bold
		"```", // Code blocks
		"- ",  // List items
		"1. ", // Numbered lists
		"## ", // Headers
	}

	count := 0
	for _, p := range markdownPatterns {
		if strings.Contains(text, p) {
			count++
		}
	}

	// If more than 2 different markdown patterns, likely a formatted response
	return count >= 2
}

// stringSimilarity calculates a simple Jaccard similarity between two strings.
func stringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	// Create word sets
	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}
	setB := make(map[string]bool)
	for _, w := range wordsB {
		setB[w] = true
	}

	// Calculate intersection
	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	// Calculate union
	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// truncate shortens a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
