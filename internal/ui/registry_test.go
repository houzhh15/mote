package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry("/tmp/test-ui")
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.uiDir != "/tmp/test-ui" {
		t.Errorf("uiDir mismatch: got %q, want %q", r.uiDir, "/tmp/test-ui")
	}
	if r.components == nil {
		t.Error("components map should be initialized")
	}
}

func TestRegistry_Scan_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewRegistry(tmpDir)

	err := r.Scan()
	if err != nil {
		t.Fatalf("Scan failed on empty dir: %v", err)
	}

	if r.Count() != 0 {
		t.Errorf("expected 0 components, got %d", r.Count())
	}
}

func TestRegistry_Scan_NonExistentDir(t *testing.T) {
	r := NewRegistry("/nonexistent/path/to/ui")

	err := r.Scan()
	if err != nil {
		t.Fatalf("Scan should not fail on non-existent dir: %v", err)
	}

	if r.Count() != 0 {
		t.Errorf("expected 0 components, got %d", r.Count())
	}
}

func TestRegistry_Scan_EmptyUIDir(t *testing.T) {
	r := NewRegistry("")

	err := r.Scan()
	if err != nil {
		t.Fatalf("Scan should not fail on empty uiDir: %v", err)
	}

	if r.Count() != 0 {
		t.Errorf("expected 0 components, got %d", r.Count())
	}
}

func TestRegistry_Scan_WithManifest(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := Manifest{
		Version: "1.0",
		Components: []Component{
			{Name: "weather", File: "components/weather.js", Description: "Weather widget"},
			{Name: "clock", File: "components/clock.js", Description: "Clock widget"},
		},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	r := NewRegistry(tmpDir)
	if err := r.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if r.Count() != 2 {
		t.Errorf("expected 2 components, got %d", r.Count())
	}

	weather, ok := r.Get("weather")
	if !ok {
		t.Error("weather component not found")
	}
	if weather.Description != "Weather widget" {
		t.Errorf("weather description mismatch: got %q", weather.Description)
	}

	clock, ok := r.Get("clock")
	if !ok {
		t.Error("clock component not found")
	}
	if clock.File != "components/clock.js" {
		t.Errorf("clock file mismatch: got %q", clock.File)
	}
}

func TestRegistry_Scan_ComponentsDir(t *testing.T) {
	tmpDir := t.TempDir()
	componentsDir := filepath.Join(tmpDir, "components")

	if err := os.MkdirAll(componentsDir, 0o755); err != nil {
		t.Fatalf("failed to create components dir: %v", err)
	}

	// Create some component files
	files := []string{"weather.js", "clock.tsx", "timer.ts", "counter.jsx"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(componentsDir, f), []byte("// "+f), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}

	// Create a non-component file that should be ignored
	if err := os.WriteFile(filepath.Join(componentsDir, "readme.md"), []byte("# README"), 0o644); err != nil {
		t.Fatalf("failed to write readme.md: %v", err)
	}

	r := NewRegistry(tmpDir)
	if err := r.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if r.Count() != 4 {
		t.Errorf("expected 4 components, got %d", r.Count())
	}

	for _, name := range []string{"weather", "clock", "timer", "counter"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("component %q not found", name)
		}
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry("")
	_ = r.Scan()

	comp, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get should return false for non-existent component")
	}
	if comp != nil {
		t.Error("Get should return nil for non-existent component")
	}
}

func TestRegistry_List(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := Manifest{
		Version: "1.0",
		Components: []Component{
			{Name: "a", File: "a.js"},
			{Name: "b", File: "b.js"},
			{Name: "c", File: "c.js"},
		},
	}

	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0o644)

	r := NewRegistry(tmpDir)
	_ = r.Scan()

	list := r.List()
	if len(list) != 3 {
		t.Errorf("expected 3 components, got %d", len(list))
	}

	names := make(map[string]bool)
	for _, comp := range list {
		names[comp.Name] = true
	}

	for _, name := range []string{"a", "b", "c"} {
		if !names[name] {
			t.Errorf("component %q not in list", name)
		}
	}
}

func TestRegistry_Refresh(t *testing.T) {
	tmpDir := t.TempDir()

	// Initial scan with one component
	manifest := Manifest{
		Version: "1.0",
		Components: []Component{
			{Name: "initial", File: "initial.js"},
		},
	}

	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0o644)

	r := NewRegistry(tmpDir)
	_ = r.Scan()

	if r.Count() != 1 {
		t.Errorf("expected 1 component, got %d", r.Count())
	}

	// Update manifest with more components
	manifest.Components = append(manifest.Components, Component{Name: "added", File: "added.js"})
	data, _ = json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0o644)

	// Refresh
	if err := r.Refresh(); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if r.Count() != 2 {
		t.Errorf("expected 2 components after refresh, got %d", r.Count())
	}

	if _, ok := r.Get("added"); !ok {
		t.Error("added component not found after refresh")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := Manifest{
		Version: "1.0",
		Components: []Component{
			{Name: "comp1", File: "comp1.js"},
			{Name: "comp2", File: "comp2.js"},
		},
	}

	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0o644)

	r := NewRegistry(tmpDir)
	_ = r.Scan()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_, _ = r.Get("comp1")
			_ = r.Count()
		}()
	}

	// Concurrent refreshes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Refresh(); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestRegistry_Scan_InvalidManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid JSON
	if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), []byte("invalid json"), 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	r := NewRegistry(tmpDir)
	err := r.Scan()

	if err == nil {
		t.Error("Scan should fail on invalid manifest JSON")
	}
}

func TestRegistry_Scan_FileInsteadOfDir(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(tmpFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	r := NewRegistry(tmpFile)
	err := r.Scan()

	if err != nil {
		t.Errorf("Scan should not fail when uiDir is a file: %v", err)
	}
	if r.Count() != 0 {
		t.Errorf("expected 0 components when uiDir is a file, got %d", r.Count())
	}
}
