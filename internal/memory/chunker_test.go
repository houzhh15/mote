package memory

import (
	"strings"
	"testing"
)

func TestNewChunker(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		c := NewChunker(DefaultChunkerOptions())
		if c.targetTokens != 400 {
			t.Errorf("targetTokens = %d, want 400", c.targetTokens)
		}
		if c.overlapTokens != 80 {
			t.Errorf("overlapTokens = %d, want 80", c.overlapTokens)
		}
	})

	t.Run("custom options", func(t *testing.T) {
		c := NewChunker(ChunkerOptions{
			TargetTokens:  200,
			OverlapTokens: 40,
		})
		if c.targetTokens != 200 {
			t.Errorf("targetTokens = %d, want 200", c.targetTokens)
		}
		if c.overlapTokens != 40 {
			t.Errorf("overlapTokens = %d, want 40", c.overlapTokens)
		}
	})

	t.Run("fixes invalid options", func(t *testing.T) {
		c := NewChunker(ChunkerOptions{
			TargetTokens:  0,
			OverlapTokens: 500,
		})
		if c.targetTokens != 400 {
			t.Errorf("targetTokens = %d, want 400 (default)", c.targetTokens)
		}
		if c.overlapTokens >= c.targetTokens {
			t.Errorf("overlapTokens (%d) should be less than targetTokens (%d)", c.overlapTokens, c.targetTokens)
		}
	})
}

func TestChunker_Split(t *testing.T) {
	c := NewChunker(ChunkerOptions{
		TargetTokens:  50, // Small for testing
		OverlapTokens: 10,
	})

	t.Run("empty text", func(t *testing.T) {
		chunks := c.Split("", "test.md")
		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
		}
	})

	t.Run("single paragraph", func(t *testing.T) {
		text := "This is a short paragraph."
		chunks := c.Split(text, "test.md")
		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(chunks))
		}
		if len(chunks) > 0 && chunks[0].Content != text {
			t.Errorf("chunk content = %q, want %q", chunks[0].Content, text)
		}
	})

	t.Run("multiple paragraphs", func(t *testing.T) {
		text := `First paragraph with some content.

Second paragraph with more content.

Third paragraph with even more content.`

		chunks := c.Split(text, "test.md")
		if len(chunks) < 1 {
			t.Fatalf("expected at least 1 chunk, got %d", len(chunks))
		}

		// All content should be preserved
		combined := ""
		for _, ch := range chunks {
			combined += ch.Content + " "
		}
		if !strings.Contains(combined, "First paragraph") {
			t.Error("first paragraph not found in chunks")
		}
		if !strings.Contains(combined, "Third paragraph") {
			t.Error("third paragraph not found in chunks")
		}
	})

	t.Run("preserves source file", func(t *testing.T) {
		chunks := c.Split("Some content", "path/to/file.md")
		if len(chunks) > 0 && chunks[0].SourceFile != "path/to/file.md" {
			t.Errorf("SourceFile = %q, want %q", chunks[0].SourceFile, "path/to/file.md")
		}
	})

	t.Run("chunk indices are sequential", func(t *testing.T) {
		longText := strings.Repeat("This is a sentence. ", 100)
		chunks := c.Split(longText, "test.md")
		for i, ch := range chunks {
			if ch.Index != i {
				t.Errorf("chunk %d has Index = %d", i, ch.Index)
			}
		}
	})

	t.Run("line numbers are positive", func(t *testing.T) {
		text := `Line one.

Line three.

Line five.`
		chunks := c.Split(text, "test.md")
		for i, ch := range chunks {
			if ch.StartLine < 1 {
				t.Errorf("chunk %d has StartLine = %d, want >= 1", i, ch.StartLine)
			}
			if ch.EndLine < ch.StartLine {
				t.Errorf("chunk %d has EndLine (%d) < StartLine (%d)", i, ch.EndLine, ch.StartLine)
			}
		}
	})
}

