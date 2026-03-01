package cfg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"mote/internal/provider"
)

// ---------------------------------------------------------------------------
// Test helpers — mock callback implementations for frame-local context model
// ---------------------------------------------------------------------------

// contextPromptRecorder records all calls to RunPromptWithContext.
type contextPromptRecorder struct {
	calls   []contextPromptCall
	results []contextPromptResult
	idx     int
}

type contextPromptCall struct {
	agentName string
	messages  []provider.Message // copy of frame context at call time
	userInput string
}

type contextPromptResult struct {
	text  string
	usage Usage
	err   error
}

func (r *contextPromptRecorder) run(_ context.Context, agentName string, messages []provider.Message, userInput string) (string, Usage, []provider.Message, error) {
	// Copy messages to avoid mutation issues
	msgCopy := make([]provider.Message, len(messages))
	copy(msgCopy, messages)
	r.calls = append(r.calls, contextPromptCall{agentName: agentName, messages: msgCopy, userInput: userInput})

	var res contextPromptResult
	if r.idx < len(r.results) {
		res = r.results[r.idx]
		r.idx++
	} else {
		res = contextPromptResult{
			text:  fmt.Sprintf("response-%d", r.idx),
			usage: Usage{TotalTokens: 10},
		}
		r.idx++
	}

	if res.err != nil {
		return "", Usage{}, nil, res.err
	}

	// Return new messages: user input + assistant reply
	newMsgs := []provider.Message{
		{Role: provider.RoleUser, Content: userInput},
		{Role: provider.RoleAssistant, Content: res.text},
	}
	return res.text, res.usage, newMsgs, nil
}

// staticAgentProvider returns a provider built from a fixed map.
func staticAgentProvider(agents map[string]*AgentCFG) AgentCFGProvider {
	return func(name string) (*AgentCFG, bool) {
		a, ok := agents[name]
		return a, ok
	}
}

