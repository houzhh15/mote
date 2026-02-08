package jsvm

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"
)

func TestSandboxSetupAndCleanup(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()
	sandbox := NewSandbox(cfg, nil, logger)

	vm := goja.New()
	ctx := context.Background()

	execCtx, err := sandbox.Setup(vm, ctx, "test.js", "exec-123")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify mote object is injected
	mote := vm.Get("mote")
	if mote == nil || goja.IsUndefined(mote) {
		t.Error("mote object not injected")
	}

	// Verify console object is injected
	console := vm.Get("console")
	if console == nil || goja.IsUndefined(console) {
		t.Error("console object not injected")
	}

	// Cleanup
	sandbox.Cleanup(vm)

	// Verify context is cancelled
	select {
	case <-execCtx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context not cancelled after cleanup")
	}

	// Verify mote object is removed
	moteAfter := vm.Get("mote")
	if moteAfter != nil && !goja.IsUndefined(moteAfter) {
		t.Error("mote object not cleaned up")
	}
}

func TestSandboxTimeout(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()
	cfg.Timeout = 100 * time.Millisecond
	sandbox := NewSandbox(cfg, nil, logger)

	vm := goja.New()
	ctx := context.Background()

	_, err := sandbox.Setup(vm, ctx, "test.js", "exec-timeout")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer sandbox.Cleanup(vm)

	// Execute infinite loop
	_, err = vm.RunString(`
		while(true) {
			// infinite loop
		}
	`)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestSandboxContextCancellation(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()
	cfg.Timeout = 5 * time.Second // Long timeout
	sandbox := NewSandbox(cfg, nil, logger)

	vm := goja.New()
	ctx, cancel := context.WithCancel(context.Background())

	_, err := sandbox.Setup(vm, ctx, "test.js", "exec-cancel")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer sandbox.Cleanup(vm)

	// Start execution in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := vm.RunString(`
			while(true) {
				// infinite loop
			}
		`)
		done <- err
	}()

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Should complete with error
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected interrupt error, got nil")
		}
	case <-time.After(1 * time.Second):
		t.Error("execution did not stop after cancellation")
	}
}

func TestSandboxValidatePath(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()
	cfg.AllowedPaths = []string{"/tmp", "~/test"}
	sandbox := NewSandbox(cfg, nil, logger)

	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{"allowed tmp", "/tmp/test.txt", true},
		{"allowed nested", "/tmp/subdir/file.txt", true},
		{"not allowed etc", "/etc/passwd", false},
		{"not allowed var", "/var/log/syslog", false},
		{"not allowed root", "/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sandbox.ValidatePath(tt.path)
			if result != tt.allowed {
				t.Errorf("ValidatePath(%q) = %v, want %v", tt.path, result, tt.allowed)
			}
		})
	}
}

func TestSandboxConcurrentSetup(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			sandbox := NewSandbox(cfg, nil, logger)
			vm := goja.New()
			ctx := context.Background()

			_, err := sandbox.Setup(vm, ctx, "test.js", "exec-concurrent")
			if err != nil {
				errors <- err
				return
			}

			// Execute some script
			_, err = vm.RunString(`1 + 1`)
			if err != nil {
				errors <- err
				return
			}

			sandbox.Cleanup(vm)
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent setup error: %v", err)
	}
}

func TestSandboxHostAPIInjection(t *testing.T) {
	logger := zerolog.Nop()
	cfg := DefaultSandboxConfig()
	cfg.AllowedPaths = []string{os.TempDir()}
	sandbox := NewSandbox(cfg, nil, logger)

	vm := goja.New()
	ctx := context.Background()

	_, err := sandbox.Setup(vm, ctx, "test.js", "exec-hostapi")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer sandbox.Cleanup(vm)

	// Test mote.log is available
	_, err = vm.RunString(`mote.log.info("test message")`)
	if err != nil {
		t.Errorf("mote.log not available: %v", err)
	}

	// Test mote.fs is available
	_, err = vm.RunString(`mote.fs.exists("/tmp/nonexistent")`)
	if err != nil {
		t.Errorf("mote.fs not available: %v", err)
	}

	// Test mote.http is available
	val, err := vm.RunString(`typeof mote.http.get`)
	if err != nil {
		t.Errorf("mote.http not available: %v", err)
	}
	if val.String() != "function" {
		t.Errorf("mote.http.get is not a function")
	}

	// Test console.log is available
	_, err = vm.RunString(`console.log("test")`)
	if err != nil {
		t.Errorf("console.log not available: %v", err)
	}
}

func TestDefaultSandboxConfig(t *testing.T) {
	cfg := DefaultSandboxConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}

	if cfg.MemoryLimit != 64*1024*1024 {
		t.Errorf("MemoryLimit = %d, want 64MB", cfg.MemoryLimit)
	}

	if cfg.MaxWriteSize != 10*1024*1024 {
		t.Errorf("MaxWriteSize = %d, want 10MB", cfg.MaxWriteSize)
	}

	if len(cfg.AllowedPaths) != 2 {
		t.Errorf("AllowedPaths length = %d, want 2", len(cfg.AllowedPaths))
	}
}
