package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mote/internal/provider"
)

// ACPProviderInterface 定义 ACPProvider 需要的最小接口
// 避免循环依赖
type ACPProviderInterface interface {
	safeSendEvent(convID string, event provider.ChatEvent) bool
}

// ACPPauseController 实现 ACP 模式的暂停控制
type ACPPauseController struct {
	states   map[string]*PauseState
	mu       sync.RWMutex
	timeout  time.Duration
	provider ACPProviderInterface // 引用 ACPProvider 用于发送事件
}

// NewACPPauseController 创建 ACP 暂停控制器
func NewACPPauseController(provider ACPProviderInterface, timeout time.Duration) *ACPPauseController {
	if timeout <= 0 {
		timeout = 5 * time.Minute // 默认 5 分钟
	}
	return &ACPPauseController{
		states:   make(map[string]*PauseState),
		timeout:  timeout,
		provider: provider,
	}
}

// Pause 设置暂停标志
func (c *ACPPauseController) Pause(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if state, exists := c.states[sessionID]; exists && state.IsPaused() {
		return fmt.Errorf("session already paused")
	}

	state := NewPauseState(sessionID, nil, c.timeout)
	c.states[sessionID] = state

	slog.Info("ACP pause flag set", "sessionID", sessionID)
	return nil
}

// ShouldPauseForTool 检查是否需要在指定 tool 前暂停
// 返回 PauseState 和是否应该暂停
func (c *ACPPauseController) ShouldPauseForTool(
	sessionID string,
	toolName string,
) (*PauseState, bool) {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists || state.IsPaused() {
		return nil, false
	}

	slog.Info("ACP pause detected before tool",
		"sessionID", sessionID,
		"toolName", toolName)

	return state, true
}

// WaitForResume 阻塞等待恢复（在 Hook 中调用）
// 返回用户输入、是否已暂停、是否超时
func (c *ACPPauseController) WaitForResume(
	ctx context.Context,
	sessionID string,
	toolName string,
	toolCallID string,
) (string, bool, bool) {
	state, shouldPause := c.ShouldPauseForTool(sessionID, toolName)
	if !shouldPause {
		return "", false, false
	}

	// 更新待执行的 tool 信息
	state.UpdatePendingTools([]provider.ToolCall{{
		ID:   toolCallID,
		Name: toolName,
	}})

	// 发送暂停事件 (如果 provider 接口可用)
	if c.provider != nil {
		c.provider.safeSendEvent(sessionID, provider.ChatEvent{
			Type: "pause",
			// Note: 更详细的事件数据在 Runner 层组装
		})
	}

	// 激活暂停（阻塞等待）
	userInput, timedOut := state.Activate()

	if timedOut {
		slog.Warn("ACP pause timed out", "sessionID", sessionID)
		if c.provider != nil {
			c.provider.safeSendEvent(sessionID, provider.ChatEvent{
				Type: "pause_timeout",
			})
		}
	}

	// 清理状态
	c.mu.Lock()
	delete(c.states, sessionID)
	c.mu.Unlock()

	return userInput, true, timedOut
}

// Resume 恢复执行
func (c *ACPPauseController) Resume(sessionID string, userInput string) error {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not paused")
	}

	err := state.Resume(userInput)
	if err != nil {
		return fmt.Errorf("ACP resume failed: %w", err)
	}

	slog.Info("ACP session resumed",
		"sessionID", sessionID,
		"hasInput", userInput != "")

	return nil
}

// IsPaused 检查是否暂停
func (c *ACPPauseController) IsPaused(sessionID string) bool {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return false
	}

	return state.IsPaused()
}

// GetStatus 获取暂停状态
func (c *ACPPauseController) GetStatus(sessionID string) (*PauseStatus, error) {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return &PauseStatus{Paused: false}, nil
	}

	// 复用 APIPauseController 的状态获取逻辑
	state.mu.RLock()
	defer state.mu.RUnlock()

	var timeoutIn int
	if state.timeoutTimer != nil && state.Paused {
		remaining := state.timeout - time.Since(state.PausedAt)
		timeoutIn = int(remaining.Seconds())
		if timeoutIn < 0 {
			timeoutIn = 0
		}
	}

	tools := make([]ToolInfo, len(state.PendingTools))
	for i, tc := range state.PendingTools {
		tools[i] = ToolInfo{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: map[string]any{}, // ACP 模式可能不需要完整参数
		}
	}

	pausedAt := state.PausedAt
	return &PauseStatus{
		Paused:       state.Paused,
		PausedAt:     &pausedAt,
		PendingTools: tools,
		TimeoutIn:    timeoutIn,
	}, nil
}

// Cleanup 清理会话状态
func (c *ACPPauseController) Cleanup(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if state, exists := c.states[sessionID]; exists {
		state.Cleanup()
		delete(c.states, sessionID)
		slog.Debug("ACP pause state cleanup",
			"sessionID", sessionID)
	}

	return nil
}