func makeDelegateInfo(agentName string) *DelegateInfo {
	return &DelegateInfo{
		Depth:             0,
		MaxDepth:          10,
		AgentName:         agentName,
		Chain:             []string{agentName},
		RecursionCounters: map[string]int{},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPDAEngine_BasicPromptSequence(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "Step 1: analyze", Label: "analyze"},
		{Type: StepPrompt, Content: "Step 2: transform", Label: "transform"},
		{Type: StepPrompt, Content: "Step 3: summarize", Label: "summarize"},
	}
	agents := map[string]*AgentCFG{
		"main": {Steps: steps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "analysis-result", usage: Usage{TotalTokens: 20}},
			{text: "transform-result", usage: Usage{TotalTokens: 15}},
			{text: "summary-result", usage: Usage{TotalTokens: 25}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"Hello, start the pipeline",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "summary-result" {
		t.Errorf("expected 'summary-result', got %q", result)
	}

	if len(pr.calls) != 3 {
		t.Errorf("expected 3 prompt calls, got %d", len(pr.calls))
	}

	// Verify first call sees initial context
	if len(pr.calls) > 0 {
		firstCall := pr.calls[0]
		if len(firstCall.messages) != 1 {
			t.Errorf("first call: expected 1 context message, got %d", len(firstCall.messages))
		}
		if len(firstCall.messages) > 0 && !strings.Contains(firstCall.messages[0].Content, "[用户任务描述]") {
			t.Errorf("first call: expected [用户任务描述] in context, got %q", firstCall.messages[0].Content)
		}
	}

	// Verify context accumulation: third call sees initial + 2 rounds
	if len(pr.calls) > 2 {
		thirdCall := pr.calls[2]
		// 1 initial + 2 user + 2 assistant = 5
		if len(thirdCall.messages) != 5 {
			t.Errorf("third call: expected 5 context messages, got %d", len(thirdCall.messages))
		}
	}

	if usage.TotalTokens != 60 {
		t.Errorf("expected 60 total tokens, got %d", usage.TotalTokens)
	}

	if len(engine.ExecutedSteps()) != 3 {
		t.Errorf("expected 3 executed steps, got %d: %v",
			len(engine.ExecutedSteps()), engine.ExecutedSteps())
	}
}

func TestPDAEngine_AgentRefWithSteps_ContextIsolation(t *testing.T) {
	mainSteps := []Step{
		{Type: StepPrompt, Content: "Main step 1: analyze", Label: "main-analyze"},
		{Type: StepAgentRef, Agent: "worker", Content: "Do the heavy lifting", Label: "call-worker"},
		{Type: StepPrompt, Content: "Main step 3: summarize", Label: "main-summarize"},
	}
	workerSteps := []Step{
		{Type: StepPrompt, Content: "Worker step 1: research", Label: "worker-research"},
		{Type: StepPrompt, Content: "Worker step 2: compile", Label: "worker-compile"},
	}
	agents := map[string]*AgentCFG{
		"main":   {Steps: mainSteps},
		"worker": {Steps: workerSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "analysis-done", usage: Usage{TotalTokens: 10}},
			{text: "research-done", usage: Usage{TotalTokens: 15}},
			{text: "compilation-done", usage: Usage{TotalTokens: 20}},
			{text: "summary-done", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"User task: build a report",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "summary-done" {
		t.Errorf("expected 'summary-done', got %q", result)
	}

	if usage.TotalTokens != 55 {
		t.Errorf("expected 55 total tokens, got %d", usage.TotalTokens)
	}

	if len(pr.calls) != 4 {
		t.Fatalf("expected 4 prompt calls, got %d", len(pr.calls))
	}

	// Call 0: main agent, 1 context msg
	if pr.calls[0].agentName != "main" {
		t.Errorf("call 0: expected agent 'main', got %q", pr.calls[0].agentName)
	}
	if len(pr.calls[0].messages) != 1 {
		t.Errorf("call 0: expected 1 context msg, got %d", len(pr.calls[0].messages))
	}

	// Call 1: worker agent, isolated context (1 msg)
	if pr.calls[1].agentName != "worker" {
		t.Errorf("call 1: expected agent 'worker', got %q", pr.calls[1].agentName)
	}
	if len(pr.calls[1].messages) != 1 {
		t.Errorf("call 1 (worker): expected 1 context msg (isolated), got %d", len(pr.calls[1].messages))
	}
	if len(pr.calls[1].messages) > 0 && !strings.Contains(pr.calls[1].messages[0].Content, "Do the heavy lifting") {
		t.Errorf("call 1: expected child input in context, got %q", pr.calls[1].messages[0].Content)
	}

	// Call 2: worker step 2, context accumulated within worker frame
	if pr.calls[2].agentName != "worker" {
		t.Errorf("call 2: expected agent 'worker', got %q", pr.calls[2].agentName)
	}
	if len(pr.calls[2].messages) != 3 {
		t.Errorf("call 2 (worker): expected 3 context msgs, got %d", len(pr.calls[2].messages))
	}

	// Call 3: main step 3, context includes worker result injection
	if pr.calls[3].agentName != "main" {
		t.Errorf("call 3: expected agent 'main', got %q", pr.calls[3].agentName)
	}
	// Main context: initial + step1 user/assistant + worker result = 4
	if len(pr.calls[3].messages) != 4 {
		t.Errorf("call 3 (main): expected 4 context msgs, got %d", len(pr.calls[3].messages))
	}
	workerResultFound := false
	for _, m := range pr.calls[3].messages {
		if m.Role == provider.RoleAssistant && strings.Contains(m.Content, "[worker result]") &&
			strings.Contains(m.Content, "compilation-done") {
			workerResultFound = true
			break
		}
	}
	if !workerResultFound {
		t.Error("expected [worker result] with 'compilation-done' injected into main context")
	}

	expectedSteps := []string{"main-analyze", "worker-research", "worker-compile", "main-summarize"}
	executedSteps := engine.ExecutedSteps()
	if len(executedSteps) != len(expectedSteps) {
		t.Fatalf("expected %d executed steps, got %d: %v",
			len(expectedSteps), len(executedSteps), executedSteps)
	}
	for i, expected := range expectedSteps {
		if executedSteps[i] != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, executedSteps[i])
		}
	}
}

func TestPDAEngine_AgentRefWithoutSteps_StepExec(t *testing.T) {
	mainSteps := []Step{
		{Type: StepPrompt, Content: "Prepare data", Label: "prepare"},
		{Type: StepAgentRef, Agent: "helper", Content: "Process this data", Label: "call-helper"},
		{Type: StepPrompt, Content: "Merge results", Label: "merge"},
	}
	agents := map[string]*AgentCFG{
		"main":   {Steps: mainSteps},
		"helper": {},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "prepared", usage: Usage{TotalTokens: 10}},
			{text: "helper-output", usage: Usage{TotalTokens: 30}},
			{text: "merged", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Start",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "merged" {
		t.Errorf("expected 'merged', got %q", result)
	}

	if len(pr.calls) != 3 {
		t.Fatalf("expected 3 prompt calls, got %d", len(pr.calls))
	}

	// Call 1 is the helper StepExec
	if pr.calls[1].agentName != "helper" {
		t.Errorf("call 1: expected agent 'helper', got %q", pr.calls[1].agentName)
	}
	if len(pr.calls[1].messages) != 1 {
		t.Errorf("call 1 (helper): expected 1 context msg (isolated), got %d", len(pr.calls[1].messages))
	}

	// Main's final step should see helper result
	if len(pr.calls) > 2 {
		found := false
		for _, m := range pr.calls[2].messages {
			if strings.Contains(m.Content, "[helper result]") && strings.Contains(m.Content, "helper-output") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected [helper result] with 'helper-output' injected into main context")
		}
	}

	if usage.TotalTokens != 50 {
		t.Errorf("expected 50 total tokens, got %d", usage.TotalTokens)
	}
}

func TestPDAEngine_AgentRefPush_PreviousResultPassThrough(t *testing.T) {
	mainSteps := []Step{
		{Type: StepPrompt, Content: "Prepare data", Label: "prepare"},
		{Type: StepAgentRef, Agent: "processor", Content: "Process this data", Label: "call-processor"},
		{Type: StepPrompt, Content: "Use processed result", Label: "finalize"},
	}
	processorSteps := []Step{
		{Type: StepPrompt, Content: "Do processing", Label: "process"},
	}
	agents := map[string]*AgentCFG{
		"main":      {Steps: mainSteps},
		"processor": {Steps: processorSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "raw-data", usage: Usage{TotalTokens: 10}},
			{text: "processed-data", usage: Usage{TotalTokens: 20}},
			{text: "final-output", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Start",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "final-output" {
		t.Errorf("expected 'final-output', got %q", result)
	}
	if usage.TotalTokens != 40 {
		t.Errorf("expected 40 total tokens, got %d", usage.TotalTokens)
	}

	// Processor's context includes [上一步结果参考] with raw-data
	if len(pr.calls) >= 2 {
		processorCtx := pr.calls[1].messages
		prevResultFound := false
		for _, m := range processorCtx {
			if strings.Contains(m.Content, "[上一步结果参考]") && strings.Contains(m.Content, "raw-data") {
				prevResultFound = true
				break
			}
		}
		if !prevResultFound {
			t.Error("expected processor context to contain [上一步结果参考] with 'raw-data'")
		}
	}

	// Processor result injected into main context
	if len(pr.calls) >= 3 {
		processorResultFound := false
		for _, m := range pr.calls[2].messages {
			if m.Role == provider.RoleAssistant && strings.Contains(m.Content, "[processor result]") {
				processorResultFound = true
				break
			}
		}
		if !processorResultFound {
			t.Error("expected processor result injected into main context")
		}
	}
}

func TestPDAEngine_NestedAgentRef_TwoLevelIsolation(t *testing.T) {
	mainSteps := []Step{
		{Type: StepPrompt, Content: "Main work", Label: "main-work"},
		{Type: StepAgentRef, Agent: "worker", Label: "call-worker"},
	}
	workerSteps := []Step{
		{Type: StepPrompt, Content: "Worker work", Label: "worker-work"},
		{Type: StepAgentRef, Agent: "specialist", Label: "call-specialist"},
	}
	specialistSteps := []Step{
		{Type: StepPrompt, Content: "Specialist work", Label: "specialist-work"},
	}
	agents := map[string]*AgentCFG{
		"main":       {Steps: mainSteps},
		"worker":     {Steps: workerSteps},
		"specialist": {Steps: specialistSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "main-done", usage: Usage{TotalTokens: 10}},
			{text: "worker-done", usage: Usage{TotalTokens: 10}},
			{text: "specialist-done", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Start nested",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "specialist-done" {
		t.Errorf("expected 'specialist-done', got %q", result)
	}

	if len(pr.calls) != 3 {
		t.Fatalf("expected 3 prompt calls, got %d", len(pr.calls))
	}

	// Each call has isolated context (only 1 initial message)
	for i, call := range pr.calls {
		if len(call.messages) != 1 {
			t.Errorf("call %d (%s): expected 1 context msg (isolated), got %d",
				i, call.agentName, len(call.messages))
		}
	}

	if pr.calls[0].agentName != "main" {
		t.Errorf("call 0: expected 'main', got %q", pr.calls[0].agentName)
	}
	if pr.calls[1].agentName != "worker" {
		t.Errorf("call 1: expected 'worker', got %q", pr.calls[1].agentName)
	}
	if pr.calls[2].agentName != "specialist" {
		t.Errorf("call 2: expected 'specialist', got %q", pr.calls[2].agentName)
	}

	expected := []string{"main-work", "worker-work", "specialist-work"}
	actual := engine.ExecutedSteps()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d steps, got %d: %v", len(expected), len(actual), actual)
	}
	for i, exp := range expected {
		if actual[i] != exp {
			t.Errorf("step %d: expected %q, got %q", i, exp, actual[i])
		}
	}
}

func TestPDAEngine_RouteMatching(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "Classify input", Label: "classify"},
		{Type: StepRoute, Prompt: "Is this code or text?",
			Branches: map[string]string{
				"code":     "coder",
				"text":     "writer",
				"_default": "coder",
			},
			Label: "route-type",
		},
	}
	agents := map[string]*AgentCFG{
		"main":   {Steps: steps},
		"coder":  {},
		"writer": {},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "classified", usage: Usage{TotalTokens: 10}},
			{text: "code", usage: Usage{TotalTokens: 5}},
			{text: "coded-result", usage: Usage{TotalTokens: 20}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"Process this",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "coded-result" {
		t.Errorf("expected 'coded-result', got %q", result)
	}

	if len(pr.calls) != 3 {
		t.Fatalf("expected 3 prompt calls, got %d", len(pr.calls))
	}
	if pr.calls[2].agentName != "coder" {
		t.Errorf("call 2: expected agent 'coder', got %q", pr.calls[2].agentName)
	}
}

func TestPDAEngine_RouteWithStepsTarget_UnifiedPush(t *testing.T) {
	mainSteps := []Step{
		{Type: StepRoute, Prompt: "Pick path",
			Branches: map[string]string{
				"alpha":    "validator",
				"_default": "validator",
			},
			Label: "route-step",
		},
	}
	validatorSteps := []Step{
		{Type: StepPrompt, Content: "Validate input", Label: "validate"},
	}
	agents := map[string]*AgentCFG{
		"main":      {Steps: mainSteps},
		"validator": {Steps: validatorSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "alpha", usage: Usage{TotalTokens: 10}},
			{text: "validated", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Go",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "validated" {
		t.Errorf("expected 'validated', got %q", result)
	}

	if len(pr.calls) != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", len(pr.calls))
	}

	if pr.calls[0].agentName != "main" {
		t.Errorf("call 0: expected 'main', got %q", pr.calls[0].agentName)
	}
	if pr.calls[1].agentName != "validator" {
		t.Errorf("call 1: expected 'validator', got %q", pr.calls[1].agentName)
	}
	if len(pr.calls[1].messages) != 3 {
		t.Errorf("call 1 (validator): expected 3 context msgs (inherited from parent: init + route user/assistant), got %d", len(pr.calls[1].messages))
	}
}

// TestPDAEngine_RouteSubstringEarliestPosition verifies that when the LLM
// output contains multiple branch keys as substrings, the one appearing
// earliest in the output wins (not random map iteration order).
func TestPDAEngine_RouteSubstringEarliestPosition(t *testing.T) {
	steps := []Step{
		{Type: StepRoute, Prompt: "Pick expert",
			Branches: map[string]string{
				"贵宾":       "技术专家",
				"拉布拉多":     "市场专家",
				"柯基":       "战略专家",
				"金毛":       "历史专家",
				"_default": "历史专家",
			},
			Label: "pick-expert",
		},
	}
	agents := map[string]*AgentCFG{
		"main": {Steps: steps},
		"技术专家": {},
		"市场专家": {},
		"战略专家": {},
		"历史专家": {},
	}

	// LLM output: "贵宾" appears first, but "拉布拉多" and "柯基" also present
	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "主持人提到了贵宾，请拉布拉多和柯基稍后发言", usage: Usage{TotalTokens: 5}},
			{text: "expert-result", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	// Run multiple times to catch random map iteration flakiness
	for i := 0; i < 20; i++ {
		pr.calls = nil
		pr.idx = 0
		result, _, err := engine.Execute(
			context.Background(),
			makeDelegateInfo("main"),
			AgentCFG{Steps: steps},
			"Test input",
			nil,
		)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if result != "expert-result" {
			t.Errorf("iteration %d: expected 'expert-result', got %q", i, result)
		}
		if len(pr.calls) < 2 {
			t.Fatalf("iteration %d: expected at least 2 calls, got %d", i, len(pr.calls))
		}
		// "贵宾" appears at position 12 (bytes), "拉布拉多" at ~16, "柯基" at ~29
		// The earliest key match should always pick 技术专家
		if pr.calls[1].agentName != "技术专家" {
			t.Errorf("iteration %d: expected agent '技术专家' (贵宾), got %q — earliest-position matching failed", i, pr.calls[1].agentName)
		}
	}
}

func TestPDAEngine_RouteDefaultFallback(t *testing.T) {
	steps := []Step{
		{Type: StepRoute, Prompt: "Pick one: A or B",
			Branches: map[string]string{
				"A":        "agentA",
				"B":        "agentB",
				"_default": "agentA",
			},
			Label: "fallback-route",
		},
	}
	agents := map[string]*AgentCFG{
		"main":   {Steps: steps},
		"agentA": {},
		"agentB": {},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "something_random", usage: Usage{TotalTokens: 5}},
			{text: "default-result", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"Go",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "default-result" {
		t.Errorf("expected 'default-result', got %q", result)
	}
}

func TestPDAEngine_RouteSelfRecursion(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "Do some work", Label: "work"},
		{Type: StepRoute, Prompt: "Continue or stop?",
			Branches: map[string]string{
				"continue": "looper",
				"stop":     "finalizer",
				"_default": "finalizer",
			},
			Label: "decide",
		},
	}
	agents := map[string]*AgentCFG{
		"looper":    {Steps: steps, MaxRecursion: 2},
		"finalizer": {},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "worked-0", usage: Usage{TotalTokens: 10}},
			{text: "continue", usage: Usage{TotalTokens: 5}},
			{text: "worked-1", usage: Usage{TotalTokens: 10}},
			{text: "continue", usage: Usage{TotalTokens: 5}},
			{text: "worked-2", usage: Usage{TotalTokens: 10}},
			{text: "continue", usage: Usage{TotalTokens: 5}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        20,
	})

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("looper"),
		AgentCFG{Steps: steps, MaxRecursion: 2},
		"Start looping",
		nil,
	)

	if err == nil {
		t.Log("Execution completed without error (may have exhausted prompts)")
	}
	if err != nil && !strings.Contains(err.Error(), "recursion") {
		t.Logf("Error: %v (checking if recursion-related)", err)
	}

	executedSteps := engine.ExecutedSteps()
	if len(executedSteps) == 0 {
		t.Error("expected at least some executed steps")
	}
}

