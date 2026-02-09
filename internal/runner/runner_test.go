package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"mote/internal/compaction"
	"mote/internal/policy"
	"mote/internal/policy/approval"
	"mote/internal/tools"
)

func TestEventType(t *testing.T) {
	tests := []struct {
		et   EventType
		want string
	}{
		{EventTypeContent, "content"},
		{EventTypeToolCall, "tool_call"},
		{EventTypeToolResult, "tool_result"},
		{EventTypeDone, "done"},
		{EventTypeError, "error"},
		{EventType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.et.String(); got != tt.want {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.et, got, tt.want)
		}
	}
}

func TestNewContentEvent(t *testing.T) {
	e := NewContentEvent("hello world")
	if e.Type != EventTypeContent {
		t.Errorf("expected EventTypeContent, got %v", e.Type)
	}
	if e.Content != "hello world" {
		t.Errorf("expected content 'hello world', got %q", e.Content)
	}
}

func TestNewDoneEvent(t *testing.T) {
	usage := &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	e := NewDoneEvent(usage)
	if e.Type != EventTypeDone {
		t.Errorf("expected EventTypeDone, got %v", e.Type)
	}
	if e.Usage == nil {
		t.Error("expected usage to be set")
	}
	if e.Usage.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", e.Usage.TotalTokens)
	}
}

func TestNewErrorEvent(t *testing.T) {
	err := context.DeadlineExceeded
	e := NewErrorEvent(err)
	if e.Type != EventTypeError {
		t.Errorf("expected EventTypeError, got %v", e.Type)
	}
	if e.Error != err {
		t.Errorf("expected error %v, got %v", err, e.Error)
	}
	if e.ErrorMsg != err.Error() {
		t.Errorf("expected error msg %q, got %q", err.Error(), e.ErrorMsg)
	}
}

func TestNewToolResultEvent(t *testing.T) {
	e := NewToolResultEvent("call1", "shell", "output", false, 100)
	if e.Type != EventTypeToolResult {
		t.Errorf("expected EventTypeToolResult, got %v", e.Type)
	}
	if e.ToolResult == nil {
		t.Fatal("expected tool result to be set")
	}
	if e.ToolResult.ToolCallID != "call1" {
		t.Errorf("expected call ID 'call1', got %q", e.ToolResult.ToolCallID)
	}
	if e.ToolResult.ToolName != "shell" {
		t.Errorf("expected tool name 'shell', got %q", e.ToolResult.ToolName)
	}
	if e.ToolResult.Output != "output" {
		t.Errorf("expected output 'output', got %q", e.ToolResult.Output)
	}
	if e.ToolResult.DurationMs != 100 {
		t.Errorf("expected duration 100ms, got %d", e.ToolResult.DurationMs)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", cfg.MaxIterations)
	}
	if cfg.MaxTokens != 8000 {
		t.Errorf("expected MaxTokens 8000, got %d", cfg.MaxTokens)
	}
	if cfg.MaxMessages != 100 {
		t.Errorf("expected MaxMessages 100, got %d", cfg.MaxMessages)
	}
	if cfg.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5m, got %v", cfg.Timeout)
	}
	if !cfg.StreamOutput {
		t.Error("expected StreamOutput true")
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected Temperature 0.7, got %f", cfg.Temperature)
	}
}

func TestConfigBuilders(t *testing.T) {
	cfg := DefaultConfig().
		WithMaxIterations(20).
		WithMaxTokens(4000).
		WithMaxMessages(50).
		WithTimeout(time.Minute).
		WithStreamOutput(false).
		WithTemperature(0.5).
		WithSystemPrompt("custom prompt")

	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations 20, got %d", cfg.MaxIterations)
	}
	if cfg.MaxTokens != 4000 {
		t.Errorf("expected MaxTokens 4000, got %d", cfg.MaxTokens)
	}
	if cfg.MaxMessages != 50 {
		t.Errorf("expected MaxMessages 50, got %d", cfg.MaxMessages)
	}
	if cfg.Timeout != time.Minute {
		t.Errorf("expected Timeout 1m, got %v", cfg.Timeout)
	}
	if cfg.StreamOutput {
		t.Error("expected StreamOutput false")
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("expected Temperature 0.5, got %f", cfg.Temperature)
	}
	if cfg.SystemPrompt != "custom prompt" {
		t.Errorf("expected SystemPrompt 'custom prompt', got %q", cfg.SystemPrompt)
	}
}

