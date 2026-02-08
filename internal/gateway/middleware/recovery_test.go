package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mote/internal/gateway/handlers"
)

func TestRecovery(t *testing.T) {
	t.Run("passes through normal requests", func(t *testing.T) {
		handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("recovers from panic", func(t *testing.T) {
		handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		// Should not panic
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}

		var resp handlers.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if resp.Error.Code != handlers.ErrCodeInternalError {
			t.Errorf("code = %s, want %s", resp.Error.Code, handlers.ErrCodeInternalError)
		}
	})
}