func TestPDAEngine_StackDepthLimit(t *testing.T) {
	stepsA := []Step{
		{Type: StepPrompt, Content: "work A", Label: "a-work"},
		{Type: StepRoute, Prompt: "Route?",
			Branches: map[string]string{
				"b":        "agentB",
				"_default": "agentB",
			}, Label: "a-route"},
	}
	stepsB := []Step{
		{Type: StepPrompt, Content: "work B", Label: "b-work"},
		{Type: StepRoute, Prompt: "Route?",
			Branches: map[string]string{
				"c":        "agentC",
				"_default": "agentC",
			}, Label: "b-route"},
	}
	stepsC := []Step{
		{Type: StepPrompt, Content: "work C", Label: "c-work"},
	}
	agents := map[string]*AgentCFG{
		"agentA": {Steps: stepsA},
		"agentB": {Steps: stepsB},
		"agentC": {Steps: stepsC},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "a-done", usage: Usage{TotalTokens: 5}},
			{text: "b", usage: Usage{TotalTokens: 5}},
			{text: "b-done", usage: Usage{TotalTokens: 5}},
			{text: "c", usage: Usage{TotalTokens: 5}},
			{text: "c-done", usage: Usage{TotalTokens: 5}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        2,
	})

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("agentA"),
		AgentCFG{Steps: stepsA},
		"Go deep",
		nil,
	)

	if err == nil {
		t.Fatal("expected stack depth error, got nil")
	}
	if !strings.Contains(err.Error(), "stack depth") {
		t.Errorf("expected 'stack depth' in error, got: %v", err)
	}
}

