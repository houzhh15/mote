package tools

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	t.Run("NewRegistry", func(t *testing.T) {
		r := NewRegistry()
		if r == nil {
			t.Fatal("expected non-nil registry")
		}
		if r.Len() != 0 {
			t.Errorf("expected empty registry, got %d tools", r.Len())
		}
	})

	t.Run("Register and Get", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{
			name:        "test",
			description: "A test tool",
			params:      map[string]any{"type": "object"},
			execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
				return NewSuccessResult("ok"), nil
			},
		}

		err := r.Register(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, ok := r.Get("test")
		if !ok {
			t.Fatal("expected tool to be found")
		}
		if got.Name() != "test" {
			t.Errorf("expected name 'test', got %q", got.Name())
		}

		_, ok = r.Get("nonexistent")
		if ok {
			t.Error("expected nonexistent tool to not be found")
		}
	})

	t.Run("Register duplicate", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{name: "dup", description: "First", params: map[string]any{}}
		tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		}

		if err := r.Register(tool); err != nil {
			t.Fatalf("first register failed: %v", err)
		}

		tool2 := &mockTool{name: "dup", description: "Second", params: map[string]any{}}
		tool2.execFn = tool.execFn
		err := r.Register(tool2)
		if !errors.Is(err, ErrToolAlreadyExists) {
			t.Errorf("expected ErrToolAlreadyExists, got %v", err)
		}
	})

	t.Run("Register nil tool", func(t *testing.T) {
		r := NewRegistry()
		err := r.Register(nil)
		if !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("expected ErrInvalidArgs, got %v", err)
		}
	})

	t.Run("Register empty name", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{name: "", description: "No name", params: map[string]any{}}
		tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		}
		err := r.Register(tool)
		if !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("expected ErrInvalidArgs, got %v", err)
		}
	})

	t.Run("MustRegister", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{name: "must", description: "Must register", params: map[string]any{}}
		tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		}

		r.MustRegister(tool)
		if r.Len() != 1 {
			t.Error("expected tool to be registered")
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on duplicate registration")
			}
		}()
		r.MustRegister(tool)
	})

	t.Run("List", func(t *testing.T) {
		r := NewRegistry()
		for i := 0; i < 3; i++ {
			name := string(rune('a' + i))
			tool := &mockTool{name: name, description: "", params: map[string]any{}}
			tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
				return NewSuccessResult(""), nil
			}
			r.MustRegister(tool)
		}

		list := r.List()
		if len(list) != 3 {
			t.Errorf("expected 3 tools, got %d", len(list))
		}
	})

	t.Run("Names", func(t *testing.T) {
		r := NewRegistry()
		for _, name := range []string{"alpha", "beta", "gamma"} {
			tool := &mockTool{name: name, description: "", params: map[string]any{}}
			tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
				return NewSuccessResult(""), nil
			}
			r.MustRegister(tool)
		}

		names := r.Names()
		if len(names) != 3 {
			t.Errorf("expected 3 names, got %d", len(names))
		}

		nameSet := make(map[string]bool)
		for _, n := range names {
			nameSet[n] = true
		}
		for _, expected := range []string{"alpha", "beta", "gamma"} {
			if !nameSet[expected] {
				t.Errorf("expected name %q in list", expected)
			}
		}
	})

	t.Run("Execute", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{
			name:        "echo",
			description: "Echo tool",
			params:      map[string]any{"type": "object"},
			execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
				msg, _ := args["message"].(string)
				return NewSuccessResult(msg), nil
			},
		}
		r.MustRegister(tool)

		result, err := r.Execute(context.Background(), "echo", map[string]any{"message": "hello"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "hello" {
			t.Errorf("expected 'hello', got %q", result.Content)
		}

		_, err = r.Execute(context.Background(), "nonexistent", nil)
		if !errors.Is(err, ErrToolNotFound) {
			t.Errorf("expected ErrToolNotFound, got %v", err)
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{name: "temp", description: "", params: map[string]any{}}
		tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		}
		r.MustRegister(tool)

		err := r.Unregister("temp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := r.Get("temp"); ok {
			t.Error("expected tool to be unregistered")
		}

		err = r.Unregister("nonexistent")
		if !errors.Is(err, ErrToolNotFound) {
			t.Errorf("expected ErrToolNotFound, got %v", err)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		r := NewRegistry()
		for i := 0; i < 5; i++ {
			name := string(rune('a' + i))
			tool := &mockTool{name: name, description: "", params: map[string]any{}}
			tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
				return NewSuccessResult(""), nil
			}
			r.MustRegister(tool)
		}

		r.Clear()
		if r.Len() != 0 {
			t.Errorf("expected empty registry after clear, got %d", r.Len())
		}
	})

	t.Run("Clone", func(t *testing.T) {
		r := NewRegistry()
		tool := &mockTool{name: "original", description: "", params: map[string]any{}}
		tool.execFn = func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		}
		r.MustRegister(tool)

		clone := r.Clone()
		if clone.Len() != r.Len() {
			t.Error("clone should have same length")
		}

		newTool := &mockTool{name: "new", description: "", params: map[string]any{}}
		newTool.execFn = tool.execFn
		clone.MustRegister(newTool)

		if r.Len() != 1 {
			t.Error("original should not be affected by clone modifications")
		}
		if clone.Len() != 2 {
			t.Error("clone should have new tool")
		}
	})
}

