package v1

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileReferences(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantFileRefs []string
		wantMessage  string
	}{
		{
			name:         "single file reference",
			message:      "Please read @src/main.go",
			wantFileRefs: []string{"src/main.go"},
			wantMessage:  "Please read @src/main.go",
		},
		{
			name:         "multiple file references",
			message:      "Compare @file1.txt and @file2.txt",
			wantFileRefs: []string{"file1.txt", "file2.txt"},
			wantMessage:  "Compare @file1.txt and @file2.txt",
		},
		{
			name:         "file with path",
			message:      "Check @internal/pkg/utils.go",
			wantFileRefs: []string{"internal/pkg/utils.go"},
			wantMessage:  "Check @internal/pkg/utils.go",
		},
		{
			name:         "no references",
			message:      "Hello world",
			wantFileRefs: []string(nil),
			wantMessage:  "Hello world",
		},
		{
			name:         "email address not matched",
			message:      "Contact user@example.com",
			wantFileRefs: []string(nil), // Email should not be matched
			wantMessage:  "Contact user@example.com",
		},
		{
			name:         "at sign at start of line",
			message:      "@README.md",
			wantFileRefs: []string{"README.md"},
			wantMessage:  "@README.md",
		},
		{
			name:         "multiple refs with whitespace",
			message:      "@file1.txt @file2.txt",
			wantFileRefs: []string{"file1.txt", "file2.txt"},
			wantMessage:  "@file1.txt @file2.txt",
		},
		{
			name:         "invalid path with traversal",
			message:      "@../../../etc/passwd",
			wantFileRefs: []string(nil), // Should be filtered by isValidFilePath
			wantMessage:  "@../../../etc/passwd",
		},
		{
			name:         "sensitive file",
			message:      "@.env",
			wantFileRefs: []string(nil), // Should be filtered by isValidFilePath
			wantMessage:  "@.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMessage, gotFileRefs := parseFileReferences(tt.message)

			if gotMessage != tt.wantMessage {
				t.Errorf("parseFileReferences() message = %v, want %v", gotMessage, tt.wantMessage)
			}

			if len(gotFileRefs) != len(tt.wantFileRefs) {
				t.Errorf("parseFileReferences() fileRefs count = %v, want %v", len(gotFileRefs), len(tt.wantFileRefs))
				return
			}

			for i, ref := range gotFileRefs {
				if ref != tt.wantFileRefs[i] {
					t.Errorf("parseFileReferences() fileRefs[%d] = %v, want %v", i, ref, tt.wantFileRefs[i])
				}
			}
		})
	}
}

func TestIsValidFilePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "valid simple path",
			path: "main.go",
			want: true,
		},
		{
			name: "valid nested path",
			path: "internal/pkg/utils.go",
			want: true,
		},
		{
			name: "path traversal with ..",
			path: "../../../etc/passwd",
			want: false,
		},
		{
			name: "path with .. in middle",
			path: "src/../main.go",
			want: false,
		},
		{
			name: "sensitive file .env",
			path: ".env",
			want: false,
		},
		{
			name: "sensitive file id_rsa",
			path: "~/.ssh/id_rsa",
			want: false,
		},
		{
			name: "sensitive file password",
			path: "config/password.txt",
			want: false,
		},
		{
			name: "sensitive file secret",
			path: "secrets.yaml",
			want: false,
		},
		{
			name: "sensitive file private_key",
			path: "certs/private_key.pem",
			want: false,
		},
		{
			name: "valid path with similar name",
			path: "config/settings.json",
			want: true,
		},
		{
			name: "valid README",
			path: "README.md",
			want: true,
		},
		{
			name: "case insensitive sensitive check",
			path: "Config/PASSWORD.txt",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidFilePath(tt.path)
			if got != tt.want {
				t.Errorf("isValidFilePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractImagePaths(t *testing.T) {
	// Create temp directory with test image files
	tmpDir := t.TempDir()
	testJPG := filepath.Join(tmpDir, "photo.jpg")
	testPNG := filepath.Join(tmpDir, "screenshot.png")
	testTXT := filepath.Join(tmpDir, "notes.txt")
	os.WriteFile(testJPG, []byte("fake jpg"), 0644)
	os.WriteFile(testPNG, []byte("fake png"), 0644)
	os.WriteFile(testTXT, []byte("not an image"), 0644)

	tests := []struct {
		name      string
		message   string
		wantCount int
		wantPaths []string
	}{
		{
			name:      "single image path",
			message:   "请看这张图片 " + testJPG,
			wantCount: 1,
			wantPaths: []string{testJPG},
		},
		{
			name:      "multiple image paths",
			message:   "比较 " + testJPG + " 和 " + testPNG + " 的区别",
			wantCount: 2,
			wantPaths: []string{testJPG, testPNG},
		},
		{
			name:      "non-image file is ignored",
			message:   "请读 " + testTXT,
			wantCount: 0,
		},
		{
			name:      "non-existent image path is ignored",
			message:   "请看 /nonexistent/image.jpg",
			wantCount: 0,
		},
		{
			name:      "no paths in message",
			message:   "hello world",
			wantCount: 0,
		},
		{
			name:      "image path at start of line",
			message:   testJPG + " 这是一张照片",
			wantCount: 1,
			wantPaths: []string{testJPG},
		},
		{
			name:      "image path with Chinese punctuation",
			message:   "分析" + testJPG + "，描述内容",
			wantCount: 1,
			wantPaths: []string{testJPG},
		},
		{
			name:      "duplicate paths are deduplicated",
			message:   testJPG + " and " + testJPG,
			wantCount: 1,
			wantPaths: []string{testJPG},
		},
		{
			name:      "path traversal image is rejected",
			message:   "/etc/../../../tmp/evil.jpg",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImagePaths(tt.message)
			if len(got) != tt.wantCount {
				t.Errorf("extractImagePaths() got %d paths, want %d: %v", len(got), tt.wantCount, got)
				return
			}
			for i, path := range tt.wantPaths {
				if i < len(got) && got[i] != path {
					t.Errorf("extractImagePaths() path[%d] = %q, want %q", i, got[i], path)
				}
			}
		})
	}
}

func TestImagePathRegex(t *testing.T) {
	// Test regex matching without filesystem dependency
	tests := []struct {
		name    string
		message string
		want    []string
	}{
		{
			name:    "absolute jpg path",
			message: "Check /Users/test/photo.jpg please",
			want:    []string{"/Users/test/photo.jpg"},
		},
		{
			name:    "home-relative png path",
			message: "See ~/Documents/screenshot.png",
			want:    []string{"~/Documents/screenshot.png"},
		},
		{
			name:    "multiple extensions",
			message: "/a/b.jpg /c/d.png /e/f.webp",
			want:    []string{"/a/b.jpg", "/c/d.png", "/e/f.webp"},
		},
		{
			name:    "numbered slides",
			message: "1. /ppt/slide_1.jpg\n2. /ppt/slide_2.png",
			want:    []string{"/ppt/slide_1.jpg", "/ppt/slide_2.png"},
		},
		{
			name:    "path with hyphens and underscores",
			message: "/path/some-file_name.jpeg",
			want:    []string{"/path/some-file_name.jpeg"},
		},
		{
			name:    "no match for non-image extension",
			message: "/path/to/file.go",
			want:    nil,
		},
		{
			name:    "relative path extracts absolute suffix",
			message: "relative/path/image.jpg",
			want:    []string{"/path/image.jpg"}, // Regex extracts from first /, os.Stat filters
		},
		{
			name:    "path followed by comma",
			message: "/path/image.jpg，这是第一张",
			want:    []string{"/path/image.jpg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := imagePathRegex.FindAllStringSubmatch(tt.message, -1)
			var got []string
			for _, m := range matches {
				if len(m) > 1 {
					got = append(got, m[1])
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("imagePathRegex got %d matches, want %d: %v", len(got), len(tt.want), got)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("match[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
