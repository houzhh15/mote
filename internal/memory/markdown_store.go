package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// MarkdownStore manages Markdown-based memory files with dual-layer structure:
// - MEMORY.md: Long-term curated knowledge
// - memory/YYYY-MM-DD.md: Daily session logs
type MarkdownStore struct {
	baseDir    string         // Base directory (~/.mote/)
	memoryFile string         // Path to MEMORY.md
	dailyDir   string         // Path to memory/ directory
	logger     zerolog.Logger // Logger
	mu         sync.RWMutex   // Protects file operations
}

// MarkdownStoreOptions configures the MarkdownStore
type MarkdownStoreOptions struct {
	BaseDir string // Base directory for memory files
	Logger  zerolog.Logger
}

// DailyFile represents a daily log file
type DailyFile struct {
	Date     time.Time
	Path     string
	Filename string
	Size     int64
}

// NewMarkdownStore creates a new MarkdownStore with the given options
func NewMarkdownStore(opts MarkdownStoreOptions) (*MarkdownStore, error) {
	if opts.BaseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}

	store := &MarkdownStore{
		baseDir:    opts.BaseDir,
		memoryFile: filepath.Join(opts.BaseDir, "MEMORY.md"),
		dailyDir:   filepath.Join(opts.BaseDir, "memory"),
		logger:     opts.Logger,
	}

	// Ensure directories exist
	if err := os.MkdirAll(store.dailyDir, 0755); err != nil {
		return nil, fmt.Errorf("create daily dir: %w", err)
	}

	return store, nil
}

// AppendDaily appends content to today's daily log file
// section: optional section header (e.g., "会话记录"), will be prefixed with timestamp
func (s *MarkdownStore) AppendDaily(content string, section string) error {
	return s.AppendDailyAt(time.Now(), content, section)
}

// AppendDailyAt appends content to the daily log file for a specific date
func (s *MarkdownStore) AppendDailyAt(date time.Time, content string, section string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := s.dailyFilename(date)
	path := filepath.Join(s.dailyDir, filename)

	// Check if file exists, if not create with date header
	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open daily file: %w", err)
	}
	defer f.Close()

	var builder strings.Builder

	if isNew {
		// Add file header with date
		builder.WriteString(fmt.Sprintf("# %s 记忆日志\n\n", date.Format("2006-01-02")))
	}

	// Add section with timestamp
	timestamp := date.Format("15:04")
	if section != "" {
		builder.WriteString(fmt.Sprintf("## %s %s\n\n", timestamp, section))
	} else {
		builder.WriteString(fmt.Sprintf("## %s\n\n", timestamp))
	}

	// Add content
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	if _, err := f.WriteString(builder.String()); err != nil {
		return fmt.Errorf("write daily content: %w", err)
	}

	s.logger.Debug().
		Str("file", filename).
		Str("section", section).
		Int("contentLen", len(content)).
		Msg("appended to daily log")

	return nil
}

// GetDaily reads the content of a specific date's log file
func (s *MarkdownStore) GetDaily(date time.Time) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := s.dailyFilename(date)
	path := filepath.Join(s.dailyDir, filename)

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil // Return empty string for non-existent files
	}
	if err != nil {
		return "", fmt.Errorf("read daily file: %w", err)
	}

	return string(content), nil
}

// GetDailyRange reads content from a range of dates
func (s *MarkdownStore) GetDailyRange(from, to time.Time) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var builder strings.Builder

	// Iterate through dates
	current := from
	for !current.After(to) {
		filename := s.dailyFilename(current)
		path := filepath.Join(s.dailyDir, filename)

		content, err := os.ReadFile(path)
		if err == nil {
			builder.WriteString(string(content))
			builder.WriteString("\n---\n\n")
		}

		current = current.AddDate(0, 0, 1)
	}

	return builder.String(), nil
}

// AppendMemory appends content to the long-term MEMORY.md file
// section: optional section header (e.g., "用户偏好")
func (s *MarkdownStore) AppendMemory(content string, section string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file exists
	isNew := false
	if _, err := os.Stat(s.memoryFile); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(s.memoryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open memory file: %w", err)
	}
	defer f.Close()

	var builder strings.Builder

	if isNew {
		// Add file header
		builder.WriteString("# 长期记忆\n\n")
		builder.WriteString("此文件包含重要的长期知识和偏好设置。\n\n")
	}

	// Add section header if provided
	if section != "" {
		builder.WriteString(fmt.Sprintf("## %s\n\n", section))
	}

	// Add content
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	if _, err := f.WriteString(builder.String()); err != nil {
		return fmt.Errorf("write memory content: %w", err)
	}

	s.logger.Debug().
		Str("section", section).
		Int("contentLen", len(content)).
		Msg("appended to MEMORY.md")

	return nil
}

// GetMemory reads the entire MEMORY.md content
func (s *MarkdownStore) GetMemory() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	content, err := os.ReadFile(s.memoryFile)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read memory file: %w", err)
	}

	return string(content), nil
}

