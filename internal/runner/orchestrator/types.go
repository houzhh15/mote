package orchestrator

import (
	"context"
	"time"

	"mote/internal/provider"
	"mote/internal/runner/types"
	"mote/internal/scheduler"
)

// Orchestrator 控制 Agent 循环的执行流程
type Orchestrator interface {
	// Run 执行完整的 Agent 循环，返回事件通道
	Run(ctx context.Context, request *RunRequest) (<-chan types.Event, error)
}

// RunRequest 封装运行请求的所有参数
type RunRequest struct {
	SessionID     string
	UserInput     string
	Attachments   []provider.Attachment
	Provider      provider.Provider
	CachedSession *scheduler.CachedSession
}

// Config 控制循环行为
type Config struct {
	MaxIterations int
	MaxTokens     int
	Temperature   float64
	StreamOutput  bool
	Timeout       time.Duration
	SystemPrompt  string // Static system prompt fallback (used when SystemPromptBuilder is nil)
}

// LoopState 封装循环状态
type LoopState struct {
	Iteration              int
	ConsecutiveErrors      int
	TotalConsecutiveErrors int
	LastResponse           *provider.ChatResponse
	TotalTokens            int64
	ContextRetried         bool
	TransientRetries       int
}

// Usage 是 runner/types.Usage 的别名
type Usage = types.Usage

// ToolExecutorFunc 执行工具调用并返回结果。
// 该函数由 Runner 注入，包含完整的工具执行逻辑（策略检查、钩子、心跳、截断等）。
// 返回工具结果消息列表和错误数量（仅统计实际执行错误，不含 JSON 解析/策略拒绝等）。
type ToolExecutorFunc func(ctx context.Context, toolCalls []provider.ToolCall, sessionID string) ([]provider.Message, int)
