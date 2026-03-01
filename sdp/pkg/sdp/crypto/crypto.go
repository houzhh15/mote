// Package crypto provides cryptographic functions for SDP protocol
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"sdp/pkg/sdp/common"
)

var (
	// ErrInvalidSignature indicates signature verification failed
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrTokenExpired indicates token has expired
	ErrTokenExpired = errors.New("token expired")
	// ErrInvalidToken indicates token is invalid
	ErrInvalidToken = errors.New("invalid token")
	// ErrInvalidKey indicates invalid key type
	ErrInvalidKey = errors.New("invalid key type")
)

// GenerateNonce generates a random nonce for SPA packet
func GenerateNonce(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

// GenerateDeviceKeyPair generates ECDSA key pair for device
func GenerateDeviceKeyPair() (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key pair: %w", err)
	}
	return key, &key.PublicKey, nil
}

// SignSPA signs SPA packet data using ECDSA private key
func SignSPA(deviceID string, timestamp int64, nonce string, privKey *ecdsa.PrivateKey) (string, error) {
	data := fmt.Sprintf("%s|%d|%s", deviceID, timestamp, nonce)
	hash := sha256.Sum256([]byte(data))

	sig, err := ecdsa.SignASN1(rand.Reader, privKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign SPA: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifySPA verifies SPA packet signature
func VerifySPA(deviceID string, timestamp int64, nonce string, signature string, pubKey *ecdsa.PublicKey) error {
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	data := fmt.Sprintf("%s|%d|%s", deviceID, timestamp, nonce)
	hash := sha256.Sum256([]byte(data))

	if !ecdsa.VerifyASN1(pubKey, hash[:], sig) {
		return ErrInvalidSignature
	}

	return nil
}

// HMACSign generates HMAC-SHA256 signature
func HMACSign(data string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// HMACVerify verifies HMAC-SHA256 signature
func HMACVerify(data string, secret string, signature string) bool {
	expected := HMACSign(data, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// PublicKeyToPEM converts public key to PEM format
func PublicKeyToPEM(pubKey *ecdsa.PublicKey) (string, error) {
	derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	})

	return string(pemBytes), nil
}

// PEMToPublicKey converts PEM format to public key
func PEMToPublicKey(pemData string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ErrInvalidKey
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	ecdsaPubKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, ErrInvalidKey
	}

	return ecdsaPubKey, nil
}

// PrivateKeyToPEM converts private key to PEM format
func PrivateKeyToPEM(privKey *ecdsa.PrivateKey) (string, error) {
	derBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: derBytes,
	})

	return string(pemBytes), nil
}

// PEMToPrivateKey converts PEM format to private key
func PEMToPrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ErrInvalidKey
	}

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return privKey, nil
}

// JWTService handles JWT token operations
type JWTService struct {
	secret        []byte
	tokenExpiry   time.Duration
}

// NewJWTService creates a new JWT service
func NewJWTService(secret string, expiryMinutes int) *JWTService {
	return &JWTService{
		secret:      []byte(secret),
		tokenExpiry: time.Duration(expiryMinutes) * time.Minute,
	}
}

// GenerateToken generates JWT token for device
func (s *JWTService) GenerateToken(deviceID string, roles, permissions []string) (string, error) {
	now := time.Now()
	claims := &common.JWTTokenClaims{
		Iss:                "SDP-Controller",
		Sub:                deviceID,
		Aud:                "application-server",
		Exp:                now.Add(s.tokenExpiry).Unix(),
		Iat:                now.Unix(),
		JTI:                fmt.Sprintf("%d-%s", now.UnixNano(), deviceID),
		Roles:              roles,
		Permissions:       permissions,
		DeviceFingerprint: "",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// VerifyToken verifies JWT token and returns claims
func (s *JWTService) VerifyToken(tokenString string) (*common.JWTTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &common.JWTTokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*common.JWTTokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.IsExpired() {
		return nil, ErrTokenExpired
	}

	return claims, nil
}

// GenerateSelfSignedCert generates a self-signed certificate
func GenerateSelfSignedCert(commonName string, days int) ([]byte, []byte, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, days),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})

	return certPEM, keyPEM, nil
}

// LoadTLSCert loads TLS certificate from file
func LoadTLSCert(certFile, keyFile string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(certFile, keyFile)
}

// LoadCA loads CA certificate from file
func LoadCA(caFile string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	return certPool, nil
}

// GetServerTLSConfig returns TLS configuration for server
func GetServerTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := LoadTLSCert(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS cert: %w", err)
	}

	caPool, err := LoadCA(caFile)
	if err != nil {
		return nil, fmt.Errorf("load CA: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// GetClientTLSConfig returns TLS configuration for client
func GetClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := LoadTLSCert(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS cert: %w", err)
	}

	caPool, err := LoadCA(caFile)
	if err != nil {
		return nil, fmt.Errorf("load CA: %w", err)
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS13,
	}, nil
}

// SignJSON signs JSON data using HMAC
func SignJSON(data interface{}, secret string) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return HMACSign(string(jsonBytes), secret), nil
}

// VerifyJSONSignature verifies JSON signature
func VerifyJSONSignature(data interface{}, secret string, signature string) bool {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return false
	}

	return HMACVerify(string(jsonBytes), secret, signature)
}

// HashDeviceID generates SHA256 hash of device identifier
func HashDeviceID(deviceID string) string {
	hash := sha256.Sum256([]byte(deviceID))
	return fmt.Sprintf("%x", hash)
}
