package copilot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTokenManager(t *testing.T) {
	tm := NewTokenManager("test-github-token")

	if tm == nil {
		t.Fatal("NewTokenManager returned nil")
	}

	if tm.githubToken != "test-github-token" {
		t.Errorf("githubToken = %s, want test-github-token", tm.githubToken)
	}

	if tm.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestGetToken_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if auth != "token test-github-token" {
			t.Errorf("Authorization = %s, want token test-github-token", auth)
		}

		// Return valid token response
		resp := map[string]interface{}{
			"token":      "copilot-token-123",
			"expires_at": time.Now().Add(1 * time.Hour).Unix(),
			"endpoints": map[string]string{
				"api": "https://api.githubcopilot.com",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tm := NewTokenManager("test-github-token")
	tm.SetHTTPClient(server.Client())

	// For testing, we need to make the request directly to our mock server
	// This requires a custom approach - let's use SetHTTPClient and a transport
	tm.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				// Verify authorization header
				auth := req.Header.Get("Authorization")
				if auth != "token test-github-token" {
					t.Errorf("Authorization = %s, want token test-github-token", auth)
				}

				// Create response
				resp := &tokenResponse{
					Token:     "copilot-token-123",
					ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
				}
				resp.Endpoints.API = "https://api.githubcopilot.com"

				body, _ := json.Marshal(resp)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       &mockBody{data: body},
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	token, err := tm.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if token != "copilot-token-123" {
		t.Errorf("token = %s, want copilot-token-123", token)
	}
}

func TestGetToken_Cached(t *testing.T) {
	callCount := 0

	tm := NewTokenManager("test-github-token")
	tm.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				callCount++
				resp := &tokenResponse{
					Token:     "copilot-token-123",
					ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
				}
				resp.Endpoints.API = "https://api.githubcopilot.com"

				body, _ := json.Marshal(resp)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       &mockBody{data: body},
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// First call - should fetch
	_, err := tm.GetToken()
	if err != nil {
		t.Fatalf("First GetToken failed: %v", err)
	}

	// Second call - should use cache
	_, err = tm.GetToken()
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("API called %d times, want 1 (should use cache)", callCount)
	}
}

func TestGetToken_Unauthorized(t *testing.T) {
	tm := NewTokenManager("invalid-token")
	tm.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       &mockBody{data: []byte(`{"error": "unauthorized"}`)},
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	_, err := tm.GetToken()
	if err != ErrUnauthorized {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestGetToken_RateLimited(t *testing.T) {
	tm := NewTokenManager("test-token")
	tm.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       &mockBody{data: []byte(`{"error": "rate limited"}`)},
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	_, err := tm.GetToken()
	if err != ErrRateLimited {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestInvalidate(t *testing.T) {
	tm := NewTokenManager("test-github-token")

	// Set up cache
	tm.cache = &CachedToken{
		Token:     "cached-token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		UpdatedAt: time.Now(),
		BaseURL:   DefaultBaseURL,
	}

	// Invalidate
	tm.Invalidate()

	if tm.cache != nil {
		t.Error("cache should be nil after Invalidate")
	}
}

func TestDeriveBaseURLFromToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "empty token",
			token:    "",
			expected: "",
		},
		{
			name:     "no proxy-ep field",
			token:    "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9",
			expected: "",
		},
		{
			name:     "proxy-ep at start",
			token:    "proxy-ep=proxy.enterprise.github.com;exp=1234567890",
			expected: "https://api.enterprise.github.com",
		},
		{
			name:     "proxy-ep in middle",
			token:    "exp=1234567890;proxy-ep=proxy.corp.example.com;user=test",
			expected: "https://api.corp.example.com",
		},
		{
			name:     "proxy-ep at end",
			token:    "exp=1234567890;user=test;proxy-ep=proxy.internal.company.io",
			expected: "https://api.internal.company.io",
		},
		{
			name:     "proxy-ep with api prefix already",
			token:    "exp=1234567890;proxy-ep=api.already.example.com",
			expected: "https://api.already.example.com",
		},
		{
			name:     "proxy-ep without proxy prefix",
			token:    "exp=1234567890;proxy-ep=custom.example.com",
			expected: "https://custom.example.com",
		},
		{
			name:     "proxy-ep with port",
			token:    "exp=1234567890;proxy-ep=proxy.example.com:8080",
			expected: "https://api.example.com:8080",
		},
		{
			name:     "real-world token format",
			token:    "tid=abc123;proxy-ep=proxy.business.github.com;exp=1706745600;sku=copilot_enterprise;st=dotcom",
			expected: "https://api.business.github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveBaseURLFromToken(tt.token)
			if result != tt.expected {
				t.Errorf("DeriveBaseURLFromToken(%q) = %q, want %q", tt.token, result, tt.expected)
			}
		})
	}
}

func TestGetBaseURL(t *testing.T) {
	t.Run("no cache returns default", func(t *testing.T) {
		tm := NewTokenManager("test-token")
		url := tm.GetBaseURL()
		if url != DefaultBaseURL {
			t.Errorf("GetBaseURL = %s, want %s", url, DefaultBaseURL)
		}
	})

	t.Run("priority 1: proxy-ep from token", func(t *testing.T) {
		tm := NewTokenManager("test-token")
		tm.cache = &CachedToken{
			Token:   "tid=abc;proxy-ep=proxy.enterprise.github.com;exp=9999999999",
			BaseURL: "https://endpoints.api.com",
		}
		url := tm.GetBaseURL()
		expected := "https://api.enterprise.github.com"
		if url != expected {
			t.Errorf("GetBaseURL = %s, want %s (proxy-ep should take priority)", url, expected)
		}
	})

	t.Run("priority 2: endpoints.api when no proxy-ep", func(t *testing.T) {
		tm := NewTokenManager("test-token")
		tm.cache = &CachedToken{
			Token:   "tid=abc;exp=9999999999", // no proxy-ep
			BaseURL: "https://custom.api.com",
		}
		url := tm.GetBaseURL()
		if url != "https://custom.api.com" {
			t.Errorf("GetBaseURL = %s, want https://custom.api.com", url)
		}
	})

	t.Run("priority 3: default when no proxy-ep and no endpoints.api", func(t *testing.T) {
		tm := NewTokenManager("test-token")
		tm.cache = &CachedToken{
			Token:   "tid=abc;exp=9999999999", // no proxy-ep
			BaseURL: "",                       // no endpoints.api
		}
		url := tm.GetBaseURL()
		if url != DefaultBaseURL {
			t.Errorf("GetBaseURL = %s, want %s (default)", url, DefaultBaseURL)
		}
	})
}

func TestCachedToken_IsValid(t *testing.T) {
	t.Run("nil token", func(t *testing.T) {
		var token *CachedToken
		if token.IsValid() {
			t.Error("nil token should not be valid")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token := &CachedToken{
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		if token.IsValid() {
			t.Error("expired token should not be valid")
		}
	})

	t.Run("expiring soon", func(t *testing.T) {
		token := &CachedToken{
			ExpiresAt: time.Now().Add(3 * time.Minute), // Within refresh margin
		}
		if token.IsValid() {
			t.Error("token expiring within margin should not be valid")
		}
	})

	t.Run("valid token", func(t *testing.T) {
		token := &CachedToken{
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		if !token.IsValid() {
			t.Error("valid token should be valid")
		}
	})
}

// mockTransport is a custom http.RoundTripper for testing.
type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

// mockBody is a simple io.ReadCloser for testing.
type mockBody struct {
	data   []byte
	offset int
}

func (m *mockBody) Read(p []byte) (n int, err error) {
	if m.offset >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockBody) Close() error {
	return nil
}

var _ = io.ReadCloser(&mockBody{})
