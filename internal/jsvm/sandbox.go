package jsvm

import (
	"context"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"

	"mote/internal/jsvm/hostapi"
	"mote/internal/storage"
)

// SandboxConfig holds configuration for the sandbox environment.
type SandboxConfig struct {
	// Timeout is the maximum execution time for scripts.
	Timeout time.Duration
	// MemoryLimit is the maximum memory usage (parsed from string like "64MB").
	MemoryLimit int64
	// AllowedPaths is the list of allowed file system paths.
	AllowedPaths []string
	// HTTPAllowlist is the list of allowed HTTP domains.
	HTTPAllowlist []string
	// MaxWriteSize is the maximum file write size in bytes.
	MaxWriteSize int64
}

// DefaultSandboxConfig returns default sandbox configuration.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Timeout:       30 * time.Second,
		MemoryLimit:   64 * 1024 * 1024, // 64MB
		AllowedPaths:  []string{"~/.mote/", "/tmp"},
		HTTPAllowlist: nil,
		MaxWriteSize:  10 * 1024 * 1024, // 10MB
	}
}

// Sandbox provides a secure execution environment for JavaScript.
type Sandbox struct {
	config SandboxConfig
	db     *storage.DB
	logger zerolog.Logger

	mu         sync.Mutex
	cancelFunc context.CancelFunc
	done       chan struct{} // signals cleanup to interrupt goroutine
}

// NewSandbox creates a new sandbox with the given configuration.
func NewSandbox(cfg SandboxConfig, db *storage.DB, logger zerolog.Logger) *Sandbox {
	return &Sandbox{
		config: cfg,
		db:     db,
		logger: logger,
	}
}

// Setup configures the VM with security restrictions and injects Host APIs.
func (s *Sandbox) Setup(vm *goja.Runtime, ctx context.Context, scriptName, executionID string) (context.Context, error) {
	s.mu.Lock()

	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	s.cancelFunc = cancel
	s.done = make(chan struct{})
	done := s.done // Copy under lock
	s.mu.Unlock()

	// Setup interrupt on context cancellation
	go func() {
		select {
		case <-execCtx.Done():
			vm.Interrupt("execution interrupted: " + execCtx.Err().Error())
		case <-done:
			// Cleanup was called, don't interrupt
			return
		}
	}()

	s.mu.Lock()

	// Configure field name mapper to restrict access
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Inject Host APIs
	hctx := &hostapi.Context{
		Ctx:         execCtx,
		DB:          s.db,
		Logger:      s.logger,
		ScriptName:  scriptName,
		ExecutionID: executionID,
		Config: hostapi.Config{
			AllowedPaths:  s.config.AllowedPaths,
			HTTPAllowlist: s.config.HTTPAllowlist,
			MaxWriteSize:  s.config.MaxWriteSize,
		},
	}
	s.mu.Unlock()

	if err := hostapi.Register(vm, hctx); err != nil {
		cancel()
		return nil, err
	}

	return execCtx, nil
}

// Cleanup removes injected objects and cancels any pending operations.
func (s *Sandbox) Cleanup(vm *goja.Runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Signal goroutine to stop before cancelling context
	if s.done != nil {
		close(s.done)
		s.done = nil
	}

	// Cancel context if still active
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}

	// Remove injected globals
	hostapi.Unregister(vm)

	// Clear any pending interrupts
	vm.ClearInterrupt()
}

// ValidatePath checks if a path is within the allowed directories.
func (s *Sandbox) ValidatePath(path string) bool {
	// Delegate to hostapi's path validation
	_, err := hostapi.ValidatePathPublic(path, s.config.AllowedPaths)
	return err == nil
}
