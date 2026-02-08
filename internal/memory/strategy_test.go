package memory

import (
	"context"
	"testing"
)

func TestWriteStrategy_Constants(t *testing.T) {
	if WriteManual != "manual" {
		t.Errorf("expected WriteManual='manual', got '%s'", WriteManual)
	}
	if WriteAuto != "auto" {
		t.Errorf("expected WriteAuto='auto', got '%s'", WriteAuto)
	}
	if WriteGated != "gated" {
		t.Errorf("expected WriteGated='gated', got '%s'", WriteGated)
	}
}

func TestRuleAction_Constants(t *testing.T) {
	if ActionAllow != "allow" {
		t.Errorf("expected ActionAllow='allow', got '%s'", ActionAllow)
	}
	if ActionDeny != "deny" {
		t.Errorf("expected ActionDeny='deny', got '%s'", ActionDeny)
	}
	if ActionTransform != "transform" {
		t.Errorf("expected ActionTransform='transform', got '%s'", ActionTransform)
	}
}

func TestRuleEngine_AddRule(t *testing.T) {
	engine := NewRuleEngine()

	rule := &GatingRule{
		ID:       "rule-1",
		Name:     "Test Rule",
		Enabled:  true,
		Priority: 10,
		Action:   ActionAllow,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "test",
		},
	}

	err := engine.AddRule(rule)
	if err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	got, exists := engine.GetRule("rule-1")
	if !exists {
		t.Fatal("expected rule to exist")
	}
	if got.Name != "Test Rule" {
		t.Errorf("expected name 'Test Rule', got '%s'", got.Name)
	}
}

func TestRuleEngine_AddRule_EmptyID(t *testing.T) {
	engine := NewRuleEngine()

	rule := &GatingRule{
		Name: "Invalid Rule",
	}

	err := engine.AddRule(rule)
	if err != ErrInvalidRule {
		t.Errorf("expected ErrInvalidRule, got %v", err)
	}
}

func TestRuleEngine_RemoveRule(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:      "rule-1",
		Name:    "Test Rule",
		Enabled: true,
	})

	err := engine.RemoveRule("rule-1")
	if err != nil {
		t.Fatalf("RemoveRule failed: %v", err)
	}

	_, exists := engine.GetRule("rule-1")
	if exists {
		t.Error("expected rule to be removed")
	}
}

func TestRuleEngine_RemoveRule_NotFound(t *testing.T) {
	engine := NewRuleEngine()

	err := engine.RemoveRule("nonexistent")
	if err != ErrRuleNotFound {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

func TestRuleEngine_ListRules_SortByPriority(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{ID: "low", Name: "Low", Priority: 1, Enabled: true})
	engine.AddRule(&GatingRule{ID: "high", Name: "High", Priority: 100, Enabled: true})
	engine.AddRule(&GatingRule{ID: "mid", Name: "Mid", Priority: 50, Enabled: true})

	rules := engine.ListRules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	if rules[0].ID != "high" {
		t.Errorf("expected first rule to be 'high', got '%s'", rules[0].ID)
	}
	if rules[1].ID != "mid" {
		t.Errorf("expected second rule to be 'mid', got '%s'", rules[1].ID)
	}
	if rules[2].ID != "low" {
		t.Errorf("expected third rule to be 'low', got '%s'", rules[2].ID)
	}
}

func TestRuleEngine_Evaluate_Keyword_Allow(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "allow-test",
		Name:     "Allow Test",
		Priority: 10,
		Enabled:  true,
		Action:   ActionAllow,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "important",
		},
	})

	result := engine.Evaluate(context.Background(), "This is important information")
	if !result.Allowed {
		t.Error("expected allowed=true")
	}
	if result.MatchedRule == nil {
		t.Error("expected matched rule")
	}
	if result.MatchedRule.ID != "allow-test" {
		t.Errorf("expected matched rule 'allow-test', got '%s'", result.MatchedRule.ID)
	}
}

