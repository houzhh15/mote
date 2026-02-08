package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mote/internal/hooks"
	"mote/internal/provider"
	"mote/internal/skills"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHookProvider implements a minimal provider for testing
type mockHookProvider struct {
	response string
	err      error
}

func (m *mockHookProvider) Name() string     { return "mock" }
func (m *mockHookProvider) Models() []string { return []string{"mock-model"} }
func (m *mockHookProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ChatResponse{
		Content:      m.response,
		FinishReason: "stop",
	}, nil
}
func (m *mockHookProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	ch := make(chan provider.ChatEvent, 2)
	go func() {
		defer close(ch)
		ch <- provider.ChatEvent{
			Type:  provider.EventTypeContent,
			Delta: m.response,
		}
		ch <- provider.ChatEvent{
			Type: provider.EventTypeDone,
		}
	}()
	return ch, nil
}

// TestRunner_HookManagerIntegration tests that hook manager is properly set
func TestRunner_HookManagerIntegration(t *testing.T) {
	prov := &mockHookProvider{response: "Hello!"}
	config := DefaultConfig()

	runner := NewRunner(prov, nil, nil, config)
	require.NotNil(t, runner)

	// Initially no hook manager
	assert.Nil(t, runner.hookManager)

	// Set hook manager
	hm := hooks.NewManager()
	runner.SetHookManager(hm)

	assert.NotNil(t, runner.hookManager)
	assert.Equal(t, hm, runner.hookManager)
}

// TestRunner_SkillManagerIntegration tests that skill manager is properly set
func TestRunner_SkillManagerIntegration(t *testing.T) {
	prov := &mockHookProvider{response: "Hello!"}
	config := DefaultConfig()

	runner := NewRunner(prov, nil, nil, config)
	require.NotNil(t, runner)

	// Initially no skill manager
	assert.Nil(t, runner.skillManager)

	// Set skill manager
	sm := skills.NewManager(skills.ManagerConfig{})
	runner.SetSkillManager(sm)

	assert.NotNil(t, runner.skillManager)
	assert.Equal(t, sm, runner.skillManager)
}

// TestRunner_HookTriggering tests that hooks are triggered at the right points
// Note: This test requires a full SessionManager setup to run properly.
// For now, we test the hook manager and skill manager integration separately.
func TestRunner_HookTriggering(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// TestRunner_HookInterrupt tests that a hook can interrupt execution
// Note: This test requires a full SessionManager setup to run properly.
func TestRunner_HookInterrupt(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// TestRunner_HookDataModification tests that hooks can modify message data
// Note: This test requires a full SessionManager setup to run properly.
func TestRunner_HookDataModification(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// TestRunner_SkillToolsIntegration tests that skill tools are available
func TestRunner_SkillToolsIntegration(t *testing.T) {
	sm := skills.NewManager(skills.ManagerConfig{})

	// Create a temp skill directory
	tmpDir := t.TempDir()
	require.NoError(t, createTestSkillFiles(t, tmpDir))

	// Load and activate skill
	_, err := sm.LoadSkill(tmpDir)
	require.NoError(t, err)

	err = sm.Activate("test-skill", nil)
	require.NoError(t, err)

	// Create runner with skill manager
	prov := &mockHookProvider{response: "Hello"}
	config := DefaultConfig()

	runner := NewRunner(prov, nil, nil, config)
	runner.SetSkillManager(sm)

	// Verify skill tools are accessible through manager
	// Note: The runner integration depends on how tools are injected
	assert.True(t, sm.IsActive("test-skill"))
}

// TestRunner_MultipleHooksOrdering tests that multiple hooks execute in priority order
// Note: This test requires a full SessionManager setup to run properly.
func TestRunner_MultipleHooksOrdering(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// TestRunner_HookContextInfo tests that hook context contains correct information
// Note: This test requires a full SessionManager setup to run properly.
func TestRunner_HookContextInfo(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// TestRunner_NoHookManager tests that runner works without hook manager
// Note: This test requires a full SessionManager setup to run properly.
func TestRunner_NoHookManager(t *testing.T) {
	t.Skip("Requires SessionManager setup - covered by integration tests")
}

// Helper function to create test skill files
func createTestSkillFiles(t *testing.T, dir string) error {
	t.Helper()

	manifest := `{
		"id": "test-skill",
		"name": "Test Skill",
		"version": "1.0.0",
		"tools": [
			{
				"name": "test_tool",
				"description": "A test tool",
				"handler": "main.js#testTool"
			}
		]
	}`

	mainJS := `function testTool() { return { result: "ok" }; }`

	if err := writeFile(dir, "manifest.json", manifest); err != nil {
		return err
	}
	return writeFile(dir, "main.js", mainJS)
}

func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}
