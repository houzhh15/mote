package delegate_test

import (
	"context"
	"testing"

	"mote/internal/config"
	"mote/internal/runner/delegate"
	"mote/internal/tools"
)

// ---- DelegateContext tests ----

func TestDelegateContext_Defaults(t *testing.T) {
	dc := delegate.GetDelegateContext(context.Background())
	if dc.Depth != 0 {
		t.Errorf("expected Depth=0, got %d", dc.Depth)
	}
	if dc.MaxDepth != 3 {
		t.Errorf("expected MaxDepth=3, got %d", dc.MaxDepth)
	}
}

func TestDelegateContext_WithAndGet(t *testing.T) {
	original := &delegate.DelegateContext{
		Depth:           2,
		MaxDepth:        5,
		ParentSessionID: "sess-123",
		AgentName:       "researcher",
		Chain:           []string{"main", "researcher"},
	}
	ctx := delegate.WithDelegateContext(context.Background(), original)
	got := delegate.GetDelegateContext(ctx)

	if got.Depth != original.Depth {
		t.Errorf("Depth: expected %d, got %d", original.Depth, got.Depth)
	}
	if got.MaxDepth != original.MaxDepth {
		t.Errorf("MaxDepth: expected %d, got %d", original.MaxDepth, got.MaxDepth)
	}
	if got.ParentSessionID != original.ParentSessionID {
		t.Errorf("ParentSessionID: expected %q, got %q", original.ParentSessionID, got.ParentSessionID)
	}
	if got.AgentName != original.AgentName {
		t.Errorf("AgentName: expected %q, got %q", original.AgentName, got.AgentName)
	}
	if len(got.Chain) != len(original.Chain) {
		t.Errorf("Chain length: expected %d, got %d", len(original.Chain), len(got.Chain))
	}
}

func TestDelegateContext_ForChild(t *testing.T) {
	parent := &delegate.DelegateContext{
		Depth:           0,
		MaxDepth:        3,
		ParentSessionID: "sess-parent",
		AgentName:       "main",
		Chain:           []string{"main"},
	}

	child := parent.ForChild("researcher")

	// Verify child
	if child.Depth != 1 {
		t.Errorf("child Depth: expected 1, got %d", child.Depth)
	}
	if child.AgentName != "researcher" {
		t.Errorf("child AgentName: expected %q, got %q", "researcher", child.AgentName)
	}
	if len(child.Chain) != 2 || child.Chain[0] != "main" || child.Chain[1] != "researcher" {
		t.Errorf("child Chain: expected [main researcher], got %v", child.Chain)
	}
	if child.MaxDepth != parent.MaxDepth {
		t.Errorf("child MaxDepth should inherit parent's, expected %d, got %d", parent.MaxDepth, child.MaxDepth)
	}

	// Verify parent not mutated
	if parent.Depth != 0 {
		t.Errorf("parent Depth mutated: expected 0, got %d", parent.Depth)
	}
	if len(parent.Chain) != 1 {
		t.Errorf("parent Chain mutated: expected length 1, got %d", len(parent.Chain))
	}
}

func TestDelegateContext_CanDelegate(t *testing.T) {
	tests := []struct {
		depth    int
		maxDepth int
		expected bool
	}{
		{0, 3, true},
		{2, 3, true},
		{3, 3, false},
		{4, 3, false},
	}
	for _, tc := range tests {
		dc := &delegate.DelegateContext{Depth: tc.depth, MaxDepth: tc.maxDepth}
		if got := dc.CanDelegate(); got != tc.expected {
			t.Errorf("CanDelegate(Depth=%d, MaxDepth=%d): expected %v, got %v",
				tc.depth, tc.maxDepth, tc.expected, got)
		}
	}
}

// ---- DelegateTool tests ----

func testAgents() map[string]config.AgentConfig {
	return map[string]config.AgentConfig{
		"researcher": {Description: "Research agent", Model: "gpt-4o"},
		"coder":      {Description: "Coding agent", Model: "claude-3.5"},
	}
}

// setupTestConfig sets up a test config with the test agents and returns a cleanup function.
func setupTestConfig() func() {
	config.SetTestConfig(&config.Config{
		Agents: testAgents(),
	})
	return func() { config.Reset() }
}

func TestDelegateTool_Name(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)
	if tool.Name() != "delegate" {
		t.Errorf("expected name %q, got %q", "delegate", tool.Name())
	}
}

func TestDelegateTool_Description(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)
	desc := tool.Description()
	if !contains(desc, "researcher") {
		t.Errorf("description should contain 'researcher', got: %s", desc)
	}
	if !contains(desc, "coder") {
		t.Errorf("description should contain 'coder', got: %s", desc)
	}
}

