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
	MaxIterations    int
	MaxTokens        int
	Temperature      float64
	StreamOutput     bool
	Timeout          time.Duration
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
	UseChat                bool // After compaction, use Chat mode instead of Stream
}

// Usage 是 runner/types.Usage 的别名
type Usage = types.Usage
