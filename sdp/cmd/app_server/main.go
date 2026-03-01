// Package main provides SDP Application Server with SPA and mTLS support
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"sdp/pkg/sdp/common"
	"sdp/pkg/sdp/config"
	"sdp/pkg/sdp/crypto"
)

var (
	configPath = flag.String("config", "", "Configuration file path")
	listenAddr = flag.String("listen", ":8443", "TCP listen address for mTLS")
	spaPort    = flag.Int("spa", 8442, "UDP port for SPA")
	controller = flag.String("controller", "localhost:8080", "Controller address")
)

const (
	spaTimeout      = 30 * time.Second
	portOpenDuration = 5 * time.Minute
)

// PortManager manages dynamic port opening
type PortManager struct {
	mu             sync.RWMutex
	openPorts      map[string]*openPort
	allowedDevices map[string]*ecdsa.PublicKey
	usedNonces     map[string]time.Time
}

type openPort struct {
	deviceID  string
	openTime  time.Time
	expiresAt time.Time
}

// NewPortManager creates a new port manager
func NewPortManager() *PortManager {
	return &PortManager{
		openPorts:      make(map[string]*openPort),
		allowedDevices: make(map[string]*ecdsa.PublicKey),
		usedNonces:     make(map[string]time.Time),
	}
}

// AllowDevice adds device to whitelist with public key
func (pm *PortManager) AllowDevice(deviceID string, pubKey *ecdsa.PublicKey) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.allowedDevices[deviceID] = pubKey
}

// GetDeviceKey retrieves device public key
func (pm *PortManager) GetDeviceKey(deviceID string) (*ecdsa.PublicKey, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	key, ok := pm.allowedDevices[deviceID]
	return key, ok
}

// IsAllowed checks if device is allowed
func (pm *PortManager) IsAllowed(deviceID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, ok := pm.allowedDevices[deviceID]
	return ok
}

// OpenPort marks port as open for device
func (pm *PortManager) OpenPort(deviceID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.allowedDevices[deviceID]; !ok {
		return false
	}

	pm.openPorts[deviceID] = &openPort{
		deviceID:  deviceID,
		openTime:  time.Now(),
		expiresAt: time.Now().Add(portOpenDuration),
	}
	return true
}

// IsPortOpen checks if port is open for device
func (pm *PortManager) IsPortOpen(deviceID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	port, ok := pm.openPorts[deviceID]
	if !ok {
		return false
	}

	return time.Now().Before(port.expiresAt)
}

// ClosePort closes port for device
func (pm *PortManager) ClosePort(deviceID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.openPorts, deviceID)
}

// CheckNonce checks if nonce is used (replay protection)
func (pm *PortManager) CheckNonce(nonce string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if _, used := pm.usedNonces[nonce]; used {
		return false // already used
	}
	
	// Store nonce with expiration (keep for 5 minutes)
	pm.usedNonces[nonce] = time.Now().Add(5 * time.Minute)
	return true
}

// AppServer represents Application Server
type AppServer struct {
	portManager   *PortManager
	httpClient    *http.Client
	controllerURL string
	tlsCertFile   string
	tlsKeyFile    string
	caCertFile    string
	listener      net.Listener
	serverStarted bool
	startMu       sync.Mutex
	serverWg      sync.WaitGroup
}

// NewAppServer creates a new application server
func NewAppServer(ctrlAddr, certFile, keyFile, caFile string) *AppServer {
	return &AppServer{
		portManager:   NewPortManager(),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		controllerURL: "http://" + ctrlAddr,
		tlsCertFile:   certFile,
		tlsKeyFile:    keyFile,
		caCertFile:    caFile,
		serverStarted: false,
	}
}

