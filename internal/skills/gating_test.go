package skills

import (
	"os"
	"testing"
)

func TestGating_ShouldInclude(t *testing.T) {
	g := NewGating()

	tests := []struct {
		name  string
		entry *SkillEntry
		want  bool
	}{
		{
			name:  "nil entry",
			entry: nil,
			want:  false,
		},
		{
			name: "manifest skill always included",
			entry: &SkillEntry{
				Name:   "Test Skill",
				Source: SourceManifest,
			},
			want: true,
		},
		{
			name: "skillmd without metadata",
			entry: &SkillEntry{
				Name:   "Test Skill",
				Source: SourceSkillMD,
			},
			want: true,
		},
		{
			name: "skillmd with empty metadata",
			entry: &SkillEntry{
				Name:     "Test Skill",
				Source:   SourceSkillMD,
				Metadata: &SkillMDMetadata{},
			},
			want: true,
		},
		{
			name: "skillmd with matching OS (darwin)",
			entry: &SkillEntry{
				Name:   "Mac Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					OS: []string{"darwin"},
				},
			},
			want: true, // Will be true on macOS, adjust for CI
		},
		{
			name: "skillmd with required bin that exists",
			entry: &SkillEntry{
				Name:   "Shell Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						Bins: []string{"sh"}, // sh should exist on all Unix systems
					},
				},
			},
			want: true,
		},
		{
			name: "skillmd with required bin that doesn't exist",
			entry: &SkillEntry{
				Name:   "Missing Bin Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						Bins: []string{"nonexistent_binary_xyz_123"},
					},
				},
			},
			want: false,
		},
		{
			name: "skillmd with anyBins - one exists",
			entry: &SkillEntry{
				Name:   "Editor Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						AnyBins: []string{"nonexistent_xyz", "sh"}, // sh exists
					},
				},
			},
			want: true,
		},
		{
			name: "skillmd with anyBins - none exist",
			entry: &SkillEntry{
				Name:   "Missing Editor Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						AnyBins: []string{"nonexistent_xyz", "nonexistent_abc"},
					},
				},
			},
			want: false,
		},
		{
			name: "skillmd with required env that exists",
			entry: &SkillEntry{
				Name:   "Path Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						Env: []string{"PATH"}, // PATH should always exist
					},
				},
			},
			want: true,
		},
		{
			name: "skillmd with required env that doesn't exist",
			entry: &SkillEntry{
				Name:   "Missing Env Skill",
				Source: SourceSkillMD,
				Metadata: &SkillMDMetadata{
					Requires: &SkillRequirements{
						Env: []string{"NONEXISTENT_ENV_VAR_XYZ_123"},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip OS-specific tests that may fail in CI
			if tt.name == "skillmd with matching OS (darwin)" {
				t.Skip("OS-specific test, skipping in CI")
			}

			got := g.ShouldInclude(tt.entry)
			if got != tt.want {
				t.Errorf("Gating.ShouldInclude() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckBinaryExists(t *testing.T) {
	// sh should exist on all Unix systems
	if !CheckBinaryExists("sh") {
		t.Error("CheckBinaryExists(sh) = false, want true")
	}

	// This should not exist
	if CheckBinaryExists("nonexistent_binary_xyz_123") {
		t.Error("CheckBinaryExists(nonexistent) = true, want false")
	}
}

func TestCheckEnvExists(t *testing.T) {
	// PATH should always exist
	if !CheckEnvExists("PATH") {
		t.Error("CheckEnvExists(PATH) = false, want true")
	}

	// This should not exist
	if CheckEnvExists("NONEXISTENT_ENV_VAR_XYZ_123") {
		t.Error("CheckEnvExists(nonexistent) = true, want false")
	}
}

func TestGating_checkPaths(t *testing.T) {
	g := NewGating()

	// Create temp directory for testing
	tempDir := t.TempDir()
	existingFile := tempDir + "/existing.txt"
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name     string
		paths    []string
		expected bool
	}{
		{
			name:     "empty paths",
			paths:    []string{},
			expected: true,
		},
		{
			name:     "existing path",
			paths:    []string{existingFile},
			expected: true,
		},
		{
			name:     "non-existing path",
			paths:    []string{"/non/existing/path/file.txt"},
			expected: false,
		},
		{
			name:     "mixed paths - one missing",
			paths:    []string{existingFile, "/non/existing/path"},
			expected: false,
		},
		{
			name:     "temp directory exists",
			paths:    []string{tempDir},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.checkPaths(tt.paths)
			if result != tt.expected {
				t.Errorf("checkPaths(%v) = %v, want %v", tt.paths, result, tt.expected)
			}
		})
	}
}

func TestGating_checkPaths_EnvExpansion(t *testing.T) {
	g := NewGating()

	// Create temp directory and set env var
	tempDir := t.TempDir()
	existingFile := tempDir + "/test.txt"
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Set environment variable
	t.Setenv("MOTE_TEST_PATH", tempDir)

	tests := []struct {
		name     string
		paths    []string
		expected bool
	}{
		{
			name:     "env var expansion - existing",
			paths:    []string{"$MOTE_TEST_PATH/test.txt"},
			expected: true,
		},
		{
			name:     "env var expansion - non-existing",
			paths:    []string{"$MOTE_TEST_PATH/nonexistent.txt"},
			expected: false,
		},
		{
			name:     "env var expansion - directory",
			paths:    []string{"$MOTE_TEST_PATH"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.checkPaths(tt.paths)
			if result != tt.expected {
				t.Errorf("checkPaths(%v) = %v, want %v", tt.paths, result, tt.expected)
			}
		})
	}
}
