package provider

import (
	"context"
	"testing"
)

// mockProvider is a mock provider for testing.
type mockProvider struct {
	name   string
	models []string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Models() []string {
	return m.models
}

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "mock response"}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	ch := make(chan ChatEvent, 1)
	ch <- ChatEvent{Type: EventTypeDone}
	close(ch)
	return ch, nil
}

func TestRegisterAndGet(t *testing.T) {
	Reset() // Clear any existing providers

	p1 := &mockProvider{name: "provider1", models: []string{"model1"}}
	p2 := &mockProvider{name: "provider2", models: []string{"model2"}}

	Register(p1)
	Register(p2)

	t.Run("Get existing provider", func(t *testing.T) {
		got, ok := Get("provider1")
		if !ok {
			t.Error("Get returned false for registered provider")
		}
		if got.Name() != "provider1" {
			t.Errorf("Name() = %s, want provider1", got.Name())
		}
	})

	t.Run("Get non-existing provider", func(t *testing.T) {
		_, ok := Get("nonexistent")
		if ok {
			t.Error("Get returned true for non-registered provider")
		}
	})
}

func TestDefault(t *testing.T) {
	Reset()

	t.Run("Default is nil when no providers registered", func(t *testing.T) {
		if Default() != nil {
			t.Error("Default should be nil when no providers registered")
		}
	})

	t.Run("First registered becomes default", func(t *testing.T) {
		p1 := &mockProvider{name: "first", models: []string{"m1"}}
		p2 := &mockProvider{name: "second", models: []string{"m2"}}

		Register(p1)
		Register(p2)

		if Default().Name() != "first" {
			t.Errorf("Default().Name() = %s, want first", Default().Name())
		}
	})
}

func TestSetDefault(t *testing.T) {
	Reset()

	p1 := &mockProvider{name: "p1", models: []string{"m1"}}
	p2 := &mockProvider{name: "p2", models: []string{"m2"}}

	Register(p1)
	Register(p2)

	t.Run("SetDefault to existing provider", func(t *testing.T) {
		ok := SetDefault("p2")
		if !ok {
			t.Error("SetDefault returned false for registered provider")
		}
		if Default().Name() != "p2" {
			t.Errorf("Default().Name() = %s, want p2", Default().Name())
		}
	})

	t.Run("SetDefault to non-existing provider", func(t *testing.T) {
		ok := SetDefault("nonexistent")
		if ok {
			t.Error("SetDefault returned true for non-registered provider")
		}
	})
}

func TestList(t *testing.T) {
	Reset()

	p1 := &mockProvider{name: "alpha", models: []string{"m1"}}
	p2 := &mockProvider{name: "beta", models: []string{"m2"}}

	Register(p1)
	Register(p2)

	names := List()

	if len(names) != 2 {
		t.Errorf("List() returned %d names, want 2", len(names))
	}

	// List should be sorted
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("List() = %v, want [alpha beta]", names)
	}
}

func TestReset(t *testing.T) {
	p := &mockProvider{name: "test", models: []string{"m"}}
	Register(p)

	Reset()

	if len(List()) != 0 {
		t.Error("List() should be empty after Reset")
	}

	if Default() != nil {
		t.Error("Default() should be nil after Reset")
	}
}
