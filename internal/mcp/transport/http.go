package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSSEConnectionClosed is returned when the SSE connection is closed.
	ErrSSEConnectionClosed = errors.New("SSE connection closed")
)

// HTTPServerTransport implements ServerTransport using HTTP+SSE.
type HTTPServerTransport struct {
	addr       string
	server     *http.Server
	sessions   map[string]chan []byte
	sessionsMu sync.RWMutex
	incoming   chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewHTTPServerTransport creates a new HTTP server transport.
func NewHTTPServerTransport(addr string) *HTTPServerTransport {
	ctx, cancel := context.WithCancel(context.Background())
	t := &HTTPServerTransport{
		addr:     addr,
		sessions: make(map[string]chan []byte),
		incoming: make(chan []byte, 100),
		ctx:      ctx,
		cancel:   cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", t.handleMCP)
	mux.HandleFunc("/mcp/sse", t.handleSSE)

	t.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return t
}

// Start starts the HTTP server.
func (t *HTTPServerTransport) Start() error {
	return t.server.ListenAndServe()
}

// handleMCP handles JSON-RPC requests via HTTP POST.
func (t *HTTPServerTransport) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Get session ID from header
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	// Get session channel
	t.sessionsMu.RLock()
	sessionCh, ok := t.sessions[sessionID]
	t.sessionsMu.RUnlock()

	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Send to incoming channel
	select {
	case t.incoming <- body:
	case <-t.ctx.Done():
		return
	}

	// Wait for response from session channel
	select {
	case resp := <-sessionCh:
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	case <-time.After(30 * time.Second):
		http.Error(w, "Timeout", http.StatusGatewayTimeout)
	case <-t.ctx.Done():
		return
	}
}

// handleSSE handles SSE connections.
func (t *HTTPServerTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get session ID from query
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Create session channel
	sessionCh := make(chan []byte, 10)
	t.sessionsMu.Lock()
	t.sessions[sessionID] = sessionCh
	t.sessionsMu.Unlock()

	defer func() {
		t.sessionsMu.Lock()
		delete(t.sessions, sessionID)
		close(sessionCh)
		t.sessionsMu.Unlock()
	}()

	// Send session ID event
	fmt.Fprintf(w, "event: session\ndata: %s\n\n", sessionID)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Heartbeat ticker
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case data := <-sessionCh:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		case <-t.ctx.Done():
			return
		}
	}
}

// Send sends a response to the specified session.
func (t *HTTPServerTransport) Send(ctx context.Context, data []byte) error {
	// For server transport, we broadcast to all sessions
	// In practice, you'd need to track which session the request came from
	t.sessionsMu.RLock()
	defer t.sessionsMu.RUnlock()

	for _, ch := range t.sessions {
		select {
		case ch <- data:
		default:
			// Channel full, skip
		}
	}
	return nil
}

// SendToSession sends a response to a specific session.
func (t *HTTPServerTransport) SendToSession(ctx context.Context, sessionID string, data []byte) error {
	t.sessionsMu.RLock()
	ch, ok := t.sessions[sessionID]
	t.sessionsMu.RUnlock()

	if !ok {
		return ErrSessionNotFound
	}

	select {
	case ch <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("session channel full")
	}
}

// Receive receives an incoming request.
func (t *HTTPServerTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.incoming:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the HTTP server.
func (t *HTTPServerTransport) Close() error {
	t.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.server.Shutdown(ctx)
}

// HTTPClientTransport implements ClientTransport using HTTP+SSE.
type HTTPClientTransport struct {
	endpoint   string
	headers    map[string]string
	httpClient *http.Client
	sseConn    io.ReadCloser
	sseReader  *bufio.Reader
	incoming   chan []byte
	sessionID  string
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	started    bool
	closed     bool
	mu         sync.Mutex
}

// NewHTTPClientTransport creates a new HTTP client transport.
func NewHTTPClientTransport(endpoint string, headers map[string]string) *HTTPClientTransport {
	return &HTTPClientTransport{
		endpoint:   strings.TrimSuffix(endpoint, "/"),
		headers:    headers,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		incoming:   make(chan []byte, 100),
	}
}

// Start establishes the SSE connection.
func (t *HTTPClientTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrTransportClosed
	}
	if t.started {
		return nil
	}

	t.ctx, t.cancel = context.WithCancel(context.Background())

	// Generate session ID
	t.sessionID = uuid.New().String()

	// Create SSE request - append /sse to the endpoint
	// If endpoint ends with /mcp, SSE endpoint is {endpoint}/sse
	// Otherwise, SSE endpoint is {endpoint}/sse
	sseURL := fmt.Sprintf("%s/sse?sessionId=%s", t.endpoint, t.sessionID)
	req, err := http.NewRequestWithContext(t.ctx, http.MethodGet, sseURL, nil)
	if err != nil {
		return fmt.Errorf("create SSE request: %w", err)
	}

	// Add custom headers
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Make SSE request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connect failed: status %d", resp.StatusCode)
	}

	t.sseConn = resp.Body
	t.sseReader = bufio.NewReader(resp.Body)

	// Start SSE loop
	t.wg.Add(1)
	go t.sseLoop()

	t.started = true
	return nil
}

