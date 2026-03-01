package cfg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"mote/internal/provider"
)

// ---------------------------------------------------------------------------
// Dependency interfaces (functional callbacks to avoid circular imports)
// cfg cannot import config, delegate, or runner/types (all lead to config→cfg cycle).
// ---------------------------------------------------------------------------

// Usage tracks token consumption across PDA execution.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AgentCFG is a cfg-local representation of agent configuration.
type AgentCFG struct {
	Steps        []Step
	MaxRecursion int
}

// HasSteps returns whether this agent has structured orchestration steps.
func (a *AgentCFG) HasSteps() bool {
	return len(a.Steps) > 0
}

// DelegateInfo carries delegation chain metadata (mirrors delegate.DelegateContext).
type DelegateInfo struct {
	Depth             int            `json:"depth"`
	MaxDepth          int            `json:"max_depth"`
	ParentSessionID   string         `json:"parent_session_id"`
	AgentName         string         `json:"agent_name"`
	Chain             []string       `json:"chain"`
	RecursionCounters map[string]int `json:"recursion_counters"`
}

// ForChild creates a child DelegateInfo for the given agent name.
func (d *DelegateInfo) ForChild(agentName string) *DelegateInfo {
	newChain := make([]string, len(d.Chain)+1)
	copy(newChain, d.Chain)
	newChain[len(d.Chain)] = agentName

	counters := make(map[string]int, len(d.RecursionCounters))
	for k, v := range d.RecursionCounters {
		counters[k] = v
	}

	return &DelegateInfo{
		Depth:             d.Depth + 1,
		MaxDepth:          d.MaxDepth,
		ParentSessionID:   d.ParentSessionID,
		AgentName:         agentName,
		Chain:             newChain,
		RecursionCounters: counters,
	}
}

// RunPromptWithContextFunc executes a prompt with explicit frame-local context.
// agentName: the agent whose config (system prompt, model, tools) should be used.
// messages:  frame-local context history (previous turns in this frame).
// userInput: current step's user input.
// Returns: (assistant reply text, token usage, new messages to append to frame context, error).
// The returned newMessages typically contains [{user, userInput}, {assistant, reply}].
type RunPromptWithContextFunc func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, Usage, []provider.Message, error)

// routeOnlyCtxKey is a context key indicating the current call is a route decision step.
// The runPromptWithContext implementation should strip complex tools (e.g. delegate)
// and limit iterations so the LLM produces a simple text decision, not tool calls.
type routeOnlyCtxKey struct{}

// WithRouteOnly returns a context flagged as a route-only decision call.
func WithRouteOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, routeOnlyCtxKey{}, true)
}

// IsRouteOnly checks if the context is flagged as route-only.
func IsRouteOnly(ctx context.Context) bool {
	v, _ := ctx.Value(routeOnlyCtxKey{}).(bool)
	return v
}

// pdaManagedCtxKey flags a call as PDA-managed. PDA-managed steps should NOT
// have access to the delegate tool — the PDA engine itself controls delegation
// via route/agent_ref steps. Without this, the LLM in a prompt step could
// call delegate and bypass PDA flow control entirely.
type pdaManagedCtxKey struct{}

// WithPDAManaged returns a context flagged as PDA-managed.
func WithPDAManaged(ctx context.Context) context.Context {
	return context.WithValue(ctx, pdaManagedCtxKey{}, true)
}

// IsPDAManaged checks if the context is flagged as PDA-managed.
func IsPDAManaged(ctx context.Context) bool {
	v, _ := ctx.Value(pdaManagedCtxKey{}).(bool)
	return v
}

// AgentCFGProvider looks up an agent's PDA configuration by name.
type AgentCFGProvider func(name string) (*AgentCFG, bool)

// ---------------------------------------------------------------------------
// Checkpoint types (for PDA state persistence)
// ---------------------------------------------------------------------------

// ErrCheckpointInvalid is returned when a checkpoint cannot be restored
// because the agent configuration has changed since the checkpoint was saved.
var ErrCheckpointInvalid = errors.New("pda: checkpoint invalid, agent config changed")

// CheckpointFunc saves a PDA checkpoint. Defined in cfg package;
// concrete storage implementation is injected from factory layer.
type CheckpointFunc func(checkpoint *PDACheckpoint) error