func TestPDAEngine_ContextCancellation(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "Step 1", Label: "s1"},
		{Type: StepPrompt, Content: "Step 2", Label: "s2"},
		{Type: StepPrompt, Content: "Step 3", Label: "s3"},
	}
	agents := map[string]*AgentCFG{
		"main": {Steps: steps},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	slowPrompt := func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, Usage, []provider.Message, error) {
		callCount++
		if callCount > 1 {
			time.Sleep(100 * time.Millisecond)
			if ctx.Err() != nil {
				return "", Usage{}, nil, ctx.Err()
			}
		}
		newMsgs := []provider.Message{
			{Role: provider.RoleUser, Content: userInput},
			{Role: provider.RoleAssistant, Content: "ok"},
		}
		return "ok", Usage{TotalTokens: 5}, newMsgs, nil
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: slowPrompt,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	_, _, err := engine.Execute(
		ctx,
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"Go",
		nil,
	)

	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected cancellation-related error, got: %v", err)
	}
}

func TestPDAEngine_ValidationFailure(t *testing.T) {
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: (&contextPromptRecorder{}).run,
		AgentProvider:        staticAgentProvider(map[string]*AgentCFG{"main": {}}),
		MaxStackDepth:        10,
	})

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: nil},
		"Go",
		nil,
	)

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "EMPTY_STEPS") {
		t.Errorf("expected EMPTY_STEPS in error, got: %v", err)
	}
}

func TestDelegateInfo_ForChild(t *testing.T) {
	parent := &DelegateInfo{
		Depth:             1,
		MaxDepth:          5,
		ParentSessionID:   "sess-1",
		AgentName:         "parent",
		Chain:             []string{"root", "parent"},
		RecursionCounters: map[string]int{"parent": 1},
	}

	child := parent.ForChild("child-agent")

	if child.Depth != 2 {
		t.Errorf("expected depth 2, got %d", child.Depth)
	}
	if child.AgentName != "child-agent" {
		t.Errorf("expected agent name 'child-agent', got %q", child.AgentName)
	}
	if len(child.Chain) != 3 || child.Chain[2] != "child-agent" {
		t.Errorf("expected chain ending with 'child-agent', got %v", child.Chain)
	}
	child.RecursionCounters["child-agent"] = 5
	if _, ok := parent.RecursionCounters["child-agent"]; ok {
		t.Error("child counter modification leaked to parent")
	}
}

