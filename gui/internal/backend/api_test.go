package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAPIClient(t *testing.T) {
	client := NewAPIClient("http://localhost:18788", 30*time.Second)
	if client == nil {
		t.Fatal("NewAPIClient returned nil")
	}
	if client.baseURL != "http://localhost:18788" { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:18788")
	}
	if client.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", client.timeout, 30*time.Second)
	}
}

func TestAPIClient_buildURL(t *testing.T) {
	client := NewAPIClient("http://localhost:18788", 30*time.Second)

	tests := []struct {
		path string
		want string
	}{
		{"/chat", "http://localhost:18788/api/v1/chat"},
		{"chat", "http://localhost:18788/api/v1/chat"},
		{"/api/v1/chat", "http://localhost:18788/api/v1/chat"},
		{"/api/v1/sessions/123", "http://localhost:18788/api/v1/sessions/123"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := client.buildURL(tt.path)
			if got != tt.want {
				t.Errorf("buildURL(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAPIClient_CallAPI_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("Path = %q, want %q", r.URL.Path, "/api/v1/health")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, 5*time.Second)
	resp, err := client.CallAPI(http.MethodGet, "/health", "")

	if err != nil {
		t.Fatalf("CallAPI() error = %v", err)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want %q", result.Status, "ok")
	}
}

func TestAPIClient_CallAPI_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("json.Decode() error = %v", err)
		}
		if body["message"] != "hello" {
			t.Errorf("body[message] = %q, want %q", body["message"], "hello")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, 5*time.Second)
	_, err := client.Post("/chat", map[string]string{"message": "hello"})

	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
}

func TestAPIClient_CallAPI_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"INVALID_REQUEST","message":"Invalid request body"}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, 5*time.Second)
	_, err := client.Get("/invalid")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if apiErr.Code != "INVALID_REQUEST" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "INVALID_REQUEST")
	}
}

func TestAPIClient_CallAPITyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"test","count":42}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, 5*time.Second)

	var result struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	err := client.CallAPITyped(http.MethodGet, "/data", nil, &result)

	if err != nil {
		t.Fatalf("CallAPITyped() error = %v", err)
	}
	if result.Name != "test" {
		t.Errorf("Name = %q, want %q", result.Name, "test")
	}
	if result.Count != 42 {
		t.Errorf("Count = %d, want %d", result.Count, 42)
	}
}

func TestAPIClient_SetTimeout(t *testing.T) {
	client := NewAPIClient("http://localhost:18788", 30*time.Second)
	client.SetTimeout(10 * time.Second)

	if client.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want %v", client.timeout, 10*time.Second)
	}
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("httpClient.Timeout = %v, want %v", client.httpClient.Timeout, 10*time.Second)
	}
}
