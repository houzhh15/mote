package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunQueue(t *testing.T) {
	t.Run("NewRunQueue", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		if rq == nil {
			t.Fatal("expected non-nil run queue")
		}
		if rq.queueSize != 10 {
			t.Errorf("expected queue size 10, got %d", rq.queueSize)
		}
		if rq.idleTimeout != time.Second {
			t.Errorf("expected idle timeout 1s, got %v", rq.idleTimeout)
		}
	})

	t.Run("NewRunQueue with defaults", func(t *testing.T) {
		rq := NewRunQueue(0, 0)
		if rq.queueSize != 100 {
			t.Errorf("expected default queue size 100, got %d", rq.queueSize)
		}
		if rq.idleTimeout != 30*time.Second {
			t.Errorf("expected default idle timeout 30s, got %v", rq.idleTimeout)
		}
	})

	t.Run("Enqueue and execute", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		executed := false
		result, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
			executed = true
			return nil
		})
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		err = <-result
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if !executed {
			t.Error("task was not executed")
		}
	})

	t.Run("Serial execution for same session", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		var order []int
		var mu sync.Mutex

		var results []<-chan error
		for i := 0; i < 5; i++ {
			idx := i
			result, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				order = append(order, idx)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Fatalf("failed to enqueue: %v", err)
			}
			results = append(results, result)
		}

		for _, result := range results {
			<-result
		}

		for i := 0; i < 5; i++ {
			if order[i] != i {
				t.Errorf("expected order[%d] = %d, got %d", i, i, order[i])
			}
		}
	})

	t.Run("Parallel execution for different sessions", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		var running int32
		var maxRunning int32
		var wg sync.WaitGroup

		for i := 0; i < 5; i++ {
			sessionID := string(rune('a' + i))
			wg.Add(1)
			result, err := rq.Enqueue(sessionID, context.Background(), func(ctx context.Context) error {
				defer wg.Done()
				cur := atomic.AddInt32(&running, 1)
				for {
					max := atomic.LoadInt32(&maxRunning)
					if cur > max {
						if atomic.CompareAndSwapInt32(&maxRunning, max, cur) {
							break
						}
					} else {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&running, -1)
				return nil
			})
			if err != nil {
				t.Fatalf("failed to enqueue: %v", err)
			}
			go func() { <-result }()
		}

		wg.Wait()
		if maxRunning < 2 {
			t.Errorf("expected multiple sessions to run in parallel, max running was %d", maxRunning)
		}
	})

	t.Run("Error propagation", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		expectedErr := errors.New("task error")
		result, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
			return expectedErr
		})
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		err = <-result
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := rq.Enqueue("session1", ctx, func(ctx context.Context) error {
			return nil
		})
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("Cancel session", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		// Start a blocking task
		started := make(chan struct{})
		done := make(chan struct{})
		result, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
			close(started)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return nil
			}
		})
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		<-started
		rq.Cancel("session1")
		close(done)

		// Wait for result with timeout
		select {
		case <-result:
			// OK - task completed or was cancelled
		case <-time.After(time.Second):
			t.Error("task did not complete after cancel")
		}
	})

	t.Run("ActiveSessions", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		if rq.ActiveSessions() != 0 {
			t.Errorf("expected 0 active sessions, got %d", rq.ActiveSessions())
		}

		// Start tasks for multiple sessions
		var results []<-chan error
		for i := 0; i < 3; i++ {
			result, err := rq.Enqueue(string(rune('a'+i)), context.Background(), func(ctx context.Context) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Fatalf("failed to enqueue: %v", err)
			}
			results = append(results, result)
		}

		// Give workers time to start
		time.Sleep(20 * time.Millisecond)

		active := rq.ActiveSessions()
		if active < 1 || active > 3 {
			t.Errorf("expected 1-3 active sessions, got %d", active)
		}

		for _, r := range results {
			<-r
		}
	})

	t.Run("Pending", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		defer func() { _ = rq.Shutdown(context.Background()) }()

		// Enqueue a blocking task
		blockCh := make(chan struct{})
		_, _ = rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
			<-blockCh
			return nil
		})

		// Give worker time to start
		time.Sleep(20 * time.Millisecond)

		// Enqueue more tasks
		for i := 0; i < 3; i++ {
			_, _ = rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
				return nil
			})
		}

		pending := rq.Pending("session1")
		if pending != 3 {
			t.Errorf("expected 3 pending tasks, got %d", pending)
		}

		close(blockCh)
	})

	t.Run("Shutdown", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)

		var completed int32
		for i := 0; i < 3; i++ {
			_, _ = rq.Enqueue(string(rune('a'+i)), context.Background(), func(ctx context.Context) error {
				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&completed, 1)
				return nil
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := rq.Shutdown(ctx)
		if err != nil {
			t.Errorf("shutdown error: %v", err)
		}
	})

	t.Run("Enqueue after shutdown", func(t *testing.T) {
		rq := NewRunQueue(10, time.Second)
		_ = rq.Shutdown(context.Background())

		_, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
			return nil
		})
		if err != ErrQueueFull {
			t.Errorf("expected ErrQueueFull after shutdown, got %v", err)
		}
	})
}

func TestRunQueueIdleTimeout(t *testing.T) {
	rq := NewRunQueue(10, 100*time.Millisecond)
	defer func() { _ = rq.Shutdown(context.Background()) }()

	// Enqueue and complete a task
	result, err := rq.Enqueue("session1", context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}
	<-result

	// Wait for idle timeout
	time.Sleep(200 * time.Millisecond)

	// Check that the worker has been cleaned up
	if rq.ActiveSessions() != 0 {
		t.Errorf("expected 0 active sessions after idle timeout, got %d", rq.ActiveSessions())
	}
}
