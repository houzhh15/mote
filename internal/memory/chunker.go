package memory

import (
	"regexp"
	"strings"
)

// Chunk represents a text segment suitable for embedding
type Chunk struct {
	Content    string // Chunk content
	StartLine  int    // Starting line number (1-based)
	EndLine    int    // Ending line number (1-based)
	SourceFile string // Source file path
	Index      int    // Chunk index within the source
}

// Chunker splits text into chunks suitable for vector embedding
type Chunker struct {
	targetTokens  int // Target chunk size in tokens (default 400)
	overlapTokens int // Overlap tokens between chunks (default 80)
}

// ChunkerOptions configures the Chunker
type ChunkerOptions struct {
	TargetTokens  int // Default 400
	OverlapTokens int // Default 80
}

// DefaultChunkerOptions returns the default chunker configuration
func DefaultChunkerOptions() ChunkerOptions {
	return ChunkerOptions{
		TargetTokens:  400,
		OverlapTokens: 80,
	}
}

// NewChunker creates a new Chunker with the given options
func NewChunker(opts ChunkerOptions) *Chunker {
	if opts.TargetTokens <= 0 {
		opts.TargetTokens = 400
	}
	if opts.OverlapTokens <= 0 {
		opts.OverlapTokens = 80
	}
	if opts.OverlapTokens >= opts.TargetTokens {
		opts.OverlapTokens = opts.TargetTokens / 5
	}

	return &Chunker{
		targetTokens:  opts.TargetTokens,
		overlapTokens: opts.OverlapTokens,
	}
}

// Split splits text into chunks based on paragraphs
func (c *Chunker) Split(text string, sourceFile string) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// Split by double newlines (paragraphs)
	paragraphs := splitParagraphs(text)
	if len(paragraphs) == 0 {
		return nil
	}

	var chunks []Chunk
	var currentContent strings.Builder
	var currentTokens int
	startLine := 1
	currentLine := 1

	for _, para := range paragraphs {
		paraTokens := c.countTokens(para.content)
		paraLines := para.lines

		// If adding this paragraph exceeds target and we have content
		if currentTokens+paraTokens > c.targetTokens && currentTokens > 0 {
			// Save current chunk
			chunks = append(chunks, Chunk{
				Content:    strings.TrimSpace(currentContent.String()),
				StartLine:  startLine,
				EndLine:    currentLine - 1,
				SourceFile: sourceFile,
				Index:      len(chunks),
			})

			// Calculate overlap: get last N tokens worth of content
			overlap := c.getOverlap(currentContent.String())
			currentContent.Reset()
			currentContent.WriteString(overlap)
			currentTokens = c.countTokens(overlap)
			startLine = currentLine
		}

		// Add paragraph to current chunk
		if currentContent.Len() > 0 {
			currentContent.WriteString("\n\n")
		}
		currentContent.WriteString(para.content)
		currentTokens += paraTokens
		currentLine += paraLines
	}

	// Save final chunk
	if currentContent.Len() > 0 {
		chunks = append(chunks, Chunk{
			Content:    strings.TrimSpace(currentContent.String()),
			StartLine:  startLine,
			EndLine:    currentLine,
			SourceFile: sourceFile,
			Index:      len(chunks),
		})
	}

	return chunks
}