// sseLoop reads SSE events from the connection.
func (t *HTTPClientTransport) sseLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		event, data, err := t.readSSEEvent()
		if err != nil {
			if t.ctx.Err() != nil {
				return
			}
			continue
		}

		// Handle session event
		if event == "session" {
			t.sessionID = string(data)
			continue
		}

		// Handle message event
		if event == "message" {
			select {
			case t.incoming <- data:
			case <-t.ctx.Done():
				return
			}
		}
	}
}

// readSSEEvent reads a single SSE event.
func (t *HTTPClientTransport) readSSEEvent() (event string, data []byte, err error) {
	var eventType string
	var dataBuilder bytes.Buffer

	for {
		line, err := t.sseReader.ReadString('\n')
		if err != nil {
			return "", nil, err
		}

		line = strings.TrimSpace(line)

		// Empty line means end of event
		if line == "" {
			if dataBuilder.Len() > 0 {
				return eventType, dataBuilder.Bytes(), nil
			}
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if dataBuilder.Len() > 0 {
				dataBuilder.WriteByte('\n')
			}
			dataBuilder.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

// Send sends a request via HTTP POST.
func (t *HTTPClientTransport) Send(ctx context.Context, data []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrTransportClosed
	}
	if !t.started {
		t.mu.Unlock()
		return ErrNotStarted
	}
	sessionID := t.sessionID
	t.mu.Unlock()

	// Create POST request - endpoint is already the full MCP URL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Accept 200 OK and 202 Accepted (for Streamable HTTP / async operations)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Read response body (it will be sent via SSE)
	// For some implementations, the response is returned directly
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// If response has content (and status is 200), add to incoming
	if len(body) > 0 && resp.StatusCode == http.StatusOK {
		select {
		case t.incoming <- body:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// Receive receives a response from the SSE stream.
func (t *HTTPClientTransport) Receive(ctx context.Context) ([]byte, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, ErrTransportClosed
	}
	if !t.started {
		t.mu.Unlock()
		return nil, ErrNotStarted
	}
	t.mu.Unlock()

	select {
	case data := <-t.incoming:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the HTTP client transport.
func (t *HTTPClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.cancel != nil {
		t.cancel()
	}

	t.wg.Wait()

	if t.sseConn != nil {
		t.sseConn.Close()
	}

	return nil
}

// SimpleHTTPClientTransport implements a simple HTTP transport without SSE.
// This is for MCP servers that use plain request-response HTTP.
type SimpleHTTPClientTransport struct {
	endpoint   string
	headers    map[string]string
	httpClient *http.Client
	incoming   chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
	started    bool
	closed     bool
	mu         sync.Mutex
}

// NewSimpleHTTPClientTransport creates a new simple HTTP client transport.
func NewSimpleHTTPClientTransport(endpoint string, headers map[string]string) *SimpleHTTPClientTransport {
	return &SimpleHTTPClientTransport{
		endpoint:   strings.TrimSuffix(endpoint, "/"),
		headers:    headers,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		incoming:   make(chan []byte, 100),
	}
}

// Start initializes the transport.
func (t *SimpleHTTPClientTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrTransportClosed
	}
	if t.started {
		return nil
	}

	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.started = true
	return nil
}

// Send sends a request and receives the response.
func (t *SimpleHTTPClientTransport) Send(ctx context.Context, data []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrTransportClosed
	}
	if !t.started {
		t.mu.Unlock()
		return ErrNotStarted
	}
	t.mu.Unlock()

	// Create POST request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Accept 200 OK and 202 Accepted (for Streamable HTTP / async operations)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Put response in incoming channel (only if we got 200 with content)
	if len(body) > 0 && resp.StatusCode == http.StatusOK {
		select {
		case t.incoming <- body:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Buffer full, drop response
		}
	}

	return nil
}

// Receive receives a response from the transport.
func (t *SimpleHTTPClientTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.incoming:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.ctx.Done():
		return nil, ErrTransportClosed
	}
}

// Close closes the transport.
func (t *SimpleHTTPClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.cancel != nil {
		t.cancel()
	}

	return nil
}