// PDACheckpoint contains all information needed to resume PDA execution.
type PDACheckpoint struct {
	SessionID       string              `json:"session_id"`
	AgentName       string              `json:"agent_name"`
	CreatedAt       time.Time           `json:"created_at"`
	Stack           []SerializableFrame `json:"stack"`
	RecursionCount  int                 `json:"recursion_count"`
	LastResult      string              `json:"last_result"`
	ExecutedSteps   []string            `json:"executed_steps"`
	TotalUsage      Usage               `json:"total_usage"`
	InterruptReason string              `json:"interrupt_reason"`
	InterruptStep   int                 `json:"interrupt_step"`
	InterruptAgent  string              `json:"interrupt_agent"`
	InitialPrompt   string              `json:"initial_prompt"`
	DelegateInfo    DelegateInfo        `json:"delegate_info"`
	Version         int                 `json:"version"`
}

// SerializableFrame is the serializable version of StackFrame.
// Context is explicitly included (unlike StackFrame where it is json:"-").
// Steps are NOT serialized — they are rebuilt from AgentCFGProvider on restore.
type SerializableFrame struct {
	AgentName      string             `json:"agent_name"`
	StepIndex      int                `json:"step_index"`
	TotalSteps     int                `json:"total_steps"`
	RecursionCount int                `json:"recursion_count"`
	Context        []provider.Message `json:"context"`
}

// ---------------------------------------------------------------------------
// PDA Engine
// ---------------------------------------------------------------------------

// StackFrame represents a frame on the PDA call stack.
// Each frame owns a frame-local Context for LLM context isolation.
// All push scenarios (agent_ref, route, self-recursion) use identical logic.
type StackFrame struct {
	AgentName      string             `json:"agent_name"`
	StepIndex      int                `json:"step_index"`
	TotalSteps     int                `json:"total_steps"`
	RecursionCount int                `json:"recursion_count"`
	Context        []provider.Message `json:"-"` // frame-local LLM context (in-memory only)
	Steps          []Step             `json:"-"` // steps owned by this frame
}

// ExecutionState holds the PDA execution state.
type ExecutionState struct {
	Stack          []StackFrame `json:"stack"`
	RecursionCount int          `json:"recursion_count"` // current agent recursion count
	TotalTokens    int          `json:"total_tokens"`
}

// PDAEngineOptions contains all dependencies for the PDA engine.
type PDAEngineOptions struct {
	RunPromptWithContext RunPromptWithContextFunc
	AgentProvider        AgentCFGProvider
	MaxStackDepth        int // 0 = unlimited
}

// PDAEngine is a pushdown-automaton-based orchestration engine.
// It processes structured step sequences, managing a call stack
// for agent expansion and recursion control.
type PDAEngine struct {
	runPromptWithContext RunPromptWithContextFunc
	agentProvider        AgentCFGProvider
	validator            *Validator
	maxStackDepth        int
	executedSteps        []string // labels of steps executed (for audit)

	// currentStack holds a reference to the live ExecutionState during executeLoop.
	// Callbacks can read it via CurrentStackDepth() / ParentFrames().
	// It is nil outside of executeLoop.
	currentStack *ExecutionState

	// Event callbacks (optional)
	OnStepStart    func(frame StackFrame, step Step)
	OnStepComplete func(frame StackFrame, step Step, result string)
	OnStackPush    func(frame StackFrame)
	OnStackPop     func(frame StackFrame, result string)

	// Checkpoint callback (optional) — called at safe points to persist state.
	OnCheckpoint CheckpointFunc
}

// CurrentStackDepth returns the current PDA stack depth during callback invocation.
// Returns 0 outside of executeLoop or if stack is empty.
func (e *PDAEngine) CurrentStackDepth() int {
	if e.currentStack == nil {
		return 0
	}
	return len(e.currentStack.Stack)
}

// ParentFrames returns stack frames below the top frame (i.e. parent agents).
// Returns nil outside of executeLoop or if there's only one frame.
func (e *PDAEngine) ParentFrames() []StackFrame {
	if e.currentStack == nil || len(e.currentStack.Stack) <= 1 {
		return nil
	}
	// Return frames from bottom to second-to-top (exclude current top)
	parents := make([]StackFrame, len(e.currentStack.Stack)-1)
	copy(parents, e.currentStack.Stack[:len(e.currentStack.Stack)-1])
	return parents
}

