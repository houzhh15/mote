// Package main provides SDP Controller service
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"sdp/pkg/sdp/common"
	"sdp/pkg/sdp/config"
	"sdp/pkg/sdp/crypto"
)

var (
	configPath = flag.String("config", "", "Configuration file path")
	addr       = flag.String("addr", ":8080", "HTTP listen address")
	jwtSecret  = flag.String("secret", "sdp-demo-secret-key", "JWT signing secret")
)

type Controller struct {
	jwtService  *crypto.JWTService
	deviceReg   *common.DeviceRegistry
	mu          sync.RWMutex
	sessions    map[string]*common.AccessSession
	sessionMu   sync.RWMutex
}

func NewController(secret string) *Controller {
	return &Controller{
		jwtService: crypto.NewJWTService(secret, 10),
		deviceReg:  common.NewDeviceRegistry(),
		sessions:   make(map[string]*common.AccessSession),
	}
}

func main() {
	flag.Parse()

	var cfg *config.ControllerConfig
	var err error

	if *configPath != "" {
		cfg, err = config.LoadControllerConfig(*configPath)
		if err != nil {
			log.Printf("Failed to load config: %v, using defaults", err)
		}
	}

	if cfg == nil {
		cfg = config.DefaultControllerConfig()
	}

	addr := *addr
	if addr == ":8080" && cfg.Server.APIListen != "" {
		addr = cfg.Server.APIListen
	}

	secret := *jwtSecret
	if cfg.Security.JWTSecret != "" {
		secret = cfg.Security.JWTSecret
	}

	ctrl := NewController(secret)

	log.Printf("Starting SDP Controller on %s", addr)
	log.Printf("JWT Secret loaded: [REDACTED]")

	// Start cleanup goroutine
	go ctrl.cleanupSessions()

	// Register handlers
	http.HandleFunc("/api/v1/auth/register", ctrl.handleRegister)
	http.HandleFunc("/api/v1/auth/token", ctrl.handleToken)
	http.HandleFunc("/api/v1/auth/verify", ctrl.handleVerify)
	http.HandleFunc("/api/v1/device/:id/publickey", ctrl.handleGetDevicePublicKey)
	http.HandleFunc("/health", ctrl.handleHealth)

	// Start server
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("Controller ready at http://%s", addr)
	log.Println("Endpoints:")
	log.Println("  POST /api/v1/auth/register      - Register device")
	log.Println("  POST /api/v1/auth/token         - Get access token")
	log.Println("  POST /api/v1/auth/verify        - Verify token")
	log.Println("  GET  /api/v1/device/:id/publickey - Get device public key")
	log.Println("  GET  /health                     - Health check")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	if err := srv.Shutdown(nil); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}

func (c *Controller) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req common.AuthRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		req.DeviceID = uuid.New().String()
	}

	device := &common.DeviceInfo{
		DeviceID:     req.DeviceID,
		DisplayName:  req.DeviceName,
		PublicKey:    req.PublicKey,
		RegisteredAt: time.Now(),
		LastSeen:     time.Now(),
		Status:       "active",
		Policy: &common.Policy{
			AllowedServers:     []string{},
			MaxSessionDuration: 3600,
			IPWhitelist:       []string{},
		},
	}

	c.mu.Lock()
	c.deviceReg.RegisterDevice(device)
	c.mu.Unlock()

	resp := common.AuthResponse{
		Token:     req.DeviceID,
		ExpiresAt: 0,
		ServerID:  "controller",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("Device registered: %s", req.DeviceID)
}

func (c *Controller) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		DeviceID string `json:"device_id"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	c.mu.RLock()
	device, exists := c.deviceReg.GetDevice(req.DeviceID)
	c.mu.RUnlock()

	if !exists {
		// Auto-register device if not exists
		device = &common.DeviceInfo{
			DeviceID:     req.DeviceID,
			DisplayName:  "Device " + req.DeviceID[:min(8, len(req.DeviceID))],
			RegisteredAt: time.Now(),
			LastSeen:     time.Now(),
			Status:       "active",
		}
		c.mu.Lock()
		c.deviceReg.RegisterDevice(device)
		c.mu.Unlock()
		log.Printf("Device auto-registered: %s", req.DeviceID)
	}

	// Generate JWT token
	token, err := c.jwtService.GenerateToken(req.DeviceID, []string{"user", "device"}, []string{"read", "write"})
	if err != nil {
		http.Error(w, "Generate token error", http.StatusInternalServerError)
		log.Printf("Token generation error: %v", err)
		return
	}

	// Record session
	session := &common.AccessSession{
		SessionID:       uuid.New().String(),
		DeviceID:        req.DeviceID,
		ServerID:        "application-server",
		IssuedAt:        time.Now(),
		ExpiresAt:       time.Now().Add(10 * time.Minute),
		IPAddress:       r.RemoteAddr,
		MTLSEstablished: false,
	}
	c.sessionMu.Lock()
	c.sessions[session.SessionID] = session
	c.sessionMu.Unlock()

	resp := common.AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
		ServerID:  "application-server",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("Token issued for device: %s, session: %s", req.DeviceID, session.SessionID)
}

func (c *Controller) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req common.VerifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	claims, err := c.jwtService.VerifyToken(req.Token)
	if err != nil {
		resp := common.VerifyResponse{
			Valid: false,
			Error: err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp := common.VerifyResponse{
		Valid:    true,
		DeviceID: claims.Sub,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("Token verified for device: %s", claims.Sub)
}

func (c *Controller) handleGetDevicePublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := r.PathValue("id")
	if deviceID == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	c.mu.RLock()
	device, exists := c.deviceReg.GetDevice(deviceID)
	c.mu.RUnlock()

	if !exists || device.PublicKey == "" {
		http.Error(w, "Device not found or no public key", http.StatusNotFound)
		return
	}

	resp := map[string]string{
		"device_id":   deviceID,
		"public_key":  device.PublicKey,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *Controller) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (c *Controller) cleanupSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.sessionMu.Lock()
		now := time.Now()
		for id, session := range c.sessions {
			if session.IsExpired() {
				delete(c.sessions, id)
				log.Printf("Session expired: %s (device: %s)", id, session.DeviceID)
			}
		}
		c.sessionMu.Unlock()
		_ = now
	}
}
