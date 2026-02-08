package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestToolResult(t *testing.T) {
	t.Run("NewSuccessResult", func(t *testing.T) {
		result := NewSuccessResult("hello world")
		if result.Content != "hello world" {
			t.Errorf("expected content 'hello world', got %q", result.Content)
		}
		if result.IsError {
			t.Error("expected IsError to be false")
		}
		if result.Metadata != nil {
			t.Error("expected Metadata to be nil")
		}
	})

	t.Run("NewErrorResult", func(t *testing.T) {
		result := NewErrorResult("something went wrong")
		if result.Content != "something went wrong" {
			t.Errorf("expected content 'something went wrong', got %q", result.Content)
		}
		if !result.IsError {
			t.Error("expected IsError to be true")
		}
	})

	t.Run("NewResultWithMetadata", func(t *testing.T) {
		meta := map[string]any{"lines": 100, "bytes": 2048}
		result := NewResultWithMetadata("file read", meta)
		if result.Content != "file read" {
			t.Errorf("expected content 'file read', got %q", result.Content)
		}
		if result.IsError {
			t.Error("expected IsError to be false")
		}
		if result.Metadata["lines"] != 100 {
			t.Errorf("expected metadata lines=100, got %v", result.Metadata["lines"])
		}
	})

	t.Run("String", func(t *testing.T) {
		success := NewSuccessResult("ok")
		if success.String() != "ok" {
			t.Errorf("expected 'ok', got %q", success.String())
		}

		errResult := NewErrorResult("failed")
		expected := "[error] failed"
		if errResult.String() != expected {
			t.Errorf("expected %q, got %q", expected, errResult.String())
		}
	})

	t.Run("JSON serialization", func(t *testing.T) {
		result := NewResultWithMetadata("test", map[string]any{"key": "value"})

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded ToolResult
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Content != result.Content {
			t.Errorf("content mismatch: expected %q, got %q", result.Content, decoded.Content)
		}
		if decoded.IsError != result.IsError {
			t.Errorf("IsError mismatch: expected %v, got %v", result.IsError, decoded.IsError)
		}
	})
}

func TestBaseTool(t *testing.T) {
	bt := &BaseTool{
		ToolName:        "test_tool",
		ToolDescription: "A test tool",
		ToolParameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"arg1": map[string]any{"type": "string"},
			},
		},
	}

	if bt.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", bt.Name())
	}

	if bt.Description() != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", bt.Description())
	}

	params := bt.Parameters()
	if params["type"] != "object" {
		t.Error("expected params type to be 'object'")
	}

	btNil := &BaseTool{ToolName: "nil_params"}
	nilParams := btNil.Parameters()
	if nilParams["type"] != "object" {
		t.Error("expected default params type to be 'object'")
	}
}

func TestErrors(t *testing.T) {
	t.Run("ErrToolNotFound", func(t *testing.T) {
		err := NewToolNotFoundError("missing_tool")
		if !errors.Is(err, ErrToolNotFound) {
			t.Error("expected error to match ErrToolNotFound")
		}
		expected := "tool not found: missing_tool"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("ErrToolAlreadyExists", func(t *testing.T) {
		err := NewToolAlreadyExistsError("dup_tool")
		if !errors.Is(err, ErrToolAlreadyExists) {
			t.Error("expected error to match ErrToolAlreadyExists")
		}
		expected := "tool already exists: dup_tool"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("ErrInvalidArgs", func(t *testing.T) {
		cause := errors.New("parse error")
		err := NewInvalidArgsError("my_tool", "bad format", cause)
		if !errors.Is(err, ErrInvalidArgs) {
			t.Error("expected error to match ErrInvalidArgs")
		}
		if !errors.Is(err, cause) {
			t.Error("expected error to wrap cause")
		}

		errNoCause := NewInvalidArgsError("my_tool", "missing field", nil)
		expected := "invalid arguments for tool my_tool: missing field"
		if errNoCause.Error() != expected {
			t.Errorf("expected %q, got %q", expected, errNoCause.Error())
		}
	})

	t.Run("ErrToolTimeout", func(t *testing.T) {
		err := NewToolTimeoutError("slow_tool", "30s")
		if !errors.Is(err, ErrToolTimeout) {
			t.Error("expected error to match ErrToolTimeout")
		}
		expected := "tool slow_tool execution timed out after 30s"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})
}

type mockTool struct {
	name        string
	description string
	params      map[string]any
	execFn      func(ctx context.Context, args map[string]any) (ToolResult, error)
}

func (m *mockTool) Name() string               { return m.name }
func (m *mockTool) Description() string        { return m.description }
func (m *mockTool) Parameters() map[string]any { return m.params }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return m.execFn(ctx, args)
}

func TestToolInterface(t *testing.T) {
	var _ Tool = (*mockTool)(nil)

	tool := &mockTool{
		name:        "echo",
		description: "Echoes the input",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
		execFn: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			text, _ := args["text"].(string)
			return NewSuccessResult(text), nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello" {
		t.Errorf("expected 'hello', got %q", result.Content)
	}
}
