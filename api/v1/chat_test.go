package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestRouter_HandleChat_NoRunner(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := ChatRequest{
		Message: "Hello",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/chat", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleChat_EmptyMessage(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := ChatRequest{
		Message: "",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/chat", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestRouter_HandleChat_InvalidJSON(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("POST", "/api/v1/chat", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestRouter_HandleChatStream_NoRunner(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := ChatRequest{
		Message: "Hello",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/chat/stream", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleChatStream_EmptyMessage(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := ChatRequest{
		SessionID: "test-session",
		Message:   "",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/chat/stream", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestChatRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request ChatRequest
		valid   bool
	}{
		{
			name:    "valid with message only",
			request: ChatRequest{Message: "Hello"},
			valid:   true,
		},
		{
			name:    "valid with session_id and message",
			request: ChatRequest{SessionID: "sess-123", Message: "Hello"},
			valid:   true,
		},
		{
			name:    "invalid empty message",
			request: ChatRequest{Message: ""},
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.request.Message != ""
			if isValid != tt.valid {
				t.Errorf("Expected valid=%v, got %v", tt.valid, isValid)
			}
		})
	}
}

func TestChatStreamEvent_Types(t *testing.T) {
	tests := []struct {
		name  string
		event ChatStreamEvent
	}{
		{
			name: "content type",
			event: ChatStreamEvent{
				Type:  "content",
				Delta: "Hello",
			},
		},
		{
			name: "tool_call type",
			event: ChatStreamEvent{
				Type: "tool_call",
				ToolCall: &ToolCallResult{
					Name:   "test_tool",
					Result: "success",
				},
			},
		},
		{
			name: "done type",
			event: ChatStreamEvent{
				Type:      "done",
				SessionID: "sess-123",
			},
		},
		{
			name: "error type",
			event: ChatStreamEvent{
				Type:  "error",
				Error: "something went wrong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify JSON serialization
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var result ChatStreamEvent
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if result.Type != tt.event.Type {
				t.Errorf("Type mismatch: got %q, want %q", result.Type, tt.event.Type)
			}
		})
	}
}