func TestRuleEngine_Evaluate_Keyword_Deny(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "deny-spam",
		Name:     "Deny Spam",
		Priority: 10,
		Enabled:  true,
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "spam",
		},
	})

	result := engine.Evaluate(context.Background(), "This is spam content")
	if result.Allowed {
		t.Error("expected allowed=false")
	}
	if result.MatchedRule == nil {
		t.Error("expected matched rule")
	}
}

func TestRuleEngine_Evaluate_Keyword_Regex(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "deny-email",
		Name:     "Deny Email",
		Priority: 10,
		Enabled:  true,
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
		},
	})

	result := engine.Evaluate(context.Background(), "Contact me at test@example.com")
	if result.Allowed {
		t.Error("expected allowed=false for email pattern")
	}
}

func TestRuleEngine_Evaluate_Length_ExceedsMax(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "deny-long",
		Name:     "Deny Long Content",
		Priority: 10,
		Enabled:  true,
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type: ConditionLength,
			Max:  10,
		},
	})

	result := engine.Evaluate(context.Background(), "This is a very long content that exceeds the limit")
	if result.Allowed {
		t.Error("expected allowed=false for long content")
	}
}

func TestRuleEngine_Evaluate_Length_WithinBounds(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "allow-reasonable",
		Name:     "Allow Reasonable Length",
		Priority: 10,
		Enabled:  true,
		Action:   ActionAllow,
		Condition: RuleCondition{
			Type: ConditionLength,
			Min:  5,
			Max:  100,
		},
	})

	result := engine.Evaluate(context.Background(), "Hello world")
	if !result.Allowed {
		t.Error("expected allowed=true for reasonable length")
	}
}

func TestRuleEngine_Evaluate_DisabledRule(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "disabled-rule",
		Name:     "Disabled Rule",
		Priority: 100,
		Enabled:  false, // Disabled
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "test",
		},
	})

	result := engine.Evaluate(context.Background(), "This is a test")
	if !result.Allowed {
		t.Error("expected allowed=true when rule is disabled")
	}
	if result.MatchedRule != nil {
		t.Error("expected no matched rule for disabled rule")
	}
}

func TestRuleEngine_Evaluate_PriorityOrder(t *testing.T) {
	engine := NewRuleEngine()

	// Low priority deny rule
	engine.AddRule(&GatingRule{
		ID:       "deny-low",
		Name:     "Low Priority Deny",
		Priority: 1,
		Enabled:  true,
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "test",
		},
	})

	// High priority allow rule
	engine.AddRule(&GatingRule{
		ID:       "allow-high",
		Name:     "High Priority Allow",
		Priority: 100,
		Enabled:  true,
		Action:   ActionAllow,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "test",
		},
	})

	result := engine.Evaluate(context.Background(), "This is a test")
	if !result.Allowed {
		t.Error("expected allowed=true because allow rule has higher priority")
	}
	if result.MatchedRule.ID != "allow-high" {
		t.Errorf("expected matched rule 'allow-high', got '%s'", result.MatchedRule.ID)
	}
}

func TestRuleEngine_Evaluate_NoMatchingRules(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "deny-spam",
		Name:     "Deny Spam",
		Priority: 10,
		Enabled:  true,
		Action:   ActionDeny,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "spam",
		},
	})

	result := engine.Evaluate(context.Background(), "This is legitimate content")
	if !result.Allowed {
		t.Error("expected allowed=true when no rules match")
	}
	if result.MatchedRule != nil {
		t.Error("expected no matched rule")
	}
}

func TestRuleEngine_Evaluate_Transform(t *testing.T) {
	engine := NewRuleEngine()

	engine.AddRule(&GatingRule{
		ID:       "transform-rule",
		Name:     "Transform Rule",
		Priority: 10,
		Enabled:  true,
		Action:   ActionTransform,
		Condition: RuleCondition{
			Type:    ConditionKeyword,
			Pattern: "transform",
		},
	})

	result := engine.Evaluate(context.Background(), "Content to transform")
	if !result.Allowed {
		t.Error("expected allowed=true for transform action")
	}
	if result.MatchedRule == nil || result.MatchedRule.ID != "transform-rule" {
		t.Error("expected transform rule to be matched")
	}
}