// mockTool is a simple tool for testing
type mockTool struct {
	tools.BaseTool
}

func newMockTool(name, desc string) *mockTool {
	return &mockTool{
		BaseTool: tools.BaseTool{
			ToolName:        name,
			ToolDescription: desc,
			ToolParameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "The input value",
					},
				},
				"required": []string{"input"},
			},
		},
	}
}

func (m *mockTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "mock result"}, nil
}

// mockPolicyChecker is a mock implementation of PolicyChecker for testing.
type mockPolicyChecker struct {
	result *policy.PolicyResult
	err    error
	calls  []*policy.ToolCall
}

func (m *mockPolicyChecker) Check(ctx context.Context, call *policy.ToolCall) (*policy.PolicyResult, error) {
	m.calls = append(m.calls, call)
	return m.result, m.err
}

// mockApprovalHandler is a mock implementation of ApprovalHandler for testing.
type mockApprovalHandler struct {
	result      *approval.ApprovalResult
	err         error
	calls       []*policy.ToolCall
	reasons     []string
	pendingReqs map[string]*approval.ApprovalRequest
}

func newMockApprovalHandler() *mockApprovalHandler {
	return &mockApprovalHandler{
		pendingReqs: make(map[string]*approval.ApprovalRequest),
	}
}

func (m *mockApprovalHandler) RequestApproval(ctx context.Context, call *policy.ToolCall, reason string) (*approval.ApprovalResult, error) {
	m.calls = append(m.calls, call)
	m.reasons = append(m.reasons, reason)
	return m.result, m.err
}

func (m *mockApprovalHandler) HandleResponse(requestID string, approved bool, message string) error {
	return nil
}

func (m *mockApprovalHandler) GetPending(requestID string) (*approval.ApprovalRequest, bool) {
	req, ok := m.pendingReqs[requestID]
	return req, ok
}

func (m *mockApprovalHandler) ListPending() []*approval.ApprovalRequest {
	var list []*approval.ApprovalRequest
	for _, req := range m.pendingReqs {
		list = append(list, req)
	}
	return list
}

func (m *mockApprovalHandler) Close() error {
	return nil
}

func TestRunner_SetPolicyExecutor(t *testing.T) {
	runner := &Runner{}
	checker := &mockPolicyChecker{}

	runner.SetPolicyExecutor(checker)

	runner.mu.RLock()
	defer runner.mu.RUnlock()
	if runner.policyExecutor == nil {
		t.Error("expected policyExecutor to be set")
	}
}

func TestRunner_SetApprovalManager(t *testing.T) {
	runner := &Runner{}
	handler := newMockApprovalHandler()

	runner.SetApprovalManager(handler)

	runner.mu.RLock()
	defer runner.mu.RUnlock()
	if runner.approvalManager == nil {
		t.Error("expected approvalManager to be set")
	}
}

func TestRunner_PolicyBlock(t *testing.T) {
	// Create a runner with a blocking policy
	registry := tools.NewRegistry()
	_ = registry.Register(newMockTool("shell", "Execute shell command"))

	runner := &Runner{
		registry: registry,
		config:   DefaultConfig(),
	}

	// Set policy checker that blocks everything
	runner.SetPolicyExecutor(&mockPolicyChecker{
		result: &policy.PolicyResult{
			Allowed: false,
			Reason:  "blocked by test policy",
		},
	})

	// Create mock tool calls
	toolCalls := []struct {
		id   string
		name string
		args string
	}{
		{"call-1", "shell", `{"command":"rm -rf /"}`},
	}

	ctx := context.Background()
	events := make(chan Event, 10)

	// Execute with session context
	var providerToolCalls []struct {
		ID   string
		Name string
		Args string
	}
	for _, tc := range toolCalls {
		providerToolCalls = append(providerToolCalls, struct { //nolint:staticcheck // SA4010: Building slice for provider response
			ID   string
			Name string
			Args string
		}{tc.id, tc.name, tc.args})
	}

	// Build provider.ToolCall slice
	var ptcs []struct {
		ID        string
		Name      string
		Arguments string
		Function  *struct {
			Name      string
			Arguments string
		}
	}
	for _, tc := range toolCalls {
		ptcs = append(ptcs, struct { //nolint:staticcheck // SA4010: Building response for mock provider
			ID        string
			Name      string
			Arguments string
			Function  *struct {
				Name      string
				Arguments string
			}
		}{
			ID:        tc.id,
			Name:      tc.name,
			Arguments: tc.args,
		})
	}

	// Direct call to test policy blocking behavior
	checker := runner.policyExecutor.(*mockPolicyChecker)
	result, _ := checker.Check(ctx, &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command":"rm -rf /"}`,
	})

	if result.Allowed {
		t.Error("expected tool call to be blocked")
	}
	if result.Reason != "blocked by test policy" {
		t.Errorf("expected reason 'blocked by test policy', got %q", result.Reason)
	}

	close(events)
}

