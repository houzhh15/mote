package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionMiddleware_CurrentVersion(t *testing.T) {
	config := VersionConfig{
		CurrentVersion:     "1",
		DeprecatedVersions: make(map[string]time.Time),
		DefaultVersion:     "1",
	}

	handler := Version(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("API-Version") != "1" {
		t.Errorf("Expected API-Version 1, got %s", rr.Header().Get("API-Version"))
	}

	if rr.Header().Get("Deprecation") != "" {
		t.Error("Current version should not have Deprecation header")
	}
}

func TestVersionMiddleware_DefaultVersion(t *testing.T) {
	config := VersionConfig{
		CurrentVersion:     "2",
		DeprecatedVersions: make(map[string]time.Time),
		DefaultVersion:     "1",
	}

	handler := Version(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// No Accept-Version header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("API-Version") != "1" {
		t.Errorf("Expected default API-Version 1, got %s", rr.Header().Get("API-Version"))
	}
}

func TestVersionMiddleware_DeprecatedVersion(t *testing.T) {
	sunsetDate := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	config := VersionConfig{
		CurrentVersion: "2",
		DeprecatedVersions: map[string]time.Time{
			"1": sunsetDate,
		},
		DefaultVersion: "2",
	}

	handler := Version(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Deprecation") != "true" {
		t.Errorf("Expected Deprecation true, got %s", rr.Header().Get("Deprecation"))
	}

	if rr.Header().Get("Sunset") == "" {
		t.Error("Expected Sunset header for deprecated version")
	}
}

func TestAPIVersionHandler_VersionRouting(t *testing.T) {
	handler := NewAPIVersionHandler()

	v1Called := false
	v2Called := false

	handler.Register("1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v1Called = true
		w.WriteHeader(http.StatusOK)
	}))

	handler.Register("2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v2Called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Test v1
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !v1Called {
		t.Error("v1 handler should be called")
	}
	if v2Called {
		t.Error("v2 handler should not be called")
	}

	// Reset and test v2
	v1Called = false
	v2Called = false

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "2")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if v1Called {
		t.Error("v1 handler should not be called")
	}
	if !v2Called {
		t.Error("v2 handler should be called")
	}
}

func TestAPIVersionHandler_Fallback(t *testing.T) {
	handler := NewAPIVersionHandler()

	fallbackCalled := false
	handler.SetFallback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request unknown version
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "99")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !fallbackCalled {
		t.Error("Fallback handler should be called for unknown version")
	}
}

func TestAPIVersionHandler_NoFallback(t *testing.T) {
	handler := NewAPIVersionHandler()

	// Request unknown version without fallback
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Version", "99")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("Expected status 406, got %d", rr.Code)
	}
}

func TestAPIVersionHandler_DefaultVersion(t *testing.T) {
	handler := NewAPIVersionHandler()

	v1Called := false
	handler.Register("1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v1Called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Request without version header (defaults to "1")
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !v1Called {
		t.Error("v1 handler should be called for default version")
	}
}
