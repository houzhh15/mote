package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseManifestBytes_ValidManifest(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"description": "A test skill",
		"author": "Test Author",
		"tools": [
			{
				"name": "test_tool",
				"description": "A test tool",
				"handler": "tools.js#testHandler"
			}
		],
		"prompts": [
			{
				"name": "test_prompt",
				"content": "This is a test prompt"
			}
		],
		"dependencies": ["other-skill"]
	}`

	skill, err := ParseManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.ID != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.ID)
	}
	if skill.Name != "Test Skill" {
		t.Errorf("expected Name 'Test Skill', got '%s'", skill.Name)
	}
	if skill.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got '%s'", skill.Version)
	}
	if len(skill.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(skill.Tools))
	}
	if len(skill.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(skill.Prompts))
	}
	if len(skill.Dependencies) != 1 {
		t.Errorf("expected 1 dependency, got %d", len(skill.Dependencies))
	}
}

func TestParseManifestBytes_MissingID(t *testing.T) {
	manifest := `{
		"name": "Test Skill",
		"version": "1.0.0"
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestParseManifestBytes_MissingName(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"version": "1.0.0"
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseManifestBytes_MissingVersion(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill"
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestParseManifestBytes_InvalidID(t *testing.T) {
	manifest := `{
		"id": "Test_Skill",
		"name": "Test Skill",
		"version": "1.0.0"
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for invalid ID format")
	}
}

func TestParseManifestBytes_InvalidVersion(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "v1.0"
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for invalid version format")
	}
}

func TestParseManifestBytes_ToolMissingName(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"handler": "tools.js#testHandler"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for tool missing name")
	}
}

func TestParseManifestBytes_ToolMissingHandler(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for tool missing handler")
	}
}

func TestParseManifestBytes_InvalidHandlerFormat(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"handler": "invalid_handler"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for invalid handler format")
	}
}

func TestParseManifestBytes_ValidHandlerWithSubdirectory(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"handler": "scripts/tools.js#run"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected error for valid subdirectory handler: %v", err)
	}
}

func TestParseManifestBytes_HandlerPathTraversalRejected(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"handler": "../scripts/tools.js#run"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for handler path traversal")
	}
}

func TestParseManifestBytes_PromptMissingName(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"prompts": [
			{
				"content": "Some content"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for prompt missing name")
	}
}

func TestParseManifestBytes_PromptMissingFileAndContent(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"prompts": [
			{
				"name": "test_prompt"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for prompt missing file and content")
	}
}

func TestParseManifestBytes_InvalidJSON(t *testing.T) {
	manifest := `{invalid json}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseManifestBytes_DefaultToolTimeout(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"handler": "tools.js#testHandler"
			}
		]
	}`

	skill, err := ParseManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Tools[0].Timeout.Duration != DefaultToolTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultToolTimeout, skill.Tools[0].Timeout.Duration)
	}
}

func TestParseManifestBytes_CustomToolTimeout(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"handler": "tools.js#testHandler",
				"timeout": "60s"
			}
		]
	}`

	skill, err := ParseManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := 60 * time.Second
	if skill.Tools[0].Timeout.Duration != expected {
		t.Errorf("expected timeout %v, got %v", expected, skill.Tools[0].Timeout.Duration)
	}
}

func TestParseManifest_File(t *testing.T) {
	// Create temp directory and manifest file
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	manifest := map[string]any{
		"id":      "test-skill",
		"name":    "Test Skill",
		"version": "1.0.0",
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	skill, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.ID != "test-skill" {
		t.Errorf("expected ID 'test-skill', got '%s'", skill.ID)
	}
}

func TestParseManifest_FileNotFound(t *testing.T) {
	_, err := ParseManifest("/nonexistent/path/manifest.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseHandler(t *testing.T) {
	tests := []struct {
		handler      string
		wantFilename string
		wantFunc     string
		wantErr      bool
	}{
		{"tools.js#myFunc", "tools.js", "myFunc", false},
		{"handlers.js#handleRequest", "handlers.js", "handleRequest", false},
		{"invalid_handler", "", "", true},
		{"no-hash", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.handler, func(t *testing.T) {
			filename, funcName, err := ParseHandler(tt.handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHandler(%q) error = %v, wantErr %v", tt.handler, err, tt.wantErr)
				return
			}
			if filename != tt.wantFilename {
				t.Errorf("ParseHandler(%q) filename = %v, want %v", tt.handler, filename, tt.wantFilename)
			}
			if funcName != tt.wantFunc {
				t.Errorf("ParseHandler(%q) funcName = %v, want %v", tt.handler, funcName, tt.wantFunc)
			}
		})
	}
}

func TestResolveHandlerPath(t *testing.T) {
	skillDir := "/path/to/skill"
	handler := "tools.js#myFunc"

	path, err := ResolveHandlerPath(skillDir, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/path/to/skill/tools.js"
	if path != expected {
		t.Errorf("expected path %q, got %q", expected, path)
	}
}

func TestParseManifestBytes_ValidHooks(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"hooks": [
			{
				"type": "before_message",
				"handler": "hooks.js#onBeforeMessage",
				"priority": 100,
				"description": "Log before message"
			}
		]
	}`

	skill, err := ParseManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skill.Hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(skill.Hooks))
	}
	if skill.Hooks[0].Type != "before_message" {
		t.Errorf("expected hook type 'before_message', got '%s'", skill.Hooks[0].Type)
	}
	if skill.Hooks[0].Priority != 100 {
		t.Errorf("expected priority 100, got %d", skill.Hooks[0].Priority)
	}
}

func TestParseManifestBytes_HookMissingType(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"hooks": [
			{
				"handler": "hooks.js#onBeforeMessage"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for hook missing type")
	}
}

func TestParseManifestBytes_HookMissingHandler(t *testing.T) {
	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"hooks": [
			{
				"type": "before_message"
			}
		]
	}`

	_, err := ParseManifestBytes([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for hook missing handler")
	}
}