// SplitBySection splits text by Markdown section headers
func (c *Chunker) SplitBySection(text string, sourceFile string) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// Regex to match markdown headers (# ## ### etc.)
	headerRegex := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	matches := headerRegex.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		// No headers found, treat as single chunk or use regular split
		return c.Split(text, sourceFile)
	}

	var chunks []Chunk
	lines := strings.Split(text, "\n")

	for i, match := range matches {
		sectionStart := match[0]
		var sectionEnd int
		if i+1 < len(matches) {
			sectionEnd = matches[i+1][0]
		} else {
			sectionEnd = len(text)
		}

		sectionContent := strings.TrimSpace(text[sectionStart:sectionEnd])
		if sectionContent == "" {
			continue
		}

		// Calculate line numbers
		startLine := countNewlines(text[:sectionStart]) + 1
		endLine := countNewlines(text[:sectionEnd])
		if endLine < startLine {
			endLine = startLine
		}

		sectionTokens := c.countTokens(sectionContent)

		// If section is too large, split it further
		if sectionTokens > c.targetTokens {
			subChunks := c.Split(sectionContent, sourceFile)
			for _, sub := range subChunks {
				sub.Index = len(chunks)
				sub.StartLine = startLine + sub.StartLine - 1
				sub.EndLine = startLine + sub.EndLine - 1
				chunks = append(chunks, sub)
			}
		} else {
			chunks = append(chunks, Chunk{
				Content:    sectionContent,
				StartLine:  startLine,
				EndLine:    endLine,
				SourceFile: sourceFile,
				Index:      len(chunks),
			})
		}
	}

	// Handle content before first header if any
	if len(matches) > 0 && matches[0][0] > 0 {
		preContent := strings.TrimSpace(text[:matches[0][0]])
		if preContent != "" {
			preChunk := Chunk{
				Content:    preContent,
				StartLine:  1,
				EndLine:    countNewlines(text[:matches[0][0]]),
				SourceFile: sourceFile,
				Index:      0,
			}
			// Prepend to chunks and renumber
			newChunks := make([]Chunk, len(chunks)+1)
			newChunks[0] = preChunk
			for i, ch := range chunks {
				ch.Index = i + 1
				newChunks[i+1] = ch
			}
			chunks = newChunks
		}
	}

	// Renumber all chunk indices
	for i := range chunks {
		chunks[i].Index = i
	}

	// Ensure line number accuracy based on actual lines array
	_ = lines // Reserved for future line validation

	return chunks
}

// countTokens estimates token count (approximately 4 chars per token)
func (c *Chunker) countTokens(text string) int {
	// Simple estimation: ~4 characters per token for English
	// For CJK characters, ~1.5 chars per token
	// We use a weighted average
	runeCount := len([]rune(text))
	byteCount := len(text)

	// If mostly ASCII, use char/4, otherwise use rune count
	if byteCount > 0 && float64(runeCount)/float64(byteCount) < 0.5 {
		// Likely CJK-heavy, use rune count * 0.7
		return int(float64(runeCount) * 0.7)
	}
	return (byteCount + 3) / 4
}

// getOverlap extracts the last overlapTokens worth of content
func (c *Chunker) getOverlap(text string) string {
	targetChars := c.overlapTokens * 4 // Approximate chars for overlap

	if len(text) <= targetChars {
		return text
	}

	// Find a good break point (paragraph or sentence)
	start := len(text) - targetChars

	// Try to find paragraph break
	if idx := strings.LastIndex(text[:start+1], "\n\n"); idx > start/2 {
		return strings.TrimSpace(text[idx:])
	}

	// Try to find sentence break
	if idx := strings.LastIndex(text[:start+1], ". "); idx > start/2 {
		return strings.TrimSpace(text[idx+2:])
	}

	// Fall back to word boundary
	if idx := strings.LastIndex(text[:start+1], " "); idx > 0 {
		return strings.TrimSpace(text[idx:])
	}

	return strings.TrimSpace(text[start:])
}

// paragraph represents a text paragraph with line count
type paragraph struct {
	content string
	lines   int
}

// splitParagraphs splits text into paragraphs, tracking line counts
func splitParagraphs(text string) []paragraph {
	var paragraphs []paragraph

	parts := strings.Split(text, "\n\n")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lines := strings.Count(part, "\n") + 1
		paragraphs = append(paragraphs, paragraph{
			content: part,
			lines:   lines + 1, // +1 for the paragraph break
		})
	}

	return paragraphs
}

// countNewlines counts the number of newlines in text
func countNewlines(text string) int {
	return strings.Count(text, "\n")
}

// TokenCount returns the estimated token count for a string
func TokenCount(text string) int {
	c := &Chunker{}
	return c.countTokens(text)
}
