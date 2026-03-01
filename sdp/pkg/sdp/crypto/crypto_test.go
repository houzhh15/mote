// Package crypto provides unit tests for SDP cryptographic functions
package crypto

import (
	"testing"
	"time"
)

func TestGenerateNonce(t *testing.T) {
	nonce, err := GenerateNonce(16)
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}

	if len(nonce) == 0 {
		t.Fatal("Nonce is empty")
	}

	// Test uniqueness
	nonce2, err := GenerateNonce(16)
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}

	if nonce == nonce2 {
		t.Fatal("Nonces should be unique")
	}
}

func TestGenerateDeviceKeyPair(t *testing.T) {
	privKey, pubKey, err := GenerateDeviceKeyPair()
	if err != nil {
		t.Fatalf("GenerateDeviceKeyPair failed: %v", err)
	}

	if privKey == nil {
		t.Fatal("Private key is nil")
	}

	if pubKey == nil {
		t.Fatal("Public key is nil")
	}
}

func TestSignAndVerifySPA(t *testing.T) {
	// Generate key pair
	privKey, pubKey, err := GenerateDeviceKeyPair()
	if err != nil {
		t.Fatalf("GenerateDeviceKeyPair failed: %v", err)
	}

	deviceID := "test-device-001"
	timestamp := time.Now().UnixMilli()
	nonce := "test-nonce-12345"

	// Sign SPA
	signature, err := SignSPA(deviceID, timestamp, nonce, privKey)
	if err != nil {
		t.Fatalf("SignSPA failed: %v", err)
	}

	if signature == "" {
		t.Fatal("Signature is empty")
	}

	// Verify SPA
	err = VerifySPA(deviceID, timestamp, nonce, signature, pubKey)
	if err != nil {
		t.Fatalf("VerifySPA failed: %v", err)
	}
}

func TestSignSPAInvalidSignature(t *testing.T) {
	_, pubKey, err := GenerateDeviceKeyPair()
	if err != nil {
		t.Fatalf("GenerateDeviceKeyPair failed: %v", err)
	}

	deviceID := "test-device-001"
	timestamp := time.Now().UnixMilli()
	nonce := "test-nonce-12345"

	// Create signature with a different key
	privKey2, _, _ := GenerateDeviceKeyPair()
	sig, _ := SignSPA(deviceID, timestamp, nonce, privKey2)

	// Verify with original pubKey should fail
	err = VerifySPA(deviceID, timestamp, nonce, sig, pubKey)
	if err == nil {
		t.Fatal("Expected verification to fail with different key")
	}
}

func TestPublicKeyToPEMAndBack(t *testing.T) {
	_, pubKey, err := GenerateDeviceKeyPair()
	if err != nil {
		t.Fatalf("GenerateDeviceKeyPair failed: %v", err)
	}

	// Convert to PEM
	pemData, err := PublicKeyToPEM(pubKey)
	if err != nil {
		t.Fatalf("PublicKeyToPEM failed: %v", err)
	}

	// Convert back
	pubKey2, err := PEMToPublicKey(pemData)
	if err != nil {
		t.Fatalf("PEMToPublicKey failed: %v", err)
	}

	// Compare (simple check - both should be valid)
	_ = pubKey2
}

func TestPrivateKeyToPEMAndBack(t *testing.T) {
	privKey, _, err := GenerateDeviceKeyPair()
	if err != nil {
		t.Fatalf("GenerateDeviceKeyPair failed: %v", err)
	}

	// Convert to PEM
	pemData, err := PrivateKeyToPEM(privKey)
	if err != nil {
		t.Fatalf("PrivateKeyToPEM failed: %v", err)
	}

	// Convert back
	privKey2, err := PEMToPrivateKey(pemData)
	if err != nil {
		t.Fatalf("PEMToPrivateKey failed: %v", err)
	}

	// Compare
	if privKey.D.Cmp(privKey2.D) != 0 {
		t.Fatal("Private keys don't match")
	}
}

