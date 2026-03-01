// Package common provides core data structures for SDP protocol
package common

import (
	"encoding/json"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SPA Packet Types
const (
	SPATypeRegister   = 0x01 // Registration request
	SPATypeAuth      = 0x02 // Authentication request
	SPATypeRevoke    = 0x03 // Revocation request
)

// SPA Packet Flags
const (
	SPAFlagTokenRequired = 0x01 // Requires token response
	SPAFlagHeartbeat    = 0x02 // Heartbeat packet
)

// SPAVersion protocol version
const SPAVersion = 0x01

// SPAPacket represents Single Packet Authorization structure
type SPAPacket struct {
	Version    uint8     `json:"version"`    // Protocol version
	Type       uint8     `json:"type"`       // Packet type
	Flags      uint8     `json:"flags"`      // Flags
	DeviceID   string    `json:"device_id"`  // Device unique identifier
	Timestamp  int64     `json:"timestamp"`  // Unix timestamp (milliseconds)
	Nonce      string    `json:"nonce"`      // Random nonce for replay protection
	Signature  string    `json:"signature"`  // HMAC-SHA256 signature (base64)
	Extensions []byte    `json:"extensions"` // Optional extensions
}

// NewSPAPacket creates a new SPA packet
func NewSPAPacket(deviceID string, pktType uint8, nonce string) *SPAPacket {
	return &SPAPacket{
		Version:   SPAVersion,
		Type:      pktType,
		Flags:     0,
		DeviceID:  deviceID,
		Timestamp: time.Now().UnixMilli(),
		Nonce:     nonce,
	}
}

// IsValid checks if SPA packet has valid timestamp (within 5 minutes)
func (s *SPAPacket) IsValid() bool {
	now := time.Now().UnixMilli()
	delta := now - s.Timestamp
	return delta >= -300000 && delta <= 300000 // ±5 minutes
}

// JWTTokenClaims represents JWT token claims
type JWTTokenClaims struct {
	Iss                string   `json:"iss"`                  // Issuer
	Sub                string   `json:"sub"`                  // Subject (device ID)
	Aud                string   `json:"aud"`                  // Audience (server ID)
	Exp                int64    `json:"exp"`                  // Expiration time
	Iat                int64    `json:"iat"`                  // Issued at time
	JTI                string   `json:"jti"`                  // Unique token ID
	Roles              []string `json:"roles"`                // User roles
	Permissions       []string `json:"permissions"`          // Access permissions
	DeviceFingerprint string   `json:"device_fingerprint"`   // Device fingerprint
}

// GetAudience returns the audience claim
func (c *JWTTokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	if c.Aud == "" {
		return nil, nil
	}
	return jwt.ClaimStrings{c.Aud}, nil
}

// GetIssuer returns the issuer claim
func (c *JWTTokenClaims) GetIssuer() (string, error) {
	return c.Iss, nil
}

// GetSubject returns the subject claim
func (c *JWTTokenClaims) GetSubject() (string, error) {
	return c.Sub, nil
}

// GetExpirationTime returns the expiration time
func (c *JWTTokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}
	return &jwt.NumericDate{Time: time.Unix(c.Exp, 0)}, nil
}

// GetIssuedAt returns the issued at time
func (c *JWTTokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.Iat == 0 {
		return nil, nil
	}
	return &jwt.NumericDate{Time: time.Unix(c.Iat, 0)}, nil
}

// GetNotBefore returns the not before time (not used)
func (c *JWTTokenClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

// IsExpired checks if token is expired
func (c *JWTTokenClaims) IsExpired() bool {
	return time.Now().Unix() > c.Exp
}

// IsValid checks if token claims are valid
func (c *JWTTokenClaims) IsValid() bool {
	if c.IsExpired() {
		return false
	}
	if c.Sub == "" || c.Iss == "" {
		return false
	}
	return true
}

// DeviceInfo represents a registered device
type DeviceInfo struct {
	DeviceID       string    `json:"device_id"`
	DisplayName    string    `json:"display_name"`
	PublicKey      string    `json:"public_key"`
	RegisteredAt   time.Time `json:"registered_at"`
	LastSeen       time.Time `json:"last_seen"`
	Status         string    `json:"status"`
	Policy         *Policy   `json:"policy"`
}

// Policy represents access policy for a device
type Policy struct {
	AllowedServers     []string `json:"allowed_servers"`
	MaxSessionDuration int      `json:"max_session_duration"`
	IPWhitelist       []string `json:"ip_whitelist"`
}

// DeviceRegistry stores registered devices
type DeviceRegistry struct {
	Devices   map[string]*DeviceInfo `json:"devices"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// NewDeviceRegistry creates a new device registry
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		Devices:   make(map[string]*DeviceInfo),
		UpdatedAt: time.Now(),
	}
}

// RegisterDevice adds a device to the registry
func (r *DeviceRegistry) RegisterDevice(device *DeviceInfo) {
	r.Devices[device.DeviceID] = device
	r.UpdatedAt = time.Now()
}

// GetDevice retrieves a device by ID
func (r *DeviceRegistry) GetDevice(deviceID string) (*DeviceInfo, bool) {
	dev, ok := r.Devices[deviceID]
	return dev, ok
}

// AccessSession represents an active access session
type AccessSession struct {
	SessionID       string    `json:"session_id"`
	DeviceID        string    `json:"device_id"`
	ServerID        string    `json:"server_id"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	IPAddress       string    `json:"ip_address"`
	MTLSEstablished bool      `json:"mtls_established"`
}

// IsExpired checks if session is expired
func (s *AccessSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// AuthRequest represents device registration request
type AuthRequest struct {
	DeviceID   string `json:"device_id"`
	PublicKey  string `json:"public_key"`
	DeviceName string `json:"device_name"`
}

// AuthResponse represents authentication response
type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	ServerID  string `json:"server_id"`
}

// VerifyRequest represents token verification request
type VerifyRequest struct {
	Token string `json:"token"`
}

// VerifyResponse represents token verification response
type VerifyResponse struct {
	Valid   bool   `json:"valid"`
	DeviceID string `json:"device_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ErrorResponse represents API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

// ToJSON converts struct to JSON bytes
func ToJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// FromJSON parses JSON to struct
func FromJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
