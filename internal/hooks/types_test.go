package hooks

import (
	"testing"
)

func TestAllHookTypes_Count(t *testing.T) {
	types := AllHookTypes()
	expectedCount := 14 // 10 original + 4 new types

	if len(types) != expectedCount {
		t.Errorf("expected %d hook types, got %d", expectedCount, len(types))
	}
}

func TestAllHookTypes_ContainsOriginal(t *testing.T) {
	types := AllHookTypes()
	expected := []HookType{
		HookBeforeMessage,
		HookAfterMessage,
		HookBeforeToolCall,
		HookAfterToolCall,
		HookSessionCreate,
		HookSessionEnd,
		HookStartup,
		HookShutdown,
		HookBeforeResponse,
		HookAfterResponse,
	}

	for _, exp := range expected {
		found := false
		for _, t := range types {
			if t == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllHookTypes missing original type: %s", exp)
		}
	}
}

func TestAllHookTypes_ContainsNew(t *testing.T) {
	types := AllHookTypes()
	newTypes := []HookType{
		HookBeforeMemoryWrite,
		HookAfterMemoryWrite,
		HookPromptBuild,
		HookOnError,
	}

	for _, exp := range newTypes {
		found := false
		for _, ht := range types {
			if ht == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllHookTypes missing new type: %s", exp)
		}
	}
}

func TestIsValidHookType_Original(t *testing.T) {
	originalTypes := []HookType{
		HookBeforeMessage,
		HookAfterMessage,
		HookBeforeToolCall,
		HookAfterToolCall,
		HookSessionCreate,
		HookSessionEnd,
		HookStartup,
		HookShutdown,
		HookBeforeResponse,
		HookAfterResponse,
	}

	for _, ht := range originalTypes {
		if !IsValidHookType(ht) {
			t.Errorf("IsValidHookType(%s) = false, expected true", ht)
		}
	}
}

func TestIsValidHookType_New(t *testing.T) {
	newTypes := []HookType{
		HookBeforeMemoryWrite,
		HookAfterMemoryWrite,
		HookPromptBuild,
		HookOnError,
	}

	for _, ht := range newTypes {
		if !IsValidHookType(ht) {
			t.Errorf("IsValidHookType(%s) = false, expected true", ht)
		}
	}
}

func TestIsValidHookType_Invalid(t *testing.T) {
	invalidTypes := []HookType{
		"invalid",
		"unknown_hook",
		"before_",
		"",
	}

	for _, ht := range invalidTypes {
		if IsValidHookType(ht) {
			t.Errorf("IsValidHookType(%s) = true, expected false", ht)
		}
	}
}

func TestMemoryContext(t *testing.T) {
	ctx := NewContext(HookBeforeMemoryWrite)
	mem := &MemoryContext{
		Key:       "test-key",
		Content:   "test content",
		Metadata:  map[string]any{"source": "test"},
		Operation: "write",
	}

	ctx.WithMemory(mem)

	if ctx.Memory == nil {
		t.Fatal("expected memory context to be set")
	}
	if ctx.Memory.Key != "test-key" {
		t.Errorf("expected key 'test-key', got '%s'", ctx.Memory.Key)
	}
	if ctx.Memory.Content != "test content" {
		t.Errorf("expected content 'test content', got '%s'", ctx.Memory.Content)
	}
	if ctx.Memory.Operation != "write" {
		t.Errorf("expected operation 'write', got '%s'", ctx.Memory.Operation)
	}
}

func TestPromptContext(t *testing.T) {
	ctx := NewContext(HookPromptBuild)
	prompt := &PromptContext{
		SystemPrompt: "You are an assistant",
		UserPrompt:   "Hello",
		Injections:   []string{"skill1 prompt", "skill2 prompt"},
	}

	ctx.WithPrompt(prompt)

	if ctx.Prompt == nil {
		t.Fatal("expected prompt context to be set")
	}
	if ctx.Prompt.SystemPrompt != "You are an assistant" {
		t.Errorf("expected system prompt, got '%s'", ctx.Prompt.SystemPrompt)
	}
	if len(ctx.Prompt.Injections) != 2 {
		t.Errorf("expected 2 injections, got %d", len(ctx.Prompt.Injections))
	}
}

func TestErrorContext(t *testing.T) {
	ctx := NewContext(HookOnError)
	errCtx := &ErrorContext{
		Code:    "TOOL_ERROR",
		Message: "Tool execution failed",
		Source:  "file_read",
	}

	ctx.WithError(errCtx)

	if ctx.Error == nil {
		t.Fatal("expected error context to be set")
	}
	if ctx.Error.Code != "TOOL_ERROR" {
		t.Errorf("expected code 'TOOL_ERROR', got '%s'", ctx.Error.Code)
	}
	if ctx.Error.Message != "Tool execution failed" {
		t.Errorf("expected message, got '%s'", ctx.Error.Message)
	}
	if ctx.Error.Source != "file_read" {
		t.Errorf("expected source 'file_read', got '%s'", ctx.Error.Source)
	}
}

func TestHandler_ScriptPath(t *testing.T) {
	handler := &Handler{
		ID:         "test-handler",
		Priority:   10,
		Source:     "test-skill",
		ScriptPath: "/path/to/handler.js",
		Async:      true,
		Enabled:    true,
	}

	if handler.ScriptPath != "/path/to/handler.js" {
		t.Errorf("expected script path, got '%s'", handler.ScriptPath)
	}
	if !handler.Async {
		t.Error("expected async to be true")
	}
}

func TestHandler_Async(t *testing.T) {
	handler := &Handler{
		ID:      "sync-handler",
		Async:   false,
		Enabled: true,
	}

	if handler.Async {
		t.Error("expected async to be false")
	}

	asyncHandler := &Handler{
		ID:      "async-handler",
		Async:   true,
		Enabled: true,
	}

	if !asyncHandler.Async {
		t.Error("expected async to be true")
	}
}