// NewPDAEngine creates a new PDA execution engine.
func NewPDAEngine(opts PDAEngineOptions) *PDAEngine {
	return &PDAEngine{
		runPromptWithContext: opts.RunPromptWithContext,
		agentProvider:        opts.AgentProvider,
		validator:            NewValidator(),
		maxStackDepth:        opts.MaxStackDepth,
	}
}

// ExecutedSteps returns labels of steps executed (for audit tracking).
func (e *PDAEngine) ExecutedSteps() []string {
	return e.executedSteps
}

// Execute runs a structured orchestration for an agent.
// initialPrompt is injected as the first user message in the root frame's context.
// checkpoint, if non-nil, resumes execution from a previously saved state.
func (e *PDAEngine) Execute(
	ctx context.Context,
	delegateInfo *DelegateInfo,
	agentCFG AgentCFG,
	initialPrompt string,
	checkpoint *PDACheckpoint,
) (string, Usage, error) {

	var state *ExecutionState
	var lastResult string
	var totalUsage Usage

	if checkpoint != nil {
		// === Resume path: classic PDA configuration restore ===
		var err error
		state, lastResult, totalUsage, err = e.restoreFromCheckpoint(checkpoint)
		if err != nil {
			return "", Usage{}, fmt.Errorf("checkpoint restore failed: %w", err)
		}
		slog.Info("pda: resuming from checkpoint",
			"agent", checkpoint.AgentName,
			"stackDepth", len(state.Stack),
			"executedSteps", len(checkpoint.ExecutedSteps))
	} else {
		// === Fresh start path: validate and initialize ===
		e.executedSteps = nil

		// 1. Validate step configuration
		lookup := func(name string) ([]Step, bool) {
			if ag, ok := e.agentProvider(name); ok {
				return ag.Steps, true
			}
			return nil, false
		}
		if results := e.validator.Validate(
			delegateInfo.AgentName, agentCFG.Steps, agentCFG.MaxRecursion, lookup,
		); hasErrors(results) {
			return "", Usage{}, fmt.Errorf("CFG validation failed: %s", formatErrors(results))
		}

		// 2. Initialize root frame with frame-local context
		taggedPrompt := fmt.Sprintf("[用户任务描述]\n%s", initialPrompt)
		rootFrame := StackFrame{
			AgentName:  delegateInfo.AgentName,
			StepIndex:  0,
			TotalSteps: len(agentCFG.Steps),
			Steps:      agentCFG.Steps,
			Context: []provider.Message{
				{Role: provider.RoleUser, Content: taggedPrompt},
			},
		}

		slog.Info("pda: starting execution with frame-local context",
			"agent", delegateInfo.AgentName,
			"steps", len(agentCFG.Steps))

		// 3. Initialize execution state
		state = &ExecutionState{
			Stack: []StackFrame{rootFrame},
		}
	}

	// 4. Shared main loop (PDA transition function)
	return e.executeLoop(ctx, delegateInfo, state, lastResult, totalUsage, initialPrompt)
}

