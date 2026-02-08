package skills

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileConfigStore_GetSet(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "configs.json")
	store := NewFileConfigStore(storePath)

	// Test Get on empty store
	cfg, err := store.Get("skill-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %v", cfg)
	}

	// Test Set
	testCfg := ConfigMap{
		"key1": "value1",
		"key2": float64(42),
	}
	if err := store.Set("skill-1", testCfg); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get after Set
	cfg, err = store.Get("skill-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if cfg["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %v", cfg["key1"])
	}
	if cfg["key2"] != float64(42) {
		t.Errorf("expected key2=42, got %v", cfg["key2"])
	}
}

func TestFileConfigStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "configs.json")
	store := NewFileConfigStore(storePath)

	// Set a config
	if err := store.Set("skill-1", ConfigMap{"key": "val"}); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify it exists
	cfg, _ := store.Get("skill-1")
	if cfg == nil {
		t.Fatal("expected config to exist")
	}

	// Delete
	if err := store.Delete("skill-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	cfg, _ = store.Get("skill-1")
	if cfg != nil {
		t.Errorf("expected nil config after delete, got %v", cfg)
	}
}

func TestFileConfigStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "configs.json")
	store := NewFileConfigStore(storePath)

	// List on empty store
	ids, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}

	// Add some configs
	_ = store.Set("skill-a", ConfigMap{"a": 1})
	_ = store.Set("skill-b", ConfigMap{"b": 2})
	_ = store.Set("skill-c", ConfigMap{"c": 3})

	// List again
	ids, err = store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 ids, got %d", len(ids))
	}
}

func TestFileConfigStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "configs.json")

	// Create store and set config
	store1 := NewFileConfigStore(storePath)
	_ = store1.Set("skill-1", ConfigMap{"persist": "test"})

	// Create new store instance reading same file
	store2 := NewFileConfigStore(storePath)
	cfg, err := store2.Get("skill-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if cfg["persist"] != "test" {
		t.Errorf("persistence failed: expected persist=test, got %v", cfg["persist"])
	}
}

func TestFileConfigStore_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "configs.json")
	store := NewFileConfigStore(storePath)

	var wg sync.WaitGroup
	numGoroutines := 10
	numOps := 50

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				skillID := "skill-" + string(rune('a'+id%26))
				_ = store.Set(skillID, ConfigMap{"iter": j})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				_, _ = store.Get("skill-a")
				_, _ = store.List()
			}
		}()
	}

	wg.Wait()

	// Verify store is still consistent
	ids, err := store.List()
	if err != nil {
		t.Fatalf("List failed after concurrent ops: %v", err)
	}
	if len(ids) == 0 {
		t.Error("expected some skills after concurrent writes")
	}
}

func TestFileConfigStore_DefaultPath(t *testing.T) {
	// Test with empty path uses default
	store := NewFileConfigStore("")
	if store.path == "" {
		t.Error("expected default path to be set")
	}

	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".mote", "skill-configs.json")
	if store.path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, store.path)
	}
}
