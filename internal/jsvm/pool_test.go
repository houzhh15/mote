package jsvm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestNewVMPool(t *testing.T) {
	pool := NewVMPool(DefaultPoolConfig())
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxSize != 5 {
		t.Errorf("expected MaxSize 5, got %d", stats.MaxSize)
	}
	if stats.Created != 0 {
		t.Errorf("expected Created 0, got %d", stats.Created)
	}
}

func TestNewVMPool_WithCustomConfig(t *testing.T) {
	cfg := PoolConfig{
		MaxSize:        10,
		IdleTimeout:    10 * time.Minute,
		AcquireTimeout: 10 * time.Second,
	}
	pool := NewVMPool(cfg)
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxSize != 10 {
		t.Errorf("expected MaxSize 10, got %d", stats.MaxSize)
	}
}

func TestNewVMPool_InvalidConfig(t *testing.T) {
	cfg := PoolConfig{
		MaxSize:        0,
		IdleTimeout:    0,
		AcquireTimeout: 0,
	}
	pool := NewVMPool(cfg)
	defer pool.Close()

	stats := pool.Stats()
	if stats.MaxSize != 5 {
		t.Errorf("expected default MaxSize 5, got %d", stats.MaxSize)
	}
}

func TestVMPool_AcquireRelease(t *testing.T) {
	pool := NewVMPool(PoolConfig{MaxSize: 2})
	defer pool.Close()

	ctx := context.Background()

	// Acquire first VM
	vm1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire VM: %v", err)
	}
	if vm1 == nil {
		t.Fatal("acquired VM is nil")
	}

	stats := pool.Stats()
	if stats.Created != 1 {
		t.Errorf("expected Created 1, got %d", stats.Created)
	}
	if stats.Active != 1 {
		t.Errorf("expected Active 1, got %d", stats.Active)
	}

	// Release VM
	pool.Release(vm1)

	stats = pool.Stats()
	if stats.Active != 0 {
		t.Errorf("expected Active 0 after release, got %d", stats.Active)
	}
	if stats.Pooled != 1 {
		t.Errorf("expected Pooled 1, got %d", stats.Pooled)
	}

	// Acquire again - should get the same VM from pool
	vm2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire VM: %v", err)
	}
	if vm2 != vm1 {
		t.Log("got different VM from pool (expected same)")
	}

	pool.Release(vm2)
}

func TestVMPool_AcquireMultiple(t *testing.T) {
	pool := NewVMPool(PoolConfig{MaxSize: 3})
	defer pool.Close()

	ctx := context.Background()

	var vms []*goja.Runtime
	for i := 0; i < 3; i++ {
		vm, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("failed to acquire VM %d: %v", i, err)
		}
		vms = append(vms, vm)
	}

	stats := pool.Stats()
	if stats.Created != 3 {
		t.Errorf("expected Created 3, got %d", stats.Created)
	}
	if stats.Active != 3 {
		t.Errorf("expected Active 3, got %d", stats.Active)
	}

	// Release all
	for _, vm := range vms {
		pool.Release(vm)
	}

	stats = pool.Stats()
	if stats.Active != 0 {
		t.Errorf("expected Active 0 after release, got %d", stats.Active)
	}
}

func TestVMPool_AcquireTimeout(t *testing.T) {
	pool := NewVMPool(PoolConfig{
		MaxSize:        1,
		AcquireTimeout: 100 * time.Millisecond,
	})
	defer pool.Close()

	ctx := context.Background()

	// Acquire the only available slot
	vm1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire VM: %v", err)
	}

	// Try to acquire another - should timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(ctx2)
	if err != ErrVMPoolExhausted {
		t.Errorf("expected ErrVMPoolExhausted, got %v", err)
	}

	pool.Release(vm1)
}

func TestVMPool_ConcurrentAccess(t *testing.T) {
	pool := NewVMPool(PoolConfig{MaxSize: 5})
	defer pool.Close()

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				vm, err := pool.Acquire(ctx)
				cancel()

				if err != nil {
					errorCount.Add(1)
					continue
				}

				// Simulate some work
				time.Sleep(time.Millisecond)

				pool.Release(vm)
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Success: %d, Errors: %d", successCount.Load(), errorCount.Load())

	if successCount.Load() == 0 {
		t.Error("expected at least some successful acquisitions")
	}
}

func TestVMPool_Close(t *testing.T) {
	pool := NewVMPool(PoolConfig{MaxSize: 2})

	ctx := context.Background()

	// Acquire and release to populate pool
	vm, _ := pool.Acquire(ctx)
	pool.Release(vm)

	// Close the pool
	err := pool.Close()
	if err != nil {
		t.Fatalf("failed to close pool: %v", err)
	}

	// Try to acquire after close
	_, err = pool.Acquire(ctx)
	if err != ErrVMPoolExhausted {
		t.Errorf("expected ErrVMPoolExhausted after close, got %v", err)
	}

	// Double close should be safe
	err = pool.Close()
	if err != nil {
		t.Errorf("double close should not error: %v", err)
	}
}

func TestVMPool_ReleaseNil(t *testing.T) {
	pool := NewVMPool(DefaultPoolConfig())
	defer pool.Close()

	// Should not panic
	pool.Release(nil)
}

func TestVMPool_ExpiredInstance(t *testing.T) {
	pool := NewVMPool(PoolConfig{
		MaxSize:     2,
		IdleTimeout: 10 * time.Millisecond,
	})
	defer pool.Close()

	ctx := context.Background()

	// Acquire and release
	vm, _ := pool.Acquire(ctx)
	pool.Release(vm)

	// Wait for expiry
	time.Sleep(50 * time.Millisecond)

	// Acquire again - should get new VM since old one expired
	vm2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire after expiry: %v", err)
	}

	// VM2 should be a fresh instance (can't verify directly, but shouldn't error)
	if vm2 == nil {
		t.Error("acquired VM is nil")
	}

	pool.Release(vm2)
}

func TestVMPool_Stats(t *testing.T) {
	pool := NewVMPool(PoolConfig{
		MaxSize:     3,
		IdleTimeout: 5 * time.Minute,
	})
	defer pool.Close()

	ctx := context.Background()

	// Initial stats
	stats := pool.Stats()
	if stats.MaxSize != 3 {
		t.Errorf("expected MaxSize 3, got %d", stats.MaxSize)
	}
	if stats.IdleTimeout != 5*time.Minute {
		t.Errorf("expected IdleTimeout 5m, got %v", stats.IdleTimeout)
	}

	// Acquire two VMs
	vm1, _ := pool.Acquire(ctx)
	vm2, _ := pool.Acquire(ctx)

	stats = pool.Stats()
	if stats.Created != 2 {
		t.Errorf("expected Created 2, got %d", stats.Created)
	}
	if stats.Active != 2 {
		t.Errorf("expected Active 2, got %d", stats.Active)
	}

	// Release one
	pool.Release(vm1)

	stats = pool.Stats()
	if stats.Active != 1 {
		t.Errorf("expected Active 1, got %d", stats.Active)
	}
	if stats.Pooled != 1 {
		t.Errorf("expected Pooled 1, got %d", stats.Pooled)
	}

	pool.Release(vm2)
}
