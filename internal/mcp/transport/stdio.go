package transport

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
)

var (
	// ErrTransportClosed is returned when attempting to use a closed transport.
	ErrTransportClosed = errors.New("transport closed")
	// ErrNotStarted is returned when attempting to use a client transport that hasn't been started.
	ErrNotStarted = errors.New("transport not started")
)

// StdioServerTransport implements ServerTransport using stdin/stdout.
// This is used by MCP servers to communicate with clients.
type StdioServerTransport struct {
	stdin   io.Reader
	stdout  io.Writer
	scanner *bufio.Scanner
	writeMu sync.Mutex
	closed  bool
	closeMu sync.Mutex
}

// NewStdioServerTransport creates a new StdioServerTransport using os.Stdin and os.Stdout.
func NewStdioServerTransport() *StdioServerTransport {
	return NewStdioServerTransportWithIO(os.Stdin, os.Stdout)
}

// NewStdioServerTransportWithIO creates a new StdioServerTransport with custom IO.
// This is useful for testing.
func NewStdioServerTransportWithIO(stdin io.Reader, stdout io.Writer) *StdioServerTransport {
	scanner := bufio.NewScanner(stdin)
	// Set a larger buffer for potentially large JSON messages
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &StdioServerTransport{
		stdin:   stdin,
		stdout:  stdout,
		scanner: scanner,
	}
}

// Send sends data through stdout.
func (t *StdioServerTransport) Send(ctx context.Context, data []byte) error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return ErrTransportClosed
	}
	t.closeMu.Unlock()

	// Check context before sending
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	// Write data followed by newline
	if _, err := t.stdout.Write(data); err != nil {
		return err
	}
	if _, err := t.stdout.Write([]byte{'\n'}); err != nil {
		return err
	}

	return nil
}

// Receive receives data from stdin.
func (t *StdioServerTransport) Receive(ctx context.Context) ([]byte, error) {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return nil, ErrTransportClosed
	}
	t.closeMu.Unlock()

	// Use a channel to make scanning cancelable
	type scanResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan scanResult, 1)

	go func() {
		if t.scanner.Scan() {
			// Make a copy of the bytes since scanner reuses buffer
			data := make([]byte, len(t.scanner.Bytes()))
			copy(data, t.scanner.Bytes())
			resultCh <- scanResult{data: data}
		} else {
			err := t.scanner.Err()
			if err == nil {
				err = io.EOF
			}
			resultCh <- scanResult{err: err}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.data, result.err
	}
}

// Close closes the transport.
func (t *StdioServerTransport) Close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()
	t.closed = true
	return nil
}

// StdioClientTransport implements ClientTransport for connecting to MCP servers via subprocess.
type StdioClientTransport struct {
	command string
	args    []string
	env     map[string]string
	workDir string

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
	writeMu sync.Mutex
	started bool
	closed  bool
	closeMu sync.Mutex
}

// StdioClientOption is a functional option for StdioClientTransport.
type StdioClientOption func(*StdioClientTransport)

// WithEnv sets environment variables for the subprocess.
func WithEnv(env map[string]string) StdioClientOption {
	return func(t *StdioClientTransport) {
		t.env = env
	}
}

// WithWorkDir sets the working directory for the subprocess.
func WithWorkDir(dir string) StdioClientOption {
	return func(t *StdioClientTransport) {
		t.workDir = dir
	}
}

// NewStdioClientTransport creates a new StdioClientTransport.
func NewStdioClientTransport(command string, args []string, opts ...StdioClientOption) *StdioClientTransport {
	t := &StdioClientTransport{
		command: command,
		args:    args,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Start starts the subprocess.
func (t *StdioClientTransport) Start() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()

	if t.closed {
		return ErrTransportClosed
	}
	if t.started {
		return nil
	}

	t.cmd = exec.Command(t.command, t.args...)

	// Set environment
	if t.env != nil {
		t.cmd.Env = os.Environ()
		for k, v := range t.env {
			t.cmd.Env = append(t.cmd.Env, k+"="+v)
		}
	}

	// Set working directory
	if t.workDir != "" {
		t.cmd.Dir = t.workDir
	}

	// Setup pipes
	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return err
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		t.stdin.Close()
		return err
	}

	// Setup scanner with larger buffer
	t.scanner = bufio.NewScanner(t.stdout)
	t.scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	// Start the process
	if err := t.cmd.Start(); err != nil {
		t.stdin.Close()
		t.stdout.Close()
		return err
	}

	t.started = true
	return nil
}

// Send sends data to the subprocess stdin.
func (t *StdioClientTransport) Send(ctx context.Context, data []byte) error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return ErrTransportClosed
	}
	if !t.started {
		t.closeMu.Unlock()
		return ErrNotStarted
	}
	t.closeMu.Unlock()

	// Check context before sending
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	// Write data followed by newline
	if _, err := t.stdin.Write(data); err != nil {
		return err
	}
	if _, err := t.stdin.Write([]byte{'\n'}); err != nil {
		return err
	}

	return nil
}

// Receive receives data from the subprocess stdout.
func (t *StdioClientTransport) Receive(ctx context.Context) ([]byte, error) {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return nil, ErrTransportClosed
	}
	if !t.started {
		t.closeMu.Unlock()
		return nil, ErrNotStarted
	}
	t.closeMu.Unlock()

	// Use a channel to make scanning cancelable
	type scanResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan scanResult, 1)

	go func() {
		if t.scanner.Scan() {
			// Make a copy of the bytes since scanner reuses buffer
			data := make([]byte, len(t.scanner.Bytes()))
			copy(data, t.scanner.Bytes())
			resultCh <- scanResult{data: data}
		} else {
			err := t.scanner.Err()
			if err == nil {
				err = io.EOF
			}
			resultCh <- scanResult{err: err}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.data, result.err
	}
}

// Close closes the transport and terminates the subprocess.
func (t *StdioClientTransport) Close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	var errs []error

	if t.stdin != nil {
		if err := t.stdin.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if t.stdout != nil {
		if err := t.stdout.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if t.cmd != nil && t.cmd.Process != nil {
		// Wait for the process to exit
		_ = t.cmd.Wait()
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