func TestToProviderTools(t *testing.T) {
	r := NewRegistry()

	tool := &mockTool{
		name:        "read_file",
		description: "Read a file from disk",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path",
				},
			},
			"required": []string{"path"},
		},
		execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult("file contents"), nil
		},
	}
	r.MustRegister(tool)

	providerTools, err := r.ToProviderTools()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(providerTools) != 1 {
		t.Fatalf("expected 1 provider tool, got %d", len(providerTools))
	}

	pt := providerTools[0]
	if pt.Type != "function" {
		t.Errorf("expected type 'function', got %q", pt.Type)
	}
	if pt.Function.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", pt.Function.Name)
	}
	if pt.Function.Description != "Read a file from disk" {
		t.Errorf("expected description mismatch")
	}
	if len(pt.Function.Parameters) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestRegistryConcurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := string(rune('a' + (idx % 26)))
			tool := &mockTool{
				name:        name,
				description: "",
				params:      map[string]any{},
				execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
					return NewSuccessResult(""), nil
				},
			}
			_ = r.Register(tool)
		}(i)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := string(rune('a' + (idx % 26)))
			_, _ = r.Get(name)
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_ = r.Names()
			_ = r.Len()
		}()
	}

	wg.Wait()
}

func TestRegistry_Filter(t *testing.T) {
	newReg := func() *Registry {
		r := NewRegistry()
		for _, name := range []string{"a", "b", "c"} {
			r.MustRegister(&mockTool{
				name: name, description: name, params: map[string]any{},
				execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
					return NewSuccessResult(""), nil
				},
			})
		}
		return r
	}

	t.Run("allowlist filters to subset", func(t *testing.T) {
		r := newReg()
		r.Filter([]string{"a", "b"})
		if r.Len() != 2 {
			t.Fatalf("expected 2 tools, got %d", r.Len())
		}
		if _, ok := r.Get("c"); ok {
			t.Error("tool 'c' should have been filtered out")
		}
	})

	t.Run("wildcard keeps all", func(t *testing.T) {
		r := newReg()
		r.Filter([]string{"*"})
		if r.Len() != 3 {
			t.Fatalf("expected 3 tools, got %d", r.Len())
		}
	})

	t.Run("empty allowlist removes all", func(t *testing.T) {
		r := newReg()
		r.Filter([]string{})
		if r.Len() != 0 {
			t.Fatalf("expected 0 tools, got %d", r.Len())
		}
	})

	t.Run("exclusion pattern", func(t *testing.T) {
		r := newReg()
		r.Filter([]string{"*", "!b"})
		if r.Len() != 2 {
			t.Fatalf("expected 2 tools, got %d", r.Len())
		}
		if _, ok := r.Get("b"); ok {
			t.Error("tool 'b' should have been excluded")
		}
	})

	t.Run("glob pattern", func(t *testing.T) {
		r := NewRegistry()
		for _, name := range []string{"mcp_server1_tool1", "mcp_server1_tool2", "mcp_server2_tool1", "read_file"} {
			r.MustRegister(&mockTool{
				name: name, description: name, params: map[string]any{},
				execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
					return NewSuccessResult(""), nil
				},
			})
		}
		r.Filter([]string{"mcp_server1_*", "read_file"})
		if r.Len() != 3 {
			t.Fatalf("expected 3 tools, got %d", r.Len())
		}
		if _, ok := r.Get("mcp_server2_tool1"); ok {
			t.Error("mcp_server2_tool1 should have been filtered out")
		}
	})

	t.Run("glob with exclusion", func(t *testing.T) {
		r := NewRegistry()
		for _, name := range []string{"mcp_s1_a", "mcp_s1_b", "mcp_s1_c", "other"} {
			r.MustRegister(&mockTool{
				name: name, description: name, params: map[string]any{},
				execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
					return NewSuccessResult(""), nil
				},
			})
		}
		r.Filter([]string{"mcp_s1_*", "!mcp_s1_c"})
		if r.Len() != 2 {
			t.Fatalf("expected 2 tools, got %d", r.Len())
		}
		if _, ok := r.Get("mcp_s1_c"); ok {
			t.Error("mcp_s1_c should have been excluded")
		}
		if _, ok := r.Get("other"); ok {
			t.Error("other should have been filtered out")
		}
	})
}

func TestRegistry_Remove(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&mockTool{
		name: "x", description: "x", params: map[string]any{},
		execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			return NewSuccessResult(""), nil
		},
	})
	if r.Len() != 1 {
		t.Fatal("expected 1 tool")
	}
	r.Remove("x")
	if r.Len() != 0 {
		t.Fatal("expected 0 tools after Remove")
	}
	// Remove non-existent should not panic
	r.Remove("nonexistent")
}

func TestRegistry_SetAgentID(t *testing.T) {
	r := NewRegistry()
	if r.GetAgentID() != "" {
		t.Error("expected empty agentID initially")
	}
	r.SetAgentID("agent-1")
	if r.GetAgentID() != "agent-1" {
		t.Errorf("expected 'agent-1', got %q", r.GetAgentID())
	}

	// Clone should inherit agentID
	clone := r.Clone()
	if clone.GetAgentID() != "agent-1" {
		t.Errorf("clone expected 'agent-1', got %q", clone.GetAgentID())
	}

	// Changing clone agentID should not affect original
	clone.SetAgentID("agent-2")
	if r.GetAgentID() != "agent-1" {
		t.Error("original agentID should not change")
	}
}
