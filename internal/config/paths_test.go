package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigDir(t *testing.T) {
	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir() error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".mote")
	if dir != expected {
		t.Errorf("DefaultConfigDir() = %q, want %q", dir, expected)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() error: %v", err)
	}

	if !strings.HasSuffix(path, filepath.Join(".mote", "config.yaml")) {
		t.Errorf("DefaultConfigPath() = %q, want suffix .mote/config.yaml", path)
	}
}

func TestDefaultDataPath(t *testing.T) {
	path, err := DefaultDataPath()
	if err != nil {
		t.Fatalf("DefaultDataPath() error: %v", err)
	}

	if !strings.HasSuffix(path, filepath.Join(".mote", "data.db")) {
		t.Errorf("DefaultDataPath() = %q, want suffix .mote/data.db", path)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty path",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
			wantErr:  false,
		},
		{
			name:     "tilde with subpath",
			input:    "~/test/path",
			expected: filepath.Join(home, "test/path"),
			wantErr:  false,
		},
		{
			name:     "absolute path",
			input:    "/absolute/path",
			expected: "/absolute/path",
			wantErr:  false,
		},
		{
			name:     "relative path",
			input:    "relative/path",
			expected: "relative/path",
			wantErr:  false,
		},
		{
			name:     "tilde in middle",
			input:    "/some/~/path",
			expected: "/some/~/path",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
