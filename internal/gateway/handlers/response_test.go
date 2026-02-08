package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		data       any
		wantStatus int
		wantBody   string
	}{
		{
			name:       "send object",
			status:     http.StatusOK,
			data:       map[string]string{"key": "value"},
			wantStatus: http.StatusOK,
			wantBody:   `{"key":"value"}`,
		},
		{
			name:       "send nil",
			status:     http.StatusNoContent,
			data:       nil,
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			SendJSON(w, tt.status, tt.data)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantBody != "" {
				var got, want map[string]string
				_ = json.Unmarshal(w.Body.Bytes(), &got)
				_ = json.Unmarshal([]byte(tt.wantBody), &want)
				if got["key"] != want["key"] {
					t.Errorf("body = %s, want %s", w.Body.String(), tt.wantBody)
				}
			}

			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", ct)
			}
		})
	}
}

func TestSendError(t *testing.T) {
	w := httptest.NewRecorder()
	SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Error.Code != ErrCodeInvalidRequest {
		t.Errorf("code = %s, want %s", resp.Error.Code, ErrCodeInvalidRequest)
	}

	if resp.Error.Message != "bad request" {
		t.Errorf("message = %s, want 'bad request'", resp.Error.Message)
	}
}
