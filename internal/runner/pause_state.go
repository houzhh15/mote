package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mote/internal/provider"
)

// PauseState 表示会话的暂停状态
type PauseState struct {
	// 基础状态
	Paused    bool      // 是否暂停中
	PausedAt  time.Time // 暂停时间
	SessionID string    // 会话 ID

	// Tool 信息
	PendingTools []provider.ToolCall // 待执行的 tools

	// 通道通信
	pauseCh     chan struct{} // 用于阻塞等待恢复
	userInputCh chan string   // 用于接收用户输入
	resumedCh   chan struct{} // 用于通知已恢复（避免重复恢复）

	// 超时控制
	timeoutTimer *time.Timer   // 超时定时器
	timeout      time.Duration // 超时时长

	// 上下文
	ctx    context.Context    // 执行上下文
	cancel context.CancelFunc // 取消函数

	mu sync.RWMutex // 保护状态字段
}

// NewPauseState 创建暂停状态
func NewPauseState(sessionID string, tools []provider.ToolCall, timeout time.Duration) *PauseState {
	ctx, cancel := context.WithCancel(context.Background())
	return &PauseState{
		Paused:       false,
		PausedAt:     time.Now(),
		SessionID:    sessionID,
		PendingTools: tools,
		pauseCh:      make(chan struct{}),
		userInputCh:  make(chan string, 1),
		resumedCh:    make(chan struct{}),
		timeout:      timeout,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Activate 激活暂停（阻塞等待）
// 返回用户输入和是否超时
func (ps *PauseState) Activate() (userInput string, timedOut bool) {
	ps.mu.Lock()
	ps.Paused = true
	ps.PausedAt = time.Now()
	ps.timeoutTimer = time.NewTimer(ps.timeout)
	ps.mu.Unlock()

	// 防止 panic
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in PauseState.Activate",
				"error", r,
				"sessionID", ps.SessionID)
		}
	}()

	select {
	case <-ps.pauseCh:
		// 用户恢复
		select {
		case input := <-ps.userInputCh:
			return input, false
		default:
			return "", false
		}
	case <-ps.timeoutTimer.C:
		// 超时
		slog.Warn("pause timed out",
			"sessionID", ps.SessionID,
			"timeout", ps.timeout)
		return "", true
	case <-ps.ctx.Done():
		// 上下文取消
		slog.Info("pause context cancelled",
			"sessionID", ps.SessionID)
		return "", true
	}
}

// Resume 恢复执行
func (ps *PauseState) Resume(userInput string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.Paused {
		return fmt.Errorf("session not paused")
	}

	// 发送用户输入（如果有）
	if userInput != "" {
		select {
		case ps.userInputCh <- userInput:
		default:
			return fmt.Errorf("failed to send user input")
		}
	}

	// 停止超时定时器
	if ps.timeoutTimer != nil {
		ps.timeoutTimer.Stop()
	}

	// 通知恢复
	close(ps.pauseCh)
	ps.Paused = false

	slog.Info("pause resumed",
		"sessionID", ps.SessionID,
		"hasInput", userInput != "")

	return nil
}

// Cleanup 清理资源
func (ps *PauseState) Cleanup() {
	ps.cancel()
	if ps.timeoutTimer != nil {
		ps.timeoutTimer.Stop()
	}

	slog.Debug("pause state cleaned up",
		"sessionID", ps.SessionID)
}

// IsPaused 检查是否暂停中
func (ps *PauseState) IsPaused() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.Paused
}

// GetPendingTools 获取待执行的 tools
func (ps *PauseState) GetPendingTools() []provider.ToolCall {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.PendingTools
}

// UpdatePendingTools 更新待执行的 tools
func (ps *PauseState) UpdatePendingTools(tools []provider.ToolCall) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.PendingTools = tools
}
