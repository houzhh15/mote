package cfg

import "testing"

// mockLookup creates an AgentLookup from a map of agent names to Steps.
func mockLookup(agents map[string][]Step) AgentLookup {
	return func(name string) ([]Step, bool) {
		s, ok := agents[name]
		return s, ok
	}
}

// --- R1 ---

func TestValidate_EmptySteps(t *testing.T) {
	v := NewValidator()
	results := v.Validate("myAgent", nil, 0, mockLookup(nil))
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Code != "EMPTY_STEPS" || r.Level != LevelError {
		t.Errorf("expected EMPTY_STEPS error, got %s level=%d", r.Code, r.Level)
	}
}

// --- R2 ---

func TestValidate_MissingAgentRef(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepAgentRef, Agent: "nonexistent"},
	}
	results := v.Validate("myAgent", steps, 0, mockLookup(map[string][]Step{}))

	found := false
	for _, r := range results {
		if r.Code == "MISSING_AGENT_REF" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected MISSING_AGENT_REF error not found")
	}
}

// --- R3 ---

func TestValidate_EmptyRouteBranches(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepRoute, Branches: nil},
	}
	results := v.Validate("myAgent", steps, 0, mockLookup(nil))

	found := false
	for _, r := range results {
		if r.Code == "EMPTY_ROUTE_BRANCHES" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EMPTY_ROUTE_BRANCHES error not found")
	}
}

func TestValidate_RouteTargetNotFound(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepRoute, Branches: map[string]string{
			"yes":      "ghost_agent",
			"_default": "myAgent",
		}},
	}
	results := v.Validate("myAgent", steps, 5, mockLookup(map[string][]Step{}))

	found := false
	for _, r := range results {
		if r.Code == "ROUTE_TARGET_NOT_FOUND" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ROUTE_TARGET_NOT_FOUND error not found")
	}
}

func TestValidate_MissingDefaultRoute(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepRoute, Branches: map[string]string{
			"yes": "other",
		}},
	}
	lookup := mockLookup(map[string][]Step{
		"other": {{Type: StepPrompt, Content: "hi"}},
	})
	results := v.Validate("myAgent", steps, 0, lookup)

	found := false
	for _, r := range results {
		if r.Code == "MISSING_DEFAULT_ROUTE" && r.Level == LevelWarning {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected MISSING_DEFAULT_ROUTE warning not found")
	}
}

// --- R4 ---

func TestValidate_SelfRouteNoLimit(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepRoute, Branches: map[string]string{
			"continue": "myAgent",
			"_default": "myAgent",
		}},
	}
	results := v.Validate("myAgent", steps, 0, mockLookup(nil))

	found := false
	for _, r := range results {
		if r.Code == "SELF_ROUTE_NO_LIMIT" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SELF_ROUTE_NO_LIMIT error not found")
	}
}

func TestValidate_ExcessiveRecursion(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepRoute, Branches: map[string]string{
			"continue": "myAgent",
			"_default": "myAgent",
		}},
	}
	results := v.Validate("myAgent", steps, 200, mockLookup(nil))

	found := false
	for _, r := range results {
		if r.Code == "EXCESSIVE_RECURSION" && r.Level == LevelWarning {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EXCESSIVE_RECURSION warning not found")
	}
}

// --- R5 ---

func TestValidate_CyclicDeps(t *testing.T) {
	v := NewValidator()

	// A references B, B references A
	stepsA := []Step{{Type: StepAgentRef, Agent: "agentB"}}
	stepsB := []Step{{Type: StepAgentRef, Agent: "agentA"}}

	lookup := mockLookup(map[string][]Step{
		"agentA": stepsA,
		"agentB": stepsB,
	})
	results := v.Validate("agentA", stepsA, 0, lookup)

	found := false
	for _, r := range results {
		if r.Code == "CYCLIC_DEPENDENCY" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CYCLIC_DEPENDENCY error not found")
	}
}

// --- Happy path ---

func TestValidate_ValidConfig(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepPrompt, Content: "Analyze input"},
		{Type: StepAgentRef, Agent: "helper"},
		{Type: StepPrompt, Content: "Summarize"},
	}
	lookup := mockLookup(map[string][]Step{
		"helper": {{Type: StepPrompt, Content: "help"}},
	})
	results := v.Validate("main", steps, 0, lookup)

	for _, r := range results {
		if r.Level == LevelError {
			t.Errorf("unexpected error: %s - %s", r.Code, r.Message)
		}
	}
}

func TestValidate_ValidConfig_WithRoute(t *testing.T) {
	v := NewValidator()
	steps := []Step{
		{Type: StepPrompt, Content: "Classify input"},
		{Type: StepRoute, Prompt: "What type is it?",
			Branches: map[string]string{
				"code":     "coder",
				"text":     "writer",
				"_default": "coder",
			}},
	}
	lookup := mockLookup(map[string][]Step{
		"coder":  {{Type: StepPrompt, Content: "code it"}},
		"writer": {{Type: StepPrompt, Content: "write it"}},
	})
	results := v.Validate("router", steps, 0, lookup)

	for _, r := range results {
		if r.Level == LevelError {
			t.Errorf("unexpected error: %s - %s", r.Code, r.Message)
		}
	}
}
