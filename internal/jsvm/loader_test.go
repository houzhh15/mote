package jsvm

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockRegistry is a test implementation of ToolRegistry.
type mockRegistry struct {
	mu    sync.Mutex
	tools map[string]Tool
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *mockRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	return nil
}

func (r *mockRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
	return nil
}

func (r *mockRegistry) Get(name string) Tool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tools[name]
}

func (r *mockRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tools)
}

func TestLoaderLoad(t *testing.T) {
	// Create temp directory with tool files
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	// Create a test tool script
	toolScript := `
		module.exports = {
			name: 'test-tool',
			description: 'A test tool',
			schema: {
				type: 'object',
				properties: {
					input: { type: 'string' }
				}
			},
			handler: function(args) {
				return 'Hello ' + args.input;
			}
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "test-tool.js"), []byte(toolScript), 0644); err != nil {
		t.Fatalf("Failed to write tool script: %v", err)
	}

	// Setup
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	// Load tools
	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	// Verify tool was registered
	if registry.Count() != 1 {
		t.Errorf("Expected 1 tool, got %d", registry.Count())
	}

	tool := registry.Get("test-tool")
	if tool == nil {
		t.Fatal("test-tool not found in registry")
	}

	if tool.Name() != "test-tool" {
		t.Errorf("Expected name 'test-tool', got '%s'", tool.Name())
	}

	if tool.Description() != "A test tool" {
		t.Errorf("Expected description 'A test tool', got '%s'", tool.Description())
	}
}

func TestLoaderLoadEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "empty-tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	if registry.Count() != 0 {
		t.Errorf("Expected 0 tools, got %d", registry.Count())
	}
}

func TestLoaderLoadNonexistentDir(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, "/nonexistent/tools", logger)

	// Should not error, just log and return
	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()
}

func TestLoaderLoadSkipsInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	// Create valid tool
	validScript := `
		module.exports = {
			name: 'valid-tool',
			description: 'Valid',
			handler: function(args) { return args; }
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "valid.js"), []byte(validScript), 0644); err != nil {
		t.Fatalf("Failed to write valid script: %v", err)
	}

	// Create invalid tool (no name)
	invalidScript := `
		module.exports = {
			description: 'Invalid - no name',
			handler: function(args) { return args; }
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "invalid.js"), []byte(invalidScript), 0644); err != nil {
		t.Fatalf("Failed to write invalid script: %v", err)
	}

	// Create non-JS file
	if err := os.WriteFile(filepath.Join(toolsDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("Failed to write txt file: %v", err)
	}

	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	// Load should succeed (invalid files are skipped)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	// Only valid tool should be loaded
	if registry.Count() != 1 {
		t.Errorf("Expected 1 tool (valid only), got %d", registry.Count())
	}

	if registry.Get("valid-tool") == nil {
		t.Error("valid-tool should be registered")
	}
}

func TestLoaderUnload(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	toolScript := `
		module.exports = {
			name: 'unload-test',
			description: 'Test',
			handler: function(args) { return args; }
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "unload.js"), []byte(toolScript), 0644); err != nil {
		t.Fatalf("Failed to write tool script: %v", err)
	}

	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	// Verify loaded
	if registry.Count() != 1 {
		t.Fatalf("Expected 1 tool, got %d", registry.Count())
	}

	// Unload
	if err := loader.Unload("unload-test"); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}

	// Verify unloaded
	if registry.Count() != 0 {
		t.Errorf("Expected 0 tools after unload, got %d", registry.Count())
	}
}

func TestLoaderUnloadNotFound(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, "/tmp", logger)

	err := loader.Unload("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent tool")
	}
}

func TestLoaderWatch(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	if err := loader.Watch(); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Create a new tool file
	toolScript := `
		module.exports = {
			name: 'hot-tool',
			description: 'Hot loaded',
			handler: function(args) { return args; }
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "hot.js"), []byte(toolScript), 0644); err != nil {
		t.Fatalf("Failed to write tool script: %v", err)
	}

	// Wait for debounce + reload
	time.Sleep(300 * time.Millisecond)

	// Verify tool was hot-loaded
	if registry.Get("hot-tool") == nil {
		t.Error("hot-tool should be registered after file creation")
	}
}

func TestLoaderTools(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	for i := 1; i <= 3; i++ {
		name := "tool" + string(rune('0'+i))
		script := `
			module.exports = {
				name: '` + name + `',
				description: 'Tool',
				handler: function(a) { return a; }
			};
		`
		filePath := filepath.Join(toolsDir, name+".js")
		if err := os.WriteFile(filePath, []byte(script), 0644); err != nil {
			t.Fatalf("Failed to write tool script: %v", err)
		}
		t.Logf("Created %s", filePath)
	}

	// List files
	entries, _ := os.ReadDir(toolsDir)
	for _, e := range entries {
		t.Logf("File: %s", e.Name())
	}

	logger := zerolog.New(os.Stdout).Level(zerolog.DebugLevel)
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer loader.Close()

	t.Logf("Loaded tools: %v", loader.Tools())
	t.Logf("Registry count: %d", registry.Count())

	tools := loader.Tools()
	if len(tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(tools))
	}
}

func TestLoaderClose(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	toolScript := `
		module.exports = {
			name: 'close-test',
			description: 'Test',
			handler: function(args) { return args; }
		};
	`
	if err := os.WriteFile(filepath.Join(toolsDir, "close.js"), []byte(toolScript), 0644); err != nil {
		t.Fatalf("Failed to write tool script: %v", err)
	}

	logger := zerolog.Nop()
	cfg := DefaultRuntimeConfig()
	rt := NewRuntime(cfg, nil, logger)
	defer rt.Close()

	registry := newMockRegistry()
	loader := NewLoader(rt, registry, toolsDir, logger)

	if err := loader.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := loader.Watch(); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Verify loaded
	if registry.Count() != 1 {
		t.Fatalf("Expected 1 tool, got %d", registry.Count())
	}

	// Close
	if err := loader.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify unloaded
	if registry.Count() != 0 {
		t.Errorf("Expected 0 tools after close, got %d", registry.Count())
	}
}
