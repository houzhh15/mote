#!/bin/bash

# SDP Zero Trust Demo Integration Test Script

set -e

echo "========================================="
echo "SDP Zero Trust Demo - Integration Test"
echo "========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CONTROLLER_ADDR="localhost:8080"
APP_SERVER_ADDR="localhost:8443"
SPA_PORT=8442

# Function to print status
print_status() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_info() {
    echo -e "${YELLOW}[i]${NC} $1"
}

# Check if binaries exist
echo ""
echo "Checking binaries..."
if [ -f "./cmd/controller/controller" ]; then
    print_status "Controller binary found"
else
    print_error "Controller binary not found"
    exit 1
fi

if [ -f "./cmd/app_server/app_server" ]; then
    print_status "App Server binary found"
else
    print_error "App Server binary not found"
    exit 1
fi

if [ -f "./cmd/host_agent/host_agent" ]; then
    print_status "Host Agent binary found"
else
    print_error "Host Agent binary not found"
    exit 1
fi

# Check certificates
echo ""
echo "Checking certificates..."
if [ -f "./certs/ca.pem" ]; then
    print_status "CA certificate found"
else
    print_error "CA certificate not found"
    exit 1
fi

# Start Controller
echo ""
echo "Starting Controller..."
cd "$(dirname "$0")"
./cmd/controller/controller -addr :8080 &
CONTROLLER_PID=$!
print_status "Controller started (PID: $CONTROLLER_PID)"

# Wait for Controller to start
sleep 2

# Check Controller health
echo ""
echo "Checking Controller health..."
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    print_status "Controller health check passed"
else
    print_error "Controller health check failed"
    kill $CONTROLLER_PID 2>/dev/null || true
    exit 1
fi

# Test device registration
echo ""
echo "Testing device registration..."
REGISTER_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"device_id":"test-device-001","device_name":"Test Device"}')

if echo "$REGISTER_RESPONSE" | grep -q "token"; then
    print_status "Device registration passed"
else
    print_error "Device registration failed"
    kill $CONTROLLER_PID 2>/dev/null || true
    exit 1
fi

# Test token generation
echo ""
echo "Testing token generation..."
TOKEN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/token \
    -H "Content-Type: application/json" \
    -d '{"device_id":"test-device-001"}')

if echo "$TOKEN_RESPONSE" | grep -q "token"; then
    print_status "Token generation passed"
    TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    print_info "Token obtained: ${TOKEN:0:20}..."
else
    print_error "Token generation failed"
    kill $CONTROLLER_PID 2>/dev/null || true
    exit 1
fi

# Test token verification
echo ""
echo "Testing token verification..."
VERIFY_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/verify \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$TOKEN\"}")

if echo "$VERIFY_RESPONSE" | grep -q '"valid":true'; then
    print_status "Token verification passed"
else
    print_error "Token verification failed"
    kill $CONTROLLER_PID 2>/dev/null || true
    exit 1
fi

# Start App Server
echo ""
echo "Starting App Server..."
./cmd/app_server/app_server -listen :8443 -spa 8442 &
APP_SERVER_PID=$!
print_status "App Server started (PID: $APP_SERVER_PID)"

# Wait for App Server to start
sleep 2

# Note: Full SPA and mTLS test would require running components in sequence
# For now, we verify that components start successfully
print_info "Note: Full SPA+mTLS test requires sequential component execution"

# Cleanup
echo ""
echo "Cleaning up..."
kill $CONTROLLER_PID 2>/dev/null || true
kill $APP_SERVER_PID 2>/dev/null || true
print_status "Cleanup completed"

echo ""
echo "========================================="
echo -e "${GREEN}Integration Test Completed!${NC}"
echo "========================================="
echo ""
echo "To run full demo:"
echo "  1. Terminal 1: ./cmd/controller/controller -addr :8080"
echo "  2. Terminal 2: ./cmd/app_server/app_server -listen :8443 -spa 8442"
echo "  3. Terminal 3: ./cmd/host_agent/host_agent -controller localhost:8080 -server localhost:8443"
echo ""