func main() {
	flag.Parse()

	var cfg *config.AppServerConfig
	var err error

	if *configPath != "" {
		cfg, err = config.LoadAppServerConfig(*configPath)
		if err != nil {
			log.Printf("Failed to load config: %v, using defaults", err)
		}
	}

	if cfg == nil {
		cfg = config.DefaultAppServerConfig()
	}

	addr := *listenAddr
	if addr == ":8443" && cfg.Server.ListenAddr != "" {
		addr = cfg.Server.ListenAddr
	}

	spaPort := *spaPort
	if spaPort == 8442 && cfg.Server.SPAPort != 0 {
		spaPort = cfg.Server.SPAPort
	}

	ctrlAddr := *controller
	if cfg.Security.ControllerAddr != "" {
		ctrlAddr = cfg.Security.ControllerAddr
	}

	certFile := cfg.TLS.CertFile
	keyFile := cfg.TLS.KeyFile
	caFile := cfg.TLS.CAFile

	server := NewAppServer(ctrlAddr, certFile, keyFile, caFile)

	log.Printf("Starting SDP Application Server")
	log.Printf("  TCP Listen: %s (mTLS, initially closed)", addr)
	log.Printf("  SPA Port: %d (UDP)", spaPort)
	log.Printf("  Controller: %s", ctrlAddr)

	// Start SPA listener (UDP 8442)
	go server.startSPAListener(spaPort)

	// Start port cleanup goroutine
	go server.cleanupPorts()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	
	// Stop server if running
	if server.listener != nil {
		server.listener.Close()
	}
}

func (s *AppServer) startSPAListener(port int) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("SPA listener started on UDP port %d (initially closed, opens after SPA auth)", port)

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		go s.handleSPAPacket(conn, clientAddr, buf[:n])
	}
}

func (s *AppServer) handleSPAPacket(conn *net.UDPConn, clientAddr *net.UDPAddr, data []byte) {
	var pkt common.SPAPacket
	if err := json.Unmarshal(data, &pkt); err != nil {
		log.Printf("Invalid SPA packet from %s: %v", clientAddr, err)
		s.sendSPAResponse(conn, clientAddr, false, "invalid packet format")
		return
	}

	log.Printf("Received SPA packet from %s: device=%s type=%d",
		clientAddr, pkt.DeviceID, pkt.Type)

	// Validate timestamp (5 minutes window)
	if !pkt.IsValid() {
		log.Printf("SPA packet timestamp invalid from %s", clientAddr)
		s.sendSPAResponse(conn, clientAddr, false, "invalid timestamp")
		return
	}

	// Check nonce for replay protection
	if !s.portManager.CheckNonce(pkt.Nonce) {
		log.Printf("SPA nonce already used (replay attack?) from %s", clientAddr)
		s.sendSPAResponse(conn, clientAddr, false, "nonce already used")
		return
	}

	// Get device public key for signature verification
	pubKey, exists := s.portManager.GetDeviceKey(pkt.DeviceID)
	if !exists {
		// Try to verify device with controller and get public key
		var err error
		pubKey, err = s.fetchDevicePublicKey(pkt.DeviceID)
		if err != nil {
			log.Printf("Device not registered: %s", pkt.DeviceID)
			s.sendSPAResponse(conn, clientAddr, false, "device not registered")
			return
		}
		// Store the public key for future use
		s.portManager.AllowDevice(pkt.DeviceID, pubKey)
	}

	// Verify SPA signature
	if err := crypto.VerifySPA(pkt.DeviceID, pkt.Timestamp, pkt.Nonce, pkt.Signature, pubKey); err != nil {
		log.Printf("SPA signature verification failed for device %s: %v", pkt.DeviceID, err)
		s.sendSPAResponse(conn, clientAddr, false, "invalid signature")
		return
	}

	log.Printf("SPA signature verified for device: %s", pkt.DeviceID)

	// Open port for device
	if s.portManager.OpenPort(pkt.DeviceID) {
		log.Printf("Port opened for device: %s", pkt.DeviceID)
		
		// Start the mTLS server if not already started
		s.startServerIfNeeded()
		
		s.sendSPAResponse(conn, clientAddr, true, "")
	} else {
		log.Printf("Failed to open port for device: %s", pkt.DeviceID)
		s.sendSPAResponse(conn, clientAddr, false, "port open failed")
	}
}

