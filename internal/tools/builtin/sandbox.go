package builtin

import (
	"context"
	"time"
)

// Sandbox abstracts command execution in an isolated environment.
// Phase 3: Currently only NoopSandbox is implemented. Future backends
// include Docker, Firecracker, etc.
type Sandbox interface {
	// Execute runs a command in the sandbox.
	Execute(ctx context.Context, cmd string, opts SandboxOpts) (string, error)
	// Available returns whether this sandbox backend is available.
	Available() bool
	// Name returns the sandbox backend name.
	Name() string
}

// SandboxOpts configures sandbox execution parameters.
type SandboxOpts struct {
	WorkDir     string
	ReadOnly    bool
	Network     bool
	MemoryLimit string
	TimeLimit   time.Duration
}

// NoopSandbox executes commands directly without any isolation (current behavior).
type NoopSandbox struct{}

// Execute runs the command directly — no sandbox isolation.
func (n *NoopSandbox) Execute(_ context.Context, _ string, _ SandboxOpts) (string, error) {
	// Phase 3: This will be replaced by an actual sandbox implementation.
	// Currently returns an error indicating sandbox is not available.
	return "", nil
}

// Available returns false — noop sandbox has no isolation.
func (n *NoopSandbox) Available() bool {
	return false
}

// Name returns the sandbox backend name.
func (n *NoopSandbox) Name() string {
	return "noop"
}

// DockerSandbox executes commands in a Docker container.
// Phase 3 implementation placeholder.
// type DockerSandbox struct {
//     Image   string
//     Timeout time.Duration
// }
