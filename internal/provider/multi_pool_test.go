package provider

import (
	"context"
	"testing"
)

// mockProviderForMultiPool implements Provider for testing MultiProviderPool
type mockProviderForMultiPool struct {
	name  string
	model string
}

func (m *mockProviderForMultiPool) Name() string     { return m.name }
func (m *mockProviderForMultiPool) Models() []string { return []string{m.model} }
func (m *mockProviderForMultiPool) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "mock response"}, nil
}
func (m *mockProviderForMultiPool) Stream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	ch := make(chan ChatEvent)
	close(ch)
	return ch, nil
}

func TestNewMultiProviderPool(t *testing.T) {
	pool := NewMultiProviderPool()
	if pool == nil {
		t.Fatal("NewMultiProviderPool returned nil")
	}
	if pool.Count() != 0 {
		t.Errorf("Expected 0 providers, got %d", pool.Count())
	}
}

func TestMultiProviderPool_AddProvider(t *testing.T) {
	pool := NewMultiProviderPool()

	// Create a mock pool
	mockPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "copilot", model: model}, nil
	})

	err := pool.AddProvider("copilot", mockPool, []string{"gpt-4o", "claude-sonnet-4"})
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	if !pool.HasProvider("copilot") {
		t.Error("Expected copilot provider to be registered")
	}

	// Adding same provider again should fail
	err = pool.AddProvider("copilot", mockPool, []string{})
	if err == nil {
		t.Error("Expected error when adding duplicate provider")
	}
}

func TestMultiProviderPool_AddProvider_OllamaPrefix(t *testing.T) {
	pool := NewMultiProviderPool()

	ollamaPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "ollama", model: model}, nil
	})

	err := pool.AddProvider("ollama", ollamaPool, []string{"llama3.2", "mistral"})
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	// Check that models have ollama: prefix
	models := pool.ListAllModels()
	for _, m := range models {
		if m.Provider == "ollama" {
			if m.ID != "ollama:"+m.OriginalID {
				t.Errorf("Expected model ID with ollama: prefix, got %s", m.ID)
			}
		}
	}
}

func TestMultiProviderPool_GetProvider(t *testing.T) {
	pool := NewMultiProviderPool()

	copilotPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "copilot", model: model}, nil
	})
	ollamaPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "ollama", model: model}, nil
	})

	_ = pool.AddProvider("copilot", copilotPool, []string{"gpt-4o"})
	_ = pool.AddProvider("ollama", ollamaPool, []string{"llama3.2"})

	// Test getting Copilot model (no prefix)
	provider, providerName, err := pool.GetProvider("gpt-4o")
	if err != nil {
		t.Fatalf("GetProvider(gpt-4o) failed: %v", err)
	}
	if providerName != "copilot" {
		t.Errorf("Expected provider name 'copilot', got '%s'", providerName)
	}
	if provider == nil {
		t.Error("Expected non-nil provider")
	}

	// Test getting Ollama model (with prefix)
	_, providerName, err = pool.GetProvider("ollama:llama3.2")
	if err != nil {
		t.Fatalf("GetProvider(ollama:llama3.2) failed: %v", err)
	}
	if providerName != "ollama" {
		t.Errorf("Expected provider name 'ollama', got '%s'", providerName)
	}
}

func TestMultiProviderPool_ListAllModels(t *testing.T) {
	pool := NewMultiProviderPool()

	copilotPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "copilot", model: model}, nil
	})
	ollamaPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "ollama", model: model}, nil
	})

	_ = pool.AddProvider("copilot", copilotPool, []string{"gpt-4o", "claude-sonnet-4"})
	_ = pool.AddProvider("ollama", ollamaPool, []string{"llama3.2"})

	models := pool.ListAllModels()
	if len(models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(models))
	}

	// Verify sorting (copilot should come first)
	if len(models) > 0 && models[0].Provider != "copilot" {
		t.Error("Expected models to be sorted by provider (copilot first)")
	}
}

func TestMultiProviderPool_SetGetDefault(t *testing.T) {
	pool := NewMultiProviderPool()

	pool.SetDefault("chat", "gpt-4o")
	pool.SetDefault("cron", "gpt-4o-mini")

	if got := pool.GetDefault("chat"); got != "gpt-4o" {
		t.Errorf("GetDefault(chat) = %s, want gpt-4o", got)
	}
	if got := pool.GetDefault("cron"); got != "gpt-4o-mini" {
		t.Errorf("GetDefault(cron) = %s, want gpt-4o-mini", got)
	}
	if got := pool.GetDefault("unknown"); got != "" {
		t.Errorf("GetDefault(unknown) = %s, want empty", got)
	}
}

func TestMultiProviderPool_RefreshModels(t *testing.T) {
	pool := NewMultiProviderPool()

	ollamaPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "ollama", model: model}, nil
	})

	_ = pool.AddProvider("ollama", ollamaPool, []string{"llama3.2"})

	// Verify initial count
	if count := pool.ModelCountByProvider("ollama"); count != 1 {
		t.Errorf("Expected 1 ollama model, got %d", count)
	}

	// Refresh with new models
	err := pool.RefreshModels("ollama", []string{"llama3.2", "mistral", "codellama"})
	if err != nil {
		t.Fatalf("RefreshModels failed: %v", err)
	}

	// Verify new count
	if count := pool.ModelCountByProvider("ollama"); count != 3 {
		t.Errorf("Expected 3 ollama models after refresh, got %d", count)
	}

	// Refresh unregistered provider should fail
	err = pool.RefreshModels("unknown", []string{})
	if err == nil {
		t.Error("Expected error when refreshing unregistered provider")
	}
}

func TestMultiProviderPool_GetOrDefault(t *testing.T) {
	pool := NewMultiProviderPool()

	copilotPool := NewPool(func(model string) (Provider, error) {
		return &mockProviderForMultiPool{name: "copilot", model: model}, nil
	})

	_ = pool.AddProvider("copilot", copilotPool, []string{"gpt-4o"})
	pool.SetDefault("chat", "gpt-4o")

	// Test with explicit model
	_, providerName, err := pool.GetOrDefault("gpt-4o", "chat")
	if err != nil {
		t.Fatalf("GetOrDefault with model failed: %v", err)
	}
	if providerName != "copilot" {
		t.Errorf("Expected provider name 'copilot', got '%s'", providerName)
	}

	// Test with empty model (should use default)
	_, providerName, err = pool.GetOrDefault("", "chat")
	if err != nil {
		t.Fatalf("GetOrDefault with default failed: %v", err)
	}
	if providerName != "copilot" {
		t.Errorf("Expected provider name 'copilot', got '%s'", providerName)
	}

	// Test with empty model and no default
	_, _, err = pool.GetOrDefault("", "unknown")
	if err == nil {
		t.Error("Expected error when no default configured")
	}
}
