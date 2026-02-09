// Package ipcbridge provides IPC server integration for the GUI main process
package ipcbridge

import (
	"context"
	"sync"
	"time"

	"mote/internal/ipc"
	"mote/internal/procmgr"

	"github.com/rs/zerolog"
)

// Bridge manages IPC server and subprocess communication
type Bridge struct {
	ipcServer   *ipc.Server
	procManager *procmgr.Manager
	logger      zerolog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex //nolint:unused // Reserved for future concurrent access

	// Callbacks for GUI events
	onTrayReady      func()
	onBubbleReady    func()
	onNotification   func(title, message string)
	onShowBubble     func(query string)
	onServiceStatus  func(running bool)
	onQuit           func() // Called when subprocess requests quit
	onShowWindow     func() // Called when subprocess requests to show main window
	onHideWindow     func() // Called when subprocess requests to hide main window
	onRestartService func() // Called when subprocess requests to restart service
	onError          func(err error)
}

// BridgeOption is a functional option for Bridge
type BridgeOption func(*Bridge)

// WithLogger sets the logger
func WithLogger(logger zerolog.Logger) BridgeOption {
	return func(b *Bridge) {
		b.logger = logger
	}
}

// WithOnTrayReady sets callback when tray process is ready
func WithOnTrayReady(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onTrayReady = fn
	}
}

// WithOnBubbleReady sets callback when bubble process is ready
func WithOnBubbleReady(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onBubbleReady = fn
	}
}

// WithOnNotification sets callback for notifications
func WithOnNotification(fn func(title, message string)) BridgeOption {
	return func(b *Bridge) {
		b.onNotification = fn
	}
}

// WithOnShowBubble sets callback to show bubble with query
func WithOnShowBubble(fn func(query string)) BridgeOption {
	return func(b *Bridge) {
		b.onShowBubble = fn
	}
}

// WithOnServiceStatus sets callback for service status changes
func WithOnServiceStatus(fn func(running bool)) BridgeOption {
	return func(b *Bridge) {
		b.onServiceStatus = fn
	}
}

// WithOnError sets callback for error handling
func WithOnError(fn func(err error)) BridgeOption {
	return func(b *Bridge) {
		b.onError = fn
	}
}

// WithOnQuit sets callback when subprocess requests quit
func WithOnQuit(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onQuit = fn
	}
}

// WithOnShowWindow sets callback when subprocess requests to show main window
func WithOnShowWindow(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onShowWindow = fn
	}
}

// WithOnHideWindow sets callback when subprocess requests to hide main window
func WithOnHideWindow(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onHideWindow = fn
	}
}

// WithOnRestartService sets callback when subprocess requests to restart service
func WithOnRestartService(fn func()) BridgeOption {
	return func(b *Bridge) {
		b.onRestartService = fn
	}
}

// NewBridge creates a new IPC bridge
func NewBridge(opts ...BridgeOption) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		logger: zerolog.Nop(),
		ctx:    ctx,
		cancel: cancel,
	}

	for _, opt := range opts {
		opt(b)
	}

	// Create IPC server with connection callbacks
	b.ipcServer = ipc.NewServer(
		ipc.WithOnClientConnect(b.handleClientConnect),
		ipc.WithOnClientDisconnect(b.handleClientDisconnect),
	)

	// Create process manager
	b.procManager = procmgr.NewManager(b.ipcServer)

	// Register message handlers
	b.registerHandlers()

	return b
}

// Start starts the IPC server
func (b *Bridge) Start() error {
	b.logger.Info().Msg("Starting IPC bridge")
	return b.ipcServer.Start()
}

// Stop stops the IPC server and all subprocesses
func (b *Bridge) Stop() error {
	b.logger.Info().Msg("Stopping IPC bridge")
	b.cancel()

	// Stop all subprocesses first
	if err := b.procManager.StopAll(); err != nil {
		b.logger.Warn().Err(err).Msg("Failed to stop all subprocesses")
	}

	// Then stop IPC server
	return b.ipcServer.Stop()
}

// StartTray starts the tray subprocess
func (b *Bridge) StartTray(execPath string) error {
	cfg := &procmgr.ProcessConfig{
		Name:         "tray",
		Path:         execPath,
		Args:         []string{"--role", "tray"},
		Role:         ipc.RoleTray,
		MaxRestarts:  3,
		RestartDelay: 2 * time.Second,
		StartTimeout: 10 * time.Second,
		Hidden:       true,
	}

	b.logger.Info().Str("path", execPath).Msg("Starting tray subprocess")
	return b.procManager.Start(cfg)
}

// StartBubble starts the bubble subprocess
func (b *Bridge) StartBubble(execPath string) error {
	cfg := &procmgr.ProcessConfig{
		Name:         "bubble",
		Path:         execPath,
		Args:         []string{"--role", "bubble"},
		Role:         ipc.RoleBubble,
		MaxRestarts:  0, // Bubble is started on-demand
		RestartDelay: 0,
		StartTimeout: 10 * time.Second,
		Hidden:       false, // Bubble window should be visible
	}

	b.logger.Info().Str("path", execPath).Msg("Starting bubble subprocess")
	return b.procManager.Start(cfg)
}

// StopTray stops the tray subprocess
func (b *Bridge) StopTray() error {
	return b.procManager.Stop("tray")
}