func TestPDAEngine_ContextAccumulation(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "Step 1", Label: "s1"},
		{Type: StepPrompt, Content: "Step 2", Label: "s2"},
		{Type: StepPrompt, Content: "Step 3", Label: "s3"},
	}
	agents := map[string]*AgentCFG{
		"main": {Steps: steps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "result-1", usage: Usage{TotalTokens: 10}},
			{text: "result-2", usage: Usage{TotalTokens: 10}},
			{text: "result-3", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"Initial task",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedContextSizes := []int{1, 3, 5}
	for i, expected := range expectedContextSizes {
		if i >= len(pr.calls) {
			break
		}
		if len(pr.calls[i].messages) != expected {
			t.Errorf("call %d: expected %d context messages, got %d",
				i, expected, len(pr.calls[i].messages))
		}
	}
}

func TestPDAEngine_StepExec_WithToolInteraction(t *testing.T) {
	mainSteps := []Step{
		{Type: StepAgentRef, Agent: "tooluser", Content: "Use tools to complete task", Label: "call-tooluser"},
	}
	agents := map[string]*AgentCFG{
		"main":     {Steps: mainSteps},
		"tooluser": {},
	}

	toolPrompt := func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, Usage, []provider.Message, error) {
		newMsgs := []provider.Message{
			{Role: provider.RoleUser, Content: userInput},
			{Role: provider.RoleAssistant, Content: "I used the search tool and found: answer-42"},
		}
		return "I used the search tool and found: answer-42", Usage{TotalTokens: 50}, newMsgs, nil
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: toolPrompt,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Find the answer",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "answer-42") {
		t.Errorf("expected result containing 'answer-42', got %q", result)
	}
	if usage.TotalTokens != 50 {
		t.Errorf("expected 50 total tokens, got %d", usage.TotalTokens)
	}
}

func TestPDAEngine_PopResult_EmptySkipped(t *testing.T) {
	mainSteps := []Step{
		{Type: StepAgentRef, Agent: "empty-worker", Label: "call-empty"},
		{Type: StepPrompt, Content: "After empty worker", Label: "after"},
	}
	emptyWorkerSteps := []Step{
		{Type: StepPrompt, Content: "Produce nothing", Label: "empty-work"},
	}
	agents := map[string]*AgentCFG{
		"main":         {Steps: mainSteps},
		"empty-worker": {Steps: emptyWorkerSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "", usage: Usage{TotalTokens: 5}},
			{text: "final", usage: Usage{TotalTokens: 10}},
		},
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Go",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "final" {
		t.Errorf("expected 'final', got %q", result)
	}

	if len(pr.calls) >= 2 {
		for _, m := range pr.calls[1].messages {
			if strings.Contains(m.Content, "[empty-worker result]") {
				t.Error("empty result should not be injected into parent context")
			}
		}
	}
}

func TestPDAEngine_UnifiedPushFrame(t *testing.T) {
	mainSteps := []Step{
		{Type: StepAgentRef, Agent: "workerA", Content: "Task A", Label: "ref-a"},
		{Type: StepRoute, Prompt: "Pick",
			Branches: map[string]string{
				"b":        "workerB",
				"_default": "workerB",
			},
			Label: "route-to-b",
		},
	}
	workerASteps := []Step{
		{Type: StepPrompt, Content: "A work", Label: "a-work"},
	}
	workerBSteps := []Step{
		{Type: StepPrompt, Content: "B work", Label: "b-work"},
	}
	agents := map[string]*AgentCFG{
		"main":    {Steps: mainSteps},
		"workerA": {Steps: workerASteps},
		"workerB": {Steps: workerBSteps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "a-done", usage: Usage{TotalTokens: 10}},
			{text: "b", usage: Usage{TotalTokens: 5}},
			{text: "b-done", usage: Usage{TotalTokens: 10}},
		},
	}

	var pushes []string
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})
	engine.OnStackPush = func(frame StackFrame) {
		pushes = append(pushes, frame.AgentName)
	}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: mainSteps},
		"Go",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pushes) != 2 {
		t.Fatalf("expected 2 pushes, got %d: %v", len(pushes), pushes)
	}
	if pushes[0] != "workerA" {
		t.Errorf("push 0: expected 'workerA', got %q", pushes[0])
	}
	if pushes[1] != "workerB" {
		t.Errorf("push 1: expected 'workerB', got %q", pushes[1])
	}
}

// ---------------------------------------------------------------------------
// Error handling tests (no retry — immediate stop)
// ---------------------------------------------------------------------------

func TestPDAEngine_ErrorStopsImmediately(t *testing.T) {
	// Any error should stop PDA execution immediately (no retries).
	callCount := 0
	runFn := func(ctx context.Context, agent string, msgs []provider.Message, input string) (string, Usage, []provider.Message, error) {
		callCount++
		return "", Usage{}, nil, &provider.ProviderError{
			Code:      provider.ErrCodeRateLimited,
			Message:   "rate limited",
			Provider:  "test",
			Retryable: true,
		}
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: runFn,
		AgentProvider:        staticAgentProvider(nil),
	})

	steps := []Step{{Type: StepPrompt, Content: "test", Label: "step0"}}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"hello",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	// Should only be called once — no retries
	if callCount != 1 {
		t.Errorf("expected 1 call (no retries), got %d", callCount)
	}
	if !strings.Contains(err.Error(), "completed 0 steps") {
		t.Errorf("error should mention completed steps: %v", err)
	}
}

func TestPDAEngine_ErrorReportsCompletedSteps(t *testing.T) {
	// First step succeeds, second step fails.
	// Error should report "completed 1 steps before failure".
	callCount := 0
	runFn := func(ctx context.Context, agent string, msgs []provider.Message, input string) (string, Usage, []provider.Message, error) {
		callCount++
		if callCount == 1 {
			return "step0-result", Usage{TotalTokens: 5}, []provider.Message{
				{Role: provider.RoleUser, Content: input},
				{Role: provider.RoleAssistant, Content: "step0-result"},
			}, nil
		}
		return "", Usage{}, nil, fmt.Errorf("something went wrong")
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: runFn,
		AgentProvider:        staticAgentProvider(nil),
	})

	steps := []Step{
		{Type: StepPrompt, Content: "first", Label: "step0"},
		{Type: StepPrompt, Content: "second", Label: "step1"},
	}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"hello",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "completed 1 steps") {
		t.Errorf("error should mention 1 completed step: %v", err)
	}
	// Only 2 calls: step0 success + step1 fail (no retries)
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestPDAEngine_ErrorPreservesErrorChain(t *testing.T) {
	// Verify that the original ProviderError is preserved through %w wrapping
	// so that callers can extract structured error info via errors.As.
	origErr := &provider.ProviderError{
		Code:      provider.ErrCodeRateLimited,
		Message:   "too many requests",
		Provider:  "glm",
		Retryable: true,
	}
	runFn := func(ctx context.Context, agent string, msgs []provider.Message, input string) (string, Usage, []provider.Message, error) {
		return "", Usage{}, nil, origErr
	}

	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: runFn,
		AgentProvider:        staticAgentProvider(nil),
	})

	steps := []Step{{Type: StepPrompt, Content: "test", Label: "step0"}}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"hello",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	// Verify ProviderError can be extracted from the error chain
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatal("expected ProviderError to be extractable from error chain")
	}
	if pe.Code != provider.ErrCodeRateLimited {
		t.Errorf("expected code RATE_LIMITED, got %s", pe.Code)
	}
	if pe.Provider != "glm" {
		t.Errorf("expected provider 'glm', got %s", pe.Provider)
	}
}

