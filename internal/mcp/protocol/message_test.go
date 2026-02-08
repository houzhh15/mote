package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewRequest(t *testing.T) {
	params := map[string]string{"key": "value"}
	req, err := NewRequest("test/method", params)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if req.Jsonrpc != JSONRPCVersion {
		t.Errorf("expected jsonrpc %s, got %s", JSONRPCVersion, req.Jsonrpc)
	}
	if req.Method != "test/method" {
		t.Errorf("expected method test/method, got %s", req.Method)
	}
	if req.ID == nil {
		t.Error("expected non-nil ID")
	}

	// Verify params serialization
	var p map[string]string
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatalf("unmarshal params failed: %v", err)
	}
	if p["key"] != "value" {
		t.Errorf("expected params key=value, got %v", p)
	}
}

func TestNewRequestWithID(t *testing.T) {
	req, err := NewRequestWithID(42, "test/method", nil)
	if err != nil {
		t.Fatalf("NewRequestWithID failed: %v", err)
	}

	if req.ID != 42 {
		t.Errorf("expected ID 42, got %v", req.ID)
	}
	if req.Params != nil {
		t.Errorf("expected nil params, got %v", req.Params)
	}
}

func TestNewNotification(t *testing.T) {
	notif, err := NewNotification("test/notify", nil)
	if err != nil {
		t.Fatalf("NewNotification failed: %v", err)
	}

	if notif.Jsonrpc != JSONRPCVersion {
		t.Errorf("expected jsonrpc %s, got %s", JSONRPCVersion, notif.Jsonrpc)
	}
	if notif.Method != "test/notify" {
		t.Errorf("expected method test/notify, got %s", notif.Method)
	}
}

func TestNewResponse(t *testing.T) {
	result := map[string]int{"count": 5}
	resp, err := NewResponse(1, result)
	if err != nil {
		t.Fatalf("NewResponse failed: %v", err)
	}

	if resp.Jsonrpc != JSONRPCVersion {
		t.Errorf("expected jsonrpc %s, got %s", JSONRPCVersion, resp.Jsonrpc)
	}
	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %v", resp.ID)
	}
	if resp.Error != nil {
		t.Error("expected nil error")
	}

	var r map[string]int
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		t.Fatalf("unmarshal result failed: %v", err)
	}
	if r["count"] != 5 {
		t.Errorf("expected result count=5, got %v", r)
	}
}

func TestNewErrorResponse(t *testing.T) {
	rpcErr := &RPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}
	resp := NewErrorResponse(1, rpcErr)

	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %v", resp.ID)
	}
	if resp.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("expected error code -32600, got %d", resp.Error.Code)
	}
	if resp.Result != nil {
		t.Error("expected nil result")
	}
}

func TestParseMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(*Message) bool
	}{
		{
			name:  "valid request",
			input: `{"jsonrpc":"2.0","id":1,"method":"test","params":{}}`,
			check: func(m *Message) bool {
				return m.IsRequest() && m.Method == "test"
			},
		},
		{
			name:  "valid notification",
			input: `{"jsonrpc":"2.0","method":"notify"}`,
			check: func(m *Message) bool {
				return m.IsNotification() && m.Method == "notify"
			},
		},
		{
			name:  "valid response",
			input: `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
			check: func(m *Message) bool {
				return m.IsResponse() && m.ID != nil
			},
		},
		{
			name:  "error response",
			input: `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid"}}`,
			check: func(m *Message) bool {
				return m.IsResponse() && m.Error != nil && m.Error.Code == -32600
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid}`,
			wantErr: true,
		},
		{
			name:    "wrong version",
			input:   `{"jsonrpc":"1.0","method":"test"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil && !tt.check(msg) {
				t.Error("check function failed")
			}
		})
	}
}

func TestMessageConversions(t *testing.T) {
	t.Run("ToRequest", func(t *testing.T) {
		msg := &Message{
			Jsonrpc: JSONRPCVersion,
			ID:      float64(1),
			Method:  "test",
		}
		req := msg.ToRequest()
		if req == nil {
			t.Fatal("expected non-nil request")
		}
		if req.Method != "test" {
			t.Errorf("expected method test, got %s", req.Method)
		}
	})

	t.Run("ToNotification", func(t *testing.T) {
		msg := &Message{
			Jsonrpc: JSONRPCVersion,
			Method:  "notify",
		}
		notif := msg.ToNotification()
		if notif == nil {
			t.Fatal("expected non-nil notification")
		}
		if notif.Method != "notify" {
			t.Errorf("expected method notify, got %s", notif.Method)
		}
	})

	t.Run("ToResponse", func(t *testing.T) {
		msg := &Message{
			Jsonrpc: JSONRPCVersion,
			ID:      float64(1),
			Result:  json.RawMessage(`{}`),
		}
		resp := msg.ToResponse()
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
	})
}

func TestRPCErrorInterface(t *testing.T) {
	err := &RPCError{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    "extra info",
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}

	// Test without data
	err2 := &RPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}
	errStr2 := err2.Error()
	if errStr2 == "" {
		t.Error("expected non-empty error string")
	}
}

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		input any
		want  int64
	}{
		{int64(42), 42},
		{float64(42), 42},
		{int(42), 42},
		{"string-id", 0},
		{nil, 0},
	}

	for _, tt := range tests {
		got := GetRequestID(tt.input)
		if got != tt.want {
			t.Errorf("GetRequestID(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestRequestSerialization(t *testing.T) {
	req, _ := NewRequestWithID(1, "test/method", map[string]string{"key": "value"})
	data, err := Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Parse it back
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}

	if !msg.IsRequest() {
		t.Error("expected message to be a request")
	}
	if msg.Method != "test/method" {
		t.Errorf("expected method test/method, got %s", msg.Method)
	}
}
