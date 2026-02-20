package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContentStore_ListSections(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MEMORY.md with sections
	memoryContent := `# MEMORY.md

## Preferences

I prefer dark themes.

## Facts

Go was created in 2009.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(memoryContent), 0644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	// Ensure memory dir exists
	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("create memory dir: %v", err)
	}

	mdStore, err := NewMarkdownStore(MarkdownStoreOptions{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("create markdown store: %v", err)
	}

	cs, err := NewContentStore(ContentStoreOptions{Store: mdStore})
	if err != nil {
		t.Fatalf("create content store: %v", err)
	}

	sections, err := cs.ListSections()
	if err != nil {
		t.Fatalf("list sections: %v", err)
	}

	if len(sections) == 0 {
		t.Error("expected sections from MEMORY.md, got 0")
	}
}

func TestContentStore_NilStore(t *testing.T) {
	_, err := NewContentStore(ContentStoreOptions{Store: nil})
	if err == nil {
		t.Error("expected error for nil store, got nil")
	}
}

func TestContentStore_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Ensure memory dir exists
	if err := os.MkdirAll(filepath.Join(tmpDir, "memory"), 0755); err != nil {
		t.Fatalf("create memory dir: %v", err)
	}

	mdStore, err := NewMarkdownStore(MarkdownStoreOptions{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("create markdown store: %v", err)
	}

	cs, err := NewContentStore(ContentStoreOptions{Store: mdStore})
	if err != nil {
		t.Fatalf("create content store: %v", err)
	}

	sections, err := cs.ListSections()
	if err != nil {
		t.Fatalf("list sections: %v", err)
	}

	if len(sections) != 0 {
		t.Errorf("expected 0 sections in empty dir, got %d", len(sections))
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantCat string
		wantImp float64
	}{
		{
			name:    "with frontmatter",
			content: "---\ncategory: preference\nimportance: 0.9\n---\nSome content",
			wantCat: "preference",
			wantImp: 0.9,
		},
		{
			name:    "without frontmatter",
			content: "Just some content",
			wantCat: CategoryOther,
			wantImp: DefaultImportance,
		},
		{
			name:    "empty",
			content: "",
			wantCat: CategoryOther,
			wantImp: DefaultImportance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, _ := ParseFrontmatter(tt.content)
			if meta.Category != tt.wantCat {
				t.Errorf("category: got %q, want %q", meta.Category, tt.wantCat)
			}
			if meta.Importance != tt.wantImp {
				t.Errorf("importance: got %f, want %f", meta.Importance, tt.wantImp)
			}
		})
	}
}
