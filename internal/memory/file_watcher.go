package memory

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// FileWatcherCallback is called when file changes are detected after debounce.
type FileWatcherCallback func(changedFiles []string)

// FileWatcher watches Markdown memory files for changes and triggers sync.
type FileWatcher struct {
	watcher       *fsnotify.Watcher
	callback      FileWatcherCallback
	debounceDelay time.Duration
	logger        zerolog.Logger

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	stopCh  chan struct{}
	stopped bool
}

// NewFileWatcher creates a new FileWatcher.
// It watches the given paths (files or directories) for changes.
// The callback is invoked after debounceDelay of inactivity.
func NewFileWatcher(paths []string, callback FileWatcherCallback, logger zerolog.Logger) (*FileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:       w,
		callback:      callback,
		debounceDelay: 500 * time.Millisecond,
		logger:        logger,
		pending:       make(map[string]struct{}),
		stopCh:        make(chan struct{}),
	}

	// Add paths to watcher
	for _, p := range paths {
		if err := w.Add(p); err != nil {
			fw.logger.Warn().Err(err).Str("path", p).Msg("file_watcher: failed to watch path")
		} else {
			fw.logger.Debug().Str("path", p).Msg("file_watcher: watching path")
		}
	}

	go fw.loop()

	return fw, nil
}

// loop runs the main event loop.
func (fw *FileWatcher) loop() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if !isMarkdownFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}

			fw.logger.Debug().
				Str("file", event.Name).
				Str("op", event.Op.String()).
				Msg("file_watcher: file changed")

			fw.addPending(event.Name)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.Error().Err(err).Msg("file_watcher: watcher error")

		case <-fw.stopCh:
			return
		}
	}
}

// addPending adds a file to the pending set and resets the debounce timer.
func (fw *FileWatcher) addPending(file string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	fw.pending[file] = struct{}{}

	if fw.timer != nil {
		fw.timer.Stop()
	}

	fw.timer = time.AfterFunc(fw.debounceDelay, fw.firePending)
}

// firePending invokes the callback with all pending files.
func (fw *FileWatcher) firePending() {
	fw.mu.Lock()
	files := make([]string, 0, len(fw.pending))
	for f := range fw.pending {
		files = append(files, f)
	}
	fw.pending = make(map[string]struct{})
	fw.mu.Unlock()

	if len(files) == 0 {
		return
	}

	fw.logger.Info().
		Int("files", len(files)).
		Msg("file_watcher: triggering sync")

	fw.callback(files)
}

// Close stops the watcher.
func (fw *FileWatcher) Close() error {
	fw.mu.Lock()
	if fw.stopped {
		fw.mu.Unlock()
		return nil
	}
	fw.stopped = true
	if fw.timer != nil {
		fw.timer.Stop()
	}
	fw.mu.Unlock()

	close(fw.stopCh)
	return fw.watcher.Close()
}

// SetDebounceDelay sets the debounce delay.
func (fw *FileWatcher) SetDebounceDelay(d time.Duration) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.debounceDelay = d
}

// isMarkdownFile checks if a file path is a Markdown file.
func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md"
}
