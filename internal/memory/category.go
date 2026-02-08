package memory

import (
	"regexp"
	"strings"
)

// CategoryDetector detects the category of a memory entry based on its content.
type CategoryDetector struct {
	patterns map[string]*regexp.Regexp
	order    []string // Priority order for pattern matching
}

// DefaultCategoryPatterns defines regex patterns for each category.
// These patterns support both English and Chinese text.
var DefaultCategoryPatterns = map[string]string{
	CategoryPreference: `(?i)\b(prefer|like|love|hate|want|need|favorite|enjoy|dislike)\b|喜欢|讨厌|偏好|最爱|喜爱|厌恶|想要|不喜欢`,
	CategoryDecision:   `(?i)\b(decided|will use|chose|choose|going to|plan to|determined)\b|我们决定|确定使用|选择了|决定了|计划`,
	CategoryEntity:     `\+\d{10,}|[\w.-]+@[\w.-]+\.\w+|(?i)\b(is called|named|my .+ is)\b|叫做|名字是|我的.+是|他的.+是|她的.+是`,
	CategoryFact:       `(?i)\b(is|are|was|were|has|have|had)\b.{5,}|是|有|存在|位于|属于`,
}

// DefaultCategoryOrder defines the priority order for category detection.
// Higher priority categories are checked first.
var DefaultCategoryOrder = []string{
	CategoryEntity,     // Entity patterns are most specific
	CategoryPreference, // Preference patterns are fairly specific
	CategoryDecision,   // Decision patterns
	CategoryFact,       // Fact patterns are very broad, check last
}

// NewCategoryDetector creates a new CategoryDetector with default patterns.
func NewCategoryDetector() (*CategoryDetector, error) {
	return NewCategoryDetectorWithPatterns(DefaultCategoryPatterns, DefaultCategoryOrder)
}

// NewCategoryDetectorWithPatterns creates a CategoryDetector with custom patterns.
func NewCategoryDetectorWithPatterns(patterns map[string]string, order []string) (*CategoryDetector, error) {
	compiled := make(map[string]*regexp.Regexp, len(patterns))

	for name, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compiled[name] = re
	}

	// Use provided order or fall back to default
	if len(order) == 0 {
		order = DefaultCategoryOrder
	}

	return &CategoryDetector{
		patterns: compiled,
		order:    order,
	}, nil
}

// Detect analyzes the text and returns the detected category.
// Returns CategoryOther if no patterns match.
func (d *CategoryDetector) Detect(text string) string {
	if text == "" {
		return CategoryOther
	}

	// Normalize text for matching
	normalized := strings.TrimSpace(text)

	// Check patterns in priority order
	for _, category := range d.order {
		pattern, ok := d.patterns[category]
		if !ok {
			continue
		}
		if pattern.MatchString(normalized) {
			return category
		}
	}

	return CategoryOther
}

// DetectWithConfidence returns the category and a confidence score.
// The confidence is based on the number of pattern matches found.
func (d *CategoryDetector) DetectWithConfidence(text string) (string, float64) {
	if text == "" {
		return CategoryOther, 0.0
	}

	normalized := strings.TrimSpace(text)
	bestCategory := CategoryOther
	bestScore := 0.0

	for _, category := range d.order {
		pattern, ok := d.patterns[category]
		if !ok {
			continue
		}

		matches := pattern.FindAllString(normalized, -1)
		if len(matches) > 0 {
			// Score based on number of matches and text length ratio
			score := float64(len(matches)) / float64(len(strings.Fields(normalized))+1)
			if score > 1.0 {
				score = 1.0
			}
			// Boost score for earlier categories (higher priority)
			priorityBoost := 0.1 * float64(len(d.order)-indexOf(d.order, category)) / float64(len(d.order))
			score += priorityBoost

			if score > bestScore {
				bestScore = score
				bestCategory = category
			}
		}
	}

	return bestCategory, bestScore
}

// indexOf returns the index of a string in a slice, or -1 if not found.
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
