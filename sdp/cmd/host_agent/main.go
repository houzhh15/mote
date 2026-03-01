// Package main provides SDP Host Agent client
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
	"syscall"
	"time"

	"github.com/google/uuid"

	"sdp/pkg/sdp/common"
	"sdp/pkg/sdp/config"
	"sdp/pkg/sdp/crypto"
)

var (
	configPath = flag.String("config", "", "Configuration file path")
	deviceID   = flag.String("device-id", "", "Device ID (auto-generated if empty)")
	controller = flag.String("controller", "localhost:8080", "Controller address")
	serverAddr = flag.String("server", "localhost:8443", "Application server address")
	certFile   = flag.String("cert", "./certs/client.pem", "Client certificate")
	keyFile    = flag.String("key", "./certs/client.key", "Client private key")
	caFile     = flag.String("ca", "./certs/ca.pem", "CA certificate")
)

// HostAgent represents the SDP client
type HostAgent struct {
	deviceID      string
	privKey       *ecdsa.PrivateKey
	pubKeyPEM     string
	privKeyPEM    string
	controllerURL string
	serverAddr    string
	tlsConfig     *tls.Config
	token         string
}

// NewHostAgent creates a new Host Agent
func NewHostAgent(deviceID string, ctrlAddr, srvAddr, cert, key, ca string) (*HostAgent, error) {
	// Generate device key pair if not exists
	privKey, _, err := crypto.GenerateDeviceKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	// Convert keys to PEM
	pubKeyPEM, err := crypto.PublicKeyToPEM(&privKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("convert public key: %w", err)
	}

	privKeyPEM, err := crypto.PrivateKeyToPEM(privKey)
	if err != nil {
		return nil, fmt.Errorf("convert private key: %w", err)
	}

	// Load TLS config
	tlsConfig, err := crypto.GetClientTLSConfig(cert, key, ca)
	if err != nil {
		return nil, fmt.Errorf("load TLS config: %w", err)
	}

	if deviceID == "" {
		deviceID = uuid.New().String()
	}

	return &HostAgent{
		deviceID:      deviceID,
		privKey:       privKey,
		pubKeyPEM:     pubKeyPEM,
		privKeyPEM:    privKeyPEM,
		controllerURL: "http://" + ctrlAddr,
		serverAddr:    srvAddr,
		tlsConfig:     tlsConfig,
	}, nil
}

func main() {
	flag.Parse()

	var cfg *config.HostAgentConfig
	var err error

	if *configPath != "" {
		cfg, err = config.LoadHostAgentConfig(*configPath)
		if err != nil {
			log.Printf("Failed to load config: %v, using defaults", err)
		}
	}

	if cfg == nil {
		cfg = config.DefaultHostAgentConfig()
	}

	devID := *deviceID
	if cfg.Client.DeviceID != "" {
		devID = cfg.Client.DeviceID
	}

	ctrlAddr := *controller
	if cfg.Client.ControllerAddr != "" {
		ctrlAddr = cfg.Client.ControllerAddr
	}

	srvAddr := *serverAddr
	if cfg.Client.ServerAddr != "" {
		srvAddr = cfg.Client.ServerAddr
	}

	certF := *certFile
	keyF := *keyFile
	caF := *caFile

	if cfg.TLS.CertFile != "" {
		certF = cfg.TLS.CertFile
		keyF = cfg.TLS.KeyFile
		caF = cfg.TLS.CAFile
	}

	agent, err := NewHostAgent(devID, ctrlAddr, srvAddr, certF, keyF, caF)
	if err != nil {
		log.Fatalf("Failed to create Host Agent: %v", err)
	}

	log.Printf("SDP Host Agent starting...")
	log.Printf("  Device ID: %s", agent.deviceID)
	log.Printf("  Controller: %s", ctrlAddr)
	log.Printf("  Server: %s", srvAddr)

	// Register with controller
	if err := agent.register(); err != nil {
		log.Printf("Registration failed: %v", err)
	}

	// Get token from controller
	if err := agent.getToken(); err != nil {
		log.Printf("Failed to get token: %v", err)
	} else {
		log.Printf("Token obtained successfully")
	}

	// Send SPA packet
	if err := agent.sendSPA(); err != nil {
		log.Printf("SPA failed: %v", err)
	} else {
		log.Printf("SPA sent successfully")
	}

	// Connect to server
	if err := agent.connect(); err != nil {
		log.Printf("Connection failed: %v", err)
	}

	log.Println("Host Agent demo completed")

	// Wait for signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func (h *HostAgent) register() error {
	url := h.controllerURL + "/api/v1/auth/register"

	reqBody := common.AuthRequest{
		DeviceID:   h.deviceID,
		PublicKey:  h.pubKeyPEM,
		DeviceName: "Host Agent " + h.deviceID[:8],
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status: %d", resp.StatusCode)
	}

	log.Printf("Device registered with controller")
	return nil
}

func (h *HostAgent) getToken() error {
	url := h.controllerURL + "/api/v1/auth/token"

	reqBody := map[string]string{
		"device_id": h.deviceID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var authResp common.AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	h.token = authResp.Token
	log.Printf("Token: %s...", h.token[:min(20, len(h.token))])
	return nil
}

func (h *HostAgent) sendSPA() error {
	// Generate nonce
	nonce, err := crypto.GenerateNonce(16)
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	// Build SPA packet
	pkt := common.NewSPAPacket(h.deviceID, common.SPATypeAuth, nonce)

	// Sign SPA packet
	sig, err := crypto.SignSPA(h.deviceID, pkt.Timestamp, nonce, h.privKey)
	if err != nil {
		return fmt.Errorf("sign SPA: %w", err)
	}
	pkt.Signature = sig

	// Send SPA via UDP
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:8442", h.serverAddr))
	if err != nil {
		return fmt.Errorf("resolve UDP address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("dial UDP: %w", err)
	}
	defer conn.Close()

	pktData, err := json.Marshal(pkt)
	if err != nil {
		return fmt.Errorf("marshal SPA: %w", err)
	}

	if _, err := conn.Write(pktData); err != nil {
		return fmt.Errorf("send SPA: %w", err)
	}

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		// SPA may fail silently in demo mode
		log.Printf("SPA response not received (expected in demo): %v", err)
		return nil
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return fmt.Errorf("parse SPA response: %w", err)
	}

	log.Printf("SPA response: %v", resp)
	return nil
}

func (h *HostAgent) connect() error {
	// Connect to server with mTLS
	addr := h.serverAddr
	conn, err := tls.Dial("tcp", addr, h.tlsConfig)
	if err != nil {
		return fmt.Errorf("dial TLS: %w", err)
	}
	defer conn.Close()

	// Verify server certificate
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no server certificate")
	}

	log.Printf("Connected to %s", state.PeerCertificates[0].Subject.CommonName)

	// Send HTTP request with token
	req := fmt.Sprintf("GET /api/resource HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n", addr, h.token)

	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read response: %w", err)
	}

	log.Printf("Server response: %s", string(buf[:n]))
	return nil
}
