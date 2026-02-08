package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mote/internal/gateway/websocket"
)

func TestNewWatcher(t *testing.T) {
	hub := websocket.NewHub()
	dir := t.TempDir()

	watcher, err := NewWatcher(hub, dir)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	if watcher.hub != hub {
		t.Error("watcher.hub mismatch")
	}
}

func TestWatcherStart(t *testing.T) {
	hub := websocket.NewHub()
	go hub.Run()

	dir := t.TempDir()

	watcher, err := NewWatcher(hub, dir)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	// Start watching
	err = watcher.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// This test mainly verifies the watcher starts without error
}

func TestWatcherDetectsFileChange(t *testing.T) {
	hub := websocket.NewHub()
	go hub.Run()

	dir := t.TempDir()

	watcher, err := NewWatcher(hub, dir)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	// Start watching
	err = watcher.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Create a file to trigger the watcher
	testFile := filepath.Join(dir, "test.js")
	if err := os.WriteFile(testFile, []byte("console.log('test')"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for debounce (100ms) + processing time
	time.Sleep(200 * time.Millisecond)

	// The watcher should have detected the change and broadcast
	// We can't easily verify the broadcast content without a full client setup
}

func TestWatcherStop(t *testing.T) {
	hub := websocket.NewHub()
	dir := t.TempDir()

	watcher, err := NewWatcher(hub, dir)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Start watching
	err = watcher.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic
	watcher.Stop()
}

func TestWatcherMultiplePaths(t *testing.T) {
	hub := websocket.NewHub()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	watcher, err := NewWatcher(hub, dir1, dir2)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	if len(watcher.paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(watcher.paths))
	}
}
