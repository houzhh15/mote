package websocket

import (
	"encoding/json"
	"sync"

	"mote/pkg/logger"
)

// ApprovalResponseHandler handles approval responses from WebSocket clients.
type ApprovalResponseHandler func(requestID string, approved bool, message string) error

// ChatHandler handles chat messages from WebSocket clients.
type ChatHandler func(sessionID string, message string) (<-chan []byte, error)

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Session to clients mapping for targeted broadcasts.
	sessions map[string]map[*Client]bool

	// Register requests from clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Broadcast messages to sessions.
	broadcast chan *BroadcastMessage

	// Mutex for thread-safe access.
	mu sync.RWMutex

	// M08: Approval response handler
	approvalHandler ApprovalResponseHandler

	// Chat message handler
	chatHandler ChatHandler
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		sessions:   make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage, 256),
	}
}

// SetApprovalHandler sets the callback for approval responses.
func (h *Hub) SetApprovalHandler(handler ApprovalResponseHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.approvalHandler = handler
}

// SetChatHandler sets the callback for chat messages.
func (h *Hub) SetChatHandler(handler ChatHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.chatHandler = handler
}

// HandleChat processes a chat message from a client.
func (h *Hub) HandleChat(sessionID, message string) (<-chan []byte, error) {
	h.mu.RLock()
	handler := h.chatHandler
	h.mu.RUnlock()

	if handler == nil {
		return nil, nil
	}

	return handler(sessionID, message)
}

// HandleApprovalResponse processes an approval response from a client.
func (h *Hub) HandleApprovalResponse(requestID string, approved bool, message string) error {
	h.mu.RLock()
	handler := h.approvalHandler
	h.mu.RUnlock()

	if handler == nil {
		logger.Warn().
			Str("request_id", requestID).
			Msg("Approval response received but no handler configured")
		return nil
	}

	return handler(requestID, approved, message)
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.Info().Str("client_id", client.id).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)

				// Remove from all session subscriptions
				for session := range client.sessions {
					if clients, ok := h.sessions[session]; ok {
						delete(clients, client)
						if len(clients) == 0 {
							delete(h.sessions, session)
						}
					}
				}
			}
			h.mu.Unlock()
			logger.Info().Str("client_id", client.id).Msg("WebSocket client disconnected")

		case msg := <-h.broadcast:
			h.mu.RLock()
			if msg.Session == "" {
				// Broadcast to all clients
				for client := range h.clients {
					select {
					case client.send <- msg.Data:
					default:
						// Client buffer full, skip
					}
				}
			} else {
				// Broadcast to session subscribers
				if clients, ok := h.sessions[msg.Session]; ok {
					for client := range clients {
						select {
						case client.send <- msg.Data:
						default:
							// Client buffer full, skip
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Subscribe adds a client to a session's subscriber list.
func (h *Hub) Subscribe(client *Client, session string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.sessions[session] = true
	if h.sessions[session] == nil {
		h.sessions[session] = make(map[*Client]bool)
	}
	h.sessions[session][client] = true

	logger.Debug().
		Str("client_id", client.id).
		Str("session", session).
		Msg("Client subscribed to session")
}

// Unsubscribe removes a client from a session's subscriber list.
func (h *Hub) Unsubscribe(client *Client, session string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.sessions, session)
	if clients, ok := h.sessions[session]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessions, session)
		}
	}

	logger.Debug().
		Str("client_id", client.id).
		Str("session", session).
		Msg("Client unsubscribed from session")
}

// Broadcast sends a message to all clients subscribed to a session.
func (h *Hub) Broadcast(session string, data []byte) {
	h.broadcast <- &BroadcastMessage{Session: session, Data: data}
}

// BroadcastAll sends a message to all connected clients.
func (h *Hub) BroadcastAll(data []byte) {
	h.broadcast <- &BroadcastMessage{Session: "", Data: data}
}

// BroadcastTyped sends a typed message to all connected clients.
// This is the preferred method for M08 approval notifications.
func (h *Hub) BroadcastTyped(messageType string, payload any) error {
	msg := struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}{
		Type: messageType,
		Data: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error().Err(err).Str("type", messageType).Msg("Failed to marshal broadcast message")
		return err
	}

	h.broadcast <- &BroadcastMessage{Session: "", Data: data}
	return nil
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
