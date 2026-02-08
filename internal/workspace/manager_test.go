package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWorkspaceManager_Bind(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	err := manager.Bind("session-1", tmpDir, false)
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	if !manager.IsBound("session-1") {
		t.Error("expected session to be bound")
	}
}

func TestWorkspaceManager_Bind_EmptySessionID(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	err := manager.Bind("", tmpDir, false)
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestWorkspaceManager_Bind_EmptyPath(t *testing.T) {
	manager := NewWorkspaceManager()

	err := manager.Bind("session-1", "", false)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestWorkspaceManager_Bind_NonexistentPath(t *testing.T) {
	manager := NewWorkspaceManager()

	err := manager.Bind("session-1", "/nonexistent/path/12345", false)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestWorkspaceManager_Bind_FileNotDir(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpFile := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(tmpFile, []byte("test"), 0644)

	err := manager.Bind("session-1", tmpFile, false)
	if err == nil {
		t.Error("expected error for file (not directory)")
	}
}

func TestWorkspaceManager_BindWithAlias(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	err := manager.BindWithAlias("session-1", tmpDir, "myproject", false)
	if err != nil {
		t.Fatalf("BindWithAlias failed: %v", err)
	}

	binding, exists := manager.Get("session-1")
	if !exists {
		t.Fatal("expected binding to exist")
	}
	if binding.Alias != "myproject" {
		t.Errorf("expected alias 'myproject', got '%s'", binding.Alias)
	}
}

func TestWorkspaceManager_Unbind(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	_ = manager.Bind("session-1", tmpDir, false)

	err := manager.Unbind("session-1")
	if err != nil {
		t.Fatalf("Unbind failed: %v", err)
	}

	if manager.IsBound("session-1") {
		t.Error("expected session to be unbound")
	}
}

func TestWorkspaceManager_Unbind_NotBound(t *testing.T) {
	manager := NewWorkspaceManager()

	err := manager.Unbind("nonexistent")
	if err == nil {
		t.Error("expected error for unbound session")
	}
}

func TestWorkspaceManager_Get(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	_ = manager.Bind("session-1", tmpDir, true)

	binding, exists := manager.Get("session-1")
	if !exists {
		t.Fatal("expected binding to exist")
	}
	if binding.Path != tmpDir {
		t.Errorf("expected path %s, got %s", tmpDir, binding.Path)
	}
	if !binding.ReadOnly {
		t.Error("expected ReadOnly to be true")
	}
}

func TestWorkspaceManager_List(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	_ = manager.Bind("session-1", tmpDir, false)
	_ = manager.Bind("session-2", tmpDir, true)

	bindings := manager.List()
	if len(bindings) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(bindings))
	}
}

func TestWorkspaceManager_ResolvePath(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, false)

	absPath, err := manager.ResolvePath("session-1", "subdir/file.txt")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	expected := filepath.Join(tmpDir, "subdir", "file.txt")
	if absPath != expected {
		t.Errorf("expected %s, got %s", expected, absPath)
	}
}

func TestWorkspaceManager_ResolvePath_PathTraversal(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, false)

	_, err := manager.ResolvePath("session-1", "../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestWorkspaceManager_ResolvePath_AbsolutePath(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, false)

	_, err := manager.ResolvePath("session-1", "/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path")
	}
}

func TestWorkspaceManager_ResolvePath_NotBound(t *testing.T) {
	manager := NewWorkspaceManager()

	_, err := manager.ResolvePath("nonexistent", "file.txt")
	if err == nil {
		t.Error("expected error for unbound session")
	}
}

func TestWorkspaceManager_ListFiles(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("test"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

	_ = manager.Bind("session-1", tmpDir, false)

	files, err := manager.ListFiles("session-1", ".")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}

	hasSubdir := false
	for _, f := range files {
		if f.Name == "subdir" && f.IsDir {
			hasSubdir = true
			break
		}
	}
	if !hasSubdir {
		t.Error("expected subdir in file list")
	}
}

func TestWorkspaceManager_ReadFile(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	content := "test content"
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644)

	_ = manager.Bind("session-1", tmpDir, false)

	data, err := manager.ReadFile("session-1", "test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected '%s', got '%s'", content, string(data))
	}
}

func TestWorkspaceManager_WriteFile(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, false)

	content := []byte("new content")
	err := manager.WriteFile("session-1", "newfile.txt", content)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "newfile.txt"))
	if string(data) != string(content) {
		t.Errorf("file content mismatch")
	}
}

func TestWorkspaceManager_WriteFile_ReadOnly(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, true)

	err := manager.WriteFile("session-1", "test.txt", []byte("content"))
	if err == nil {
		t.Error("expected error for read-only workspace")
	}
}

func TestWorkspaceManager_WriteFile_CreateSubdir(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()
	_ = manager.Bind("session-1", tmpDir, false)

	err := manager.WriteFile("session-1", "subdir/nested/file.txt", []byte("content"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = os.Stat(filepath.Join(tmpDir, "subdir", "nested", "file.txt"))
	if err != nil {
		t.Error("expected file to exist")
	}
}

func TestWorkspaceManager_Concurrent(t *testing.T) {
	manager := NewWorkspaceManager()
	tmpDir := t.TempDir()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := string(rune('a' + id))
			_ = manager.Bind(sessionID, tmpDir, false)
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				manager.List()
				manager.Get("a")
				manager.IsBound("b")
			}
		}()
	}

	wg.Wait()

	if manager.Len() == 0 {
		t.Error("expected some bindings after concurrent ops")
	}
}
