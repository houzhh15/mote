package websocket

import (
	"testing"
	"time"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()

	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.clients == nil { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Error("clients map is nil")
	}

	if hub.sessions == nil { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Error("sessions map is nil")
	}
}

func TestHubClientCount(t *testing.T) {
	hub := NewHub()

	if hub.ClientCount() != 0 {
		t.Errorf("ClientCount = %d, want 0", hub.ClientCount())
	}
}

func TestHubRegisterUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Create a mock client
	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	// Register
	hub.Register(client)
	time.Sleep(10 * time.Millisecond) // Allow goroutine to process

	if hub.ClientCount() != 1 {
		t.Errorf("ClientCount after register = %d, want 1", hub.ClientCount())
	}

	// Unregister
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond) // Allow goroutine to process

	if hub.ClientCount() != 0 {
		t.Errorf("ClientCount after unregister = %d, want 0", hub.ClientCount())
	}
}

func TestHubSubscribe(t *testing.T) {
	hub := NewHub()

	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	hub.Subscribe(client, "session-1")

	if !client.sessions["session-1"] {
		t.Error("client.sessions does not contain session-1")
	}

	if _, ok := hub.sessions["session-1"]; !ok {
		t.Error("hub.sessions does not contain session-1")
	}

	if !hub.sessions["session-1"][client] {
		t.Error("hub.sessions[session-1] does not contain client")
	}
}

func TestHubUnsubscribe(t *testing.T) {
	hub := NewHub()

	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	hub.Subscribe(client, "session-1")
	hub.Unsubscribe(client, "session-1")

	if client.sessions["session-1"] {
		t.Error("client.sessions still contains session-1")
	}

	if _, ok := hub.sessions["session-1"]; ok {
		t.Error("hub.sessions still contains session-1 (should be cleaned up)")
	}
}

func TestHubBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	// Register and subscribe
	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()
	hub.Subscribe(client, "session-1")

	// Broadcast to session
	testMsg := []byte(`{"type":"stream","delta":"test"}`)
	hub.Broadcast("session-1", testMsg)

	// Wait for message
	select {
	case msg := <-client.send:
		if string(msg) != string(testMsg) {
			t.Errorf("received message = %s, want %s", msg, testMsg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for broadcast message")
	}
}

func TestHubBroadcastAll(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	// Register client
	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	// Broadcast to all
	testMsg := []byte(`{"type":"reload","path":"test.js"}`)
	hub.BroadcastAll(testMsg)

	// Wait for message
	select {
	case msg := <-client.send:
		if string(msg) != string(testMsg) {
			t.Errorf("received message = %s, want %s", msg, testMsg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for broadcast message")
	}
}