func TestRunner_PolicyApproval(t *testing.T) {
	// Create a runner with approval-required policy
	registry := tools.NewRegistry()
	_ = registry.Register(newMockTool("shell", "Execute shell command"))

	runner := &Runner{
		registry: registry,
		config:   DefaultConfig(),
	}

	// Set policy checker that requires approval
	runner.SetPolicyExecutor(&mockPolicyChecker{
		result: &policy.PolicyResult{
			Allowed:         true,
			RequireApproval: true,
			ApprovalReason:  "sudo command requires approval",
		},
	})

	// Set approval handler that approves
	approvalHandler := newMockApprovalHandler()
	approvalHandler.result = &approval.ApprovalResult{
		Approved: true,
		Message:  "approved by admin",
		Decision: approval.DecisionApproved,
	}
	runner.SetApprovalManager(approvalHandler)

	ctx := context.Background()

	// Direct test of approval flow
	policyChecker := runner.policyExecutor.(*mockPolicyChecker)
	pResult, _ := policyChecker.Check(ctx, &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command":"sudo apt update"}`,
	})

	if !pResult.Allowed {
		t.Error("expected policy check to allow (with approval)")
	}
	if !pResult.RequireApproval {
		t.Error("expected RequireApproval to be true")
	}

	// Test approval handler
	aResult, _ := approvalHandler.RequestApproval(ctx, &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command":"sudo apt update"}`,
	}, pResult.ApprovalReason)

	if !aResult.Approved {
		t.Error("expected approval to be granted")
	}
	if len(approvalHandler.reasons) != 1 || approvalHandler.reasons[0] != "sudo command requires approval" {
		t.Errorf("expected approval reason 'sudo command requires approval', got %v", approvalHandler.reasons)
	}
}

func TestRunner_PolicyApprovalRejected(t *testing.T) {
	// Create a runner with approval-required policy
	registry := tools.NewRegistry()
	_ = registry.Register(newMockTool("shell", "Execute shell command"))

	runner := &Runner{
		registry: registry,
		config:   DefaultConfig(),
	}

	// Set policy checker that requires approval
	runner.SetPolicyExecutor(&mockPolicyChecker{
		result: &policy.PolicyResult{
			Allowed:         true,
			RequireApproval: true,
			ApprovalReason:  "dangerous operation",
		},
	})

	// Set approval handler that rejects
	approvalHandler := newMockApprovalHandler()
	approvalHandler.result = &approval.ApprovalResult{
		Approved: false,
		Message:  "rejected: too dangerous",
		Decision: approval.DecisionRejected,
	}
	runner.SetApprovalManager(approvalHandler)

	ctx := context.Background()

	// Test rejection flow
	aResult, _ := approvalHandler.RequestApproval(ctx, &policy.ToolCall{
		Name:      "shell",
		Arguments: `{"command":"rm -rf /"}`,
	}, "dangerous operation")

	if aResult.Approved {
		t.Error("expected approval to be rejected")
	}
	if aResult.Message != "rejected: too dangerous" {
		t.Errorf("expected message 'rejected: too dangerous', got %q", aResult.Message)
	}
}