// UpdateMemorySection updates a specific section in MEMORY.md
// If the section doesn't exist, it will be appended
func (s *MarkdownStore) UpdateMemorySection(section string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := os.ReadFile(s.memoryFile)
	if os.IsNotExist(err) {
		// File doesn't exist, create new
		return s.appendMemoryUnlocked(content, section)
	}
	if err != nil {
		return fmt.Errorf("read memory file: %w", err)
	}

	lines := strings.Split(string(existing), "\n")
	sectionHeader := "## " + section
	found := false
	var result []string
	i := 0

	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == sectionHeader {
			found = true
			// Add the section header
			result = append(result, lines[i])
			result = append(result, "")
			// Add new content
			result = append(result, strings.Split(content, "\n")...)
			result = append(result, "")

			// Skip old section content until next section or EOF
			i++
			for i < len(lines) {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
					break
				}
				i++
			}
			continue
		}
		result = append(result, lines[i])
		i++
	}

	if !found {
		// Section not found, append it
		result = append(result, sectionHeader)
		result = append(result, "")
		result = append(result, strings.Split(content, "\n")...)
		result = append(result, "")
	}

	if err := os.WriteFile(s.memoryFile, []byte(strings.Join(result, "\n")), 0644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}

	return nil
}

// appendMemoryUnlocked is the internal version without locking
func (s *MarkdownStore) appendMemoryUnlocked(content string, section string) error {
	f, err := os.OpenFile(s.memoryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open memory file: %w", err)
	}
	defer f.Close()

	var builder strings.Builder
	if section != "" {
		builder.WriteString(fmt.Sprintf("## %s\n\n", section))
	}
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	_, err = f.WriteString(builder.String())
	return err
}

// ListDailyFiles returns all daily log files sorted by date (newest first)
func (s *MarkdownStore) ListDailyFiles() ([]DailyFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dailyDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read daily dir: %w", err)
	}

	var files []DailyFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		// Parse date from filename (YYYY-MM-DD.md)
		name := strings.TrimSuffix(entry.Name(), ".md")
		date, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue // Skip files with invalid date format
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, DailyFile{
			Date:     date,
			Path:     filepath.Join(s.dailyDir, entry.Name()),
			Filename: entry.Name(),
			Size:     info.Size(),
		})
	}

	// Sort by date (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Date.After(files[j].Date)
	})

	return files, nil
}

// ScanSections parses a markdown file and returns its sections
func (s *MarkdownStore) ScanSections(content string) []MarkdownSection {
	var sections []MarkdownSection
	var current *MarkdownSection
	var contentBuilder strings.Builder
	lineNum := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for header
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			// Save previous section
			if current != nil {
				current.Content = strings.TrimSpace(contentBuilder.String())
				current.EndLine = lineNum - 1
				sections = append(sections, *current)
				contentBuilder.Reset()
			}

			// Start new section
			level := strings.Count(strings.Split(line, " ")[0], "#")
			title := strings.TrimSpace(strings.TrimLeft(line, "# "))
			current = &MarkdownSection{
				Level:     level,
				Title:     title,
				StartLine: lineNum,
			}
		} else if current != nil {
			contentBuilder.WriteString(line)
			contentBuilder.WriteString("\n")
		}
	}

	// Save last section
	if current != nil {
		current.Content = strings.TrimSpace(contentBuilder.String())
		current.EndLine = lineNum
		sections = append(sections, *current)
	}

	return sections
}

// MarkdownSection represents a section in a markdown file
type MarkdownSection struct {
	Level     int    // Header level (1, 2, 3)
	Title     string // Section title
	Content   string // Section content
	StartLine int    // Starting line number
	EndLine   int    // Ending line number
}

// dailyFilename generates the filename for a date's log
func (s *MarkdownStore) dailyFilename(date time.Time) string {
	return date.Format("2006-01-02") + ".md"
}

// CleanOldDailyFiles removes daily files older than the specified number of days
func (s *MarkdownStore) CleanOldDailyFiles(maxAgeDays int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	files, err := s.listDailyFilesUnlocked()
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, f := range files {
		if f.Date.Before(cutoff) {
			if err := os.Remove(f.Path); err != nil {
				s.logger.Warn().Err(err).Str("file", f.Filename).Msg("failed to remove old daily file")
				continue
			}
			removed++
		}
	}

	s.logger.Info().
		Int("removed", removed).
		Int("maxAgeDays", maxAgeDays).
		Msg("cleaned old daily files")

	return removed, nil
}

// listDailyFilesUnlocked is the internal version without locking
func (s *MarkdownStore) listDailyFilesUnlocked() ([]DailyFile, error) {
	entries, err := os.ReadDir(s.dailyDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read daily dir: %w", err)
	}

	var files []DailyFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		date, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, DailyFile{
			Date:     date,
			Path:     filepath.Join(s.dailyDir, entry.Name()),
			Filename: entry.Name(),
			Size:     info.Size(),
		})
	}

	return files, nil
}

// Stats returns statistics about the memory store
func (s *MarkdownStore) Stats() (*MarkdownStoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &MarkdownStoreStats{}

	// MEMORY.md stats
	if info, err := os.Stat(s.memoryFile); err == nil {
		stats.MemoryFileSize = info.Size()
		stats.MemoryFileExists = true
	}

	// Daily files stats
	files, err := s.listDailyFilesUnlocked()
	if err != nil {
		return nil, err
	}

	stats.DailyFileCount = len(files)
	for _, f := range files {
		stats.TotalDailySize += f.Size
	}

	if len(files) > 0 {
		stats.OldestDaily = files[len(files)-1].Date
		stats.NewestDaily = files[0].Date
	}

	return stats, nil
}

// MarkdownStoreStats contains statistics about the memory store
type MarkdownStoreStats struct {
	MemoryFileExists bool
	MemoryFileSize   int64
	DailyFileCount   int
	TotalDailySize   int64
	OldestDaily      time.Time
	NewestDaily      time.Time
}
