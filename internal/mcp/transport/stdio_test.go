package transport

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStdioServerTransport_SendReceive(t *testing.T) {
	// Create pipes for testing
	clientToServer := &bytes.Buffer{}
	serverToClient := &bytes.Buffer{}

	// Create server transport with custom IO
	transport := NewStdioServerTransportWithIO(
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"test"}`+"\n"),
		serverToClient,
	)

	ctx := context.Background()

	// Test Receive
	data, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	expected := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	if string(data) != expected {
		t.Errorf("Received data mismatch: got %q, want %q", string(data), expected)
	}

	// Test Send
	response := `{"jsonrpc":"2.0","id":1,"result":{}}`
	err = transport.Send(ctx, []byte(response))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify sent data includes newline
	sentData := serverToClient.String()
	if sentData != response+"\n" {
		t.Errorf("Sent data mismatch: got %q, want %q", sentData, response+"\n")
	}

	_ = clientToServer // unused in this test
}

func TestStdioServerTransport_Close(t *testing.T) {
	transport := NewStdioServerTransportWithIO(
		strings.NewReader(""),
		&bytes.Buffer{},
	)

	// Close should succeed
	err := transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	ctx := context.Background()

	err = transport.Send(ctx, []byte("test"))
	if err != ErrTransportClosed {
		t.Errorf("Send after close: got %v, want ErrTransportClosed", err)
	}

	_, err = transport.Receive(ctx)
	if err != ErrTransportClosed {
		t.Errorf("Receive after close: got %v, want ErrTransportClosed", err)
	}
}

func TestStdioServerTransport_ContextCancellation(t *testing.T) {
	// Use a pipe that will block
	pr, _ := io.Pipe()
	transport := NewStdioServerTransportWithIO(pr, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	// Send should return context error
	err := transport.Send(ctx, []byte("test"))
	if err != context.Canceled {
		t.Errorf("Send with cancelled context: got %v, want context.Canceled", err)
	}
}

func TestStdioServerTransport_MultipleMessages(t *testing.T) {
	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call"}`,
	}

	input := strings.Join(messages, "\n") + "\n"
	output := &bytes.Buffer{}

	transport := NewStdioServerTransportWithIO(
		strings.NewReader(input),
		output,
	)

	ctx := context.Background()

	// Receive all messages
	for i, expected := range messages {
		data, err := transport.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive message %d failed: %v", i+1, err)
		}
		if string(data) != expected {
			t.Errorf("Message %d mismatch: got %q, want %q", i+1, string(data), expected)
		}
	}

	// Next receive should return EOF
	_, err := transport.Receive(ctx)
	if err != io.EOF {
		t.Errorf("Receive after EOF: got %v, want io.EOF", err)
	}
}

func TestStdioServerTransport_LargeMessage(t *testing.T) {
	// Create a large message (100KB)
	largeData := strings.Repeat("x", 100*1024)
	message := `{"jsonrpc":"2.0","id":1,"data":"` + largeData + `"}`

	output := &bytes.Buffer{}
	transport := NewStdioServerTransportWithIO(
		strings.NewReader(message+"\n"),
		output,
	)

	ctx := context.Background()

	data, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive large message failed: %v", err)
	}
	if string(data) != message {
		t.Errorf("Large message mismatch: lengths differ, got %d, want %d", len(data), len(message))
	}
}

func TestStdioClientTransport_NotStarted(t *testing.T) {
	transport := NewStdioClientTransport("echo", []string{"test"})

	ctx := context.Background()

	// Operations before Start should fail
	err := transport.Send(ctx, []byte("test"))
	if err != ErrNotStarted {
		t.Errorf("Send before start: got %v, want ErrNotStarted", err)
	}

	_, err = transport.Receive(ctx)
	if err != ErrNotStarted {
		t.Errorf("Receive before start: got %v, want ErrNotStarted", err)
	}
}

func TestStdioClientTransport_Options(t *testing.T) {
	env := map[string]string{"TEST_VAR": "test_value"}
	workDir := "/tmp"

	transport := NewStdioClientTransport(
		"echo",
		[]string{"test"},
		WithEnv(env),
		WithWorkDir(workDir),
	)

	if transport.env["TEST_VAR"] != "test_value" {
		t.Errorf("Env not set correctly")
	}
	if transport.workDir != workDir {
		t.Errorf("WorkDir not set correctly")
	}
}

func TestStdioClientTransport_StartAndCommunicate(t *testing.T) {
	// Use cat as a simple echo server
	transport := NewStdioClientTransport("cat", []string{})

	err := transport.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send a message
	message := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	err = transport.Send(ctx, []byte(message))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive the echoed message
	data, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if string(data) != message {
		t.Errorf("Received data mismatch: got %q, want %q", string(data), message)
	}
}

func TestStdioClientTransport_Close(t *testing.T) {
	transport := NewStdioClientTransport("cat", []string{})

	err := transport.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	ctx := context.Background()

	err = transport.Send(ctx, []byte("test"))
	if err != ErrTransportClosed {
		t.Errorf("Send after close: got %v, want ErrTransportClosed", err)
	}

	_, err = transport.Receive(ctx)
	if err != ErrTransportClosed {
		t.Errorf("Receive after close: got %v, want ErrTransportClosed", err)
	}
}

func TestStdioClientTransport_DoubleStart(t *testing.T) {
	transport := NewStdioClientTransport("cat", []string{})

	err := transport.Start()
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	defer transport.Close()

	// Second start should be a no-op
	err = transport.Start()
	if err != nil {
		t.Errorf("Second Start should not fail: %v", err)
	}
}

func TestStdioClientTransport_DoubleClose(t *testing.T) {
	transport := NewStdioClientTransport("cat", []string{})

	err := transport.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = transport.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should be a no-op
	err = transport.Close()
	if err != nil {
		t.Errorf("Second Close should not fail: %v", err)
	}
}

func TestTransportType(t *testing.T) {
	tests := []struct {
		name     string
		typ      TransportType
		expected string
	}{
		{"Stdio", TransportStdio, "stdio"},
		{"HTTP+SSE", TransportHTTPSSE, "http+sse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.typ) != tt.expected {
				t.Errorf("TransportType %s: got %q, want %q", tt.name, string(tt.typ), tt.expected)
			}
		})
	}
}

func TestErrTransportClosed(t *testing.T) {
	if ErrTransportClosed.Error() != "transport closed" {
		t.Errorf("ErrTransportClosed message: got %q, want %q", ErrTransportClosed.Error(), "transport closed")
	}
}

func TestErrNotStarted(t *testing.T) {
	if ErrNotStarted.Error() != "transport not started" {
		t.Errorf("ErrNotStarted message: got %q, want %q", ErrNotStarted.Error(), "transport not started")
	}
}
