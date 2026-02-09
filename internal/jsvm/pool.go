package jsvm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

// PoolConfig holds configuration for the VM pool.
type PoolConfig struct {
	// MaxSize is the maximum number of VM instances in the pool.
	MaxSize int
	// IdleTimeout is the duration after which an idle VM is evicted.
	IdleTimeout time.Duration
	// AcquireTimeout is the maximum time to wait for a VM.
	AcquireTimeout time.Duration
}

// DefaultPoolConfig returns a PoolConfig with sensible defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize:        5,
		IdleTimeout:    5 * time.Minute,
		AcquireTimeout: 5 * time.Second,
	}
}

// vmInstance wraps a goja.Runtime with metadata.
type vmInstance struct {
	vm         *goja.Runtime
	createdAt  time.Time
	lastUsedAt time.Time
	execCount  int64 //nolint:unused // Reserved for metrics/monitoring
}

// isExpired checks if the instance has exceeded the idle timeout.
func (v *vmInstance) isExpired(idleTimeout time.Duration) bool {
	return time.Since(v.lastUsedAt) > idleTimeout
}

// VMPool manages a pool of goja.Runtime instances.
type VMPool struct {
	pool           chan *vmInstance
	maxSize        int
	idleTimeout    time.Duration
	acquireTimeout time.Duration
	createCount    atomic.Int64
	activeCount    atomic.Int64

	mu       sync.Mutex
	closed   bool
	closedCh chan struct{}
	wg       sync.WaitGroup
}

// NewVMPool creates a new VM pool with the given configuration.
func NewVMPool(cfg PoolConfig) *VMPool {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 5
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.AcquireTimeout <= 0 {
		cfg.AcquireTimeout = 5 * time.Second
	}

	p := &VMPool{
		pool:           make(chan *vmInstance, cfg.MaxSize),
		maxSize:        cfg.MaxSize,
		idleTimeout:    cfg.IdleTimeout,
		acquireTimeout: cfg.AcquireTimeout,
		closedCh:       make(chan struct{}),
	}

	// Start background cleanup goroutine
	p.wg.Add(1)
	go p.cleanupLoop()

	return p
}

// Acquire retrieves a VM instance from the pool or creates a new one.
// It blocks until a VM is available or the context is cancelled.
func (p *VMPool) Acquire(ctx context.Context) (*goja.Runtime, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrVMPoolExhausted
	}
	p.mu.Unlock()

	// Create timeout context if none provided
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), p.acquireTimeout)
		defer cancel()
	}

	// Try to get from pool first (non-blocking)
	select {
	case inst := <-p.pool:
		if inst != nil && !inst.isExpired(p.idleTimeout) {
			inst.lastUsedAt = time.Now()
			p.activeCount.Add(1)
			return inst.vm, nil
		}
		// Instance expired, create new one
	default:
		// Pool empty, try to create new
	}

	// Check if we can create a new instance
	for {
		current := p.createCount.Load()
		if current < int64(p.maxSize) {
			if p.createCount.CompareAndSwap(current, current+1) {
				vm := goja.New()
				p.activeCount.Add(1)
				return vm, nil
			}
			continue
		}
		break
	}

	// Pool is at capacity, wait for a VM to be released
	select {
	case inst := <-p.pool:
		if inst != nil {
			if inst.isExpired(p.idleTimeout) {
				// Expired, create new one
				vm := goja.New()
				p.activeCount.Add(1)
				return vm, nil
			}
			inst.lastUsedAt = time.Now()
			p.activeCount.Add(1)
			return inst.vm, nil
		}
		// nil instance, create new
		vm := goja.New()
		p.activeCount.Add(1)
		return vm, nil
	case <-ctx.Done():
		return nil, ErrVMPoolExhausted
	case <-p.closedCh:
		return nil, ErrVMPoolExhausted
	}
}

// Release returns a VM instance to the pool.
// The VM's global state is cleared before returning to the pool.
func (p *VMPool) Release(vm *goja.Runtime) {
	if vm == nil {
		return
	}

	p.activeCount.Add(-1)

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	// Clear global state by creating a fresh global object
	// Note: goja doesn't have a built-in reset, so we clear known globals
	p.clearGlobals(vm)

	inst := &vmInstance{
		vm:         vm,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
	}

	// Try to put back in pool (non-blocking)
	select {
	case p.pool <- inst:
		// Successfully returned to pool
	default:
		// Pool is full, discard this VM
		p.createCount.Add(-1)
	}
}

// clearGlobals removes injected global objects from the VM.
func (p *VMPool) clearGlobals(vm *goja.Runtime) {
	// Remove mote namespace if it exists
	_ = vm.GlobalObject().Delete("mote")
	_ = vm.GlobalObject().Delete("console")
	// Clear any scheduled interrupts
	vm.ClearInterrupt()
}

// Close shuts down the pool and releases all resources.
func (p *VMPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.closedCh)
	p.mu.Unlock()

	// Drain the pool
	close(p.pool)
	for range p.pool {
		p.createCount.Add(-1)
	}

	// Wait for cleanup goroutine to finish
	p.wg.Wait()

	return nil
}

// cleanupLoop periodically evicts idle VMs from the pool.
func (p *VMPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.evictExpired()
		case <-p.closedCh:
			return
		}
	}
}

// evictExpired removes expired instances from the pool.
func (p *VMPool) evictExpired() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	// Collect non-expired instances
	var kept []*vmInstance
	timeout := time.After(100 * time.Millisecond)

loop:
	for {
		select {
		case inst := <-p.pool:
			if inst == nil {
				continue
			}
			if inst.isExpired(p.idleTimeout) {
				p.createCount.Add(-1)
			} else {
				kept = append(kept, inst)
			}
		case <-timeout:
			break loop
		default:
			break loop
		}
	}

	// Put back non-expired instances
	for _, inst := range kept {
		select {
		case p.pool <- inst:
		default:
			// Pool somehow full, discard
			p.createCount.Add(-1)
		}
	}
}

// Stats returns current pool statistics.
func (p *VMPool) Stats() PoolStats {
	return PoolStats{
		MaxSize:     p.maxSize,
		Created:     int(p.createCount.Load()),
		Active:      int(p.activeCount.Load()),
		Pooled:      len(p.pool),
		IdleTimeout: p.idleTimeout,
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	MaxSize     int
	Created     int
	Active      int
	Pooled      int
	IdleTimeout time.Duration
}