// StopBubble stops the bubble subprocess
func (b *Bridge) StopBubble() error {
	return b.procManager.Stop("bubble")
}

// IsTrayConnected returns whether tray is connected
func (b *Bridge) IsTrayConnected() bool {
	return b.ipcServer.IsClientConnected(ipc.RoleTray)
}

// IsBubbleConnected returns whether bubble is connected
func (b *Bridge) IsBubbleConnected() bool {
	return b.ipcServer.IsClientConnected(ipc.RoleBubble)
}

// SendNotificationToTray sends a notification to the tray
func (b *Bridge) SendNotificationToTray(title, message string) error {
	msg := ipc.NewMessage(ipc.MsgShowNotification, ipc.RoleMain)
	msg.WithPayload(&ipc.NotificationPayload{
		Title: title,
		Body:  message,
	})
	return b.ipcServer.Send(ipc.RoleTray, msg)
}

// SendShowBubble sends show command to bubble process
func (b *Bridge) SendShowBubble(query string) error {
	msg := ipc.NewMessage(ipc.MsgAction, ipc.RoleMain)
	msg.WithPayload(&ipc.ActionPayload{
		Source: "main",
		Action: "show",
		Data:   map[string]interface{}{"query": query},
	})
	return b.ipcServer.Send(ipc.RoleBubble, msg)
}

// SendHideBubble sends hide command to bubble process
func (b *Bridge) SendHideBubble() error {
	msg := ipc.NewMessage(ipc.MsgAction, ipc.RoleMain)
	msg.WithPayload(&ipc.ActionPayload{
		Source: "main",
		Action: "hide",
	})
	return b.ipcServer.Send(ipc.RoleBubble, msg)
}

// BroadcastServiceStatus broadcasts service status to all clients
func (b *Bridge) BroadcastServiceStatus(running bool) error {
	status := "stopped"
	if running {
		status = "running"
	}
	msg := ipc.NewMessage(ipc.MsgStatusUpdate, ipc.RoleMain)
	msg.WithPayload(&ipc.StatusUpdatePayload{
		Status: status,
	})
	return b.ipcServer.Broadcast(msg)
}

// registerHandlers registers IPC message handlers
func (b *Bridge) registerHandlers() {
	// Handle status requests
	b.ipcServer.RegisterHandler(ipc.MsgStatusUpdate, ipc.HandlerFunc(b.handleStatusRequest))

	// Handle notifications from subprocesses
	b.ipcServer.RegisterHandler(ipc.MsgShowNotification, ipc.HandlerFunc(b.handleNotification))

	// Handle action messages from subprocesses
	b.ipcServer.RegisterHandler(ipc.MsgAction, ipc.HandlerFunc(b.handleAction))
}

// handleClientConnect is called when a client connects
func (b *Bridge) handleClientConnect(role ipc.ProcessRole) {
	b.logger.Info().Str("role", string(role)).Msg("Client connected")

	switch role {
	case ipc.RoleTray:
		if b.onTrayReady != nil {
			b.onTrayReady()
		}
	case ipc.RoleBubble:
		if b.onBubbleReady != nil {
			b.onBubbleReady()
		}
	}
}

// handleClientDisconnect is called when a client disconnects
func (b *Bridge) handleClientDisconnect(role ipc.ProcessRole) {
	b.logger.Info().Str("role", string(role)).Msg("Client disconnected")
}

// handleStatusRequest handles status requests from clients
func (b *Bridge) handleStatusRequest(msg *ipc.Message) error {
	// This would be called when a subprocess requests status
	// The main process should respond with current status
	return nil
}

// handleNotification handles notifications from subprocesses
func (b *Bridge) handleNotification(msg *ipc.Message) error {
	var payload ipc.NotificationPayload
	if err := msg.ParsePayload(&payload); err != nil {
		return err
	}

	if b.onNotification != nil {
		b.onNotification(payload.Title, payload.Body)
	}
	return nil
}

// handleAction handles action messages from subprocesses
func (b *Bridge) handleAction(msg *ipc.Message) error {
	var payload ipc.ActionPayload
	if err := msg.ParsePayload(&payload); err != nil {
		return err
	}

	b.logger.Debug().
		Str("action", payload.Action).
		Interface("data", payload.Data).
		Msg("Received action from subprocess")

	switch payload.Action {
	case "show-main-window":
		// Request to show main GUI window
		b.logger.Info().Msg("Show main window action received")
		if b.onShowWindow != nil {
			b.onShowWindow()
		}
	case "hide-main-window":
		// Request to hide main GUI window
		b.logger.Info().Msg("Hide main window action received")
		if b.onHideWindow != nil {
			b.onHideWindow()
		}
	case "show-bubble":
		query := ""
		if q, ok := payload.Data["query"].(string); ok {
			query = q
		}
		if b.onShowBubble != nil {
			b.onShowBubble(query)
		}
	case "quit":
		// Request to quit application from tray/bubble
		b.logger.Info().Msg("Quit action received from subprocess")
		if b.onQuit != nil {
			b.onQuit()
		}
	case "restart-service":
		// Request to restart mote service
		b.logger.Info().Msg("Restart service action received")
		if b.onRestartService != nil {
			b.onRestartService()
		}
	}

	return nil
}
