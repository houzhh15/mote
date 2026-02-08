package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewClient(t *testing.T) {
	hub := NewHub()
	client := NewClient(hub, nil)

	if client.hub != hub {
		t.Error("client.hub != hub")
	}

	if client.sessions == nil {
		t.Error("client.sessions is nil")
	}

	if client.send == nil {
		t.Error("client.send is nil")
	}

	if client.id == "" {
		t.Error("client.id is empty")
	}

	if client.connectedAt.IsZero() {
		t.Error("client.connectedAt is zero")
	}
}

func TestClientHandleMessage(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          "test-client",
		connectedAt: time.Now(),
	}

	// Register client first
	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	t.Run("subscribe message", func(t *testing.T) {
		msg := WSMessage{Type: TypeSubscribe, Session: "test-session"}
		data, _ := json.Marshal(msg)
		client.handleMessage(data)

		if !client.sessions["test-session"] {
			t.Error("client not subscribed to test-session")
		}
	})

	t.Run("ping message", func(t *testing.T) {
		msg := WSMessage{Type: TypePing}
		data, _ := json.Marshal(msg)
		client.handleMessage(data)

		select {
		case response := <-client.send:
			var respMsg WSMessage
			if err := json.Unmarshal(response, &respMsg); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if respMsg.Type != TypePong {
				t.Errorf("response type = %s, want %s", respMsg.Type, TypePong)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for pong response")
		}
	})

	t.Run("unsubscribe message", func(t *testing.T) {
		msg := WSMessage{Type: TypeUnsubscribe, Session: "test-session"}
		data, _ := json.Marshal(msg)
		client.handleMessage(data)

		if client.sessions["test-session"] {
			t.Error("client still subscribed to test-session")
		}
	})

	t.Run("invalid message", func(t *testing.T) {
		client.handleMessage([]byte("invalid json"))

		select {
		case response := <-client.send:
			var respMsg WSMessage
			if err := json.Unmarshal(response, &respMsg); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if respMsg.Type != TypeError {
				t.Errorf("response type = %s, want %s", respMsg.Type, TypeError)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for error response")
		}
	})
}

func TestServeWs(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for client registration
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("ClientCount = %d, want 1", hub.ClientCount())
	}

	// Send subscribe message
	subMsg := WSMessage{Type: TypeSubscribe, Session: "test-session"}
	if err := ws.WriteJSON(subMsg); err != nil {
		t.Fatalf("failed to send subscribe: %v", err)
	}

	// Send ping and expect pong
	pingMsg := WSMessage{Type: TypePing}
	if err := ws.WriteJSON(pingMsg); err != nil {
		t.Fatalf("failed to send ping: %v", err)
	}

	var pongMsg WSMessage
	if err := ws.ReadJSON(&pongMsg); err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}

	if pongMsg.Type != TypePong {
		t.Errorf("response type = %s, want %s", pongMsg.Type, TypePong)
	}
}