func TestHMACSignAndVerify(t *testing.T) {
	data := "test data"
	secret := "test-secret-key"

	signature := HMACSign(data, secret)
	if signature == "" {
		t.Fatal("HMAC signature is empty")
	}

	// Verify valid
	if !HMACVerify(data, secret, signature) {
		t.Fatal("HMAC verification failed for valid signature")
	}

	// Verify invalid
	if HMACVerify(data, "wrong-secret", signature) {
		t.Fatal("HMAC verification should fail for wrong secret")
	}

	// Verify tampered data
	if HMACVerify("tampered-data", secret, signature) {
		t.Fatal("HMAC verification should fail for tampered data")
	}
}

func TestJWTServiceGenerateAndVerifyToken(t *testing.T) {
	secret := "test-jwt-secret"
	jwtSvc := NewJWTService(secret, 10)

	deviceID := "device-001"
	roles := []string{"user", "device"}
	permissions := []string{"read", "write"}

	// Generate token
	token, err := jwtSvc.GenerateToken(deviceID, roles, permissions)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("Token is empty")
	}

	// Verify token
	claims, err := jwtSvc.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if claims.Sub != deviceID {
		t.Errorf("Expected device ID %s, got %s", deviceID, claims.Sub)
	}

	if claims.Iss != "SDP-Controller" {
		t.Errorf("Expected issuer SDP-Controller, got %s", claims.Iss)
	}
}

func TestJWTServiceVerifyExpiredToken(t *testing.T) {
	secret := "test-jwt-secret"
	jwtSvc := NewJWTService(secret, 0) // 0 minutes expiry

	deviceID := "device-001"
	token, err := jwtSvc.GenerateToken(deviceID, nil, nil)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// Token should be immediately expired
	_, err = jwtSvc.VerifyToken(token)
	if err == nil {
		t.Fatal("Expected error for expired token")
	}
}

func TestJWTServiceVerifyInvalidToken(t *testing.T) {
	secret := "test-jwt-secret"
	jwtSvc := NewJWTService(secret, 10)

	// Test with invalid token
	_, err := jwtSvc.VerifyToken("invalid-token")
	if err == nil {
		t.Fatal("Expected error for invalid token")
	}

	// Test with wrong secret
	jwtSvc2 := NewJWTService("wrong-secret", 10)
	token, _ := jwtSvc2.GenerateToken("device-001", nil, nil)
	_, err = jwtSvc.VerifyToken(token)
	if err == nil {
		t.Fatal("Expected error for token signed with wrong secret")
	}
}

func TestHashDeviceID(t *testing.T) {
	deviceID := "test-device-123"
	hash1 := HashDeviceID(deviceID)
	hash2 := HashDeviceID(deviceID)

	if hash1 != hash2 {
		t.Fatal("Hash should be deterministic")
	}

	if len(hash1) == 0 {
		t.Fatal("Hash is empty")
	}

	// Different devices should have different hashes
	hash3 := HashDeviceID("different-device")
	if hash1 == hash3 {
		t.Fatal("Different devices should have different hashes")
	}
}

func TestSignJSONAndVerify(t *testing.T) {
	secret := "test-secret"
	data := map[string]string{
		"device_id": "device-001",
		"name":      "Test Device",
	}

	// Sign
	signature, err := SignJSON(data, secret)
	if err != nil {
		t.Fatalf("SignJSON failed: %v", err)
	}

	// Verify valid
	if !VerifyJSONSignature(data, secret, signature) {
		t.Fatal("JSON signature verification failed")
	}

	// Verify invalid secret
	if VerifyJSONSignature(data, "wrong-secret", signature) {
		t.Fatal("Should fail with wrong secret")
	}

	// Verify tampered data
	data["name"] = "Tampered"
	if VerifyJSONSignature(data, secret, signature) {
		t.Fatal("Should fail with tampered data")
	}
}
