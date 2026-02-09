package jsvm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// ToolRegistry is an interface for registering tools.
type ToolRegistry interface {
	Register(tool Tool) error
	Unregister(name string) error
}

// Tool represents a callable tool.
type Tool interface {
	Name() string
	Description() string
}

// Loader manages loading JavaScript tools from a directory.
type Loader struct {
	runtime    *Runtime
	registry   ToolRegistry
	toolsDir   string
	logger     zerolog.Logger
	watcher    *fsnotify.Watcher
	tools      map[string]*JSTool
	mu         sync.RWMutex
	closed     bool
	debounce   map[string]*time.Timer
	debounceMu sync.Mutex
}

// NewLoader creates a new tool loader.
func NewLoader(runtime *Runtime, registry ToolRegistry, toolsDir string, logger zerolog.Logger) *Loader {
	return &Loader{
		runtime:  runtime,
		registry: registry,
		toolsDir: toolsDir,
		logger:   logger,
		tools:    make(map[string]*JSTool),
		debounce: make(map[string]*time.Timer),
	}
}

// Load scans the tools directory and loads all JavaScript tools.
func (l *Loader) Load() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure directory exists
	if _, err := os.Stat(l.toolsDir); os.IsNotExist(err) {
		l.logger.Debug().Str("dir", l.toolsDir).Msg("tools directory does not exist")
		return nil
	}

	// Scan directory for JS files
	entries, err := os.ReadDir(l.toolsDir)
	if err != nil {
		return fmt.Errorf("failed to read tools directory: %w", err)
	}

	var loadErrors []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".js") {
			continue
		}

		scriptPath := filepath.Join(l.toolsDir, name)
		if err := l.loadToolLocked(scriptPath); err != nil {
			l.logger.Warn().Err(err).Str("file", name).Msg("failed to load tool")
			loadErrors = append(loadErrors, err) //nolint:staticcheck // SA4010: Accumulating errors for potential future use
		}
	}

	l.logger.Info().Int("count", len(l.tools)).Msg("loaded JS tools")
	return nil
}

// loadToolLocked loads a single tool file (must hold lock).
func (l *Loader) loadToolLocked(scriptPath string) error {
	// Extract metadata from script
	meta, err := ExtractToolMetadata(l.runtime, scriptPath)
	if err != nil {
		return fmt.Errorf("failed to extract metadata from %s: %w", scriptPath, err)
	}

	// Create tool config
	cfg := ToolConfig{
		Name:        meta.Name,
		Description: meta.Description,
		Schema:      meta.Schema,
		ScriptPath:  scriptPath,
	}

	// Create tool instance
	tool := NewJSTool(cfg, l.runtime, l.logger)

	// Unregister existing tool if any
	if existing, ok := l.tools[meta.Name]; ok {
		if l.registry != nil {
			_ = l.registry.Unregister(existing.Name())
		}
	}

	// Register new tool
	if l.registry != nil {
		if err := l.registry.Register(tool); err != nil {
			return fmt.Errorf("failed to register tool: %w", err)
		}
	}

	l.tools[meta.Name] = tool
	l.logger.Debug().Str("name", meta.Name).Str("path", scriptPath).Msg("loaded tool")
	return nil
}

// Watch starts watching the tools directory for changes.
func (l *Loader) Watch() error {
	if l.closed {
		return fmt.Errorf("loader is closed")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	l.watcher = watcher

	// Start watching in background
	go l.watchLoop()

	// Add tools directory to watcher
	if err := watcher.Add(l.toolsDir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	l.logger.Info().Str("dir", l.toolsDir).Msg("watching tools directory")
	return nil
}

// watchLoop processes file system events.
func (l *Loader) watchLoop() {
	for {
		select {
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}

			// Only handle JS files
			if !strings.HasSuffix(event.Name, ".js") {
				continue
			}

			// Handle create/write events with debouncing
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				l.debouncedReload(event.Name)
			}

			// Handle remove events
			if event.Op&fsnotify.Remove != 0 {
				l.handleRemove(event.Name)
			}

		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			l.logger.Error().Err(err).Msg("watcher error")
		}
	}
}

// debouncedReload reloads a tool file with 100ms debounce.
func (l *Loader) debouncedReload(path string) {
	l.debounceMu.Lock()
	defer l.debounceMu.Unlock()

	// Cancel existing timer
	if timer, ok := l.debounce[path]; ok {
		timer.Stop()
	}

	// Set new timer
	l.debounce[path] = time.AfterFunc(100*time.Millisecond, func() {
		l.mu.Lock()
		defer l.mu.Unlock()

		if err := l.loadToolLocked(path); err != nil {
			l.logger.Warn().Err(err).Str("path", path).Msg("failed to reload tool")
		} else {
			l.logger.Info().Str("path", path).Msg("reloaded tool")
		}
	})
}

// handleRemove handles a tool file removal.
func (l *Loader) handleRemove(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Find tool by path
	for name, tool := range l.tools {
		if tool.scriptPath == path {
			if l.registry != nil {
				_ = l.registry.Unregister(name)
			}
			delete(l.tools, name)
			l.logger.Info().Str("name", name).Msg("unloaded removed tool")
			return
		}
	}
}

// Unload removes a specific tool.
func (l *Loader) Unload(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tool, ok := l.tools[name]
	if !ok {
		return fmt.Errorf("tool not found: %s", name)
	}

	if l.registry != nil {
		if err := l.registry.Unregister(tool.Name()); err != nil {
			return err
		}
	}

	delete(l.tools, name)
	return nil
}

// Close stops watching and unloads all tools.
func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true

	// Stop watcher
	if l.watcher != nil {
		l.watcher.Close()
	}

	// Cancel all debounce timers
	l.debounceMu.Lock()
	for _, timer := range l.debounce {
		timer.Stop()
	}
	l.debounceMu.Unlock()

	// Unregister all tools
	for name := range l.tools {
		if l.registry != nil {
			_ = l.registry.Unregister(name)
		}
	}
	l.tools = make(map[string]*JSTool)

	return nil
}

// Tools returns a list of loaded tool names.
func (l *Loader) Tools() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	names := make([]string, 0, len(l.tools))
	for name := range l.tools {
		names = append(names, name)
	}
	return names
}
