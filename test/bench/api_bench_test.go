package bench

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkHealthEndpoint(b *testing.B) {
	benchRequest(b, "GET", "/api/v1/health")
}

func BenchmarkSessionsList(b *testing.B) {
	router := benchServer.Router()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Sessions list might return 503 without DB, that's ok for benchmark
		if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
			b.Errorf("Expected status 200 or 503, got %d", rr.Code)
		}
	}
}

func BenchmarkToolsList(b *testing.B) {
	router := benchServer.Router()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/tools", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Tools list might return 503 without registry, that's ok
		if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
			b.Errorf("Expected status 200 or 503, got %d", rr.Code)
		}
	}
}

func BenchmarkCronJobsList(b *testing.B) {
	router := benchServer.Router()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/cron/jobs", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			b.Errorf("Expected status 200, got %d", rr.Code)
		}
	}
}

func BenchmarkMCPServersList(b *testing.B) {
	benchRequest(b, "GET", "/api/v1/mcp/servers")
}

func BenchmarkMCPToolsList(b *testing.B) {
	benchRequest(b, "GET", "/api/v1/mcp/tools")
}

func BenchmarkConfigGet(b *testing.B) {
	benchRequest(b, "GET", "/api/v1/config")
}

func BenchmarkChatRequestParsing(b *testing.B) {
	router := benchServer.Router()

	body := map[string]interface{}{
		"message":    "Hello, world!",
		"session_id": "test-session-123",
	}
	bodyBytes, _ := json.Marshal(body)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/v1/chat", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Chat endpoint requires runner, so 503 is expected
		if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
			b.Errorf("Expected status 200 or 503, got %d", rr.Code)
		}
	}
}

func BenchmarkMemorySearchParsing(b *testing.B) {
	router := benchServer.Router()

	body := map[string]interface{}{
		"query": "test query for memory search",
		"top_k": 10,
	}
	bodyBytes, _ := json.Marshal(body)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/v1/memory/search", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Memory search requires memory index, so 503 is expected
		if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
			b.Errorf("Expected status 200 or 503, got %d", rr.Code)
		}
	}
}

func BenchmarkRouterParallel(b *testing.B) {
	router := benchServer.Router()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/api/v1/health", nil)
			req.Header.Set("Accept", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				b.Errorf("Expected status 200, got %d", rr.Code)
			}
		}
	})
}
