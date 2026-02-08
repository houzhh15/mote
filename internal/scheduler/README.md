# Scheduler Package

This package provides session management and task execution queue for the Mote agent runtime.

## Overview

The scheduler package handles:
- Session caching with LRU eviction
- Per-session FIFO task queues
- Run state management

## SessionManager

Manages session lifecycle with caching:

```go
// Create a new session manager (max 100 cached sessions)
db, _ := storage.Open("data.db")
sm := scheduler.NewSessionManager(db, 100)

// Create a new session
cached, err := sm.Create(nil)

// Get existing session (loads from cache or DB)
cached, err := sm.Get(sessionID)

// Get or create
cached, err := sm.GetOrCreate(sessionID, nil)

// Add message to session
msg, err := sm.AddMessage(sessionID, "user", "Hello", nil, "")

// Get all messages
messages, err := sm.GetMessages(sessionID)

// Delete session
err := sm.Delete(sessionID)

// Invalidate cache only (session remains in DB)
sm.Invalidate(sessionID)

// Clear all cache
sm.Clear()
```

### CachedSession

```go
type CachedSession struct {
    Session    *storage.Session
    Messages   []*storage.Message
    Dirty      bool
    LastAccess time.Time
}
```

## RunQueue

Per-session FIFO execution queue with parallel session support:

```go
// Create queue (100 task buffer, 30s idle timeout)
rq := scheduler.NewRunQueue(100, 30*time.Second)

// Enqueue a task
result, err := rq.Enqueue(sessionID, ctx, func(ctx context.Context) error {
    // Do work
    return nil
})

// Wait for result
err := <-result

// Check pending tasks
pending := rq.Pending(sessionID)

// Cancel session queue
rq.Cancel(sessionID)

// Graceful shutdown
err := rq.Shutdown(ctx)
```

### Key Features

- **Session Isolation**: Tasks for the same session execute serially
- **Parallel Sessions**: Different sessions can execute in parallel
- **Idle Cleanup**: Worker goroutines exit after idle timeout
- **Graceful Shutdown**: Waits for in-flight tasks to complete

## Run States

```go
const (
    RunStatePending   RunState = iota  // Not started
    RunStateRunning                     // Executing
    RunStateCompleted                   // Success
    RunStateFailed                      // Error
    RunStateCancelled                   // Cancelled
)
```

## Error Types

- `ErrSessionNotFound`: Session doesn't exist
- `ErrSessionClosed`: Session has been closed
- `ErrQueueFull`: Task queue is at capacity
- `ErrRunNotFound`: Run doesn't exist
- `ErrRunCancelled`: Run was cancelled
