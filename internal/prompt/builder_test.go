package prompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mote/internal/tools"
)

type mockTool struct {
	name        string
	description string
	params      map[string]any
}

func (m *mockTool) Name() string               { return m.name }
func (m *mockTool) Description() string        { return m.description }
func (m *mockTool) Parameters() map[string]any { return m.params }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

type mockMemorySearcher struct {
	results []MemoryResult
	err     error
}

func (m *mockMemorySearcher) Search(ctx context.Context, query string, topK int) ([]MemoryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestSystemPromptBuilder_Build(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&mockTool{
		name:        "read_file",
		description: "Read a file from disk",
		params:      map[string]any{"path": "string"},
	})

	config := PromptConfig{
		AgentName:    "TestAgent",
		Timezone:     "Asia/Shanghai",
		WorkspaceDir: "/tmp/test",
		Constraints:  []string{"Be helpful", "Be concise"},
	}

	t.Run("basic build", func(t *testing.T) {
		b := NewSystemPromptBuilder(config, registry)
		result, err := b.Build(context.Background(), "test query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "TestAgent") {
			t.Error("expected agent name in result")
		}
		if !strings.Contains(result, "read_file") {
			t.Error("expected tool name in result")
		}
		if !strings.Contains(result, "Be helpful") {
			t.Error("expected constraint in result")
		}
	})

	t.Run("with memory", func(t *testing.T) {
		mem := &mockMemorySearcher{
			results: []MemoryResult{
				{Content: "Previous conversation about Go", Score: 0.9},
			},
		}
		b := NewSystemPromptBuilder(config, registry).WithMemory(mem)
		result, err := b.Build(context.Background(), "tell me about Go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "Previous conversation about Go") {
			t.Error("expected memory content in result")
		}
	})

	t.Run("memory error is ignored", func(t *testing.T) {
		mem := &mockMemorySearcher{
			err: errors.New("search failed"),
		}
		b := NewSystemPromptBuilder(config, registry).WithMemory(mem)
		result, err := b.Build(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "TestAgent") {
			t.Error("expected agent name even with memory error")
		}
	})
}

func TestSystemPromptBuilder_BuildStatic(t *testing.T) {
	config := PromptConfig{
		AgentName: "StaticAgent",
	}
	b := NewSystemPromptBuilder(config, nil)
	result := b.BuildStatic()
	if !strings.Contains(result, "StaticAgent") {
		t.Error("expected agent name in static result")
	}
}

func TestSystemPromptBuilder_DefaultConfig(t *testing.T) {
	config := PromptConfig{}
	b := NewSystemPromptBuilder(config, nil)
	result := b.BuildStatic()
	if !strings.Contains(result, "Mote") {
		t.Error("expected default agent name 'Mote'")
	}
	if !strings.Contains(result, "UTC") {
		t.Error("expected default timezone 'UTC'")
	}
}

func TestDefaultPromptConfig(t *testing.T) {
	config := DefaultPromptConfig()
	if config.AgentName != "Mote" {
		t.Errorf("expected AgentName 'Mote', got %s", config.AgentName)
	}
	if config.Timezone != "UTC" {
		t.Errorf("expected Timezone 'UTC', got %s", config.Timezone)
	}
}

func TestSystemPromptBuilder_MCPInjectionMode(t *testing.T) {
	config := PromptConfig{
		AgentName: "TestAgent",
	}
	b := NewSystemPromptBuilder(config, nil)

	// Default should be Full (optimistic - ready for MCP if configured)
	if b.GetMCPInjectionMode() != MCPInjectionFull {
		t.Error("expected default MCP injection mode to be Full")
	}

	// Set to Summary
	b.SetMCPInjectionMode(MCPInjectionSummary)
	if b.GetMCPInjectionMode() != MCPInjectionSummary {
		t.Error("expected MCP injection mode to be Summary")
	}

	// Set to None
	b.SetMCPInjectionMode(MCPInjectionNone)
	if b.GetMCPInjectionMode() != MCPInjectionNone {
		t.Error("expected MCP injection mode to be None")
	}

	// Set back to Full
	b.SetMCPInjectionMode(MCPInjectionFull)
	if b.GetMCPInjectionMode() != MCPInjectionFull {
		t.Error("expected MCP injection mode to be Full")
	}
}

func TestMCPInjectionMode_String(t *testing.T) {
	tests := []struct {
		mode     MCPInjectionMode
		expected string
	}{
		{MCPInjectionNone, "none"},
		{MCPInjectionSummary, "summary"},
		{MCPInjectionFull, "full"},
		{MCPInjectionMode(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.mode.String() != tc.expected {
			t.Errorf("MCPInjectionMode(%d).String() = %q, expected %q",
				tc.mode, tc.mode.String(), tc.expected)
		}
	}
}
