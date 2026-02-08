package ipc

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	handler := func(cmd *Command) *Response {
		return &Response{Success: true}
	}
	server := NewServer(handler)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.handler == nil {
		t.Error("handler is nil")
	}
}

func TestCommand_Constants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ActionShowWindow", ActionShowWindow, "show_window"},
		{"ActionHideWindow", ActionHideWindow, "hide_window"},
		{"ActionRestartService", ActionRestartService, "restart_service"},
		{"ActionQuit", ActionQuit, "quit"},
		{"ActionGetStatus", ActionGetStatus, "get_status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestServer_IsRunning_WhenNotStarted(t *testing.T) {
	server := NewServer(nil)
	if server.IsRunning() {
		t.Error("IsRunning() = true, want false when not started")
	}
}
