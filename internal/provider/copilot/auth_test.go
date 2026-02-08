package copilot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewAuthManager(t *testing.T) {
	am := NewAuthManager()

	if am == nil {
		t.Fatal("NewAuthManager returned nil")
	}

	if am.clientID != CopilotClientID {
		t.Errorf("clientID = %s, want %s", am.clientID, CopilotClientID)
	}

	if am.scope != CopilotScope {
		t.Errorf("scope = %s, want %s", am.scope, CopilotScope)
	}

	if am.pollInterval != DefaultPollInterval {
		t.Errorf("pollInterval = %v, want %v", am.pollInterval, DefaultPollInterval)
	}

	if am.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNewAuthManager_WithOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	customClientID := "custom-client-id"
	customScope := "custom:scope"
	customInterval := 10 * time.Second
	customTimeout := 30 * time.Minute

	am := NewAuthManager(
		WithHTTPClient(customClient),
		WithClientID(customClientID),
		WithScope(customScope),
		WithPollInterval(customInterval),
		WithTimeout(customTimeout),
	)

	if am.httpClient != customClient {
		t.Error("httpClient not set correctly")
	}

	if am.clientID != customClientID {
		t.Errorf("clientID = %s, want %s", am.clientID, customClientID)
	}

	if am.scope != customScope {
		t.Errorf("scope = %s, want %s", am.scope, customScope)
	}

	if am.pollInterval != customInterval {
		t.Errorf("pollInterval = %v, want %v", am.pollInterval, customInterval)
	}

	if am.timeout != customTimeout {
		t.Errorf("timeout = %v, want %v", am.timeout, customTimeout)
	}
}

func TestRequestDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %s, want application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		}

		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %s, want application/json", r.Header.Get("Accept"))
		}

		// Read and verify body
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "client_id=") {
			t.Error("Body missing client_id")
		}

		resp := DeviceCodeResponse{
			DeviceCode:      "test-device-code",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create auth manager with custom client that redirects to our test server
	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
	)

	ctx := context.Background()
	resp, err := am.RequestDeviceCode(ctx)

	if err != nil {
		t.Fatalf("RequestDeviceCode failed: %v", err)
	}

	if resp.DeviceCode != "test-device-code" {
		t.Errorf("DeviceCode = %s, want test-device-code", resp.DeviceCode)
	}

	if resp.UserCode != "ABCD-1234" {
		t.Errorf("UserCode = %s, want ABCD-1234", resp.UserCode)
	}

	if resp.VerificationURI != "https://github.com/login/device" {
		t.Errorf("VerificationURI = %s, want https://github.com/login/device", resp.VerificationURI)
	}

	if resp.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900", resp.ExpiresIn)
	}
}

func TestRequestDeviceCode_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_request"}`))
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
	)

	ctx := context.Background()
	_, err := am.RequestDeviceCode(ctx)

	if err == nil {
		t.Fatal("RequestDeviceCode should have failed")
	}

	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("Error should mention status 400: %v", err)
	}
}

func TestPollForAccessToken_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// First call: pending
		if callCount == 1 {
			resp := AccessTokenResponse{
				Error:     "authorization_pending",
				ErrorDesc: "Waiting for user",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Second call: success
		resp := AccessTokenResponse{
			AccessToken: "gho_test_token_123",
			TokenType:   "bearer",
			Scope:       "read:user",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
		WithPollInterval(10*time.Millisecond), // Fast polling for test
	)

	ctx := context.Background()
	resp, err := am.PollForAccessToken(ctx, "test-device-code")

	if err != nil {
		t.Fatalf("PollForAccessToken failed: %v", err)
	}

	if resp.AccessToken != "gho_test_token_123" {
		t.Errorf("AccessToken = %s, want gho_test_token_123", resp.AccessToken)
	}

	if callCount != 2 {
		t.Errorf("Server called %d times, want 2", callCount)
	}
}

func TestPollForAccessToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AccessTokenResponse{
			Error:     "expired_token",
			ErrorDesc: "Device code has expired",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
		WithPollInterval(10*time.Millisecond),
	)

	ctx := context.Background()
	_, err := am.PollForAccessToken(ctx, "test-device-code")

	if err != ErrExpiredToken {
		t.Errorf("Error = %v, want ErrExpiredToken", err)
	}
}

func TestPollForAccessToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AccessTokenResponse{
			Error:     "access_denied",
			ErrorDesc: "User denied access",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
		WithPollInterval(10*time.Millisecond),
	)

	ctx := context.Background()
	_, err := am.PollForAccessToken(ctx, "test-device-code")

	if err != ErrAccessDenied {
		t.Errorf("Error = %v, want ErrAccessDenied", err)
	}
}

func TestPollForAccessToken_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AccessTokenResponse{
			Error: "authorization_pending",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
		WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := am.PollForAccessToken(ctx, "test-device-code")

	if err != context.Canceled {
		t.Errorf("Error = %v, want context.Canceled", err)
	}
}

func TestValidateGitHubToken_Valid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "token valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login": "testuser", "id": 12345}`))
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
	)

	ctx := context.Background()
	valid, err := am.ValidateGitHubToken(ctx, "valid-token")

	if err != nil {
		t.Fatalf("ValidateGitHubToken failed: %v", err)
	}

	if !valid {
		t.Error("Token should be valid")
	}
}

func TestValidateGitHubToken_Invalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	am := NewAuthManager(
		WithHTTPClient(&http.Client{
			Transport: &redirectTransport{target: server.URL},
		}),
	)

	ctx := context.Background()
	valid, err := am.ValidateGitHubToken(ctx, "invalid-token")

	if err != nil {
		t.Fatalf("ValidateGitHubToken failed: %v", err)
	}

	if valid {
		t.Error("Token should be invalid")
	}
}

func TestAuthError(t *testing.T) {
	err := &AuthError{
		Code:    "test_error",
		Message: "Test message",
	}

	expected := "test_error: Test message"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}

	// Test without message
	err2 := &AuthError{Code: "code_only"}
	if err2.Error() != "code_only" {
		t.Errorf("Error() = %s, want code_only", err2.Error())
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
		want bool
	}{
		{
			name: "matching auth error",
			err:  &AuthError{Code: "test_code"},
			code: "test_code",
			want: true,
		},
		{
			name: "non-matching auth error",
			err:  &AuthError{Code: "other_code"},
			code: "test_code",
			want: false,
		},
		{
			name: "non-auth error",
			err:  io.EOF,
			code: "test_code",
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			code: "test_code",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAuthError(tt.err, tt.code)
			if got != tt.want {
				t.Errorf("isAuthError(%v, %q) = %v, want %v", tt.err, tt.code, got, tt.want)
			}
		})
	}
}

// redirectTransport is a custom http.RoundTripper that redirects all requests
// to a target URL (used for testing with httptest servers).
type redirectTransport struct {
	target string
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse target URL and replace the request URL
	targetURL, _ := http.NewRequest(req.Method, rt.target+req.URL.Path, req.Body)
	targetURL.Header = req.Header
	return http.DefaultClient.Do(targetURL)
}
