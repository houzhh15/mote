package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestVersionChecker_CheckVersion(t *testing.T) {
	tests := []struct {
		name           string
		skillID        string
		embedVersion   string
		localVersion   string
		wantHasUpdate  bool
		wantErr        bool
	}{
		{
			name:           "newer version available",
			skillID:        "test-skill",
			embedVersion:   "1.2.0",
			localVersion:   "1.0.0",
			wantHasUpdate:  true,
			wantErr:        false,
		},
		{
			name:           "same version",
			skillID:        "test-skill",
			embedVersion:   "1.0.0",
			localVersion:   "1.0.0",
			wantHasUpdate:  false,
			wantErr:        false,
		},
		{
			name:           "local version newer",
			skillID:        "test-skill",
			embedVersion:   "1.0.0",
			localVersion:   "1.2.0",
			wantHasUpdate:  false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()
			skillDir := filepath.Join(tmpDir, tt.skillID)
			if err := os.MkdirAll(skillDir, 0755); err != nil {
				t.Fatalf("failed to create skill dir: %v", err)
			}

			// Create local manifest
			localManifest := map[string]interface{}{
				"id":      tt.skillID,
				"name":    "Test Skill",
				"version": tt.localVersion,
			}
			data, _ := json.Marshal(localManifest)
			if err := os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644); err != nil {
				t.Fatalf("failed to write local manifest: %v", err)
			}

			// Test version comparison directly
			vc := &VersionChecker{}

			// Compare versions directly
			hasUpdate, err := vc.compareVersions(tt.embedVersion, tt.localVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("compareVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if hasUpdate != tt.wantHasUpdate {
				t.Errorf("compareVersions() = %v, want %v", hasUpdate, tt.wantHasUpdate)
			}
		})
	}
}

func TestBackupManager_Backup(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	backupDir := filepath.Join(tmpDir, ".backup")
	skillID := "test-skill"

	bm := &BackupManager{
		skillsDir:  skillsDir,
		backupDir:  backupDir,
		maxBackups: 3,
	}

	// Create source directory with test files
	srcDir := filepath.Join(skillsDir, skillID)
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	testFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test backup
	backupPath, err := bm.Backup(skillID, "1.0.0")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Errorf("backup directory does not exist: %s", backupPath)
	}

	// Verify file in backup
	backupFile := filepath.Join(backupPath, "test.txt")
	content, err := os.ReadFile(backupFile)
	if err != nil {
		t.Errorf("failed to read backup file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("backup content = %s, want 'test content'", string(content))
	}
}

func TestBackupManager_Restore(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, ".backup")
	skillID := "test-skill"

	bm := &BackupManager{
		backupDir:  backupDir,
		maxBackups: 3,
	}

	// Create backup directory with test files
	backupPath := filepath.Join(backupDir, skillID+"_20260212_120000")
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}
	testFile := filepath.Join(backupPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("backup content"), 0644); err != nil {
		t.Fatalf("failed to write backup file: %v", err)
	}

	// Create destination directory
	dstDir := filepath.Join(tmpDir, "skills", skillID)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("failed to create dst dir: %v", err)
	}

	// Test restore
	if err := bm.Restore(backupPath, dstDir); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify restored file
	restoredFile := filepath.Join(dstDir, "test.txt")
	content, err := os.ReadFile(restoredFile)
	if err != nil {
		t.Errorf("failed to read restored file: %v", err)
	}
	if string(content) != "backup content" {
		t.Errorf("restored content = %s, want 'backup content'", string(content))
	}
}

func TestBackupManager_cleanupOldBackups(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	backupDir := filepath.Join(tmpDir, ".backup")
	skillID := "test-skill"

	// Create logger
	logger := zerolog.Nop()

	bm := &BackupManager{
		skillsDir:  skillsDir,
		backupDir:  backupDir,
		maxBackups: 2,
		logger:     logger,
	}

	// Create source skill
	srcDir := filepath.Join(skillsDir, skillID)
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create multiple backups using Backup method
	for i := 1; i <= 4; i++ {
		_, err := bm.Backup(skillID, "1.0."+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("failed to create backup %d: %v", i, err)
		}
	}

	// Verify only maxBackups remain (cleanup is called automatically by Backup)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("failed to read backup dir: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), skillID) {
			count++
		}
	}

	if count != bm.maxBackups {
		t.Errorf("backup count = %d, want %d", count, bm.maxBackups)
	}
}
