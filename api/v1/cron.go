package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"mote/internal/cron"
	"mote/internal/gateway/handlers"
)

// HandleListCronJobs returns a list of cron jobs.
func (r *Router) HandleListCronJobs(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendJSON(w, http.StatusOK, CronJobsListResponse{
			Jobs: []CronJob{},
		})
		return
	}

	jobs, err := r.cronScheduler.ListJobs(req.Context())
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	var cronJobs []CronJob
	for _, j := range jobs {
		cronJobs = append(cronJobs, cronJobFromInternal(j))
	}

	if cronJobs == nil {
		cronJobs = []CronJob{}
	}

	handlers.SendJSON(w, http.StatusOK, CronJobsListResponse{
		Jobs: cronJobs,
	})
}

// HandleCreateCronJob creates a new cron job.
func (r *Router) HandleCreateCronJob(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Cron scheduler not available")
		return
	}

	var createReq CreateCronJobRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if createReq.Name == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Name is required")
		return
	}
	if createReq.Schedule == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Schedule is required")
		return
	}

	// Determine job type (default to prompt)
	jobType := createReq.Type
	if jobType == "" {
		jobType = string(cron.JobTypePrompt)
	}

	// Get payload - prefer explicit Payload field, fall back to Prompt for backwards compatibility
	payload := createReq.Payload
	if payload == "" && createReq.Prompt != "" {
		// Wrap prompt in JSON for backwards compatibility
		payloadMap := map[string]string{"prompt": createReq.Prompt}
		if createReq.Model != "" {
			payloadMap["model"] = createReq.Model
		}
		payloadData, _ := json.Marshal(payloadMap)
		payload = string(payloadData)
	}
	if payload == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeValidationFailed, "Payload or prompt is required")
		return
	}

	jobCreate := cron.JobCreate{
		Name:           createReq.Name,
		Schedule:       createReq.Schedule,
		Type:           cron.JobType(jobType),
		Payload:        payload,
		Enabled:        true,
		WorkspacePath:  createReq.WorkspacePath,
		WorkspaceAlias: createReq.WorkspaceAlias,
	}

	job, err := r.cronScheduler.AddJob(req.Context(), jobCreate)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusCreated, cronJobFromInternal(job))
}

// HandleGetCronJob returns details of a specific cron job.
func (r *Router) HandleGetCronJob(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Cron scheduler not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	job, err := r.cronScheduler.GetJob(req.Context(), name)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Cron job not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, cronJobFromInternal(job))
}

// HandleUpdateCronJob updates an existing cron job.
func (r *Router) HandleUpdateCronJob(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Cron scheduler not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	// Check if job exists
	_, err := r.cronScheduler.GetJob(req.Context(), name)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Cron job not found")
		return
	}

	var updateReq UpdateCronJobRequest
	if err := json.NewDecoder(req.Body).Decode(&updateReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	patch := cron.JobPatch{}
	if updateReq.Schedule != nil {
		patch.Schedule = updateReq.Schedule
	}
	if updateReq.Prompt != nil || updateReq.Model != nil {
		// Need to get existing payload and merge changes
		existingJob, _ := r.cronScheduler.GetJob(req.Context(), name)
		var payloadMap map[string]string
		if err := json.Unmarshal([]byte(existingJob.Payload), &payloadMap); err != nil {
			payloadMap = make(map[string]string)
		}
		if updateReq.Prompt != nil {
			payloadMap["prompt"] = *updateReq.Prompt
		}
		if updateReq.Model != nil {
			payloadMap["model"] = *updateReq.Model
		}
		payloadData, _ := json.Marshal(payloadMap)
		payloadStr := string(payloadData)
		patch.Payload = &payloadStr
	}
	if updateReq.Enabled != nil {
		patch.Enabled = updateReq.Enabled
	}
	if updateReq.WorkspacePath != nil {
		patch.WorkspacePath = updateReq.WorkspacePath
	}
	if updateReq.WorkspaceAlias != nil {
		patch.WorkspaceAlias = updateReq.WorkspaceAlias
	}

	job, err := r.cronScheduler.UpdateJob(req.Context(), name, patch)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, cronJobFromInternal(job))
}

// HandleDeleteCronJob deletes a cron job.
func (r *Router) HandleDeleteCronJob(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Cron scheduler not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	if err := r.cronScheduler.RemoveJob(req.Context(), name); err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Cron job not found")
		return
	}

	handlers.SendJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "Cron job deleted",
	})
}

// HandleRunCronJob manually triggers a cron job.
func (r *Router) HandleRunCronJob(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Cron scheduler not available")
		return
	}

	vars := mux.Vars(req)
	name := vars["name"]

	result, err := r.cronScheduler.RunNow(req.Context(), name)
	if err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, CronRunResponse{
		Success:   result.Error == nil,
		Message:   "Cron job triggered",
		HistoryID: fmt.Sprintf("%d", result.HistoryID),
	})
}

// HandleCronHistory returns execution history for cron jobs.
func (r *Router) HandleCronHistory(w http.ResponseWriter, req *http.Request) {
	// History is not directly available on Scheduler - it's in HistoryStore
	// For now, return empty list. This can be enhanced later.
	handlers.SendJSON(w, http.StatusOK, CronHistoryResponse{
		History: []CronHistoryEntry{},
	})
}

// HandleGetExecutingJobs returns the list of currently executing cron jobs.
func (r *Router) HandleGetExecutingJobs(w http.ResponseWriter, req *http.Request) {
	if r.cronScheduler == nil {
		handlers.SendJSON(w, http.StatusOK, CronExecutingResponse{Jobs: []CronExecutingJob{}})
		return
	}

	executing := r.cronScheduler.GetExecutingJobs()
	now := time.Now()
	var jobs []CronExecutingJob
	for _, ej := range executing {
		jobs = append(jobs, CronExecutingJob{
			Name:       ej.Name,
			SessionID:  ej.SessionID,
			StartedAt:  ej.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
			RunningFor: int(now.Sub(ej.StartedAt).Seconds()),
		})
	}
	if jobs == nil {
		jobs = []CronExecutingJob{}
	}

	handlers.SendJSON(w, http.StatusOK, CronExecutingResponse{Jobs: jobs})
}

// cronJobFromInternal converts internal cron.Job to API CronJob.
func cronJobFromInternal(j *cron.Job) CronJob {
	// Parse payload to extract prompt and model
	var prompt, model string
	var payloadMap map[string]string
	if err := json.Unmarshal([]byte(j.Payload), &payloadMap); err == nil {
		prompt = payloadMap["prompt"]
		model = payloadMap["model"]
	} else {
		// Fallback: treat whole payload as prompt (legacy)
		prompt = j.Payload
	}

	return CronJob{
		Name:           j.Name,
		Schedule:       j.Schedule,
		Type:           string(j.Type),
		Prompt:         prompt,
		Model:          model,
		Enabled:        j.Enabled,
		SessionID:      "cron-" + j.Name,
		WorkspacePath:  j.WorkspacePath,
		WorkspaceAlias: j.WorkspaceAlias,
		LastRun:        j.LastRun,
		NextRun:        j.NextRun,
	}
}

// CronRunResponse is the response for running a cron job.
type CronRunResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	HistoryID string `json:"history_id,omitempty"`
}
