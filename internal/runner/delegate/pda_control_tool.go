package delegate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mote/internal/runner/delegate/cfg"
	"mote/internal/storage"
	"mote/internal/tools"
)

// PDAResumeFunc is a callback that actually executes (resumes) the PDA pipeline.
// It is injected by the runner when the pda_control tool is created.
// Returns the final PDA result string, or an error.
type PDAResumeFunc func(ctx context.Context) (string, error)

// PDAControlTool is a dynamically injected tool that allows the LLM to
// decide how to handle a previously interrupted PDA execution.
// It is only registered when a checkpoint exists for the current session.
type PDAControlTool struct {
	tools.BaseTool
	store      *storage.DB
	sessionID  string
	checkpoint *cfg.PDACheckpoint
	resumeFn   PDAResumeFunc // injected callback to actually resume PDA
}

// PDAControlToolOptions groups the parameters for NewPDAControlTool.
type PDAControlToolOptions struct {
	Store      *storage.DB
	SessionID  string
	Checkpoint *cfg.PDACheckpoint
	ResumeFn   PDAResumeFunc // optional; if nil, "continue" returns text-only hint
}

// NewPDAControlTool creates a pda_control tool pre-loaded with checkpoint info.
func NewPDAControlTool(opts PDAControlToolOptions) *PDAControlTool {
	desc := buildPDAControlDescription(opts.Checkpoint)
	return &PDAControlTool{
		BaseTool: tools.BaseTool{
			ToolName:        "pda_control",
			ToolDescription: desc,
			ToolParameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"continue", "restart"},
						"description": "How to handle the interrupted workflow. 'continue' resumes from the checkpoint. 'restart' clears the checkpoint and starts fresh.",
					},
				},
				"required": []string{"action"},
			},
		},
		store:      opts.Store,
		sessionID:  opts.SessionID,
		checkpoint: opts.Checkpoint,
		resumeFn:   opts.ResumeFn,
	}
}

// Execute handles the pda_control action.
func (t *PDAControlTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	action, _ := args["action"].(string)

	switch action {
	case "continue":
		slog.Info("pda_control: resuming PDA from checkpoint",
			"agent", t.checkpoint.AgentName,
			"interruptStep", t.checkpoint.InterruptStep,
			"sessionID", t.sessionID)

		if t.resumeFn == nil {
			// Fallback: no resume callback wired — return text hint
			slog.Warn("pda_control: resumeFn not set, cannot resume PDA directly",
				"sessionID", t.sessionID)
			return tools.NewSuccessResult(fmt.Sprintf(
				"PDA checkpoint retained for agent %q. Use @%s to re-invoke the agent and resume from step %d.",
				t.checkpoint.AgentName, t.checkpoint.AgentName, t.checkpoint.InterruptStep)), nil
		}

		// Actually execute (resume) the PDA pipeline — this may take minutes.
		startTime := time.Now()
		result, err := t.resumeFn(ctx)
		duration := time.Since(startTime)

		if err != nil {
			slog.Warn("pda_control: PDA resume failed",
				"agent", t.checkpoint.AgentName,
				"sessionID", t.sessionID,
				"duration", duration,
				"error", err)
			return tools.NewErrorResult(fmt.Sprintf(
				"PDA resume for agent %q failed after %s: %v",
				t.checkpoint.AgentName, duration.Round(time.Millisecond), err)), nil
		}

		slog.Info("pda_control: PDA resumed successfully",
			"agent", t.checkpoint.AgentName,
			"sessionID", t.sessionID,
			"duration", duration,
			"resultLen", len(result))

		return tools.ToolResult{
			Content: fmt.Sprintf("[PDA Resumed — Agent: %s | Duration: %s]\n\n%s",
				t.checkpoint.AgentName, duration.Round(time.Millisecond), result),
			Metadata: map[string]any{
				"pda_resumed": true,
				"agent":       t.checkpoint.AgentName,
				"duration_ms": duration.Milliseconds(),
			},
		}, nil

	case "restart":
		slog.Info("pda_control: user chose to restart, clearing checkpoint",
			"agent", t.checkpoint.AgentName,
			"sessionID", t.sessionID)

		if err := ClearPDACheckpoint(t.store, t.sessionID); err != nil {
			slog.Warn("pda_control: failed to clear checkpoint", "error", err)
			return tools.NewErrorResult(fmt.Sprintf("Failed to clear checkpoint: %v", err)), nil
		}

		return tools.NewSuccessResult(fmt.Sprintf(
			"PDA checkpoint cleared for agent %q. The workflow will start fresh on the next invocation.",
			t.checkpoint.AgentName)), nil

	default:
		return tools.NewErrorResult(fmt.Sprintf(
			"Unknown action %q. Use 'continue' or 'restart'.", action)), nil
	}
}

// buildPDAControlDescription generates a dynamic description containing checkpoint details.
func buildPDAControlDescription(cp *cfg.PDACheckpoint) string {
	var sb strings.Builder
	sb.WriteString("Control a previously interrupted PDA workflow. ")
	sb.WriteString(fmt.Sprintf("Agent: %s. ", cp.AgentName))

	if cp.InterruptStep >= 0 {
		sb.WriteString(fmt.Sprintf("Interrupted at step %d", cp.InterruptStep))
		if cp.InterruptAgent != "" && cp.InterruptAgent != cp.AgentName {
			sb.WriteString(fmt.Sprintf(" (agent: %s)", cp.InterruptAgent))
		}
		sb.WriteString(". ")
	}

	if cp.InterruptReason != "" {
		reason := cp.InterruptReason
		if len(reason) > 200 {
			reason = reason[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("Reason: %s. ", reason))
	}

	if len(cp.ExecutedSteps) > 0 {
		sb.WriteString(fmt.Sprintf("Completed steps: %s. ", strings.Join(cp.ExecutedSteps, ", ")))
	}

	sb.WriteString("Actions: 'continue' (resume from checkpoint) or 'restart' (clear and start over).")
	return sb.String()
}

// PDAResumeHint generates a system prompt hint for checkpoint awareness.
func PDAResumeHint(cp *cfg.PDACheckpoint) string {
	var sb strings.Builder
	sb.WriteString("\n\n[IMPORTANT — PDA Checkpoint Detected]\n")
	sb.WriteString(fmt.Sprintf("Agent %q was interrupted at step %d", cp.AgentName, cp.InterruptStep))
	if cp.InterruptReason != "" {
		reason := cp.InterruptReason
		if len(reason) > 300 {
			reason = reason[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf(": %s", reason))
	}
	sb.WriteString(".\n")
	if len(cp.ExecutedSteps) > 0 {
		sb.WriteString(fmt.Sprintf("Completed: %s.\n", strings.Join(cp.ExecutedSteps, ", ")))
	}
	sb.WriteString("YOU MUST call the pda_control tool IMMEDIATELY with action='continue' to resume the workflow, or action='restart' to start over.\n")
	sb.WriteString("Do NOT respond with text. Call pda_control first.\n")
	return sb.String()
}