// executeLoop is the core PDA transition function loop.
// Extracted from Execute() with zero logic changes to the loop body.
// delegateInfo and initialPrompt are passed through for checkpoint building.
func (e *PDAEngine) executeLoop(
	ctx context.Context,
	delegateInfo *DelegateInfo,
	state *ExecutionState,
	lastResult string,
	totalUsage Usage,
	initialPrompt string,
) (string, Usage, error) {
	// Expose stack to callbacks via CurrentStackDepth() / ParentFrames()
	e.currentStack = state
	defer func() { e.currentStack = nil }()

	for len(state.Stack) > 0 {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return lastResult, totalUsage, fmt.Errorf("execution cancelled: %w", err)
		}

		// Stack depth limit
		if e.maxStackDepth > 0 && len(state.Stack) > e.maxStackDepth {
			return lastResult, totalUsage, fmt.Errorf(
				"stack depth %d exceeds max_stack_depth %d",
				len(state.Stack), e.maxStackDepth)
		}

		top := &state.Stack[len(state.Stack)-1]

		// All steps in current frame completed → pop
		if top.StepIndex >= top.TotalSteps {
			poppedFrame := *top // capture before truncation
			if e.OnStackPop != nil {
				e.OnStackPop(poppedFrame, lastResult)
			}
			state.Stack = state.Stack[:len(state.Stack)-1]
			if len(state.Stack) > 0 {
				parentFrame := &state.Stack[len(state.Stack)-1]
				// Unified injection: inject child result into parent frame context (unconditional)
				if lastResult != "" {
					parentFrame.Context = append(parentFrame.Context, provider.Message{
						Role:    provider.RoleAssistant,
						Content: fmt.Sprintf("[%s result]: %s", poppedFrame.AgentName, lastResult),
					})
				}
				parentFrame.StepIndex++

				// Checkpoint after frame pop (parent StepIndex advanced)
				e.saveCheckpoint(state, lastResult, delegateInfo, initialPrompt)
			}
			continue
		}

		// Get current step from frame's own Steps slice (no agentProvider lookup needed)
		if top.StepIndex >= len(top.Steps) {
			return lastResult, totalUsage, fmt.Errorf(
				"step index %d out of range for agent %q (has %d steps)",
				top.StepIndex, top.AgentName, len(top.Steps))
		}
		step := top.Steps[top.StepIndex]

		if e.OnStepStart != nil {
			e.OnStepStart(*top, step)
		}

		slog.Debug("pda: executing step",
			"agent", top.AgentName,
			"step", top.StepIndex,
			"type", step.Type,
			"label", step.Label,
			"stackDepth", len(state.Stack))

		// Track stack depth before execution to detect pushes
		stackLenBefore := len(state.Stack)

		// Execute based on step type
		var stepResult string
		var stepUsage Usage
		var stepErr error

		switch step.Type {
		case StepPrompt:
			stepResult, stepUsage, stepErr = e.executePrompt(ctx, top, step)

		case StepAgentRef:
			stepResult, stepUsage, stepErr = e.executeAgentRef(ctx, state, step, lastResult)

		case StepRoute:
			stepResult, stepUsage, stepErr = e.executeRoute(ctx, delegateInfo, state, step)

		case StepExec:
			stepResult, stepUsage, stepErr = e.executeExec(ctx, top, step)

		default:
			stepErr = fmt.Errorf("unknown step type: %s", step.Type)
		}

		// On any step error, stop PDA execution immediately.
		// Save interrupt checkpoint before returning so state can be recovered.
		if stepErr != nil {
			// Save interrupt checkpoint (StepIndex not advanced — resume retries this step)
			if e.OnCheckpoint != nil {
				cp := e.buildCheckpoint(state, lastResult, delegateInfo, initialPrompt)
				cp.InterruptReason = stepErr.Error()
				cp.InterruptStep = top.StepIndex
				cp.InterruptAgent = top.AgentName
				if cpErr := e.OnCheckpoint(cp); cpErr != nil {
					// Log but don't mask step error — checkpoint failure is secondary
					slog.Warn("pda: interrupt checkpoint save failed",
						"agent", top.AgentName,
						"step", top.StepIndex,
						"error", cpErr)
				} else {
					slog.Info("pda: interrupt checkpoint saved",
						"agent", top.AgentName,
						"step", top.StepIndex,
						"reason", stepErr.Error())
				}
			}

			completedCount := len(e.executedSteps)
			return lastResult, totalUsage, fmt.Errorf(
				"step %d (%s) in agent %q failed (completed %d steps before failure): %w",
				top.StepIndex, step.Type, top.AgentName, completedCount, stepErr)
		}

		// Accumulate usage
		totalUsage.TotalTokens += stepUsage.TotalTokens
		totalUsage.PromptTokens += stepUsage.PromptTokens
		totalUsage.CompletionTokens += stepUsage.CompletionTokens

		// If a new frame was pushed (agent_ref or route expansion),
		// skip step-completion tracking — the pushed frame needs to execute first.
		if len(state.Stack) > stackLenBefore {
			continue
		}

		lastResult = stepResult

		// Track executed step label
		label := step.Label
		if label == "" {
			label = fmt.Sprintf("%s:%d", top.AgentName, top.StepIndex)
		}
		e.executedSteps = append(e.executedSteps, label)

		if e.OnStepComplete != nil {
			e.OnStepComplete(*top, step, stepResult)
		}

		// Advance to next step. For route steps that pushed a frame,
		// this code is unreachable (the 'continue' above skips it).
		// For route steps that completed without push (e.g. _end),
		// we advance normally just like other step types.
		top.StepIndex++

		// Checkpoint after step completion (StepIndex advanced)
		e.saveCheckpoint(state, lastResult, delegateInfo, initialPrompt)
	}

	return lastResult, totalUsage, nil
}

