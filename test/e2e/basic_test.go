package e2e

import (
	"net"
	"testing"
	"time"
)

func isServerRunning() bool {
	conn, err := net.DialTimeout("tcp", "localhost:18788", 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestHealth_Status(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	health := getHealth(t)

	status, ok := health["status"].(string)
	if !ok {
		t.Fatal("status field not found")
	}

	if status != "healthy" && status != "degraded" {
		t.Errorf("Unexpected health status: %s", status)
	}

	// Check for required fields
	if _, ok := health["timestamp"]; !ok {
		t.Error("timestamp field not found")
	}
}

func TestSessions_List(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	// This should return an empty list or existing sessions
	sessions := listSessions(t)

	// Just verify it's a valid response
	if sessions == nil {
		t.Error("Expected sessions array, got nil")
	}
}

func TestTools_List(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	// This should return available tools
	tools := listTools(t)

	// Just verify it's a valid response
	if tools == nil {
		t.Error("Expected tools array, got nil")
	}
}

func TestCronJobs_List(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	// This should return cron jobs
	jobs := listCronJobs(t)

	// Just verify it's a valid response
	if jobs == nil {
		t.Error("Expected jobs array, got nil")
	}
}

func TestMCP_Servers_List(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	// This should return MCP servers
	servers := listMCPServers(t)

	// Just verify it's a valid response
	if servers == nil {
		t.Error("Expected servers array, got nil")
	}
}

func TestMCP_Tools_List(t *testing.T) {
	if !isServerRunning() {
		t.Skip("Mote service not running, skipping e2e test")
	}

	// This should return MCP tools
	tools := listMCPTools(t)

	// Just verify it's a valid response
	if tools == nil {
		t.Error("Expected tools array, got nil")
	}
}