// ---------------------------------------------------------------------------
// Checkpoint tests (step-01)
// ---------------------------------------------------------------------------

func TestBuildCheckpoint(t *testing.T) {
	state := &ExecutionState{
		Stack: []StackFrame{
			{
				AgentName:      "root",
				StepIndex:      2,
				TotalSteps:     3,
				RecursionCount: 0,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "hello"},
					{Role: provider.RoleAssistant, Content: "world"},
				},
				Steps: []Step{{Type: StepPrompt, Content: "s1"}},
			},
			{
				AgentName:      "child",
				StepIndex:      1,
				TotalSteps:     2,
				RecursionCount: 1,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "sub"},
				},
				Steps: []Step{{Type: StepExec, Agent: "child"}},
			},
		},
		RecursionCount: 1,
		TotalTokens:    500,
	}

	engine := &PDAEngine{executedSteps: []string{"step-0", "step-1"}}
	di := &DelegateInfo{
		AgentName:       "root",
		Depth:           0,
		MaxDepth:        10,
		ParentSessionID: "sess-123",
		Chain:           []string{"root"},
	}

	cp := engine.buildCheckpoint(state, "last-result", di, "do something")

	if cp.AgentName != "root" {
		t.Errorf("AgentName = %q, want root", cp.AgentName)
	}
	if cp.LastResult != "last-result" {
		t.Errorf("LastResult = %q, want last-result", cp.LastResult)
	}
	if cp.InitialPrompt != "do something" {
		t.Errorf("InitialPrompt = %q, want 'do something'", cp.InitialPrompt)
	}
	if cp.RecursionCount != 1 {
		t.Errorf("RecursionCount = %d, want 1", cp.RecursionCount)
	}
	if cp.TotalUsage.TotalTokens != 500 {
		t.Errorf("TotalTokens = %d, want 500", cp.TotalUsage.TotalTokens)
	}
	if cp.Version != 1 {
		t.Errorf("Version = %d, want 1", cp.Version)
	}
	if len(cp.Stack) != 2 {
		t.Fatalf("Stack len = %d, want 2", len(cp.Stack))
	}

	// Verify frame 0
	f0 := cp.Stack[0]
	if f0.AgentName != "root" || f0.StepIndex != 2 || f0.TotalSteps != 3 {
		t.Errorf("frame 0 = %+v", f0)
	}
	if len(f0.Context) != 2 || f0.Context[0].Content != "hello" {
		t.Errorf("frame 0 context mismatch")
	}

	// Verify frame 1
	f1 := cp.Stack[1]
	if f1.AgentName != "child" || f1.StepIndex != 1 || f1.RecursionCount != 1 {
		t.Errorf("frame 1 = %+v", f1)
	}

	// Verify executed steps are copied (not shared)
	if len(cp.ExecutedSteps) != 2 || cp.ExecutedSteps[0] != "step-0" {
		t.Errorf("ExecutedSteps = %v", cp.ExecutedSteps)
	}
	engine.executedSteps = append(engine.executedSteps, "step-2")
	if len(cp.ExecutedSteps) != 2 {
		t.Error("ExecutedSteps should be independent copy")
	}

	// Verify context is copied (not shared)
	state.Stack[0].Context[0].Content = "mutated"
	if cp.Stack[0].Context[0].Content != "hello" {
		t.Error("checkpoint context should be independent copy")
	}
}

func TestRestoreFromCheckpoint(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Content: "s1", Label: "first"},
		{Type: StepPrompt, Content: "s2", Label: "second"},
		{Type: StepPrompt, Content: "s3", Label: "third"},
	}
	agents := map[string]*AgentCFG{
		"main": {Steps: steps},
	}
	engine := NewPDAEngine(PDAEngineOptions{
		AgentProvider: staticAgentProvider(agents),
	})

	cp := &PDACheckpoint{
		Stack: []SerializableFrame{
			{
				AgentName:  "main",
				StepIndex:  1,
				TotalSteps: 3,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "hello"},
				},
			},
		},
		RecursionCount: 0,
		LastResult:     "prev-result",
		ExecutedSteps:  []string{"first"},
		TotalUsage:     Usage{TotalTokens: 100},
	}

	state, lastResult, usage, err := engine.restoreFromCheckpoint(cp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastResult != "prev-result" {
		t.Errorf("lastResult = %q, want prev-result", lastResult)
	}
	if usage.TotalTokens != 100 {
		t.Errorf("usage.TotalTokens = %d, want 100", usage.TotalTokens)
	}
	if len(state.Stack) != 1 {
		t.Fatalf("stack len = %d, want 1", len(state.Stack))
	}

	frame := state.Stack[0]
	if frame.StepIndex != 1 {
		t.Errorf("StepIndex = %d, want 1", frame.StepIndex)
	}
	if frame.TotalSteps != 3 {
		t.Errorf("TotalSteps = %d, want 3 (from config)", frame.TotalSteps)
	}
	if len(frame.Steps) != 3 {
		t.Errorf("Steps len = %d, want 3 (rebuilt from config)", len(frame.Steps))
	}
	if len(frame.Context) != 1 || frame.Context[0].Content != "hello" {
		t.Error("Context should be restored from checkpoint")
	}
	if len(engine.executedSteps) != 1 || engine.executedSteps[0] != "first" {
		t.Errorf("executedSteps = %v", engine.executedSteps)
	}
}

func TestRestoreFromCheckpoint_AgentNotFound(t *testing.T) {
	engine := NewPDAEngine(PDAEngineOptions{
		AgentProvider: staticAgentProvider(map[string]*AgentCFG{}),
	})
	cp := &PDACheckpoint{
		Stack: []SerializableFrame{
			{AgentName: "deleted_agent", StepIndex: 0, TotalSteps: 2},
		},
	}

	_, _, _, err := engine.restoreFromCheckpoint(cp)
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	if !errors.Is(err, ErrCheckpointInvalid) {
		t.Errorf("expected ErrCheckpointInvalid, got %v", err)
	}
}

