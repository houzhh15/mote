package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"mote/pkg/logger"
)

// Client is an IPC client that runs in child processes (tray, bubble)
type Client struct {
	role       ProcessRole
	socketPath string

	conn    net.Conn
	connMu  sync.Mutex
	encoder *Encoder
	decoder *Decoder

	handlers   map[MessageType][]Handler
	handlersMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	reconnectEnabled bool
	reconnectDelay   time.Duration
	maxReconnects    int

	onConnect    func()
	onDisconnect func()
	onExit       func() // Custom exit handler for graceful shutdown

	connected   bool
	connectedMu sync.RWMutex
}

// ClientOption is a functional option for Client
type ClientOption func(*Client)

// WithSocketPath sets a custom socket path
func WithSocketPath(path string) ClientOption {
	return func(c *Client) {
		c.socketPath = path
	}
}

// WithReconnect enables automatic reconnection
func WithReconnect(enabled bool, delay time.Duration, maxAttempts int) ClientOption {
	return func(c *Client) {
		c.reconnectEnabled = enabled
		c.reconnectDelay = delay
		c.maxReconnects = maxAttempts
	}
}

// WithClientOnConnect sets a callback for when connection is established
func WithClientOnConnect(fn func()) ClientOption {
	return func(c *Client) {
		c.onConnect = fn
	}
}

// WithClientOnDisconnect sets a callback for when connection is lost
func WithClientOnDisconnect(fn func()) ClientOption {
	return func(c *Client) {
		c.onDisconnect = fn
	}
}

// WithClientOnExit sets a custom exit handler for graceful shutdown
// This callback is invoked when MsgExit is received from the main process.
// The callback should perform any necessary cleanup (e.g., systray.Quit())
// and then exit the process. If no callback is set, os.Exit(0) is called directly.
func WithClientOnExit(fn func()) ClientOption {
	return func(c *Client) {
		c.onExit = fn
	}
}

// NewClient creates a new IPC client with the given role
func NewClient(role ProcessRole, opts ...ClientOption) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		role:             role,
		handlers:         make(map[MessageType][]Handler),
		ctx:              ctx,
		cancel:           cancel,
		reconnectEnabled: true,
		reconnectDelay:   time.Second,
		maxReconnects:    10,
	}

	if runtime.GOOS == "windows" {
		c.socketPath = WindowsPipeName
	} else {
		c.socketPath = UnixSocketPath
	}

	for _, opt := range opts {
		opt(c)
	}

	c.RegisterHandler(MsgPing, HandlerFunc(c.handlePing))
	c.RegisterHandler(MsgExit, HandlerFunc(c.handleExit))

	return c
}

// Connect connects to the IPC server
func (c *Client) Connect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		return nil
	}

	var conn net.Conn
	var err error
	attempts := 0

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		if runtime.GOOS == "windows" {
			conn, err = dialPipe(c.socketPath)
		} else {
			conn, err = net.Dial("unix", c.socketPath)
		}

		if err == nil {
			break
		}

		attempts++
		if !c.reconnectEnabled || (c.maxReconnects > 0 && attempts >= c.maxReconnects) {
			return fmt.Errorf("failed to connect to IPC server after %d attempts: %w", attempts, err)
		}

		logger.Warnf("failed to connect to IPC server, retrying in %v (attempt %d/%d): %v",
			c.reconnectDelay, attempts, c.maxReconnects, err)

		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(c.reconnectDelay):
		}
	}

	c.conn = conn
	c.encoder = NewEncoder(conn)
	c.decoder = NewDecoder(conn)

	c.setConnected(true)

	if err := c.sendRegister(); err != nil {
		c.conn.Close()
		c.conn = nil
		c.setConnected(false)
		return fmt.Errorf("failed to register with IPC server: %w", err)
	}

	logger.Infof("IPC client connected as %s", c.role)

	if c.onConnect != nil {
		c.onConnect()
	}

	go c.readLoop()

	return nil
}

// Close closes the connection to the IPC server
func (c *Client) Close() error {
	c.cancel()

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.setConnected(false)
		return err
	}
	return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.connectedMu.RLock()
	defer c.connectedMu.RUnlock()
	return c.connected
}

// RegisterHandler registers a handler for a message type
func (c *Client) RegisterHandler(msgType MessageType, handler Handler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers[msgType] = append(c.handlers[msgType], handler)
}

// Send sends a message to the server
func (c *Client) Send(msg *Message) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg.Source = c.role

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	return c.encoder.Encode(msg)
}

// SendAction sends an action message to the main process
func (c *Client) SendAction(action string, data map[string]any) error {
	msg := NewMessage(MsgAction, c.role).
		WithTarget(RoleMain).
		WithPayload(&ActionPayload{
			Source: string(c.role),
			Action: action,
			Data:   data,
		})

	return c.Send(msg)
}

func (c *Client) setConnected(connected bool) {
	c.connectedMu.Lock()
	c.connected = connected
	c.connectedMu.Unlock()
}

func (c *Client) sendRegister() error {
	msg := NewMessage(MsgRegister, c.role).
		WithPayload(&RegisterPayload{
			Role:    c.role,
			PID:     os.Getpid(),
			Version: ProtocolVersion,
		})

	return c.encoder.Encode(msg)
}

func (c *Client) readLoop() {
	defer func() {
		c.setConnected(false)
		if c.onDisconnect != nil {
			c.onDisconnect()
		}

		if c.reconnectEnabled {
			select {
			case <-c.ctx.Done():
				return
			default:
				logger.Info().Msg("attempting to reconnect...")
				c.connMu.Lock()
				c.conn = nil
				c.connMu.Unlock()

				if err := c.Connect(); err != nil {
					logger.Errorf("failed to reconnect: %v", err)
				}
			}
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.connMu.Lock()
		if c.conn == nil {
			c.connMu.Unlock()
			return
		}
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.connMu.Unlock()

		msg, err := c.decoder.Decode()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-c.ctx.Done():
				return
			default:
				if err.Error() != "EOF" {
					logger.Warnf("failed to decode message: %v", err)
				}
				return
			}
		}

		c.handleMessage(msg)
	}
}

func (c *Client) handleMessage(msg *Message) {
	c.handlersMu.RLock()
	handlers, ok := c.handlers[msg.Type]
	c.handlersMu.RUnlock()

	if !ok || len(handlers) == 0 {
		logger.Debugf("no handler for message type: %s", msg.Type)
		return
	}

	for _, handler := range handlers {
		if err := handler.Handle(msg); err != nil {
			logger.Warnf("handler error for %s: %v", msg.Type, err)
		}
	}
}

func (c *Client) handlePing(msg *Message) error {
	pong := NewMessage(MsgPong, c.role).
		WithTarget(RoleMain).
		WithReplyTo(msg.ID)

	return c.Send(pong)
}

func (c *Client) handleExit(msg *Message) error {
	logger.Info().Msg("received exit signal from main process")
	c.cancel()

	// Use custom exit handler if provided
	if c.onExit != nil {
		c.onExit()
		return nil
	}

	// Default: exit immediately
	os.Exit(0)
	return nil
}
