package tools

import (
	"context"
	"sync"
	"testing"
)

// factoryMockTool is a simple tool implementation for factory testing.
type factoryMockTool struct {
	name   string
	config map[string]any
}

func (t *factoryMockTool) Name() string               { return t.name }
func (t *factoryMockTool) Description() string        { return "factory mock tool" }
func (t *factoryMockTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (t *factoryMockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return NewSuccessResult("mock result"), nil
}

// testFactory creates a factoryMockTool from config.
func testFactory(config map[string]any) (Tool, error) {
	name := "mock"
	if n, ok := config["name"].(string); ok {
		name = n
	}
	return &factoryMockTool{name: name, config: config}, nil
}

func TestFactoryRegistry_Register(t *testing.T) {
	r := NewFactoryRegistry()

	// Register valid factory
	err := r.Register("mock", testFactory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it exists
	if !r.Has("mock") {
		t.Error("expected factory to be registered")
	}
}

func TestFactoryRegistry_Register_EmptyName(t *testing.T) {
	r := NewFactoryRegistry()

	err := r.Register("", testFactory)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestFactoryRegistry_Register_NilFactory(t *testing.T) {
	r := NewFactoryRegistry()

	err := r.Register("nil-factory", nil)
	if err == nil {
		t.Error("expected error for nil factory")
	}
}

func TestFactoryRegistry_Register_Duplicate(t *testing.T) {
	r := NewFactoryRegistry()

	// First registration succeeds
	err := r.Register("mock", testFactory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duplicate registration fails
	err = r.Register("mock", testFactory)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestFactoryRegistry_MustRegister_Panic(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("mock", testFactory)

	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate registration")
		}
	}()

	r.MustRegister("mock", testFactory) // should panic
}

func TestFactoryRegistry_Create(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("mock", testFactory)

	// Create with config
	config := map[string]any{"name": "custom-mock"}
	tool, err := r.Create("mock", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	if tool.Name() != "custom-mock" {
		t.Errorf("expected name 'custom-mock', got '%s'", tool.Name())
	}
}

func TestFactoryRegistry_Create_NilConfig(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("mock", testFactory)

	// Create with nil config should work
	tool, err := r.Create("mock", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
}

func TestFactoryRegistry_Create_NotFound(t *testing.T) {
	r := NewFactoryRegistry()

	_, err := r.Create("nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent factory")
	}
}

func TestFactoryRegistry_List(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("beta", testFactory)
	r.MustRegister("alpha", testFactory)
	r.MustRegister("gamma", testFactory)

	names := r.List()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	// Verify sorted order
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Errorf("expected sorted order [alpha, beta, gamma], got %v", names)
	}
}

func TestFactoryRegistry_Unregister(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("mock", testFactory)

	// Unregister
	err := r.Unregister("mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify removed
	if r.Has("mock") {
		t.Error("expected factory to be unregistered")
	}

	// Unregister again should fail
	err = r.Unregister("mock")
	if err == nil {
		t.Error("expected error for unregistering nonexistent factory")
	}
}

func TestFactoryRegistry_Len(t *testing.T) {
	r := NewFactoryRegistry()
	if r.Len() != 0 {
		t.Errorf("expected 0, got %d", r.Len())
	}

	r.MustRegister("mock1", testFactory)
	r.MustRegister("mock2", testFactory)

	if r.Len() != 2 {
		t.Errorf("expected 2, got %d", r.Len())
	}
}

func TestFactoryRegistry_Clear(t *testing.T) {
	r := NewFactoryRegistry()
	r.MustRegister("mock1", testFactory)
	r.MustRegister("mock2", testFactory)

	r.Clear()

	if r.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", r.Len())
	}
}

func TestFactoryRegistry_Concurrent(t *testing.T) {
	r := NewFactoryRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := string(rune('a' + id))
			r.Register(name, testFactory)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.List()
				r.Has("a")
				r.Get("a")
			}
		}()
	}

	wg.Wait()

	// Verify consistency
	if r.Len() == 0 {
		t.Error("expected some factories after concurrent writes")
	}
}

func TestFactoryRegistry_FactoryError(t *testing.T) {
	r := NewFactoryRegistry()

	// Factory that returns an error
	errorFactory := func(config map[string]any) (Tool, error) {
		return nil, NewInvalidArgsError("test", "intentional error", nil)
	}
	r.MustRegister("error-factory", errorFactory)

	_, err := r.Create("error-factory", nil)
	if err == nil {
		t.Error("expected error from factory")
	}
}