func TestDelegateTool_Parameters(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)
	params := tool.Parameters()

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters should have 'properties' map")
	}

	if _, ok := props["agent"]; !ok {
		t.Error("parameters should have 'agent' property")
	}
	if _, ok := props["prompt"]; !ok {
		t.Error("parameters should have 'prompt' property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("parameters should have 'required' array")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}
	if !requiredSet["agent"] || !requiredSet["prompt"] {
		t.Errorf("both 'agent' and 'prompt' should be required, got: %v", required)
	}
}

func TestDelegateTool_Execute_UnknownAgent(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)
	result, err := tool.Execute(context.Background(), map[string]any{
		"agent":  "nonexistent",
		"prompt": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for unknown agent")
	}
	if !contains(result.Content, "unknown agent") {
		t.Errorf("expected content to contain 'unknown agent', got: %s", result.Content)
	}
}

func TestDelegateTool_Execute_MaxDepthExceeded(t *testing.T) {
	// Set MaxStackDepth=5 so Depth=5 triggers the limit
	config.SetTestConfig(&config.Config{
		Agents:   testAgents(),
		Delegate: config.DelegateConfig{MaxStackDepth: 5},
	})
	defer config.Reset()
	tool := delegate.NewDelegateTool(nil, 3)

	// Inject DelegateContext with Depth=5 (>= MaxStackDepth)
	dc := &delegate.DelegateContext{
		Depth:             5,
		MaxDepth:          10,
		Chain:             []string{"a", "b", "c", "d", "e"},
		RecursionCounters: map[string]int{},
	}
	ctx := delegate.WithDelegateContext(context.Background(), dc)

	result, err := tool.Execute(ctx, map[string]any{
		"agent":  "researcher",
		"prompt": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for depth exceeded")
	}
	if !contains(result.Content, "depth") {
		t.Errorf("expected content to contain 'depth', got: %s", result.Content)
	}
}

func TestDelegateTool_Execute_ConfiguredDepthExceeded(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)

	// Inject DelegateContext at configured max depth
	dc := &delegate.DelegateContext{
		Depth:    3,
		MaxDepth: 3,
		Chain:    []string{"a", "b", "c"},
	}
	ctx := delegate.WithDelegateContext(context.Background(), dc)

	result, err := tool.Execute(ctx, map[string]any{
		"agent":  "researcher",
		"prompt": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for configured depth exceeded")
	}
	if !contains(result.Content, "depth") {
		t.Errorf("expected content to contain 'depth', got: %s", result.Content)
	}
}

func TestDelegateTool_Execute_CircularDetection(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 5)

	// Inject DelegateContext with "researcher" already in chain
	dc := &delegate.DelegateContext{
		Depth:    1,
		MaxDepth: 5,
		Chain:    []string{"main", "researcher"},
	}
	ctx := delegate.WithDelegateContext(context.Background(), dc)

	result, err := tool.Execute(ctx, map[string]any{
		"agent":  "researcher",
		"prompt": "do something again",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for circular delegation")
	}
	if !contains(result.Content, "circular") {
		t.Errorf("expected content to contain 'circular', got: %s", result.Content)
	}
}

func TestDelegateTool_Execute_InvalidParams(t *testing.T) {
	cleanup := setupTestConfig()
	defer cleanup()
	tool := delegate.NewDelegateTool(nil, 3)

	t.Run("missing agent", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{
			"prompt": "do something",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for missing agent param")
		}
	})

	t.Run("missing prompt", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{
			"agent": "researcher",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for missing prompt param")
		}
	})
}

// ---- Registry integration tests ----

func TestBuildFilteredRegistry(t *testing.T) {
	// Build a parent registry with 3 tools + delegate
	parent := tools.NewRegistry()
	parent.Register(&dummyTool{name: "read_file"})
	parent.Register(&dummyTool{name: "write_file"})
	parent.Register(&dummyTool{name: "search"})
	parent.Register(&dummyTool{name: "delegate"})

	t.Run("whitelist subset", func(t *testing.T) {
		cloned := parent.Clone()
		cloned.Filter([]string{"read_file", "write_file"})
		cloned.SetAgentID("researcher")

		available := toolNames(cloned.List())
		nameSet := toSet(available)

		if !nameSet["read_file"] || !nameSet["write_file"] {
			t.Errorf("expected read_file and write_file, got: %v", available)
		}
		if nameSet["search"] || nameSet["delegate"] {
			t.Errorf("search and delegate should be filtered out, got: %v", available)
		}
		if cloned.GetAgentID() != "researcher" {
			t.Errorf("expected agentID 'researcher', got %q", cloned.GetAgentID())
		}
	})

	t.Run("wildcard keeps all", func(t *testing.T) {
		cloned := parent.Clone()
		cloned.Filter([]string{"*"})

		available := toolNames(cloned.List())
		if len(available) != 4 {
			t.Errorf("wildcard should keep all 4 tools, got %d: %v", len(available), available)
		}
	})

	t.Run("empty keeps all", func(t *testing.T) {
		cloned := parent.Clone()
		// No Filter call = all tools remain

		available := toolNames(cloned.List())
		if len(available) != 4 {
			t.Errorf("no filter should keep all 4 tools, got %d: %v", len(available), available)
		}
	})

	t.Run("remove delegate at depth limit", func(t *testing.T) {
		cloned := parent.Clone()
		cloned.Filter([]string{"*"})
		cloned.Remove("delegate")

		available := toolNames(cloned.List())
		nameSet := toSet(available)
		if nameSet["delegate"] {
			t.Error("delegate should be removed at depth limit")
		}
		if len(available) != 3 {
			t.Errorf("expected 3 tools after removing delegate, got %d", len(available))
		}
	})
}

// ---- Helpers ----

type dummyTool struct {
	name string
}

func (d *dummyTool) Name() string               { return d.name }
func (d *dummyTool) Description() string        { return "dummy " + d.name }
func (d *dummyTool) Parameters() map[string]any { return map[string]any{} }
func (d *dummyTool) Execute(_ context.Context, _ map[string]any) (tools.ToolResult, error) {
	return tools.NewSuccessResult("ok"), nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

func toolNames(toolList []tools.Tool) []string {
	names := make([]string, len(toolList))
	for i, t := range toolList {
		names[i] = t.Name()
	}
	return names
}
