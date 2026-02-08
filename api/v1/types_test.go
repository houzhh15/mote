package v1

import (
	"encoding/json"
	"testing"
	"time"
)

func TestChatRequest_JSONSerialization(t *testing.T) {
	tests := []struct {
		name    string
		input   ChatRequest
		wantKey []string // keys that should be present
	}{
		{
			name: "with session_id",
			input: ChatRequest{
				SessionID: "sess-123",
				Message:   "Hello",
			},
			wantKey: []string{"session_id", "message"},
		},
		{
			name: "without session_id",
			input: ChatRequest{
				Message: "Hello",
			},
			wantKey: []string{"message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			for _, key := range tt.wantKey {
				if _, ok := result[key]; !ok {
					t.Errorf("Expected key %q not found in JSON", key)
				}
			}
		})
	}
}

func TestChatResponse_JSONSerialization(t *testing.T) {
	resp := ChatResponse{
		SessionID: "sess-123",
		Message:   "Hello!",
		ToolCalls: []ToolCallResult{
			{Name: "test_tool", Result: "ok"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result ChatResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.SessionID != resp.SessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", result.SessionID, resp.SessionID)
	}
	if result.Message != resp.Message {
		t.Errorf("Message mismatch: got %q, want %q", result.Message, resp.Message)
	}
	if len(result.ToolCalls) != len(resp.ToolCalls) {
		t.Errorf("ToolCalls length mismatch: got %d, want %d", len(result.ToolCalls), len(resp.ToolCalls))
	}
}

func TestChatStreamEvent_JSONSerialization(t *testing.T) {
	tests := []struct {
		name  string
		event ChatStreamEvent
	}{
		{
			name: "content event",
			event: ChatStreamEvent{
				Type:  "content",
				Delta: "Hello",
			},
		},
		{
			name: "tool_call event",
			event: ChatStreamEvent{
				Type:     "tool_call",
				ToolCall: &ToolCallResult{Name: "test", Result: "ok"},
			},
		},
		{
			name: "done event",
			event: ChatStreamEvent{
				Type:      "done",
				SessionID: "sess-123",
			},
		},
		{
			name: "error event",
			event: ChatStreamEvent{
				Type:  "error",
				Error: "something went wrong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var result ChatStreamEvent
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if result.Type != tt.event.Type {
				t.Errorf("Type mismatch: got %q, want %q", result.Type, tt.event.Type)
			}
		})
	}
}

func TestToolInfo_JSONSerialization(t *testing.T) {
	tool := ToolInfo{
		Name:        "test_tool",
		Description: "A test tool",
		Schema:      map[string]any{"type": "object"},
		Type:        "builtin",
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result ToolInfo
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Name != tool.Name {
		t.Errorf("Name mismatch: got %q, want %q", result.Name, tool.Name)
	}
	if result.Type != tool.Type {
		t.Errorf("Type mismatch: got %q, want %q", result.Type, tool.Type)
	}
}

func TestMemoryResult_JSONSerialization(t *testing.T) {
	mem := MemoryResult{
		ID:        "mem-123",
		Content:   "Test content",
		Score:     0.95,
		Source:    "conversation",
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(mem)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result MemoryResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.ID != mem.ID {
		t.Errorf("ID mismatch: got %q, want %q", result.ID, mem.ID)
	}
	if result.Score != mem.Score {
		t.Errorf("Score mismatch: got %f, want %f", result.Score, mem.Score)
	}
}

func TestSessionSummary_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	session := SessionSummary{
		ID:           "sess-123",
		CreatedAt:    now,
		UpdatedAt:    now,
		MessageCount: 5,
		Preview:      "Hello...",
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result SessionSummary
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", result.ID, session.ID)
	}
	if result.MessageCount != session.MessageCount {
		t.Errorf("MessageCount mismatch: got %d, want %d", result.MessageCount, session.MessageCount)
	}
}

func TestHealthResponse_JSONSerialization(t *testing.T) {
	health := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Uptime:    "1h30m",
		Timestamp: time.Now().Format(time.RFC3339),
		Components: map[string]ComponentHealth{
			"database": {Status: "healthy", Message: "connected"},
			"memory":   {Status: "healthy"},
		},
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result HealthResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Status != health.Status {
		t.Errorf("Status mismatch: got %q, want %q", result.Status, health.Status)
	}
	if len(result.Components) != len(health.Components) {
		t.Errorf("Components length mismatch: got %d, want %d", len(result.Components), len(health.Components))
	}
}

func TestErrorResponse_JSONSerialization(t *testing.T) {
	errResp := ErrorResponse{
		Error:   "Something went wrong",
		Code:    ErrCodeInternalError,
		Details: map[string]any{"field": "value"},
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result ErrorResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Code != errResp.Code {
		t.Errorf("Code mismatch: got %q, want %q", result.Code, errResp.Code)
	}
}

func TestCronJob_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	nextRun := now.Add(24 * time.Hour)
	job := CronJob{
		Name:        "daily_backup",
		Schedule:    "0 0 * * *",
		Prompt:      "Run backup",
		Enabled:     true,
		LastRun:     &now,
		NextRun:     &nextRun,
		RunCount:    10,
		Description: "Daily backup job",
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result CronJob
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Name != job.Name {
		t.Errorf("Name mismatch: got %q, want %q", result.Name, job.Name)
	}
	if result.Schedule != job.Schedule {
		t.Errorf("Schedule mismatch: got %q, want %q", result.Schedule, job.Schedule)
	}
}

func TestMCPServerInfo_JSONSerialization(t *testing.T) {
	server := MCPServerInfo{
		Name:      "local-server",
		Status:    "connected",
		Transport: "stdio",
		Tools:     []string{"tool1", "tool2"},
	}

	data, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result MCPServerInfo
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Name != server.Name {
		t.Errorf("Name mismatch: got %q, want %q", result.Name, server.Name)
	}
	if result.Status != server.Status {
		t.Errorf("Status mismatch: got %q, want %q", result.Status, server.Status)
	}
	if len(result.Tools) != len(server.Tools) {
		t.Errorf("Tools length mismatch: got %d, want %d", len(result.Tools), len(server.Tools))
	}
}

// TestErrorCodes verifies all error codes are valid strings.
func TestErrorCodes(t *testing.T) {
	codes := []string{
		ErrCodeInvalidRequest,
		ErrCodeNotFound,
		ErrCodeMethodNotAllowed,
		ErrCodeRateLimitExceeded,
		ErrCodeValidationFailed,
		ErrCodeInternalError,
		ErrCodeServiceUnavailable,
		ErrCodeProviderError,
		ErrCodeToolExecutionError,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("Error code should not be empty")
		}
	}
}
