package backend

import (
	"os"
	"testing"
	"time"
)

func TestNewProcessManager(t *testing.T) {
	pm := NewProcessManager("/usr/local/bin/mote", 18788)
	if pm == nil {
		t.Fatal("NewProcessManager returned nil")
	}
	if pm.motePath != "/usr/local/bin/mote" {
		t.Errorf("motePath = %q, want %q", pm.motePath, "/usr/local/bin/mote")
	}
	if pm.port != 18788 {
		t.Errorf("port = %d, want %d", pm.port, 18788)
	}
}

func TestProcessManager_IsRunning_WhenNotStarted(t *testing.T) {
	pm := NewProcessManager("mote", 18799)
	if pm.IsRunning() {
		t.Error("IsRunning() = true, want false when not started")
	}
}

func TestProcessManager_GetStatus_WhenNotRunning(t *testing.T) {
	pm := NewProcessManager("mote", 18799)
	status := pm.GetStatus()
	if status.Running {
		t.Error("status.Running = true, want false")
	}
	if status.Port != 18799 {
		t.Errorf("status.Port = %d, want %d", status.Port, 18799)
	}
	if status.PID != 0 {
		t.Errorf("status.PID = %d, want 0", status.PID)
	}
}

func TestProcessManager_SetStateChangeCallback(t *testing.T) {
	pm := NewProcessManager("mote", 18799)
	pm.SetStateChangeCallback(func(running bool) {
		// callback set
	})
	if pm.onStateChange == nil {
		t.Error("onStateChange callback not set")
	}
}

func TestProcessManager_WaitForReady_Timeout(t *testing.T) {
	pm := NewProcessManager("mote", 18799)
	start := time.Now()
	err := pm.WaitForReady(500 * time.Millisecond)
	elapsed := time.Since(start)

	if err != ErrServiceTimeout {
		t.Errorf("WaitForReady() error = %v, want ErrServiceTimeout", err)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("WaitForReady returned too quickly: %v", elapsed)
	}
}

func TestProcessManager_StartStop_Integration(t *testing.T) {
	if os.Getenv("MOTE_INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test; set MOTE_INTEGRATION_TEST=1 to run")
	}

	pm := NewProcessManager("mote", 18799)

	if err := pm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := pm.WaitForReady(10 * time.Second); err != nil {
		t.Fatalf("WaitForReady() error = %v", err)
	}

	if !pm.IsRunning() {
		t.Error("IsRunning() = false after start")
	}

	if err := pm.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if pm.IsRunning() {
		t.Error("IsRunning() = true after stop")
	}
}