// saveCheckpoint invokes OnCheckpoint if set. Errors are logged but not propagated.
func (e *PDAEngine) saveCheckpoint(state *ExecutionState, lastResult string, delegateInfo *DelegateInfo, initialPrompt string) {
	if e.OnCheckpoint == nil {
		return
	}
	cp := e.buildCheckpoint(state, lastResult, delegateInfo, initialPrompt)
	if err := e.OnCheckpoint(cp); err != nil {
		slog.Warn("pda: checkpoint save failed", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Checkpoint: build & restore
// ---------------------------------------------------------------------------

// buildCheckpoint creates a PDACheckpoint from the current execution state.
func (e *PDAEngine) buildCheckpoint(
	state *ExecutionState,
	lastResult string,
	delegateInfo *DelegateInfo,
	initialPrompt string,
) *PDACheckpoint {
	frames := make([]SerializableFrame, len(state.Stack))
	for i, f := range state.Stack {
		ctxCopy := make([]provider.Message, len(f.Context))
		copy(ctxCopy, f.Context)
		frames[i] = SerializableFrame{
			AgentName:      f.AgentName,
			StepIndex:      f.StepIndex,
			TotalSteps:     f.TotalSteps,
			RecursionCount: f.RecursionCount,
			Context:        ctxCopy,
		}
	}

	stepsCopy := make([]string, len(e.executedSteps))
	copy(stepsCopy, e.executedSteps)

	agentName := ""
	if delegateInfo != nil {
		agentName = delegateInfo.AgentName
	}

	cp := &PDACheckpoint{
		AgentName:      agentName,
		CreatedAt:      time.Now(),
		Stack:          frames,
		RecursionCount: state.RecursionCount,
		LastResult:     lastResult,
		ExecutedSteps:  stepsCopy,
		TotalUsage: Usage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      state.TotalTokens,
		},
		InitialPrompt: initialPrompt,
		Version:       1,
	}
	if delegateInfo != nil {
		cp.DelegateInfo = *delegateInfo
	}
	return cp
}

// restoreFromCheckpoint rebuilds ExecutionState from a PDACheckpoint.
// Steps are rebuilt from AgentCFGProvider; Context is restored from checkpoint.
// Returns ErrCheckpointInvalid if agent config has changed incompatibly.
func (e *PDAEngine) restoreFromCheckpoint(cp *PDACheckpoint) (*ExecutionState, string, Usage, error) {
	frames := make([]StackFrame, len(cp.Stack))
	for i, sf := range cp.Stack {
		agCfg, ok := e.agentProvider(sf.AgentName)
		if !ok {
			return nil, "", Usage{}, fmt.Errorf(
				"%w: agent %q no longer exists (frame %d)",
				ErrCheckpointInvalid, sf.AgentName, i)
		}
		if sf.StepIndex > len(agCfg.Steps) {
			return nil, "", Usage{}, fmt.Errorf(
				"%w: step index %d out of range for agent %q (has %d steps)",
				ErrCheckpointInvalid, sf.StepIndex, sf.AgentName, len(agCfg.Steps))
		}
		frames[i] = StackFrame{
			AgentName:      sf.AgentName,
			StepIndex:      sf.StepIndex,
			TotalSteps:     len(agCfg.Steps),
			RecursionCount: sf.RecursionCount,
			Context:        sf.Context,
			Steps:          agCfg.Steps,
		}
	}

	state := &ExecutionState{
		Stack:          frames,
		RecursionCount: cp.RecursionCount,
		TotalTokens:    cp.TotalUsage.TotalTokens,
	}
	e.executedSteps = cp.ExecutedSteps

	return state, cp.LastResult, cp.TotalUsage, nil
}

// executePrompt runs a prompt step using frame-local context.
func (e *PDAEngine) executePrompt(
	ctx context.Context,
	frame *StackFrame,
	step Step,
) (string, Usage, error) {
	result, usage, newMsgs, err := e.runPromptWithContext(WithPDAManaged(ctx), frame.AgentName, frame.Context, step.Content)
	if err != nil {
		return "", Usage{}, fmt.Errorf("prompt step failed: %w", err)
	}
	// Append new messages to frame context
	frame.Context = append(frame.Context, newMsgs...)
	return result, usage, nil
}

// executeExec runs a StepExec step — a synthesized step for agents without explicit Steps.
// It invokes a full orchestrator loop (with tool support) via RunPromptWithContext.
func (e *PDAEngine) executeExec(
	ctx context.Context,
	frame *StackFrame,
	step Step,
) (string, Usage, error) {
	execInput := step.Prompt
	if execInput == "" {
		execInput = step.Content
	}
	if execInput == "" {
		execInput = fmt.Sprintf("Execute task as %s agent", step.Agent)
	}

	agentName := step.Agent
	if agentName == "" {
		agentName = frame.AgentName
	}

	result, usage, newMsgs, err := e.runPromptWithContext(WithPDAManaged(ctx), agentName, frame.Context, execInput)
	if err != nil {
		return "", Usage{}, fmt.Errorf("exec step failed: %w", err)
	}
	frame.Context = append(frame.Context, newMsgs...)
	return result, usage, nil
}

// executeAgentRef handles an agent_ref step by pushing a new frame.
// Uses the unified pushFrame for both has-steps and no-steps agents.
func (e *PDAEngine) executeAgentRef(
	ctx context.Context,
	state *ExecutionState,
	step Step,
	previousResult string,
) (string, Usage, error) {

	// Build child input from step.Content + previousResult
	childInput := buildChildInput(step.Content, step.Agent, previousResult)

	// Look up target agent
	refCfg, ok := e.agentProvider(step.Agent)
	if !ok {
		return "", Usage{}, fmt.Errorf("agent_ref target %q not found", step.Agent)
	}

	// Determine child steps: use agent's own steps, or synthesize StepExec
	var childSteps []Step
	if refCfg.HasSteps() {
		childSteps = refCfg.Steps
	} else {
		childSteps = []Step{{
			Type:   StepExec,
			Agent:  step.Agent,
			Prompt: childInput,
		}}
	}

	// Build initial context for child frame
	initCtx := []provider.Message{
		{Role: provider.RoleUser, Content: fmt.Sprintf("[用户任务描述]\n%s", childInput)},
	}

	e.pushFrame(state, step.Agent, childSteps, initCtx, 0)

	slog.Info("pda: agent_ref push (unified)",
		"agent", step.Agent,
		"hasSteps", refCfg.HasSteps(),
		"stackDepth", len(state.Stack))

	// Return without result — child frame will execute and produce lastResult.
	return previousResult, Usage{}, nil
}

// executeRoute runs an LLM routing decision and expands the chosen branch.
func (e *PDAEngine) executeRoute(
	ctx context.Context,
	delegateInfo *DelegateInfo,
	state *ExecutionState,
	step Step,
) (string, Usage, error) {
	top := &state.Stack[len(state.Stack)-1]

	// 1. Build route prompt with branch KEYS as selectable options.
	// Branch keys are the human-readable labels (e.g. "继续", "结束",
	// or agent names like "心理学专家") that the LLM should choose from.
	var branchKeys []string
	for key := range step.Branches {
		if key == "_default" || key == "" || strings.HasPrefix(key, "_new_") {
			continue
		}
		branchKeys = append(branchKeys, key)
	}
	// Sort for deterministic prompt
	sort.Strings(branchKeys)

	routePrompt := fmt.Sprintf(
		"%s\n\n[IMPORTANT] You MUST respond with ONLY one of the following options, nothing else: %s",
		step.Prompt, strings.Join(branchKeys, ", "))

	// 2. Execute LLM routing in frame-local context.
	// Use WithRouteOnly to signal that this is a simple decision call —
	// the orchestrator should NOT give the LLM complex tools like delegate.
	routeResult, usage, newMsgs, err := e.runPromptWithContext(WithRouteOnly(ctx), top.AgentName, top.Context, routePrompt)
	if err != nil {
		return "", Usage{}, fmt.Errorf("route LLM call failed: %w", err)
	}
	// Append route decision messages to frame context
	top.Context = append(top.Context, newMsgs...)
	routeResult = strings.TrimSpace(routeResult)

	// 3. Match LLM response against branches.
	// Priority: key exact > key substring > value exact > value substring.
	// Keys are presented to the LLM as selectable options, so key matching
	// takes priority. Value matching is a backward-compat fallback.
	var targetAgent string
	// 3a. Exact match against branch keys
	for key, target := range step.Branches {
		if key == "_default" || key == "" {
			continue
		}
		if strings.EqualFold(key, routeResult) {
			targetAgent = target
			break
		}
	}
	// 3b. Substring match against branch keys — pick the earliest occurrence
	// in routeResult to avoid Go map random iteration order choosing the wrong match.
	if targetAgent == "" {
		upperResult := strings.ToUpper(routeResult)
		bestPos := len(routeResult) + 1
		for key, target := range step.Branches {
			if key == "_default" || key == "" {
				continue
			}
			pos := strings.Index(upperResult, strings.ToUpper(key))
			if pos >= 0 && pos < bestPos {
				bestPos = pos
				targetAgent = target
			}
		}
	}
	// 3c. Exact match against target values (backward compat)
	if targetAgent == "" {
		for _, target := range step.Branches {
			if target == "" {
				continue
			}
			if strings.EqualFold(target, routeResult) {
				targetAgent = target
				break
			}
		}
	}
	// 3d. Substring match against target values — pick the earliest occurrence
	if targetAgent == "" {
		upperResult := strings.ToUpper(routeResult)
		bestPos := len(routeResult) + 1
		for _, target := range step.Branches {
			if target == "" {
				continue
			}
			pos := strings.Index(upperResult, strings.ToUpper(target))
			if pos >= 0 && pos < bestPos {
				bestPos = pos
				targetAgent = target
			}
		}
	}

	// 4. Fallback to _default
	if targetAgent == "" {
		if def, ok := step.Branches["_default"]; ok {
			targetAgent = def
		} else {
			return routeResult, usage, fmt.Errorf(
				"route: no matching branch for %q and no _default", routeResult)
		}
	}

	slog.Info("pda: route decision",
		"result", routeResult,
		"targetAgent", targetAgent,
		"selfAgent", delegateInfo.AgentName)

	// 5. _end marker: terminate route without expansion
	if targetAgent == RouteEndMarker {
		slog.Info("pda: route ended (no expansion)",
			"result", routeResult,
			"agent", delegateInfo.AgentName)
		return routeResult, usage, nil
	}

	// 6. Self-recursion check
	if targetAgent == delegateInfo.AgentName {
		return e.handleSelfRecursion(ctx, delegateInfo, state, routeResult, usage)
	}

	// 7. Expand target agent (unified — same logic as agent_ref)
	return e.expandTargetAgent(state, targetAgent, step, routeResult, usage)
}

// handleSelfRecursion handles a route branch pointing back to the current agent.
func (e *PDAEngine) handleSelfRecursion(
	ctx context.Context,
	delegateInfo *DelegateInfo,
	state *ExecutionState,
	routeResult string,
	usage Usage,
) (string, Usage, error) {
	state.RecursionCount++

	agCfg, ok := e.agentProvider(delegateInfo.AgentName)
	if !ok {
		return routeResult, usage, fmt.Errorf("self agent %q not found", delegateInfo.AgentName)
	}

	maxRec := agCfg.MaxRecursion
	if maxRec <= 0 {
		maxRec = 10 // safety default
	}

	if state.RecursionCount > maxRec {
		return routeResult, usage, fmt.Errorf(
			"recursion limit reached: %d/%d for agent %q",
			state.RecursionCount, maxRec, delegateInfo.AgentName)
	}

	slog.Info("pda: self-recursion (reset)",
		"agent", delegateInfo.AgentName,
		"recursionCount", state.RecursionCount,
		"maxRecursion", maxRec)

	// Reset the current frame's StepIndex instead of pushing a new frame.
	// This avoids nested frames that cause later steps (e.g. summary) to
	// execute once per nesting level. With reset, only one frame exists and
	// the summary runs exactly once when the loop finally exits via _end.
	//
	// StepIndex is set to -1 because the main loop unconditionally increments
	// it after executeRoute returns (the push-detection guard is not triggered
	// since no frame was pushed), so -1 + 1 = 0 → restart from step 0.
	top := &state.Stack[len(state.Stack)-1]
	top.StepIndex = -1
	top.RecursionCount = state.RecursionCount

	return routeResult, usage, nil
}

// expandTargetAgent expands a non-recursive route target agent.
// Uses the unified pushFrame — identical logic for has-steps and no-steps agents.
// The child frame inherits the parent frame's accumulated context so that
// sub-agents can see previous discussion history and other agents' contributions.
func (e *PDAEngine) expandTargetAgent(
	state *ExecutionState,
	targetAgent string,
	routeStep Step,
	routeResult string,
	usage Usage,
) (string, Usage, error) {
	refCfg, ok := e.agentProvider(targetAgent)
	if !ok {
		return routeResult, usage, fmt.Errorf("route target agent %q not found", targetAgent)
	}

	// Build task instruction for the target agent.
	// Use route step's Content field (if set) as the instruction — this allows
	// the config to provide context-setting instructions (e.g. "speak naturally").
	// Do NOT inject routing metadata (route prompt, branch keys) — sub-agents
	// should not see internal routing mechanics.
	var childInput string
	if routeStep.Content != "" {
		childInput = routeStep.Content
	} else {
		childInput = fmt.Sprintf("请以 %s 的身份执行任务", targetAgent)
	}

	// Determine child steps
	var childSteps []Step
	if refCfg.HasSteps() {
		childSteps = refCfg.Steps
	} else {
		childSteps = []Step{{
			Type:   StepExec,
			Agent:  targetAgent,
			Prompt: childInput,
		}}
	}

	// Inherit parent frame's context so the sub-agent can see previous
	// discussion history and other agents' contributions.
	parentFrame := &state.Stack[len(state.Stack)-1]
	initCtx := make([]provider.Message, len(parentFrame.Context))
	copy(initCtx, parentFrame.Context)
	e.pushFrame(state, targetAgent, childSteps, initCtx, 0)

	return routeResult, usage, nil
}

// pushFrame is the single unified entry point for all push scenarios.
// agent_ref (has-steps / no-steps), route expansion, and self-recursion
// all go through this function with identical logic.
func (e *PDAEngine) pushFrame(
	state *ExecutionState,
	agentName string,
	steps []Step,
	initialContext []provider.Message,
	recursionCount int,
) {
	newFrame := StackFrame{
		AgentName:      agentName,
		StepIndex:      0,
		TotalSteps:     len(steps),
		Steps:          steps,
		Context:        initialContext,
		RecursionCount: recursionCount,
	}
	if e.OnStackPush != nil {
		e.OnStackPush(newFrame)
	}
	state.Stack = append(state.Stack, newFrame)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// buildChildInput constructs the input prompt for a child agent from step content
// and the previous step's result.
func buildChildInput(stepContent, agentName, previousResult string) string {
	var parts []string
	if stepContent != "" {
		parts = append(parts, stepContent)
	}
	if previousResult != "" {
		parts = append(parts, fmt.Sprintf("[上一步结果参考]\n%s", previousResult))
	}
	result := strings.Join(parts, "\n\n")
	if result == "" {
		result = fmt.Sprintf("Execute task as %s agent", agentName)
	}
	return result
}

// hasErrors checks if any validation result is an error.
func hasErrors(results []ValidationResult) bool {
	for _, r := range results {
		if r.Level == LevelError {
			return true
		}
	}
	return false
}

// formatErrors formats validation errors into a readable string.
func formatErrors(results []ValidationResult) string {
	var errs []string
	for _, r := range results {
		if r.Level == LevelError {
			errs = append(errs, fmt.Sprintf("[%s] %s (step %d)", r.Code, r.Message, r.StepIndex))
		}
	}
	return strings.Join(errs, "; ")
}
