package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestNewMarkdownStore(t *testing.T) {
	t.Run("creates directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		baseDir := filepath.Join(tmpDir, "subdir", ".mote")

		store, err := NewMarkdownStore(MarkdownStoreOptions{
			BaseDir: baseDir,
			Logger:  zerolog.Nop(),
		})
		if err != nil {
			t.Fatalf("NewMarkdownStore failed: %v", err)
		}

		// Check daily dir was created
		dailyDir := filepath.Join(baseDir, "memory")
		if _, err := os.Stat(dailyDir); os.IsNotExist(err) {
			t.Errorf("daily directory was not created")
		}

		// Verify store fields
		if store.baseDir != baseDir {
			t.Errorf("baseDir = %q, want %q", store.baseDir, baseDir)
		}
	})

	t.Run("requires baseDir", func(t *testing.T) {
		_, err := NewMarkdownStore(MarkdownStoreOptions{})
		if err == nil {
			t.Error("expected error for empty baseDir")
		}
	})
}

func TestMarkdownStore_AppendDaily(t *testing.T) {
	store := newTestStore(t)

	t.Run("creates new daily file", func(t *testing.T) {
		err := store.AppendDaily("Test content", "会话记录")
		if err != nil {
			t.Fatalf("AppendDaily failed: %v", err)
		}

		// Read today's file
		content, err := store.GetDaily(time.Now())
		if err != nil {
			t.Fatalf("GetDaily failed: %v", err)
		}

		if !strings.Contains(content, "# ") && !strings.Contains(content, "记忆日志") {
			t.Errorf("expected header in content")
		}
		if !strings.Contains(content, "会话记录") {
			t.Errorf("expected section header in content")
		}
		if !strings.Contains(content, "Test content") {
			t.Errorf("expected content in file")
		}
	})

	t.Run("appends to existing file", func(t *testing.T) {
		err := store.AppendDaily("First entry", "Section 1")
		if err != nil {
			t.Fatalf("first AppendDaily failed: %v", err)
		}

		err = store.AppendDaily("Second entry", "Section 2")
		if err != nil {
			t.Fatalf("second AppendDaily failed: %v", err)
		}

		content, err := store.GetDaily(time.Now())
		if err != nil {
			t.Fatalf("GetDaily failed: %v", err)
		}

		if !strings.Contains(content, "First entry") {
			t.Errorf("expected first entry in content")
		}
		if !strings.Contains(content, "Second entry") {
			t.Errorf("expected second entry in content")
		}
	})

	t.Run("handles empty section", func(t *testing.T) {
		store := newTestStore(t)
		err := store.AppendDaily("Content without section", "")
		if err != nil {
			t.Fatalf("AppendDaily failed: %v", err)
		}

		content, err := store.GetDaily(time.Now())
		if err != nil {
			t.Fatalf("GetDaily failed: %v", err)
		}

		if !strings.Contains(content, "Content without section") {
			t.Errorf("expected content in file")
		}
	})
}

func TestMarkdownStore_AppendDailyAt(t *testing.T) {
	store := newTestStore(t)

	testDate := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	err := store.AppendDailyAt(testDate, "Past content", "Test Section")
	if err != nil {
		t.Fatalf("AppendDailyAt failed: %v", err)
	}

	content, err := store.GetDaily(testDate)
	if err != nil {
		t.Fatalf("GetDaily failed: %v", err)
	}

	if !strings.Contains(content, "2026-01-15") {
		t.Errorf("expected date in header, got: %s", content)
	}
	if !strings.Contains(content, "10:30") {
		t.Errorf("expected timestamp in section, got: %s", content)
	}
	if !strings.Contains(content, "Past content") {
		t.Errorf("expected content in file")
	}
}

func TestMarkdownStore_GetDaily(t *testing.T) {
	store := newTestStore(t)

	t.Run("returns empty for non-existent date", func(t *testing.T) {
		content, err := store.GetDaily(time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("GetDaily failed: %v", err)
		}
		if content != "" {
			t.Errorf("expected empty content for non-existent file, got: %q", content)
		}
	})
}

