package runner

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mote/internal/provider"
)

// PauseController 定义暂停控制接口
type PauseController interface {
	// Pause 发起暂停请求
	Pause(sessionID string) error

	// Resume 恢复执行
	Resume(sessionID string, userInput string) error

	// IsPaused 检查是否暂停中
	IsPaused(sessionID string) bool

	// GetStatus 获取暂停状态
	GetStatus(sessionID string) (*PauseStatus, error)

	// Cleanup 清理会话暂停状态
	Cleanup(sessionID string) error
}

// PauseStatus 表示暂停状态信息
type PauseStatus struct {
	Paused       bool       `json:"paused"`
	PausedAt     *time.Time `json:"paused_at,omitempty"`
	PendingTools []ToolInfo `json:"pending_tools,omitempty"`
	TimeoutIn    int        `json:"timeout_in"` // 剩余超时秒数
}

// ToolInfo 表示 tool 信息
type ToolInfo struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// APIPauseController 实现 API 模式的暂停控制
type APIPauseController struct {
	states  map[string]*PauseState // sessionID -> PauseState
	mu      sync.RWMutex
	timeout time.Duration // 默认超时时长
}

// NewAPIPauseController 创建 API 暂停控制器
func NewAPIPauseController(timeout time.Duration) *APIPauseController {
	if timeout <= 0 {
		timeout = 5 * time.Minute // 默认 5 分钟
	}
	return &APIPauseController{
		states:  make(map[string]*PauseState),
		timeout: timeout,
	}
}

// Pause 设置暂停标志
func (c *APIPauseController) Pause(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已存在暂停状态，不重复创建
	if state, exists := c.states[sessionID]; exists && state.IsPaused() {
		return fmt.Errorf("session already paused")
	}

	// 创建暂停状态（但不激活）
	state := NewPauseState(sessionID, nil, c.timeout)
	c.states[sessionID] = state

	slog.Info("pause flag set", "sessionID", sessionID)
	return nil
}

// ShouldPauseBeforeTools 检查是否需要在 tools 执行前暂停
// 返回 PauseState 和是否应该暂停
func (c *APIPauseController) ShouldPauseBeforeTools(
	sessionID string,
	tools []provider.ToolCall,
) (*PauseState, bool) {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists || state.IsPaused() {
		return nil, false
	}

	// 更新待执行的 tools
	state.UpdatePendingTools(tools)

	slog.Info("pause detected before tools",
		"sessionID", sessionID,
		"toolCount", len(tools))

	return state, true
}

// Resume 恢复执行
func (c *APIPauseController) Resume(sessionID string, userInput string) error {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not paused")
	}

	err := state.Resume(userInput)
	if err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}

	slog.Info("session resumed",
		"sessionID", sessionID,
		"hasInput", userInput != "")

	return nil
}

// IsPaused 检查是否暂停
func (c *APIPauseController) IsPaused(sessionID string) bool {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return false
	}

	return state.IsPaused()
}

// GetStatus 获取暂停状态
func (c *APIPauseController) GetStatus(sessionID string) (*PauseStatus, error) {
	c.mu.RLock()
	state, exists := c.states[sessionID]
	c.mu.RUnlock()

	if !exists {
		return &PauseStatus{Paused: false}, nil
	}

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
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			slog.Warn("failed to unmarshal tool arguments",
				"sessionID", sessionID,
				"toolID", tc.ID,
				"error", err)
			args = map[string]any{"raw": tc.Arguments}
		}
		tools[i] = ToolInfo{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: args,
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
func (c *APIPauseController) Cleanup(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if state, exists := c.states[sessionID]; exists {
		state.Cleanup()
		delete(c.states, sessionID)
		slog.Debug("pause state cleanup",
			"sessionID", sessionID)
	}

	return nil
}
