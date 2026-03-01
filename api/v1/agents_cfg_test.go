package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"

	"mote/internal/config"
	"mote/internal/runner/delegate/cfg"
)

// setupConfigWithAgents initialises a temp config file with the given agents,
// loads it via config.Load, and returns a cleanup function.
func setupConfigWithAgents(t *testing.T, agents map[string]config.AgentConfig) func() {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Build a minimal YAML config
	cfgMap := map[string]any{
		"agents": agents,
		"provider": map[string]any{
			"default": "copilot",
		},
	}
	raw, _ := json.Marshal(cfgMap)
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	viper.Reset()
	if _, err := config.Load(cfgPath); err != nil {
		t.Fatalf("load config: %v", err)
	}

	return func() {
		viper.Reset()
	}
}

// routerForAgents creates a mux router with the agent-CFG routes registered.
func routerForAgents(t *testing.T) (*Router, *mux.Router) {
	t.Helper()
	r := NewRouter(nil)
	m := mux.NewRouter()
	r.RegisterRoutes(m)
	return r, m
}

// ================================================================
// Validate-CFG endpoint tests
// ================================================================

func TestHandleValidateAgentCFG_ValidConfig(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"writer":   {Description: "writes stuff"},
		"reviewer": {Description: "reviews stuff"},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{
			{"type": "prompt", "label": "greet", "content": "Say hello"},
			{"type": "agent_ref", "label": "delegate", "agent": "reviewer"},
		},
		"max_recursion": 0,
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/writer/validate-cfg", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatal("expected results array")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 validation errors, got %d: %v", len(results), results)
	}
}

func TestHandleValidateAgentCFG_EmptySteps(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"writer": {},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps":         []map[string]any{},
		"max_recursion": 0,
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/writer/validate-cfg", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	results := resp["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(results))
	}

	first := results[0].(map[string]any)
	if first["code"] != "EMPTY_STEPS" {
		t.Errorf("expected EMPTY_STEPS error, got %s", first["code"])
	}
	if first["level"] != "error" {
		t.Errorf("expected error level, got %s", first["level"])
	}
}

func TestHandleValidateAgentCFG_MissingAgentRef(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"writer": {},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{
			{"type": "agent_ref", "label": "delegate", "agent": "nonexistent"},
		},
		"max_recursion": 0,
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/writer/validate-cfg", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	results := resp["results"].([]any)
	found := false
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["code"] == "MISSING_AGENT_REF" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MISSING_AGENT_REF error in results: %v", results)
	}
}

func TestHandleValidateAgentCFG_RouteWithSelfRecursion(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"looper":   {},
		"fallback": {},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{
			{
				"type":   "route",
				"label":  "decision",
				"prompt": "Decide where to go",
				"branches": map[string]string{
					"retry":    "looper",
					"_default": "fallback",
				},
			},
		},
		"max_recursion": 0,
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/looper/validate-cfg", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	results := resp["results"].([]any)
	found := false
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["code"] == "SELF_ROUTE_NO_LIMIT" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SELF_ROUTE_NO_LIMIT warning, got: %v", results)
	}
}

func TestHandleValidateAgentCFG_RouteWithMaxRecursion(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"looper":   {},
		"fallback": {},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{
			{
				"type":   "route",
				"label":  "retry-loop",
				"prompt": "Check result quality",
				"branches": map[string]string{
					"retry":    "looper",
					"_default": "fallback",
				},
			},
		},
		"max_recursion": 3,
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/looper/validate-cfg", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	results := resp["results"].([]any)
	// With max_recursion set, there should be no SELF_ROUTE_NO_LIMIT warning
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["code"] == "SELF_ROUTE_NO_LIMIT" {
			t.Error("should not have SELF_ROUTE_NO_LIMIT when max_recursion is set")
		}
	}
}

func TestHandleValidateAgentCFG_InvalidJSON(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{})
	defer cleanup()

	_, m := routerForAgents(t)

	req := httptest.NewRequest("POST", "/api/v1/agents/test/validate-cfg", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ================================================================
// Draft endpoint tests
// ================================================================

func TestHandleSaveAgentDraft_Success(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"writer": {Description: "test agent"},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{
			{"type": "prompt", "label": "step1", "content": "Do something"},
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/writer/draft", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["name"] != "writer" {
		t.Errorf("expected name=writer, got %v", resp["name"])
	}
	if _, ok := resp["saved_at"]; !ok {
		t.Error("expected saved_at in response")
	}

	// Verify draft is persisted in config
	currentCfg := config.GetConfig()
	agent, ok := currentCfg.Agents["writer"]
	if !ok {
		t.Fatal("agent 'writer' not found in config")
	}
	if agent.Draft == nil {
		t.Fatal("expected draft to be set")
	}
	if len(agent.Draft.Steps) != 1 {
		t.Errorf("expected 1 draft step, got %d", len(agent.Draft.Steps))
	}
	if agent.Draft.Steps[0].Type != cfg.StepPrompt {
		t.Errorf("expected step type prompt, got %s", agent.Draft.Steps[0].Type)
	}
}

func TestHandleSaveAgentDraft_AgentNotFound(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{})
	defer cleanup()

	_, m := routerForAgents(t)

	body := map[string]any{
		"steps": []map[string]any{},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/agents/nonexistent/draft", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandleDiscardAgentDraft_Success(t *testing.T) {
	draft := &config.AgentDraft{
		Steps: []cfg.Step{{Type: cfg.StepPrompt, Label: "old", Content: "old content"}},
	}
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"writer": {Description: "test", Draft: draft},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	req := httptest.NewRequest("DELETE", "/api/v1/agents/writer/draft", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["draft_discarded"] != true {
		t.Errorf("expected draft_discarded=true, got %v", resp["draft_discarded"])
	}

	// Verify draft is cleared
	currentCfg := config.GetConfig()
	agent := currentCfg.Agents["writer"]
	if agent.Draft != nil {
		t.Error("expected draft to be nil after discard")
	}
}

func TestHandleDiscardAgentDraft_AgentNotFound(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{})
	defer cleanup()

	_, m := routerForAgents(t)

	req := httptest.NewRequest("DELETE", "/api/v1/agents/nonexistent/draft", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// ================================================================
// Route registration test
// ================================================================

func TestRouter_PDARoutes_Registered(t *testing.T) {
	_, m := routerForAgents(t)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/agents/testAgent/validate-cfg"},
		{"POST", "/api/v1/agents/testAgent/draft"},
		{"DELETE", "/api/v1/agents/testAgent/draft"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			match := &mux.RouteMatch{}
			if !m.Match(req, match) {
				t.Errorf("route %s %s not registered", rt.method, rt.path)
			}
		})
	}
}

// ================================================================
// Legacy agent CRUD regression (no steps = behavior unchanged)
// ================================================================

func TestHandleListAgents_NoSteps_Regression(t *testing.T) {
	cleanup := setupConfigWithAgents(t, map[string]config.AgentConfig{
		"simple": {Description: "A simple agent", Model: "gpt-4"},
	})
	defer cleanup()

	_, m := routerForAgents(t)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	agents := resp["agents"].(map[string]any)
	simple := agents["simple"].(map[string]any)

	if simple["description"] != "A simple agent" {
		t.Errorf("expected description 'A simple agent', got %v", simple["description"])
	}
	// No steps field should be present or empty
	if steps, ok := simple["steps"]; ok && steps != nil {
		stepsArr, isArr := steps.([]any)
		if isArr && len(stepsArr) > 0 {
			t.Error("simple agent should not have steps")
		}
	}
}