func TestRunner_PolicyAllowWithoutApproval(t *testing.T) {
	// Create a runner with allow policy (no approval needed)
	registry := tools.NewRegistry()
	_ = registry.Register(newMockTool("file_read", "Read file"))

	runner := &Runner{
		registry: registry,
		config:   DefaultConfig(),
	}

	// Set policy checker that allows without approval
	runner.SetPolicyExecutor(&mockPolicyChecker{
		result: &policy.PolicyResult{
			Allowed:         true,
			RequireApproval: false,
		},
	})

	ctx := context.Background()

	// Direct test
	policyChecker := runner.policyExecutor.(*mockPolicyChecker)
	pResult, _ := policyChecker.Check(ctx, &policy.ToolCall{
		Name:      "file_read",
		Arguments: `{"path":"/tmp/test.txt"}`,
	})

	if !pResult.Allowed {
		t.Error("expected tool call to be allowed")
	}
	if pResult.RequireApproval {
		t.Error("expected no approval requirement")
	}
}

// ==================== Memory Flush Tests ====================

func TestEstimateTokens(t *testing.T) {
	// EstimateTokens uses len(text) / 3 formula
	tests := []struct {
		name   string
		text   string
		expect int64
	}{
		{"empty string", "", 0},
		{"short text", "hello", 2},                         // 5 chars / 3 = 1.67 -> 2
		{"medium text", "hello world test", 6},             // 16 chars / 3 = 5.33 -> 6
		{"longer text", strings.Repeat("word ", 100), 167}, // 500 chars / 3 = 166.67 -> 167
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.text)
			if got != tt.expect {
				t.Errorf("EstimateTokens(%q...) = %d, want %d (len=%d)", tt.text[:min(20, len(tt.text))], got, tt.expect, len(tt.text))
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRunner_SessionTokensTracking(t *testing.T) {
	runner := &Runner{
		sessionTokens: make(map[string]*SessionTokens),
		// tokenMu is a sync.RWMutex value, no initialization needed
	}

	sessionID := "test-session"

	// Initially should return nil (session doesn't exist yet)
	tokens := runner.GetSessionTokens(sessionID)
	if tokens != nil {
		t.Errorf("expected nil tokens initially, got %+v", tokens)
	}

	// Update tokens (this will create the session)
	runner.UpdateTokens(sessionID, 100, 50)
	tokens = runner.GetSessionTokens(sessionID)
	if tokens == nil {
		t.Fatal("expected non-nil tokens after update")
	}
	if tokens.RequestTokens != 100 {
		t.Errorf("expected 100 request tokens, got %d", tokens.RequestTokens)
	}
	if tokens.ResponseTokens != 50 {
		t.Errorf("expected 50 response tokens, got %d", tokens.ResponseTokens)
	}
	if tokens.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", tokens.TotalTokens)
	}

	// Update again and verify accumulation
	runner.UpdateTokens(sessionID, 200, 100)
	tokens = runner.GetSessionTokens(sessionID)
	if tokens.TotalTokens != 450 {
		t.Errorf("expected 450 total tokens after accumulation, got %d", tokens.TotalTokens)
	}
}

func TestRunner_MemoryFlushStateTracking(t *testing.T) {
	runner := &Runner{
		flushStates: make(map[string]*memoryFlushState),
		// flushMu is a sync.RWMutex value, no initialization needed
	}

	sessionID := "test-session"

	// Get state should create new one
	state := runner.getMemoryFlushState(sessionID)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.lastCompactionCount != 0 {
		t.Errorf("expected initial compaction count 0, got %d", state.lastCompactionCount)
	}

	// Update state
	state.lastCompactionCount = 5

	// Get state again should return same state
	state2 := runner.getMemoryFlushState(sessionID)
	if state2.lastCompactionCount != 5 {
		t.Errorf("expected compaction count 5, got %d", state2.lastCompactionCount)
	}
}

func TestRunner_ShouldRunMemoryFlush_DisabledConfig(t *testing.T) {
	runner := &Runner{
		sessionTokens: make(map[string]*SessionTokens),
		flushStates:   make(map[string]*memoryFlushState),
	}

	// No compactionConfig set
	if runner.shouldRunMemoryFlush("session1") {
		t.Error("expected false when compactionConfig is nil")
	}
}

func TestRunner_ShouldRunMemoryFlush_ThresholdLogic(t *testing.T) {
	// Create config with small context window for testing
	// threshold = contextWindow - reserveTokens - softThreshold
	// With contextWindow=1000, reserve=100, soft=100 => threshold = 800
	config := compaction.CompactionConfig{
		MaxContextTokens: 1000, // Small context for testing
		MemoryFlush: compaction.MemoryFlushConfig{
			Enabled:             true,
			SoftThresholdTokens: 100,
			ReserveTokens:       100,
		},
	}

	runner := &Runner{
		sessionTokens:    make(map[string]*SessionTokens),
		flushStates:      make(map[string]*memoryFlushState),
		compactionConfig: &config,
	}

	sessionID := "test-session"

	// threshold = 1000 - 100 - 100 = 800
	// Below threshold: should not trigger
	runner.UpdateTokens(sessionID, 300, 200) // 500 tokens (< 800)
	if runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected false when below threshold (500 < 800)")
	}

	// At threshold: should trigger
	runner.UpdateTokens(sessionID, 200, 100) // 800 tokens (= 800)
	if !runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected true when at threshold (800 >= 800)")
	}

	// Above threshold: should trigger
	runner.UpdateTokens(sessionID, 200, 0) // 1000 tokens (> 800)
	if !runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected true when above threshold (1000 > 800)")
	}
}

