package gateway

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"mote/internal/gateway/websocket"
	"mote/pkg/logger"
)

const debounceDelay = 100 * time.Millisecond

// Watcher monitors file changes and notifies clients.
type Watcher struct {
	watcher  *fsnotify.Watcher
	hub      *websocket.Hub
	paths    []string
	stopCh   chan struct{}
	debounce map[string]*time.Timer
	mu       sync.Mutex
}

// NewWatcher creates a new file watcher.
func NewWatcher(hub *websocket.Hub, paths ...string) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		watcher:  w,
		hub:      hub,
		paths:    paths,
		stopCh:   make(chan struct{}),
		debounce: make(map[string]*time.Timer),
	}, nil
}

// Start begins watching for file changes.
func (w *Watcher) Start() error {
	// Add paths to watch
	for _, path := range w.paths {
		if err := w.watcher.Add(path); err != nil {
			logger.Warn().Err(err).Str("path", path).Msg("Failed to watch path")
		}
	}

	go w.run()
	return nil
}

// run processes file system events.
func (w *Watcher) run() {
	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only handle write and create events
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.handleEvent(event.Name)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logger.Error().Err(err).Msg("File watcher error")
		}
	}
}

// handleEvent handles a file change event with debouncing.
func (w *Watcher) handleEvent(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Cancel existing timer for this path
	if timer, ok := w.debounce[path]; ok {
		timer.Stop()
	}

	// Create new debounced timer
	w.debounce[path] = time.AfterFunc(debounceDelay, func() {
		w.broadcastReload(path)

		// Clean up timer
		w.mu.Lock()
		delete(w.debounce, path)
		w.mu.Unlock()
	})
}

// broadcastReload sends a reload message to all clients.
func (w *Watcher) broadcastReload(path string) {
	msg := websocket.WSMessage{
		Type: websocket.TypeReload,
		Path: path,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal reload message")
		return
	}

	w.hub.BroadcastAll(data)
	logger.Debug().Str("path", path).Msg("Broadcast reload")
}

// Stop stops the file watcher.
func (w *Watcher) Stop() {
	close(w.stopCh)

	// Cancel all pending timers
	w.mu.Lock()
	for _, timer := range w.debounce {
		timer.Stop()
	}
	w.mu.Unlock()

	w.watcher.Close()
}