func TestMarkdownStore_GetDailyRange(t *testing.T) {
	store := newTestStore(t)

	// Create entries for multiple days
	day1 := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC)

	_ = store.AppendDailyAt(day1, "Day 1 content", "")
	_ = store.AppendDailyAt(day2, "Day 2 content", "")
	_ = store.AppendDailyAt(day3, "Day 3 content", "")

	content, err := store.GetDailyRange(day1, day3)
	if err != nil {
		t.Fatalf("GetDailyRange failed: %v", err)
	}

	if !strings.Contains(content, "Day 1 content") {
		t.Errorf("expected day 1 content")
	}
	if !strings.Contains(content, "Day 2 content") {
		t.Errorf("expected day 2 content")
	}
	if !strings.Contains(content, "Day 3 content") {
		t.Errorf("expected day 3 content")
	}
}

func TestMarkdownStore_AppendMemory(t *testing.T) {
	store := newTestStore(t)

	t.Run("creates new MEMORY.md", func(t *testing.T) {
		err := store.AppendMemory("User prefers Python", "用户偏好")
		if err != nil {
			t.Fatalf("AppendMemory failed: %v", err)
		}

		content, err := store.GetMemory()
		if err != nil {
			t.Fatalf("GetMemory failed: %v", err)
		}

		if !strings.Contains(content, "# 长期记忆") {
			t.Errorf("expected header in MEMORY.md")
		}
		if !strings.Contains(content, "## 用户偏好") {
			t.Errorf("expected section header in content")
		}
		if !strings.Contains(content, "User prefers Python") {
			t.Errorf("expected content in file")
		}
	})

	t.Run("appends to existing MEMORY.md", func(t *testing.T) {
		err := store.AppendMemory("Uses VS Code", "工具偏好")
		if err != nil {
			t.Fatalf("AppendMemory failed: %v", err)
		}

		content, err := store.GetMemory()
		if err != nil {
			t.Fatalf("GetMemory failed: %v", err)
		}

		if !strings.Contains(content, "User prefers Python") {
			t.Errorf("expected previous content in file")
		}
		if !strings.Contains(content, "Uses VS Code") {
			t.Errorf("expected new content in file")
		}
	})
}

func TestMarkdownStore_GetMemory(t *testing.T) {
	store := newTestStore(t)

	t.Run("returns empty for non-existent file", func(t *testing.T) {
		content, err := store.GetMemory()
		if err != nil {
			t.Fatalf("GetMemory failed: %v", err)
		}
		if content != "" {
			t.Errorf("expected empty content, got: %q", content)
		}
	})
}

func TestMarkdownStore_UpdateMemorySection(t *testing.T) {
	store := newTestStore(t)

	// First, create initial content
	err := store.AppendMemory("Original preference", "用户偏好")
	if err != nil {
		t.Fatalf("AppendMemory failed: %v", err)
	}

	// Update the section
	err = store.UpdateMemorySection("用户偏好", "Updated preference\nNew line")
	if err != nil {
		t.Fatalf("UpdateMemorySection failed: %v", err)
	}

	content, err := store.GetMemory()
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}

	if strings.Contains(content, "Original preference") {
		t.Errorf("expected original content to be replaced")
	}
	if !strings.Contains(content, "Updated preference") {
		t.Errorf("expected updated content, got: %s", content)
	}
}

