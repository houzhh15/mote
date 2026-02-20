package memory

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ContentStore provides a higher-level interface over MarkdownStore for
// managing memory content as structured sections with metadata.
// It implements the ContentStorer interface.
type ContentStore struct {
	store    *MarkdownStore
	baseDir  string
	sections map[string]Section // section ID -> Section cache
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// ContentStoreOptions holds options for creating a ContentStore.
type ContentStoreOptions struct {
	Store  *MarkdownStore
	Logger zerolog.Logger
}

// NewContentStore creates a new ContentStore wrapping a MarkdownStore.
func NewContentStore(opts ContentStoreOptions) (*ContentStore, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("content_store: MarkdownStore is required")
	}

	cs := &ContentStore{
		store:    opts.Store,
		baseDir:  opts.Store.baseDir,
		sections: make(map[string]Section),
		logger:   opts.Logger,
	}

	// Build initial section cache
	if err := cs.refreshCache(); err != nil {
		return nil, fmt.Errorf("content_store: initial cache build: %w", err)
	}

	return cs, nil
}

// ListSections returns all sections across MEMORY.md and daily files.
func (cs *ContentStore) ListSections() ([]Section, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	sections := make([]Section, 0, len(cs.sections))
	for _, s := range cs.sections {
		sections = append(sections, s)
	}
	return sections, nil
}

// GetSection returns a section by its ID.
func (cs *ContentStore) GetSection(id string) (*Section, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	s, ok := cs.sections[id]
	if !ok {
		return nil, fmt.Errorf("content_store: section %q not found", id)
	}
	return &s, nil
}

// UpsertSection creates or updates a section.
// For MEMORY.md, it updates the corresponding section in the file.
func (cs *ContentStore) UpsertSection(section Section) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Recalculate hash
	section.Hash = hashContent(section.Content)

	// Write to the appropriate file via MarkdownStore
	if err := cs.store.UpdateMemorySection(section.Heading, section.Content); err != nil {
		return fmt.Errorf("content_store: upsert section %q: %w", section.ID, err)
	}

	cs.sections[section.ID] = section

	cs.logger.Debug().
		Str("section_id", section.ID).
		Str("heading", section.Heading).
		Msg("upserted section")

	return nil
}

// DeleteSection removes a section by ID from the cache.
// Note: Actual file deletion is handled by modifying MEMORY.md content.
func (cs *ContentStore) DeleteSection(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, ok := cs.sections[id]; !ok {
		return fmt.Errorf("content_store: section %q not found", id)
	}

	delete(cs.sections, id)

	cs.logger.Debug().
		Str("section_id", id).
		Msg("deleted section from cache")

	return nil
}

// Refresh rebuilds the section cache from all Markdown files.
func (cs *ContentStore) Refresh() error {
	return cs.refreshCache()
}

// refreshCache scans all Markdown files and rebuilds the section cache.
func (cs *ContentStore) refreshCache() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	newSections := make(map[string]Section)

	// Parse MEMORY.md
	memContent, err := cs.store.GetMemory()
	if err != nil {
		return fmt.Errorf("content_store: read MEMORY.md: %w", err)
	}
	if memContent != "" {
		meta, body := ParseFrontmatter(memContent)
		mdSections := cs.store.ScanSections(body)
		for _, ms := range mdSections {
			id := sectionID(cs.store.memoryFile, ms.Title)
			s := Section{
				ID:       id,
				FilePath: cs.store.memoryFile,
				Heading:  ms.Title,
				Content:  ms.Content,
				Hash:     hashContent(ms.Content),
				Metadata: meta,
			}
			// Override metadata if section has its own frontmatter hint
			if ms.Level == 2 {
				s.Metadata = parseSectionMetadata(ms.Content, meta)
			}
			newSections[id] = s
		}
	}

	// Parse daily files
	dailyFiles, err := cs.store.ListDailyFiles()
	if err != nil {
		return fmt.Errorf("content_store: list daily files: %w", err)
	}
	for _, df := range dailyFiles {
		content, err := os.ReadFile(df.Path)
		if err != nil {
			cs.logger.Warn().Err(err).Str("file", df.Filename).Msg("skip daily file")
			continue
		}
		mdSections := cs.store.ScanSections(string(content))
		for _, ms := range mdSections {
			id := sectionID(df.Path, ms.Title)
			s := Section{
				ID:       id,
				FilePath: df.Path,
				Heading:  ms.Title,
				Content:  ms.Content,
				Hash:     hashContent(ms.Content),
				Metadata: DefaultSectionMetadata(),
			}
			newSections[id] = s
		}
	}

	cs.sections = newSections

	cs.logger.Info().
		Int("sections", len(newSections)).
		Msg("content store cache refreshed")

	return nil
}

// ParseFrontmatter extracts YAML frontmatter from Markdown content.
// Returns the parsed metadata and the remaining body content.
func ParseFrontmatter(content string) (SectionMetadata, string) {
	meta := DefaultSectionMetadata()

	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return meta, content
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return meta, content
	}

	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	// Simple YAML parsing (key: value)
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "category":
			meta.Category = val
		case "importance":
			if v := parseFloat(val); v > 0 {
				meta.Importance = v
			}
		case "tags":
			meta.Tags = parseTags(val)
		}
	}

	return meta, body
}

// parseSectionMetadata extracts metadata hints from section content.
// Falls back to the file-level metadata if no section-specific hints found.
func parseSectionMetadata(content string, fileMeta SectionMetadata) SectionMetadata {
	meta := fileMeta

	// Look for inline metadata markers (e.g., "category: preference")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "category":
			meta.Category = val
		case "importance":
			if v := parseFloat(val); v > 0 {
				meta.Importance = v
			}
		}
	}

	return meta
}

// sectionID generates a deterministic section ID from file path and heading.
func sectionID(filePath, heading string) string {
	// Use relative path if possible for portability
	base := filepath.Base(filePath)
	h := sha256.Sum256([]byte(base + "::" + heading))
	return fmt.Sprintf("sec_%x", h[:8])
}

// hashContent computes SHA256 hash of content.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// parseFloat parses a float64 from string, returning 0 on error.
func parseFloat(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// parseTags parses a YAML-style tag list (e.g., "[tag1, tag2]" or "tag1, tag2").
func parseTags(s string) []string {
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