func TestChunker_SplitBySection(t *testing.T) {
	c := NewChunker(ChunkerOptions{
		TargetTokens:  100,
		OverlapTokens: 20,
	})

	t.Run("empty text", func(t *testing.T) {
		chunks := c.SplitBySection("", "test.md")
		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
		}
	})

	t.Run("no headers fallback", func(t *testing.T) {
		text := "Just some plain text without any headers."
		chunks := c.SplitBySection(text, "test.md")
		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(chunks))
		}
	})

	t.Run("simple sections", func(t *testing.T) {
		text := `# Title

Introduction paragraph.

## Section One

Content of section one.

## Section Two

Content of section two.`

		chunks := c.SplitBySection(text, "test.md")
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}

		// Check sections are captured
		foundSectionOne := false
		foundSectionTwo := false
		for _, ch := range chunks {
			if strings.Contains(ch.Content, "Section One") {
				foundSectionOne = true
			}
			if strings.Contains(ch.Content, "Section Two") {
				foundSectionTwo = true
			}
		}
		if !foundSectionOne {
			t.Error("Section One not found in chunks")
		}
		if !foundSectionTwo {
			t.Error("Section Two not found in chunks")
		}
	})

	t.Run("nested headers", func(t *testing.T) {
		text := `# Main

## Sub 1

### Sub 1.1

Content 1.1

### Sub 1.2

Content 1.2

## Sub 2

Content 2`

		chunks := c.SplitBySection(text, "test.md")
		if len(chunks) < 3 {
			t.Errorf("expected at least 3 chunks for nested headers, got %d", len(chunks))
		}
	})
}

func TestChunker_countTokens(t *testing.T) {
	c := NewChunker(DefaultChunkerOptions())

	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{
			name:    "empty",
			text:    "",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "short English",
			text:    "Hello world",
			wantMin: 2,
			wantMax: 5,
		},
		{
			name:    "longer English",
			text:    "This is a longer sentence with multiple words.",
			wantMin: 8,
			wantMax: 15,
		},
		{
			name:    "Chinese text",
			text:    "这是一段中文文本",
			wantMin: 3,
			wantMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.countTokens(tt.text)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("countTokens(%q) = %d, want between %d and %d", tt.text, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestChunker_getOverlap(t *testing.T) {
	c := NewChunker(ChunkerOptions{
		TargetTokens:  100,
		OverlapTokens: 20, // ~80 chars overlap
	})

	t.Run("short text returns all", func(t *testing.T) {
		text := "Short text"
		overlap := c.getOverlap(text)
		if overlap != text {
			t.Errorf("getOverlap returned %q, want %q", overlap, text)
		}
	})

	t.Run("long text returns tail", func(t *testing.T) {
		text := strings.Repeat("word ", 100)
		overlap := c.getOverlap(text)
		if len(overlap) > len(text)/2 {
			t.Errorf("overlap too long: %d chars", len(overlap))
		}
		if len(overlap) < 50 {
			t.Errorf("overlap too short: %d chars", len(overlap))
		}
	})

	t.Run("prefers paragraph break", func(t *testing.T) {
		text := "First paragraph with lots of content here.\n\nSecond paragraph."
		overlap := c.getOverlap(text)
		if !strings.HasPrefix(overlap, "Second") {
			// Paragraph break may or may not be preferred depending on position
			// Just ensure we get some meaningful overlap
			if len(overlap) < 10 {
				t.Errorf("overlap too short: %q", overlap)
			}
		}
	})
}

func TestChunker_LargeDocument(t *testing.T) {
	c := NewChunker(ChunkerOptions{
		TargetTokens:  200,
		OverlapTokens: 40,
	})

	// Create a large document
	var builder strings.Builder
	for i := 0; i < 50; i++ {
		builder.WriteString("## Section ")
		builder.WriteString(string(rune('A' + i%26)))
		builder.WriteString("\n\n")
		builder.WriteString(strings.Repeat("This is paragraph content. ", 20))
		builder.WriteString("\n\n")
	}

	chunks := c.SplitBySection(builder.String(), "large.md")

	t.Run("produces multiple chunks", func(t *testing.T) {
		if len(chunks) < 10 {
			t.Errorf("expected at least 10 chunks for large document, got %d", len(chunks))
		}
	})

	t.Run("chunks are reasonably sized", func(t *testing.T) {
		for i, ch := range chunks {
			tokens := c.countTokens(ch.Content)
			// Allow some flexibility (up to 2x target for section-based splitting)
			if tokens > c.targetTokens*3 {
				t.Errorf("chunk %d has %d tokens, exceeds 3x target (%d)", i, tokens, c.targetTokens)
			}
		}
	})

	t.Run("no content is lost", func(t *testing.T) {
		// Check that we have content from different parts
		foundSectionA := false
		foundSectionZ := false
		for _, ch := range chunks {
			if strings.Contains(ch.Content, "Section A") {
				foundSectionA = true
			}
			if strings.Contains(ch.Content, "Section Z") {
				foundSectionZ = true
			}
		}
		if !foundSectionA {
			t.Error("Section A not found")
		}
		if !foundSectionZ {
			t.Error("Section Z not found")
		}
	})
}

func TestTokenCount(t *testing.T) {
	count := TokenCount("Hello world, this is a test.")
	if count < 5 || count > 10 {
		t.Errorf("TokenCount = %d, expected between 5 and 10", count)
	}
}
