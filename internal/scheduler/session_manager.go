package scheduler

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"mote/internal/storage"
)

// SessionManager manages session caching and message persistence.
// It provides an LRU cache layer on top of the database.
type SessionManager struct {
	db      *storage.DB
	cache   map[string]*CachedSession
	mu      sync.RWMutex
	maxSize int
}

// NewSessionManager creates a new session manager.
// maxSize specifies the maximum number of sessions to keep in cache.
func NewSessionManager(db *storage.DB, maxSize int) *SessionManager {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &SessionManager{
		db:      db,
		cache:   make(map[string]*CachedSession),
		maxSize: maxSize,
	}
}

// DB returns the underlying storage database.
func (m *SessionManager) DB() *storage.DB {
	return m.db
}

// Get retrieves a session by ID.
// It first checks the cache, then falls back to the database.
func (m *SessionManager) Get(sessionID string) (*CachedSession, error) {
	m.mu.RLock()
	cached, ok := m.cache[sessionID]
	m.mu.RUnlock()

	if ok {
		m.mu.Lock()
		cached.LastAccess = time.Now()
		m.mu.Unlock()
		return cached, nil
	}

	// Cache miss - load from database
	session, err := m.db.GetSession(sessionID)
	if err != nil {
		if err == storage.ErrNotFound {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	messages, err := m.db.GetMessages(sessionID, 0)
	if err != nil {
		return nil, err
	}

	cached = &CachedSession{
		Session:    session,
		Messages:   messages,
		Dirty:      false,
		LastAccess: time.Now(),
	}

	m.mu.Lock()
	m.cache[sessionID] = cached
	m.evict()
	m.mu.Unlock()

	return cached, nil
}

// GetOrCreate retrieves a session by ID or creates a new one if it doesn't exist.
func (m *SessionManager) GetOrCreate(sessionID string, metadata json.RawMessage) (*CachedSession, error) {
	cached, err := m.Get(sessionID)
	if err == nil {
		return cached, nil
	}

	if err != ErrSessionNotFound {
		return nil, err
	}

	// Create new session with the specified ID
	session, err := m.db.CreateSessionWithID(sessionID, metadata)
	if err != nil {
		return nil, err
	}

	cached = &CachedSession{
		Session:    session,
		Messages:   []*storage.Message{},
		Dirty:      false,
		LastAccess: time.Now(),
	}

	m.mu.Lock()
	m.cache[session.ID] = cached
	m.evict()
	m.mu.Unlock()

	return cached, nil
}

// Create creates a new session with optional metadata.
func (m *SessionManager) Create(metadata json.RawMessage) (*CachedSession, error) {
	session, err := m.db.CreateSession(metadata)
	if err != nil {
		return nil, err
	}

	cached := &CachedSession{
		Session:    session,
		Messages:   []*storage.Message{},
		Dirty:      false,
		LastAccess: time.Now(),
	}

	m.mu.Lock()
	m.cache[session.ID] = cached
	m.evict()
	m.mu.Unlock()

	return cached, nil
}

// AddMessage adds a message to a session.
// The message is immediately persisted to the database and added to the cache.
func (m *SessionManager) AddMessage(sessionID, role, content string, toolCalls []storage.ToolCall, toolCallID string) (*storage.Message, error) {
	// Persist to database first
	msg, err := m.db.AppendMessage(sessionID, role, content, toolCalls, toolCallID)
	if err != nil {
		return nil, err
	}

	// Update cache if session is cached
	m.mu.Lock()
	if cached, ok := m.cache[sessionID]; ok {
		cached.Messages = append(cached.Messages, msg)
		cached.LastAccess = time.Now()
		cached.Dirty = true
	}
	m.mu.Unlock()

	return msg, nil
}

// GetMessages returns all messages for a session.
// Returns cached messages if available, otherwise loads from database.
func (m *SessionManager) GetMessages(sessionID string) ([]*storage.Message, error) {
	m.mu.RLock()
	cached, ok := m.cache[sessionID]
	m.mu.RUnlock()

	if ok {
		m.mu.Lock()
		cached.LastAccess = time.Now()
		m.mu.Unlock()
		return cached.Messages, nil
	}

	// Load from database
	messages, err := m.db.GetMessages(sessionID, 0)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// Delete removes a session from cache and database.
func (m *SessionManager) Delete(sessionID string) error {
	m.mu.Lock()
	delete(m.cache, sessionID)
	m.mu.Unlock()

	return m.db.DeleteSession(sessionID)
}

// ReplaceMessages atomically replaces all messages for a session in both
// the database and cache. This is used to persist compaction results so that
// subsequent session loads don't need to re-compact.
func (m *SessionManager) ReplaceMessages(sessionID string, messages []*storage.Message) error {
	// Persist to database (transactional)
	if err := m.db.ReplaceMessages(sessionID, messages); err != nil {
		return err
	}

	// Update cache
	m.mu.Lock()
	if cached, ok := m.cache[sessionID]; ok {
		cached.Messages = messages
		cached.LastAccess = time.Now()
		cached.Dirty = false // Just persisted, so clean
	}
	m.mu.Unlock()

	return nil
}

// Sync writes all dirty sessions to the database.
// This is called periodically or on shutdown to ensure data consistency.
func (m *SessionManager) Sync() error {
	m.mu.RLock()
	var dirtyIDs []string
	for id, cached := range m.cache {
		if cached.Dirty {
			dirtyIDs = append(dirtyIDs, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range dirtyIDs {
		m.mu.Lock()
		if cached, ok := m.cache[id]; ok {
			cached.Dirty = false
		}
		m.mu.Unlock()
	}

	return nil
}

// Invalidate removes a session from the cache.
// The session remains in the database.
func (m *SessionManager) Invalidate(sessionID string) {
	m.mu.Lock()
	delete(m.cache, sessionID)
	m.mu.Unlock()
}

// Clear removes all sessions from the cache.
func (m *SessionManager) Clear() {
	m.mu.Lock()
	m.cache = make(map[string]*CachedSession)
	m.mu.Unlock()
}

// Len returns the number of cached sessions.
func (m *SessionManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.cache)
}

// evict removes the oldest sessions if cache exceeds maxSize.
// Must be called with mu held.
func (m *SessionManager) evict() {
	if len(m.cache) <= m.maxSize {
		return
	}

	// Build list of sessions sorted by LastAccess
	type entry struct {
		id         string
		lastAccess time.Time
	}
	entries := make([]entry, 0, len(m.cache))
	for id, cached := range m.cache {
		entries = append(entries, entry{id, cached.LastAccess})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	// Remove oldest entries
	toRemove := len(m.cache) - m.maxSize
	for i := 0; i < toRemove && i < len(entries); i++ {
		delete(m.cache, entries[i].id)
	}
}
