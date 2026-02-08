package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	InitStartTime() // Initialize start time

	handler := HealthHandler("v1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("status = %s, want ok", resp.Status)
	}

	if resp.Version != "v1.0.0" {
		t.Errorf("version = %s, want v1.0.0", resp.Version)
	}

	if resp.Uptime < 0 {
		t.Errorf("uptime = %d, want >= 0", resp.Uptime)
	}
}
