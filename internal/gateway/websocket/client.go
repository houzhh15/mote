package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"mote/pkg/logger"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period.
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 1024 * 1024 // 1MB
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// Client represents a WebSocket client connection.
type Client struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	sessions    map[string]bool
	id          string
	connectedAt time.Time
}

// NewClient creates a new client.
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		sessions:    make(map[string]bool),
		id:          uuid.New().String(),
		connectedAt: time.Now(),
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error().Err(err).Str("client_id", c.id).Msg("WebSocket read error")
			}
			break
		}

		c.handleMessage(message)
	}
}

// handleMessage processes incoming WebSocket messages.
func (c *Client) handleMessage(message []byte) {
	var msg WSMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		logger.Error().Err(err).Str("client_id", c.id).Msg("Failed to parse WebSocket message")
		c.sendError("INVALID_MESSAGE", "failed to parse message")
		return
	}

	logger.Debug().
		Str("client_id", c.id).
		Str("type", msg.Type).
		Str("session", msg.Session).
		Msg("Received WebSocket message")

	switch msg.Type {
	case TypeSubscribe:
		if msg.Session != "" {
			c.hub.Subscribe(c, msg.Session)
		}

	case TypeUnsubscribe:
		if msg.Session != "" {
			c.hub.Unsubscribe(c, msg.Session)
		}

	case TypePing:
		c.sendPong()

	case TypeApprovalResponse:
		// M08: Handle approval response from UI
		if msg.RequestID == "" {
			c.sendError("INVALID_REQUEST", "approval response requires request_id")
			return
		}
		if err := c.hub.HandleApprovalResponse(msg.RequestID, msg.Approved, msg.Message); err != nil {
			logger.Error().
				Err(err).
				Str("client_id", c.id).
				Str("request_id", msg.RequestID).
				Msg("Failed to handle approval response")
			c.sendError("APPROVAL_ERROR", err.Error())
			return
		}
		logger.Debug().
			Str("client_id", c.id).
			Str("request_id", msg.RequestID).
			Bool("approved", msg.Approved).
			Msg("Processed approval response")

	case TypeChat:
		// Handle chat message from UI
		if msg.Message == "" {
			c.sendError("INVALID_REQUEST", "chat message is required")
			return
		}
		sessionID := msg.Session
		if sessionID == "" {
			sessionID = c.id // Use client ID as default session
		}

		// Subscribe to this session if not already
		c.hub.Subscribe(c, sessionID)

		events, err := c.hub.HandleChat(sessionID, msg.Message)
		if err != nil {
			logger.Error().
				Err(err).
				Str("client_id", c.id).
				Str("session", sessionID).
				Msg("Failed to handle chat message")
			c.sendError("CHAT_ERROR", err.Error())
			return
		}

		if events == nil {
			c.sendError("CHAT_ERROR", "chat handler not configured")
			return
		}

		// Stream events back to client
		go func() {
			for data := range events {
				select {
				case c.send <- data:
				default:
					// Buffer full, skip this event
				}
			}
		}()

		logger.Debug().
			Str("client_id", c.id).
			Str("session", sessionID).
			Msg("Started chat stream")

	default:
		logger.Debug().
			Str("client_id", c.id).
			Str("type", msg.Type).
			Msg("Unknown message type")
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logger.Error().Err(err).Str("client_id", c.id).Msg("WebSocket write error")
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendPong sends a pong response.
func (c *Client) sendPong() {
	msg := WSMessage{Type: TypePong}
	data, _ := json.Marshal(msg)
	select {
	case c.send <- data:
	default:
		// Buffer full
	}
}

// sendError sends an error message to the client.
func (c *Client) sendError(code, message string) {
	msg := WSMessage{
		Type:    TypeError,
		Code:    code,
		Message: message,
	}
	data, _ := json.Marshal(msg)
	select {
	case c.send <- data:
	default:
		// Buffer full
	}
}

// ServeWs handles WebSocket requests from clients.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to upgrade WebSocket connection")
		return
	}

	client := NewClient(hub, conn)
	hub.Register(client)

	go client.writePump()
	go client.readPump()
}