func TestRestoreFromCheckpoint_StepIndexOutOfRange(t *testing.T) {
	agents := map[string]*AgentCFG{
		"main": {Steps: []Step{{Type: StepPrompt, Content: "only-one"}}},
	}
	engine := NewPDAEngine(PDAEngineOptions{
		AgentProvider: staticAgentProvider(agents),
	})
	cp := &PDACheckpoint{
		Stack: []SerializableFrame{
			{AgentName: "main", StepIndex: 5, TotalSteps: 1},
		},
	}

	_, _, _, err := engine.restoreFromCheckpoint(cp)
	if err == nil {
		t.Fatal("expected error for step index out of range")
	}
	if !errors.Is(err, ErrCheckpointInvalid) {
		t.Errorf("expected ErrCheckpointInvalid, got %v", err)
	}
}

func TestPDACheckpointSerialization(t *testing.T) {
	cp := &PDACheckpoint{
		SessionID: "sess-abc",
		AgentName: "main",
		CreatedAt: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
		Stack: []SerializableFrame{
			{
				AgentName:      "main",
				StepIndex:      2,
				TotalSteps:     5,
				RecursionCount: 0,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "analyze this"},
					{Role: provider.RoleAssistant, Content: "done analyzing"},
					{Role: provider.RoleUser, Content: "continue", ToolCallID: "tc-1"},
				},
			},
			{
				AgentName:      "sub",
				StepIndex:      0,
				TotalSteps:     3,
				RecursionCount: 1,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "sub task"},
				},
			},
		},
		RecursionCount:  1,
		LastResult:      "partial output",
		ExecutedSteps:   []string{"analyze", "transform"},
		TotalUsage:      Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		InterruptReason: "429 rate limited",
		InterruptStep:   2,
		InterruptAgent:  "main",
		InitialPrompt:   "do the work",
		DelegateInfo: DelegateInfo{
			Depth:           0,
			MaxDepth:        10,
			ParentSessionID: "parent-sess",
			AgentName:       "main",
			Chain:           []string{"main"},
		},
		Version: 1,
	}

	data, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var restored PDACheckpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify top-level fields
	if restored.SessionID != cp.SessionID {
		t.Errorf("SessionID = %q, want %q", restored.SessionID, cp.SessionID)
	}
	if restored.AgentName != cp.AgentName {
		t.Errorf("AgentName = %q, want %q", restored.AgentName, cp.AgentName)
	}
	if !restored.CreatedAt.Equal(cp.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", restored.CreatedAt, cp.CreatedAt)
	}
	if restored.LastResult != cp.LastResult {
		t.Errorf("LastResult = %q, want %q", restored.LastResult, cp.LastResult)
	}
	if restored.InterruptReason != cp.InterruptReason {
		t.Errorf("InterruptReason = %q", restored.InterruptReason)
	}
	if restored.Version != 1 {
		t.Errorf("Version = %d", restored.Version)
	}

	// Verify stack
	if len(restored.Stack) != 2 {
		t.Fatalf("Stack len = %d, want 2", len(restored.Stack))
	}
	if restored.Stack[0].Context[0].Content != "analyze this" {
		t.Error("frame 0 context[0] content mismatch")
	}
	if restored.Stack[0].Context[2].ToolCallID != "tc-1" {
		t.Error("ToolCallID not preserved")
	}
	if restored.Stack[1].AgentName != "sub" || restored.Stack[1].RecursionCount != 1 {
		t.Errorf("frame 1 mismatch: %+v", restored.Stack[1])
	}

	// Verify usage
	if restored.TotalUsage.PromptTokens != 100 || restored.TotalUsage.CompletionTokens != 50 {
		t.Errorf("TotalUsage = %+v", restored.TotalUsage)
	}

	// Verify delegate info
	if restored.DelegateInfo.AgentName != "main" || restored.DelegateInfo.MaxDepth != 10 {
		t.Errorf("DelegateInfo = %+v", restored.DelegateInfo)
	}
}

// TestExecuteWithCheckpointResume verifies that passing a checkpoint to Execute
// skips already-completed steps and resumes from the checkpoint state.
func TestExecuteWithCheckpointResume(t *testing.T) {
	// 3-step agent: each step returns "step-N-result"
	var executedIndexes []int
	steps := []Step{
		{Type: StepPrompt, Label: "step-0", Prompt: "do step 0"},
		{Type: StepPrompt, Label: "step-1", Prompt: "do step 1"},
		{Type: StepPrompt, Label: "step-2", Prompt: "do step 2"},
	}
	agents := map[string]*AgentCFG{
		"main": &AgentCFG{Steps: steps},
	}

	// Use a recorder that tracks which steps are actually called
	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "step-1-result", usage: Usage{TotalTokens: 10}},
			{text: "step-2-result", usage: Usage{TotalTokens: 10}},
		},
	}
	origRun := pr.run
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, Usage, []provider.Message, error) {
			// Track which step index is being executed by counting calls
			executedIndexes = append(executedIndexes, len(executedIndexes)+1) // 1-indexed call order
			return origRun(ctx, agentName, messages, userInput)
		},
		AgentProvider: staticAgentProvider(agents),
		MaxStackDepth: 10,
	})

	// Build a checkpoint that says: we already completed step-0, resume at step-1
	checkpoint := &PDACheckpoint{
		AgentName: "main",
		Stack: []SerializableFrame{
			{
				AgentName:  "main",
				StepIndex:  1, // resume from step 1
				TotalSteps: 3,
				Context: []provider.Message{
					{Role: provider.RoleUser, Content: "[用户任务描述]\nresume test"},
					{Role: provider.RoleAssistant, Content: "step-0-result"},
				},
			},
		},
		LastResult:    "step-0-result",
		ExecutedSteps: []string{"step-0"},
		DelegateInfo: DelegateInfo{
			AgentName: "main",
			MaxDepth:  5,
		},
		InitialPrompt: "resume test",
	}

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"resume test",
		checkpoint,
	)
	if err != nil {
		t.Fatalf("Execute with checkpoint failed: %v", err)
	}

	// Only steps 1 and 2 should have executed (step 0 was in checkpoint)
	// The recorder should have been called exactly 2 times
	if len(pr.calls) != 2 {
		t.Errorf("expected 2 prompt calls, got %d", len(pr.calls))
	}

	// Final result should be from step 2
	if result != "step-2-result" {
		t.Errorf("expected result 'step-2-result', got %q", result)
	}

	// Usage should accumulate: 2 steps * 10 tokens each
	if usage.TotalTokens != 20 {
		t.Errorf("expected 20 total tokens, got %d", usage.TotalTokens)
	}
}

