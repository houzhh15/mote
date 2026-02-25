package v1

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"mote/internal/gateway/handlers"
	"mote/internal/policy"
	"mote/internal/policy/approval"
)

// PolicyStatusResponse represents the policy status.
type PolicyStatusResponse struct {
	DefaultAllow        bool   `json:"default_allow"`
	RequireApproval     bool   `json:"require_approval"`
	BlocklistCount      int    `json:"blocklist_count"`
	AllowlistCount      int    `json:"allowlist_count"`
	DangerousRulesCount int    `json:"dangerous_rules_count"`
	ParamRulesCount     int    `json:"param_rules_count"`
	Message             string `json:"message,omitempty"`
}

// PolicyCheckRequest represents a request to check a tool call.
type PolicyCheckRequest struct {
	Tool      string `json:"tool"`
	Arguments string `json:"arguments"`
	SessionID string `json:"session_id,omitempty"`
}

// PolicyCheckResponse represents the result of a policy check.
type PolicyCheckResponse struct {
	Tool            string   `json:"tool"`
	Allowed         bool     `json:"allowed"`
	RequireApproval bool     `json:"require_approval"`
	Blocked         bool     `json:"blocked"`
	Reason          string   `json:"reason"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ApprovalListResponse represents the list of pending approvals.
type ApprovalListResponse struct {
	Pending []*approval.ApprovalRequest `json:"pending"`
	Count   int                         `json:"count"`
}

// ApprovalRespondRequest represents a request to respond to an approval.
type ApprovalRespondRequest struct {
	Approved          bool   `json:"approved"`
	Reason            string `json:"reason,omitempty"`
	ModifiedArguments string `json:"modified_arguments,omitempty"`
}

// ApprovalRespondResponse represents the response to an approval action.
type ApprovalRespondResponse struct {
	Success   bool   `json:"success"`
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
	Message   string `json:"message"`
}

// HandlePolicyStatus returns the current policy status.
func (r *Router) HandlePolicyStatus(w http.ResponseWriter, req *http.Request) {
	if r.policyExecutor == nil {
		handlers.SendJSON(w, http.StatusOK, PolicyStatusResponse{
			DefaultAllow:        true,
			RequireApproval:     false,
			BlocklistCount:      0,
			AllowlistCount:      0,
			DangerousRulesCount: 0,
			ParamRulesCount:     0,
			Message:             "Policy executor not initialized",
		})
		return
	}

	status := r.policyExecutor.Status()
	handlers.SendJSON(w, http.StatusOK, PolicyStatusResponse{
		DefaultAllow:        status.DefaultAllow,
		RequireApproval:     status.RequireApproval,
		BlocklistCount:      status.BlocklistCount,
		AllowlistCount:      status.AllowlistCount,
		DangerousRulesCount: status.DangerousRulesCount,
		ParamRulesCount:     status.ParamRulesCount,
	})
}

// HandlePolicyCheck checks if a tool call would be allowed.
func (r *Router) HandlePolicyCheck(w http.ResponseWriter, req *http.Request) {
	var checkReq PolicyCheckRequest
	if err := json.NewDecoder(req.Body).Decode(&checkReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if checkReq.Tool == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Tool name is required")
		return
	}

	if r.policyExecutor == nil {
		handlers.SendJSON(w, http.StatusOK, PolicyCheckResponse{
			Tool:            checkReq.Tool,
			Allowed:         true,
			RequireApproval: false,
			Blocked:         false,
			Reason:          "Policy executor not initialized, allowing by default",
		})
		return
	}

	// Parse arguments as JSON to validate, but pass as string
	if checkReq.Arguments != "" {
		var args map[string]any
		if err := json.Unmarshal([]byte(checkReq.Arguments), &args); err != nil {
			handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Invalid arguments JSON")
			return
		}
	}

	call := &policy.ToolCall{
		Name:      checkReq.Tool,
		Arguments: checkReq.Arguments,
		SessionID: checkReq.SessionID,
	}

	result, err := r.policyExecutor.Check(req.Context(), call)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, "Policy check failed: "+err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, PolicyCheckResponse{
		Tool:            checkReq.Tool,
		Allowed:         result.Allowed,
		RequireApproval: result.RequireApproval,
		Blocked:         !result.Allowed && !result.RequireApproval,
		Reason:          result.Reason,
		Warnings:        result.Warnings,
	})
}

// HandleApprovalList lists all pending approval requests.
func (r *Router) HandleApprovalList(w http.ResponseWriter, req *http.Request) {
	if r.approvalManager == nil {
		handlers.SendJSON(w, http.StatusOK, ApprovalListResponse{
			Pending: []*approval.ApprovalRequest{},
			Count:   0,
		})
		return
	}

	pending := r.approvalManager.ListPending()
	handlers.SendJSON(w, http.StatusOK, ApprovalListResponse{
		Pending: pending,
		Count:   len(pending),
	})
}

// HandleApprovalRespond responds to a pending approval request.
func (r *Router) HandleApprovalRespond(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	requestID := vars["id"]

	if requestID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Request ID is required")
		return
	}

	var respondReq ApprovalRespondRequest
	if err := json.NewDecoder(req.Body).Decode(&respondReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body")
		return
	}

	if r.approvalManager == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Approval manager not initialized")
		return
	}

	err := r.approvalManager.HandleResponse(requestID, respondReq.Approved, respondReq.Reason, respondReq.ModifiedArguments)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Approval request not found or already processed")
		return
	}

	message := "Request denied"
	if respondReq.Approved {
		message = "Request approved"
	}

	handlers.SendJSON(w, http.StatusOK, ApprovalRespondResponse{
		Success:   true,
		RequestID: requestID,
		Approved:  respondReq.Approved,
		Message:   message,
	})
}

// HandleGetPolicyConfig returns the full policy configuration.
func (r *Router) HandleGetPolicyConfig(w http.ResponseWriter, req *http.Request) {
	if r.policyExecutor == nil {
		handlers.SendJSON(w, http.StatusOK, policy.DefaultPolicy())
		return
	}
	handlers.SendJSON(w, http.StatusOK, r.policyExecutor.GetPolicy())
}

// HandleUpdatePolicyConfig updates the policy configuration.
func (r *Router) HandleUpdatePolicyConfig(w http.ResponseWriter, req *http.Request) {
	var newPolicy policy.ToolPolicy
	if err := json.NewDecoder(req.Body).Decode(&newPolicy); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body")
		return
	}
	if err := policy.ValidatePolicy(&newPolicy); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if r.policyExecutor == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Policy executor not initialized")
		return
	}
	r.policyExecutor.SetPolicy(&newPolicy)

	// Sync new policy fields to runner (scrub rules, block message, circuit breaker)
	if r.runner != nil {
		if err := r.runner.SetScrubRules(newPolicy.ScrubRules); err != nil {
			// Log but don't fail the request — policy itself was saved
			handlers.SendJSON(w, http.StatusOK, map[string]any{"success": true, "warning": "scrub rules compile error: " + err.Error()})
			return
		}
		r.runner.SetBlockMessageTemplate(newPolicy.BlockMessageTemplate)
		r.runner.SetCircuitBreakerThreshold(newPolicy.CircuitBreakerThreshold)
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{"success": true})
}