func TestMarkdownStore_ListDailyFiles(t *testing.T) {
	store := newTestStore(t)

	// Create files for different dates
	dates := []time.Time{
		time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC),
	}

	for _, d := range dates {
		if err := store.AppendDailyAt(d, "Content for "+d.Format("2006-01-02"), ""); err != nil {
			t.Fatalf("AppendDailyAt failed: %v", err)
		}
	}

	files, err := store.ListDailyFiles()
	if err != nil {
		t.Fatalf("ListDailyFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}

	// Verify sorted newest first (compare by date string since timezone may differ)
	if len(files) >= 3 {
		if files[0].Date.Format("2006-01-02") != dates[2].Format("2006-01-02") {
			t.Errorf("expected newest file first (2026-01-20), got %v", files[0].Date.Format("2006-01-02"))
		}
		if files[2].Date.Format("2006-01-02") != dates[0].Format("2006-01-02") {
			t.Errorf("expected oldest file last (2026-01-10), got %v", files[2].Date.Format("2006-01-02"))
		}
	}
}

func TestMarkdownStore_ScanSections(t *testing.T) {
	store := newTestStore(t)

	content := `# Main Title

This is intro text.

## Section One

Content of section one.
More content.

## Section Two

Content of section two.

### Subsection

Nested content.
`

	sections := store.ScanSections(content)

	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(sections))
	}

	// Check first section
	if sections[0].Level != 1 {
		t.Errorf("section 0: expected level 1, got %d", sections[0].Level)
	}
	if sections[0].Title != "Main Title" {
		t.Errorf("section 0: expected title 'Main Title', got %q", sections[0].Title)
	}

	// Check second section
	if sections[1].Level != 2 {
		t.Errorf("section 1: expected level 2, got %d", sections[1].Level)
	}
	if sections[1].Title != "Section One" {
		t.Errorf("section 1: expected title 'Section One', got %q", sections[1].Title)
	}
	if !strings.Contains(sections[1].Content, "Content of section one") {
		t.Errorf("section 1: expected content, got %q", sections[1].Content)
	}

	// Check subsection
	if sections[3].Level != 3 {
		t.Errorf("section 3: expected level 3, got %d", sections[3].Level)
	}
}

func TestMarkdownStore_CleanOldDailyFiles(t *testing.T) {
	store := newTestStore(t)

	// Create old files
	oldDate := time.Now().AddDate(0, 0, -100)
	recentDate := time.Now().AddDate(0, 0, -5)

	if err := store.AppendDailyAt(oldDate, "Old content", ""); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}
	if err := store.AppendDailyAt(recentDate, "Recent content", ""); err != nil {
		t.Fatalf("failed to create recent file: %v", err)
	}

	removed, err := store.CleanOldDailyFiles(30)
	if err != nil {
		t.Fatalf("CleanOldDailyFiles failed: %v", err)
	}

	if removed != 1 {
		t.Errorf("expected 1 file removed, got %d", removed)
	}

	// Verify old file is gone
	content, _ := store.GetDaily(oldDate)
	if content != "" {
		t.Errorf("expected old file to be deleted")
	}

	// Verify recent file still exists
	content, _ = store.GetDaily(recentDate)
	if content == "" {
		t.Errorf("expected recent file to still exist")
	}
}

func TestMarkdownStore_Stats(t *testing.T) {
	store := newTestStore(t)

	// Create some content
	_ = store.AppendMemory("Memory content", "Section")
	store.AppendDailyAt(time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC), "Day 1", "")
	store.AppendDailyAt(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "Day 2", "")

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if !stats.MemoryFileExists {
		t.Errorf("expected MemoryFileExists to be true")
	}
	if stats.MemoryFileSize == 0 {
		t.Errorf("expected MemoryFileSize > 0")
	}
	if stats.DailyFileCount != 2 {
		t.Errorf("expected 2 daily files, got %d", stats.DailyFileCount)
	}
	if stats.TotalDailySize == 0 {
		t.Errorf("expected TotalDailySize > 0")
	}
}

// newTestStore creates a test MarkdownStore with a temporary directory
func newTestStore(t *testing.T) *MarkdownStore {
	t.Helper()
	tmpDir := t.TempDir()

	store, err := NewMarkdownStore(MarkdownStoreOptions{
		BaseDir: tmpDir,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	return store
}
