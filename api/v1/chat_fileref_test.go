package v1

import (
	"testing"
)

func TestParseFileReferences(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		wantFileRefs  []string
		wantMessage   string
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