func (s *AppServer) fetchDevicePublicKey(deviceID string) (*ecdsa.PublicKey, error) {
	// Query controller for device public key
	url := s.controllerURL + "/api/v1/device/" + deviceID + "/publickey"
	
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device not found: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return crypto.PEMToPublicKey(result.PublicKey)
}

func (s *AppServer) sendSPAResponse(conn *net.UDPConn, addr *net.UDPAddr, success bool, errMsg string) {
	response := map[string]interface{}{
		"status": "ok",
		"open":   success,
	}
	if errMsg != "" {
		response["error"] = errMsg
	}
	data, _ := json.Marshal(response)
	conn.WriteToUDP(data, addr)
}

func (s *AppServer) startServerIfNeeded() {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.serverStarted {
		return
	}

	s.serverStarted = true
	
	// Get the listen address
	addr := *listenAddr
	if addr == ":8443" {
		addr = ":8443"
	}

	// Start server in goroutine
	go func() {
		s.startHTTPServer(addr)
	}()
}

func (s *AppServer) startHTTPServer(addr string) {
	// Create TLS config
	tlsConfig, err := crypto.GetServerTLSConfig(s.tlsCertFile, s.tlsKeyFile, s.caCertFile)
	if err != nil {
		log.Fatalf("Failed to create TLS config: %v", err)
	}

	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	s.listener = listener
	
	log.Printf("mTLS server started and listening on %s (opened after SPA auth)", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *AppServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Check TLS connection
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		log.Printf("Not a TLS connection")
		return
	}

	// Get client certificate
	if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		log.Printf("No client certificate provided")
		return
	}

	clientCert := tlsConn.ConnectionState().PeerCertificates[0]
	log.Printf("Client connected: %s", clientCert.Subject.CommonName)

	// Read the request
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("Read error: %v", err)
		return
	}

	// Parse HTTP request to extract token
	req := string(buf[:n])
	var token string
	for _, line := range strings.Split(req, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "authorization:") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 && strings.HasPrefix(parts[1], "Bearer ") {
				token = strings.TrimPrefix(parts[1], "Bearer ")
			}
		}
	}

	// Verify token with controller if provided
	if token != "" {
		if !s.verifyTokenWithController(token) {
			log.Printf("Token verification failed for client: %s", clientCert.Subject.CommonName)
			response := "HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json\r\n\r\n{\"error\":\"invalid token\"}"
			conn.Write([]byte(response))
			return
		}
		log.Printf("Token verified for device")
	} else {
		log.Printf("No token provided, allowing mTLS-only connection")
	}

	// Simple HTTP response
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"status\":\"connected\",\"message\":\"SDP Zero Trust Connection Established\"}"
	conn.Write([]byte(response))
}

func (s *AppServer) verifyTokenWithController(token string) bool {
	url := s.controllerURL + "/api/v1/auth/verify"
	
	reqBody, _ := json.Marshal(common.VerifyRequest{Token: token})
	
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Printf("Failed to verify token with controller: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var verifyResp common.VerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return false
	}

	return verifyResp.Valid
}

func (s *AppServer) cleanupPorts() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.portManager.mu.Lock()
		now := time.Now()
		for deviceID, port := range s.portManager.openPorts {
			if now.After(port.expiresAt) {
				delete(s.portManager.openPorts, deviceID)
				log.Printf("Port closed for device: %s", deviceID)
			}
		}
		// Clean up old nonces
		for nonce, expiry := range s.portManager.usedNonces {
			if now.After(expiry) {
				delete(s.portManager.usedNonces, nonce)
			}
		}
		s.portManager.mu.Unlock()
	}
}