func TestRunner_ShouldRunMemoryFlush_CompactionCycleLogic(t *testing.T) {
	// Create config with small context window for testing
	// threshold = contextWindow - reserveTokens - softThreshold = 1000 - 100 - 100 = 800
	compactorConfig := compaction.CompactionConfig{
		MaxContextTokens: 1000,
		MemoryFlush: compaction.MemoryFlushConfig{
			Enabled:             true,
			SoftThresholdTokens: 100,
			ReserveTokens:       100,
		},
	}

	compactor := compaction.NewCompactor(compactorConfig, nil)

	runner := &Runner{
		sessionTokens:    make(map[string]*SessionTokens),
		flushStates:      make(map[string]*memoryFlushState),
		compactionConfig: &compactorConfig,
		compactor:        compactor,
	}

	sessionID := "session1"

	// Simulate some compaction happened
	compactor.IncrementCompactionCount(sessionID)
	compactor.IncrementCompactionCount(sessionID)

	// Update tokens to trigger threshold (800+)
	runner.UpdateTokens(sessionID, 500, 400) // 900 tokens > 800 threshold

	// First check should trigger (compaction count changed from 0 to 2)
	if !runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected true on first check with compaction count difference")
	}

	// Simulate flush executed, update state
	state := runner.getMemoryFlushState(sessionID)
	state.lastCompactionCount = compactor.GetCompactionCount(sessionID)

	// After updating lastCompactionCount, should not trigger again in same cycle
	if runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected false after flush in same compaction cycle")
	}

	// New compaction cycle should allow flush again
	compactor.IncrementCompactionCount(sessionID)
	if !runner.shouldRunMemoryFlush(sessionID) {
		t.Error("expected true after new compaction cycle")
	}
}

// Test compaction count tracking in compactor
func TestCompactor_CompactionCount(t *testing.T) {
	compactorConfig := compaction.DefaultConfig()
	compactor := compaction.NewCompactor(compactorConfig, nil)

	sessionID := "test-session"

	// Initially 0
	if count := compactor.GetCompactionCount(sessionID); count != 0 {
		t.Errorf("expected initial count 0, got %d", count)
	}

	// Increment
	compactor.IncrementCompactionCount(sessionID)
	if count := compactor.GetCompactionCount(sessionID); count != 1 {
		t.Errorf("expected count 1 after increment, got %d", count)
	}

	// Increment again
	compactor.IncrementCompactionCount(sessionID)
	if count := compactor.GetCompactionCount(sessionID); count != 2 {
		t.Errorf("expected count 2 after second increment, got %d", count)
	}

	// Different session should have independent count
	sessionID2 := "test-session-2"
	if count := compactor.GetCompactionCount(sessionID2); count != 0 {
		t.Errorf("expected initial count 0 for new session, got %d", count)
	}
}
