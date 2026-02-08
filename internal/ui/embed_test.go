package ui

import (
	"io/fs"
	"testing"
)

func TestGetEmbedFS(t *testing.T) {
	embedFS := GetEmbedFS()
	if embedFS == nil {
		t.Fatal("GetEmbedFS returned nil")
	}

	// Check that we can read from the embedded filesystem
	_, err := fs.Stat(embedFS, "index.html")
	if err != nil {
		t.Fatalf("failed to stat index.html: %v", err)
	}
}

func TestGetEmbedFS_ReadFile(t *testing.T) {
	embedFS := GetEmbedFS()

	content, err := fs.ReadFile(embedFS, "index.html")
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	if len(content) == 0 {
		t.Error("index.html is empty")
	}

	// Verify it contains expected HTML content
	str := string(content)
	if !containsString(str, "<!DOCTYPE html>") {
		t.Error("index.html should contain DOCTYPE declaration")
	}
	if !containsString(str, "Mote UI") {
		t.Error("index.html should contain 'Mote UI' title")
	}
}

func TestGetEmbedFS_WalkDir(t *testing.T) {
	embedFS := GetEmbedFS()

	var files []string
	err := fs.WalkDir(embedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk embedded filesystem: %v", err)
	}

	if len(files) == 0 {
		t.Error("embedded filesystem should contain at least one file")
	}

	// Verify index.html is in the list
	found := false
	for _, f := range files {
		if f == "index.html" {
			found = true
			break
		}
	}
	if !found {
		t.Error("index.html should be in embedded filesystem")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
