package runner

import (
	"fmt"
	"testing"
	"time"

	"mote/internal/provider"
)

func TestAPIPauseController_Pause(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 测试正常暂停
	err := ctrl.Pause("session1")
	if err != nil {
		t.Errorf("Pause failed: %v", err)
	}

	// 验证暂停标志已设置（通过检查是否可以获取 state）
	_, shouldPause := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{})
	if !shouldPause {
		t.Error("Expected pause flag to be set")
	}

	// 测试重复暂停（创建状态后再次暂停应该失败）
	// 但因为第一次 Pause 后状态未激活，所以 IsPaused 返回 false
	// 我们需要先激活状态才能测试重复暂停
	state, _ := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{{ID: "test"}})
	go state.Activate()
	time.Sleep(50 * time.Millisecond) // 等待激活

	err = ctrl.Pause("session1")
	if err == nil {
		t.Error("Expected error when pausing already active session, but got nil")
	}
}

func TestAPIPauseController_Resume(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 测试未暂停恢复（应该失败）
	err := ctrl.Resume("session1", "")
	if err == nil {
		t.Error("Expected error when resuming non-paused session, but got nil")
	}

	// 设置暂停标志
	ctrl.Pause("session1")

	// 启动一个 goroutine 来激活暂停
	state, shouldPause := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{
		{ID: "call1", Name: "test_tool", Arguments: "{}"},
	})
	if !shouldPause {
		t.Fatal("Expected should pause, but got false")
	}

	done := make(chan struct{})
	go func() {
		userInput, timedOut := state.Activate()
		if timedOut {
			t.Error("Pause timed out unexpectedly")
		}
		if userInput != "test input" {
			t.Errorf("Expected user input 'test input', got '%s'", userInput)
		}
		close(done)
	}()

	// 等待一小段时间确保 Activate 已经开始阻塞
	time.Sleep(100 * time.Millisecond)

	// 恢复执行
	err = ctrl.Resume("session1", "test input")
	if err != nil {
		t.Errorf("Resume failed: %v", err)
	}

	// 等待 goroutine 完成
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Resume did not complete within timeout")
	}
}

func TestAPIPauseController_GetStatus(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 测试未暂停状态
	status, err := ctrl.GetStatus("session1")
	if err != nil {
		t.Errorf("GetStatus failed: %v", err)
	}
	if status.Paused {
		t.Error("Expected paused=false for non-existent session")
	}

	// 设置暂停
	ctrl.Pause("session1")
	tools := []provider.ToolCall{
		{ID: "call1", Name: "shell", Arguments: `{"command":"ls"}`},
	}
	ctrl.ShouldPauseBeforeTools("session1", tools)

	// 激活暂停
	state, _ := ctrl.ShouldPauseBeforeTools("session1", tools)
	go state.Activate()

	// 等待激活
	time.Sleep(100 * time.Millisecond)

	// 获取状态
	status, err = ctrl.GetStatus("session1")
	if err != nil {
		t.Errorf("GetStatus failed: %v", err)
	}
	if !status.Paused {
		t.Error("Expected paused=true")
	}
	if len(status.PendingTools) != 1 {
		t.Errorf("Expected 1 pending tool, got %d", len(status.PendingTools))
	}
	if status.TimeoutIn <= 0 || status.TimeoutIn > 300 {
		t.Errorf("Expected timeout_in in range (0, 300], got %d", status.TimeoutIn)
	}
}

func TestAPIPauseController_Cleanup(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 设置暂停
	ctrl.Pause("session1")

	// 清理
	err := ctrl.Cleanup("session1")
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// 验证已清理
	if ctrl.IsPaused("session1") {
		t.Error("Expected session to be cleaned up, but still has pause flag")
	}

	// 测试重复清理（不应该报错）
	err = ctrl.Cleanup("session1")
	if err != nil {
		t.Errorf("Cleanup non-existent session failed: %v", err)
	}
}

func TestAPIPauseController_Timeout(t *testing.T) {
	// 使用短超时进行测试
	ctrl := NewAPIPauseController(200 * time.Millisecond)

	// 设置暂停
	ctrl.Pause("session1")
	state, shouldPause := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{
		{ID: "call1", Name: "test_tool"},
	})
	if !shouldPause {
		t.Fatal("Expected should pause")
	}

	// 激活暂停并等待超时
	userInput, timedOut := state.Activate()
	if !timedOut {
		t.Error("Expected timeout, but got resume")
	}
	if userInput != "" {
		t.Errorf("Expected empty input on timeout, got '%s'", userInput)
	}
}

func TestAPIPauseController_ShouldPauseBeforeTools(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 未设置暂停标志
	_, shouldPause := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{})
	if shouldPause {
		t.Error("Expected should not pause when flag not set")
	}

	// 设置暂停标志
	ctrl.Pause("session1")
	state, shouldPause := ctrl.ShouldPauseBeforeTools("session1", []provider.ToolCall{
		{ID: "call1", Name: "test"},
	})
	if !shouldPause {
		t.Error("Expected should pause when flag set")
	}
	if state == nil {
		t.Fatal("Expected state to be non-nil")
	}

	// 验证 pending tools 已更新
	tools := state.GetPendingTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 pending tool, got %d", len(tools))
	}
}

func TestAPIPauseController_ConcurrentAccess(t *testing.T) {
	ctrl := NewAPIPauseController(5 * time.Minute)

	// 并发暂停多个会话
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session%d", id)
			err := ctrl.Pause(sessionID)
			if err != nil {
				t.Errorf("Concurrent pause failed for %s: %v", sessionID, err)
			}
			done <- struct{}{}
		}(i)
	}

	// 等待所有完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证所有会话都已设置暂停标志（通过 ShouldPauseBeforeTools 检查）
	for i := 0; i < 10; i++ {
		sessionID := fmt.Sprintf("session%d", i)
		_, shouldPause := ctrl.ShouldPauseBeforeTools(sessionID, []provider.ToolCall{})
		if !shouldPause {
			t.Errorf("Session %s does not have pause flag after concurrent access", sessionID)
		}
	}
}
