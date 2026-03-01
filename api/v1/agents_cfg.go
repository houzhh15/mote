package v1

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"mote/internal/config"
	"mote/internal/gateway/handlers"
	"mote/internal/runner/delegate/cfg"
)

// HandleValidateAgentCFG validates the PDA steps configuration for an agent.
// POST /api/v1/agents/{name}/validate-cfg
func (r *Router) HandleValidateAgentCFG(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]

	var body struct {
		Steps        []cfg.Step `json:"steps"`
		MaxRecursion int        `json:"max_recursion"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid JSON body")
		return
	}

	// Build lookup from current config
	currentCfg := config.GetConfig()
	lookup := func(agentName string) ([]cfg.Step, bool) {
		if currentCfg == nil || currentCfg.Agents == nil {
			return nil, false
		}
		agent, ok := currentCfg.Agents[agentName]
		if !ok {
			return nil, false
		}
		return agent.Steps, true
	}

	validator := cfg.NewValidator()
	results := validator.Validate(name, body.Steps, body.MaxRecursion, lookup)

	// Convert to JSON-friendly format
	type jsonResult struct {
		Level     string `json:"level"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		AgentName string `json:"agent_name"`
		StepIndex int    `json:"step_index"`
	}

	out := make([]jsonResult, len(results))
	for i, vr := range results {
		level := "error"
		if vr.Level == cfg.LevelWarning {
			level = "warning"
		}
		out[i] = jsonResult{
			Level:     level,
			Code:      vr.Code,
			Message:   vr.Message,
			AgentName: vr.AgentName,
			StepIndex: vr.StepIndex,
		}
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{"results": out})
}

// HandleSaveAgentDraft saves a draft of the agent's PDA steps configuration.
// POST /api/v1/agents/{name}/draft
func (r *Router) HandleSaveAgentDraft(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]

	currentCfg := config.GetConfig()
	if currentCfg == nil || currentCfg.Agents == nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}
	agent, ok := currentCfg.Agents[name]
	if !ok {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}

	var body struct {
		Steps []cfg.Step `json:"steps"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		handlers.SendError(w, http.StatusBadRequest, handlers.ErrCodeInvalidRequest, "invalid JSON body")
		return
	}

	agent.Draft = &config.AgentDraft{
		Steps:   body.Steps,
		SavedAt: time.Now(),
	}

	if err := config.UpdateAgent(name, agent); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{"name": name, "saved_at": agent.Draft.SavedAt})
}

// HandleDiscardAgentDraft removes the draft from an agent's configuration.
// DELETE /api/v1/agents/{name}/draft
func (r *Router) HandleDiscardAgentDraft(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["name"]

	currentCfg := config.GetConfig()
	if currentCfg == nil || currentCfg.Agents == nil {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}
	agent, ok := currentCfg.Agents[name]
	if !ok {
		handlers.SendError(w, http.StatusNotFound, "NOT_FOUND", "agent not found: "+name)
		return
	}

	agent.Draft = nil

	if err := config.UpdateAgent(name, agent); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{"name": name, "draft_discarded": true})
}