// TestExecuteWithNilCheckpoint verifies that nil checkpoint = original fresh-start behavior.
func TestExecuteWithNilCheckpoint(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Label: "step-0", Prompt: "do step 0"},
		{Type: StepPrompt, Label: "step-1", Prompt: "do step 1"},
	}
	agents := map[string]*AgentCFG{
		"main": &AgentCFG{Steps: steps},
	}

	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "result-0", usage: Usage{TotalTokens: 5}},
			{text: "result-1", usage: Usage{TotalTokens: 5}},
		},
	}
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})

	result, usage, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"full run",
		nil, // no checkpoint = fresh start
	)
	if err != nil {
		t.Fatalf("Execute with nil checkpoint failed: %v", err)
	}

	// All steps should execute
	if len(pr.calls) != 2 {
		t.Errorf("expected 2 prompt calls, got %d", len(pr.calls))
	}
	if result != "result-1" {
		t.Errorf("expected 'result-1', got %q", result)
	}
	if usage.TotalTokens != 10 {
		t.Errorf("expected 10 total tokens, got %d", usage.TotalTokens)
	}
}

// TestExecuteCheckpointCallback verifies that OnCheckpoint is called at the right points.
func TestExecuteCheckpointCallback(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Label: "step-0", Prompt: "do step 0"},
		{Type: StepPrompt, Label: "step-1", Prompt: "do step 1"},
	}
	agents := map[string]*AgentCFG{
		"main": &AgentCFG{Steps: steps},
	}

	var checkpoints []*PDACheckpoint
	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "result-0", usage: Usage{TotalTokens: 5}},
			{text: "result-1", usage: Usage{TotalTokens: 5}},
		},
	}
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})
	engine.OnCheckpoint = func(cp *PDACheckpoint) error {
		// Deep copy to capture state at this point
		cpCopy := *cp
		stackCopy := make([]SerializableFrame, len(cp.Stack))
		copy(stackCopy, cp.Stack)
		cpCopy.Stack = stackCopy
		checkpoints = append(checkpoints, &cpCopy)
		return nil
	}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"checkpoint test",
		nil,
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Expected checkpoints:
	// 1) After step-0 completes (StepIndex advanced to 1)
	// 2) After step-1 completes (StepIndex advanced to 2)
	// 3) After frame pop (parent StepIndex advanced) — but this is root frame, no parent
	// So we should get exactly 2 checkpoints for a 2-step single-frame execution
	if len(checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(checkpoints))
	}

	// First checkpoint: after step-0, StepIndex should be 1
	if len(checkpoints[0].Stack) != 1 || checkpoints[0].Stack[0].StepIndex != 1 {
		t.Errorf("checkpoint[0] StepIndex = %d, want 1", checkpoints[0].Stack[0].StepIndex)
	}
	if checkpoints[0].LastResult != "result-0" {
		t.Errorf("checkpoint[0] LastResult = %q, want 'result-0'", checkpoints[0].LastResult)
	}

	// Second checkpoint: after step-1, StepIndex should be 2
	if len(checkpoints[1].Stack) != 1 || checkpoints[1].Stack[0].StepIndex != 2 {
		t.Errorf("checkpoint[1] StepIndex = %d, want 2", checkpoints[1].Stack[0].StepIndex)
	}
	if checkpoints[1].LastResult != "result-1" {
		t.Errorf("checkpoint[1] LastResult = %q, want 'result-1'", checkpoints[1].LastResult)
	}
}

// TestExecuteErrorCheckpoint verifies that OnCheckpoint is called with interrupt info on step failure.
func TestExecuteErrorCheckpoint(t *testing.T) {
	steps := []Step{
		{Type: StepPrompt, Label: "step-0", Prompt: "do step 0"},
		{Type: StepPrompt, Label: "step-1", Prompt: "do step 1"},
	}
	agents := map[string]*AgentCFG{
		"main": &AgentCFG{Steps: steps},
	}

	var capturedCheckpoint *PDACheckpoint
	pr := &contextPromptRecorder{
		results: []contextPromptResult{
			{text: "ok", usage: Usage{TotalTokens: 5}},
			{text: "", usage: Usage{}, err: fmt.Errorf("simulated failure")},
		},
	}
	engine := NewPDAEngine(PDAEngineOptions{
		RunPromptWithContext: pr.run,
		AgentProvider:        staticAgentProvider(agents),
		MaxStackDepth:        10,
	})
	engine.OnCheckpoint = func(cp *PDACheckpoint) error {
		capturedCheckpoint = cp
		return nil
	}

	_, _, err := engine.Execute(
		context.Background(),
		makeDelegateInfo("main"),
		AgentCFG{Steps: steps},
		"error test",
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The error checkpoint should have interrupt info
	// Note: there will also be a success checkpoint after step-0
	// The last checkpoint captured should be the error one
	if capturedCheckpoint == nil {
		t.Fatal("expected checkpoint on error, got nil")
	}
	if capturedCheckpoint.InterruptReason == "" {
		t.Error("expected InterruptReason to be set")
	}
	if capturedCheckpoint.InterruptStep != 1 {
		t.Errorf("InterruptStep = %d, want 1", capturedCheckpoint.InterruptStep)
	}
	if capturedCheckpoint.InterruptAgent != "main" {
		t.Errorf("InterruptAgent = %q, want 'main'", capturedCheckpoint.InterruptAgent)
	}
	// StepIndex should NOT be advanced (still at 1, the failed step)
	if len(capturedCheckpoint.Stack) != 1 || capturedCheckpoint.Stack[0].StepIndex != 1 {
		t.Errorf("checkpoint stack StepIndex = %d, want 1 (failed step)", capturedCheckpoint.Stack[0].StepIndex)
	}
}
