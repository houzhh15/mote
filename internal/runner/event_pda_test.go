package runner

import (
	"encoding/json"
	"testing"

	"mote/internal/runner/types"
)

func TestEventTypePDAProgressString(t *testing.T) {
	if got := EventTypePDAProgress.String(); got != "pda_progress" {
		t.Errorf("EventTypePDAProgress.String() = %q, want %q", got, "pda_progress")
	}
}

func TestFromTypesEvent_PDAProgress(t *testing.T) {
	te := types.Event{
		Type: types.EventTypePDAProgress,
		PDAProgress: &types.PDAProgressEvent{
			AgentName:     "researcher",
			StepIndex:     2,
			TotalSteps:    5,
			StepLabel:     "Gather data",
			StepType:      "delegate",
			Phase:         "started",
			StackDepth:    1,
			ExecutedSteps: []string{"step-0", "step-1"},
			TotalTokens:   1234,
		},
	}

	got := FromTypesEvent(te)

	if got.Type != EventTypePDAProgress {
		t.Fatalf("Type = %v, want EventTypePDAProgress", got.Type)
	}
	if got.PDAProgress == nil {
		t.Fatal("PDAProgress is nil")
	}
	p := got.PDAProgress
	if p.AgentName != "researcher" {
		t.Errorf("AgentName = %q, want %q", p.AgentName, "researcher")
	}
	if p.StepIndex != 2 {
		t.Errorf("StepIndex = %d, want 2", p.StepIndex)
	}
	if p.TotalSteps != 5 {
		t.Errorf("TotalSteps = %d, want 5", p.TotalSteps)
	}
	if p.StepLabel != "Gather data" {
		t.Errorf("StepLabel = %q, want %q", p.StepLabel, "Gather data")
	}
	if p.StepType != "delegate" {
		t.Errorf("StepType = %q, want %q", p.StepType, "delegate")
	}
	if p.Phase != "started" {
		t.Errorf("Phase = %q, want %q", p.Phase, "started")
	}
	if p.StackDepth != 1 {
		t.Errorf("StackDepth = %d, want 1", p.StackDepth)
	}
	if len(p.ExecutedSteps) != 2 || p.ExecutedSteps[0] != "step-0" {
		t.Errorf("ExecutedSteps = %v, want [step-0, step-1]", p.ExecutedSteps)
	}
	if p.TotalTokens != 1234 {
		t.Errorf("TotalTokens = %d, want 1234", p.TotalTokens)
	}
}

func TestFromTypesEvent_PDAProgressNil(t *testing.T) {
	te := types.Event{
		Type:        types.EventTypePDAProgress,
		PDAProgress: nil,
	}

	got := FromTypesEvent(te)

	if got.Type != EventTypePDAProgress {
		t.Fatalf("Type = %v, want EventTypePDAProgress", got.Type)
	}
	if got.PDAProgress != nil {
		t.Error("PDAProgress should be nil when source is nil")
	}
}

func TestPDAProgressEventJSON(t *testing.T) {
	ev := PDAProgressEvent{
		AgentName:     "writer",
		StepIndex:     0,
		TotalSteps:    3,
		StepLabel:     "Draft outline",
		StepType:      "delegate",
		Phase:         "completed",
		StackDepth:    0,
		ExecutedSteps: []string{"Draft outline"},
		TotalTokens:   500,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got PDAProgressEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.AgentName != ev.AgentName {
		t.Errorf("AgentName = %q, want %q", got.AgentName, ev.AgentName)
	}
	if got.Phase != ev.Phase {
		t.Errorf("Phase = %q, want %q", got.Phase, ev.Phase)
	}
	if got.StepIndex != ev.StepIndex {
		t.Errorf("StepIndex = %d, want %d", got.StepIndex, ev.StepIndex)
	}
	if len(got.ExecutedSteps) != 1 || got.ExecutedSteps[0] != "Draft outline" {
		t.Errorf("ExecutedSteps = %v, want [Draft outline]", got.ExecutedSteps)
	}
}
