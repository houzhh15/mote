// Package protocol implements JSON-RPC 2.0 message structures for MCP.
package protocol

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// JSON-RPC version constant.
const JSONRPCVersion = "2.0"

// idCounter is used to generate unique request IDs.
var idCounter int64

// Request represents a JSON-RPC 2.0 request message.
type Request struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response message.
type Response struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (request without ID).
type Notification struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface for RPCError.
func (e *RPCError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("RPC error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Message is a union type that can represent any JSON-RPC message.
type Message struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// IsRequest returns true if the message is a request (has method and id).
func (m *Message) IsRequest() bool {
	return m.Method != "" && m.ID != nil
}

// IsNotification returns true if the message is a notification (has method but no id).
func (m *Message) IsNotification() bool {
	return m.Method != "" && m.ID == nil
}

// IsResponse returns true if the message is a response (has result or error).
func (m *Message) IsResponse() bool {
	return m.Result != nil || m.Error != nil
}

// NewRequest creates a new JSON-RPC request with an auto-generated ID.
func NewRequest(method string, params any) (*Request, error) {
	id := atomic.AddInt64(&idCounter, 1)
	return NewRequestWithID(id, method, params)
}

// NewRequestWithID creates a new JSON-RPC request with a specific ID.
func NewRequestWithID(id any, method string, params any) (*Request, error) {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = data
	}

	return &Request{
		Jsonrpc: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}, nil
}

// NewNotification creates a new JSON-RPC notification.
func NewNotification(method string, params any) (*Notification, error) {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = data
	}

	return &Notification{
		Jsonrpc: JSONRPCVersion,
		Method:  method,
		Params:  paramsRaw,
	}, nil
}

// NewResponse creates a new JSON-RPC success response.
func NewResponse(id any, result any) (*Response, error) {
	var resultRaw json.RawMessage
	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
		resultRaw = data
	}

	return &Response{
		Jsonrpc: JSONRPCVersion,
		ID:      id,
		Result:  resultRaw,
	}, nil
}

// NewErrorResponse creates a new JSON-RPC error response.
func NewErrorResponse(id any, rpcErr *RPCError) *Response {
	return &Response{
		Jsonrpc: JSONRPCVersion,
		ID:      id,
		Error:   rpcErr,
	}
}

// ParseMessage parses a JSON-RPC message from bytes.
func ParseMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	if msg.Jsonrpc != JSONRPCVersion {
		return nil, fmt.Errorf("invalid jsonrpc version: %s", msg.Jsonrpc)
	}

	return &msg, nil
}

// ToRequest converts a Message to a Request if valid.
func (m *Message) ToRequest() *Request {
	if !m.IsRequest() {
		return nil
	}
	return &Request{
		Jsonrpc: m.Jsonrpc,
		ID:      m.ID,
		Method:  m.Method,
		Params:  m.Params,
	}
}

// ToNotification converts a Message to a Notification if valid.
func (m *Message) ToNotification() *Notification {
	if !m.IsNotification() {
		return nil
	}
	return &Notification{
		Jsonrpc: m.Jsonrpc,
		Method:  m.Method,
		Params:  m.Params,
	}
}

// ToResponse converts a Message to a Response if valid.
func (m *Message) ToResponse() *Response {
	if !m.IsResponse() {
		return nil
	}
	return &Response{
		Jsonrpc: m.Jsonrpc,
		ID:      m.ID,
		Result:  m.Result,
		Error:   m.Error,
	}
}

// Marshal serializes a message to JSON bytes.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// GetRequestID extracts the request ID as int64 if possible.
// Returns 0 if the ID is not a number.
func GetRequestID(id any) int64 {
	switch v := id.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}
