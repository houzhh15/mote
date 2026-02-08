package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents a unit of work to be executed.
type Task struct {
	SessionID string
	Fn        func(context.Context) error
	Ctx       context.Context
	Cancel    context.CancelFunc
	Result    chan error
}

// sessionQueue manages the task queue for a single session.
type sessionQueue struct {
	tasks     chan *Task
	closed    atomic.Bool
	closeCh   chan struct{}
	closeOnce sync.Once
}

// RunQueue provides per-session FIFO execution queues.
// Tasks for the same session are executed serially, while different sessions can run in parallel.
type RunQueue struct {
	queues      sync.Map // map[string]*sessionQueue
	wg          sync.WaitGroup
	closed      atomic.Bool
	mu          sync.Mutex
	idleTimeout time.Duration
	queueSize   int
}

// NewRunQueue creates a new RunQueue.
func NewRunQueue(queueSize int, idleTimeout time.Duration) *RunQueue {
	if queueSize <= 0 {
		queueSize = 100
	}
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}
	return &RunQueue{
		queueSize:   queueSize,
		idleTimeout: idleTimeout,
	}
}

// Enqueue adds a task to the session's queue and returns a channel for the result.
// Tasks for the same session are executed serially in FIFO order.
func (rq *RunQueue) Enqueue(sessionID string, ctx context.Context, fn func(context.Context) error) (<-chan error, error) {
	if rq.closed.Load() {
		return nil, ErrQueueFull
	}

	// Check context first
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	taskCtx, cancel := context.WithCancel(ctx)
	task := &Task{
		SessionID: sessionID,
		Fn:        fn,
		Ctx:       taskCtx,
		Cancel:    cancel,
		Result:    make(chan error, 1),
	}

	sq := rq.getOrCreateQueue(sessionID)
	if sq.closed.Load() {
		cancel()
		return nil, ErrSessionClosed
	}

	select {
	case sq.tasks <- task:
		return task.Result, nil
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	default:
		cancel()
		return nil, ErrQueueFull
	}
}

// getOrCreateQueue gets an existing session queue or creates a new one.
func (rq *RunQueue) getOrCreateQueue(sessionID string) *sessionQueue {
	if v, ok := rq.queues.Load(sessionID); ok {
		return v.(*sessionQueue)
	}

	rq.mu.Lock()
	defer rq.mu.Unlock()

	// Double check after acquiring lock
	if v, ok := rq.queues.Load(sessionID); ok {
		return v.(*sessionQueue)
	}

	sq := &sessionQueue{
		tasks:   make(chan *Task, rq.queueSize),
		closeCh: make(chan struct{}),
	}
	rq.queues.Store(sessionID, sq)

	rq.wg.Add(1)
	go rq.worker(sessionID, sq)

	return sq
}

// worker processes tasks for a session queue.
func (rq *RunQueue) worker(sessionID string, sq *sessionQueue) {
	defer rq.wg.Done()
	defer func() {
		sq.closed.Store(true)
		rq.queues.Delete(sessionID)
	}()

	idleTimer := time.NewTimer(rq.idleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case task, ok := <-sq.tasks:
			if !ok {
				return
			}

			// Reset idle timer
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(rq.idleTimeout)

			// Execute the task
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = ErrRunCancelled
					}
				}()
				err = task.Fn(task.Ctx)
			}()
			task.Result <- err
			close(task.Result)

		case <-idleTimer.C:
			// Idle timeout, clean up this queue
			return

		case <-sq.closeCh:
			// Queue cancelled
			return
		}
	}
}

// Cancel cancels all pending tasks for a session and stops its worker.
func (rq *RunQueue) Cancel(sessionID string) {
	if v, ok := rq.queues.Load(sessionID); ok {
		sq := v.(*sessionQueue)
		sq.closed.Store(true)
		sq.closeOnce.Do(func() {
			close(sq.closeCh)
		})
	}
}

// Pending returns the number of pending tasks for a session.
func (rq *RunQueue) Pending(sessionID string) int {
	if v, ok := rq.queues.Load(sessionID); ok {
		sq := v.(*sessionQueue)
		return len(sq.tasks)
	}
	return 0
}

// ActiveSessions returns the number of sessions with active workers.
func (rq *RunQueue) ActiveSessions() int {
	count := 0
	rq.queues.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Shutdown gracefully shuts down the queue, waiting for all tasks to complete.
func (rq *RunQueue) Shutdown(ctx context.Context) error {
	rq.closed.Store(true)

	// Close all session queues
	rq.queues.Range(func(key, value any) bool {
		sq := value.(*sessionQueue)
		sq.closed.Store(true)
		sq.closeOnce.Do(func() {
			close(sq.closeCh)
		})
		return true
	})

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		rq.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
