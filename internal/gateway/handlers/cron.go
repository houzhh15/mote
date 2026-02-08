package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"mote/internal/cron"
)

// CronHandler handles cron job HTTP endpoints.
type CronHandler struct {
	scheduler *cron.Scheduler
	history   *cron.HistoryStore
}

// NewCronHandler creates a new cron handler.
func NewCronHandler(scheduler *cron.Scheduler, history *cron.HistoryStore) *CronHandler {
	return &CronHandler{
		scheduler: scheduler,
		history:   history,
	}
}

// RegisterRoutes registers cron routes on the router.
func (h *CronHandler) RegisterRoutes(router *mux.Router) {
	sub := router.PathPrefix("/api/cron").Subrouter()

	// Job management
	sub.HandleFunc("/jobs", h.HandleListJobs).Methods("GET")
	sub.HandleFunc("/jobs", h.HandleCreateJob).Methods("POST")
	sub.HandleFunc("/jobs/{name}", h.HandleGetJob).Methods("GET")
	sub.HandleFunc("/jobs/{name}", h.HandleUpdateJob).Methods("PUT", "PATCH")
	sub.HandleFunc("/jobs/{name}", h.HandleDeleteJob).Methods("DELETE")

	// Job actions
	sub.HandleFunc("/jobs/{name}/run", h.HandleRunJob).Methods("POST")
	sub.HandleFunc("/jobs/{name}/enable", h.HandleEnableJob).Methods("POST")
	sub.HandleFunc("/jobs/{name}/disable", h.HandleDisableJob).Methods("POST")

	// History
	sub.HandleFunc("/history", h.HandleListHistory).Methods("GET")
	sub.HandleFunc("/history/{id}", h.HandleGetHistoryEntry).Methods("GET")
}

// HandleListJobs returns all jobs.
func (h *CronHandler) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.scheduler.ListJobs(r.Context())
	if err != nil {
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
	})
}

// HandleCreateJob creates a new job.
func (h *CronHandler) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	var create cron.JobCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}

	job, err := h.scheduler.AddJob(r.Context(), create)
	if err != nil {
		// Check for validation error
		var invErr *cron.InvalidScheduleError
		if errors.As(err, &invErr) || strings.Contains(err.Error(), "invalid cron expression") {
			SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusCreated, job)
}

// HandleGetJob returns a job by name.
func (h *CronHandler) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	job, err := h.scheduler.GetJob(r.Context(), name)
	if err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Include next run time if scheduled
	response := map[string]any{
		"job": job,
	}
	if nextRun, ok := h.scheduler.GetNextRun(name); ok {
		response["next_run"] = nextRun
	}

	SendJSON(w, http.StatusOK, response)
}

// HandleUpdateJob updates a job.
func (h *CronHandler) HandleUpdateJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	var patch cron.JobPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}

	job, err := h.scheduler.UpdateJob(r.Context(), name, patch)
	if err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		var invErr *cron.InvalidScheduleError
		if errors.As(err, &invErr) || strings.Contains(err.Error(), "invalid cron expression") {
			SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, job)
}

// HandleDeleteJob deletes a job.
func (h *CronHandler) HandleDeleteJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	if err := h.scheduler.RemoveJob(r.Context(), name); err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRunJob triggers immediate execution of a job.
func (h *CronHandler) HandleRunJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	result, err := h.scheduler.RunNow(r.Context(), name)
	if err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		// Return result even if execution failed
		if result != nil {
			SendJSON(w, http.StatusOK, map[string]any{
				"success":    result.Success,
				"result":     result.Result,
				"error":      result.Error.Error(),
				"retries":    result.Retries,
				"duration":   result.Duration.String(),
				"history_id": result.HistoryID,
			})
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, map[string]any{
		"success":    result.Success,
		"result":     result.Result,
		"retries":    result.Retries,
		"duration":   result.Duration.String(),
		"history_id": result.HistoryID,
	})
}

// HandleEnableJob enables a job.
func (h *CronHandler) HandleEnableJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	job, err := h.scheduler.EnableJob(r.Context(), name)
	if err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, job)
}

// HandleDisableJob disables a job.
func (h *CronHandler) HandleDisableJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	job, err := h.scheduler.DisableJob(r.Context(), name)
	if err != nil {
		if err == cron.ErrJobNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, job)
}

// HandleListHistory returns execution history.
func (h *CronHandler) HandleListHistory(w http.ResponseWriter, r *http.Request) {
	jobName := r.URL.Query().Get("job")
	limitStr := r.URL.Query().Get("limit")

	limit := 50 // default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	var entries []*cron.HistoryEntry
	var err error

	if jobName != "" {
		entries, err = h.history.ListByJob(jobName, limit)
	} else {
		entries, err = h.history.List(limit)
	}

	if err != nil {
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
	})
}

// HandleGetHistoryEntry returns a specific history entry.
func (h *CronHandler) HandleGetHistoryEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid history id")
		return
	}

	entry, err := h.history.Get(id)
	if err != nil {
		if err == cron.ErrHistoryNotFound {
			SendError(w, http.StatusNotFound, ErrCodeNotFound, "history entry not found")
			return
		}
		SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	SendJSON(w, http.StatusOK, entry)
}
