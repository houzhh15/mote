package orchestrator

import "errors"

// Common errors used by orchestrators
var (
	// ErrMaxIterations 表示达到最大迭代次数
	ErrMaxIterations = errors.New("max iterations reached")

	// ErrHookInterrupted 表示钩子中断了执行
	ErrHookInterrupted = errors.New("execution interrupted by hook")

	// ErrContextCanceled 表示上下文被取消
	ErrContextCanceled = errors.New("context canceled or timeout")
)
