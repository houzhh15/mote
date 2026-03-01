// Package common provides unit tests for SDP common data structures
package common

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSPAPacketCreation(t *testing.T) {
	deviceID := "test-device-001"
	pktType := uint8(SPATypeAuth)
	nonce := "test-nonce-12345"

	pkt := NewSPAPacket(deviceID, pktType, nonce)

	if pkt.Version != SPAVersion {
		t.Errorf("Expected version %d, got %d", SPAVersion, pkt.Version)
	}

	if pkt.Type != pktType {
		t.Errorf("Expected type %d, got %d", pktType, pkt.Type)
	}

	if pkt.DeviceID != deviceID {
		t.Errorf("Expected device ID %s, got %s", deviceID, pkt.DeviceID)
	}

	if pkt.Nonce != nonce {
		t.Errorf("Expected nonce %s, got %s", nonce, pkt.Nonce)
	}

	if pkt.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}
}

func TestSPAPacketIsValid(t *testing.T) {
	pkt := NewSPAPacket("device-001", SPATypeAuth, "nonce")

	// Current packet should be valid
	if !pkt.IsValid() {
		t.Error("Current packet should be valid")
	}

	// Packet with old timestamp should be invalid
	pkt.Timestamp = time.Now().Add(-10 * time.Minute).UnixMilli()
	if pkt.IsValid() {
		t.Error("Old packet should be invalid")
	}

	// Packet with future timestamp should be invalid
	pkt.Timestamp = time.Now().Add(10 * time.Minute).UnixMilli()
	if pkt.IsValid() {
		t.Error("Future packet should be invalid")
	}
}

func TestJWTTokenClaimsIsExpired(t *testing.T) {
	claims := &JWTTokenClaims{
		Exp: time.Now().Add(1 * time.Hour).Unix(),
	}

	if claims.IsExpired() {
		t.Error("Token should not be expired")
	}

	claims.Exp = time.Now().Add(-1 * time.Hour).Unix()
	if !claims.IsExpired() {
		t.Error("Token should be expired")
	}
}

func TestJWTTokenClaimsIsValid(t *testing.T) {
	claims := &JWTTokenClaims{
		Iss: "SDP-Controller",
		Sub: "device-001",
		Exp: time.Now().Add(1 * time.Hour).Unix(),
	}

	if !claims.IsValid() {
		t.Error("Valid claims should pass validation")
	}

	// Test with empty subject
	claims.Sub = ""
	if claims.IsValid() {
		t.Error("Claims with empty subject should be invalid")
	}

	// Test with empty issuer
	claims.Sub = "device-001"
	claims.Iss = ""
	if claims.IsValid() {
		t.Error("Claims with empty issuer should be invalid")
	}

	// Test with expired token
	claims.Iss = "SDP-Controller"
	claims.Exp = time.Now().Add(-1 * time.Hour).Unix()
	if claims.IsValid() {
		t.Error("Expired claims should be invalid")
	}
}

func TestDeviceRegistry(t *testing.T) {
	registry := NewDeviceRegistry()

	device := &DeviceInfo{
		DeviceID:     "device-001",
		DisplayName:  "Test Device",
		RegisteredAt: time.Now(),
		Status:       "active",
	}

	// Register device
	registry.RegisterDevice(device)

	// Get device
	retrieved, exists := registry.GetDevice("device-001")
	if !exists {
		t.Error("Device should exist")
	}

	if retrieved.DeviceID != device.DeviceID {
		t.Errorf("Expected device ID %s, got %s", device.DeviceID, retrieved.DeviceID)
	}

	// Get non-existent device
	_, exists = registry.GetDevice("non-existent")
	if exists {
		t.Error("Non-existent device should not be found")
	}
}

func TestAccessSessionIsExpired(t *testing.T) {
	session := &AccessSession{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if session.IsExpired() {
		t.Error("Session should not be expired")
	}

	session.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !session.IsExpired() {
		t.Error("Session should be expired")
	}
}

func TestToJSONAndFromJSON(t *testing.T) {
	data := &DeviceInfo{
		DeviceID:    "device-001",
		DisplayName: "Test Device",
		Status:      "active",
	}

	// Convert to JSON
	jsonBytes, err := ToJSON(data)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Convert back
	var decoded DeviceInfo
	err = FromJSON(jsonBytes, &decoded)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if decoded.DeviceID != data.DeviceID {
		t.Errorf("Expected device ID %s, got %s", data.DeviceID, decoded.DeviceID)
	}

	if decoded.DisplayName != data.DisplayName {
		t.Errorf("Expected display name %s, got %s", data.DisplayName, decoded.DisplayName)
	}
}

func TestSPAPacketJSONSerialization(t *testing.T) {
	pkt := NewSPAPacket("device-001", SPATypeAuth, "nonce-123")
	pkt.Signature = "test-signature"

	// Serialize
	data, err := json.Marshal(pkt)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize
	var decoded SPAPacket
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.DeviceID != pkt.DeviceID {
		t.Errorf("Expected device ID %s, got %s", pkt.DeviceID, decoded.DeviceID)
	}

	if decoded.Type != pkt.Type {
		t.Errorf("Expected type %d, got %d", pkt.Type, decoded.Type)
	}
}
