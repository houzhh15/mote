package bench

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"mote/internal/config"
	"mote/internal/gateway"
	"mote/internal/gateway/websocket"
)

var benchServer *gateway.Server

func TestMain(m *testing.M) {
	// Create minimal config for benchmarks
	cfg := &config.Config{
		Version: "bench-test",
		Gateway: config.GatewayConfig{
			Host:      "127.0.0.1",
			Port:      0,
			StaticDir: os.TempDir(),
			UIDir:     os.TempDir(),
			RateLimit: config.RateLimitConfig{
				Enabled: false,
			},
		},
	}

	hub := websocket.NewHub()
	go hub.Run()

	benchServer = gateway.NewServer(cfg, hub, nil)

	code := m.Run()
	os.Exit(code)
}

// getBenchServer returns the benchmark server.
func getBenchServer() *gateway.Server {
	return benchServer
}

// benchRequest is a helper to run a benchmark request.
func benchRequest(b *testing.B, method, path string) {
	b.Helper()

	router := benchServer.Router()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			b.Errorf("Expected status 200, got %d", rr.Code)
		}
	}
}
