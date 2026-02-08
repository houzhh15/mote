package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogging(t *testing.T) {
	called := false
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remoteIP string
		want     string
	}{
		{
			name:     "X-Forwarded-For",
			headers:  map[string]string{"X-Forwarded-For": "192.168.1.1"},
			remoteIP: "127.0.0.1:12345",
			want:     "192.168.1.1",
		},
		{
			name:     "X-Real-IP",
			headers:  map[string]string{"X-Real-IP": "10.0.0.1"},
			remoteIP: "127.0.0.1:12345",
			want:     "10.0.0.1",
		},
		{
			name:     "RemoteAddr fallback",
			headers:  map[string]string{},
			remoteIP: "127.0.0.1:12345",
			want:     "127.0.0.1:12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteIP
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rw.status, http.StatusNotFound)
	}
}
