package handlers

import (
	"net/http"
	"sync"
	"time"
)

var (
	startTime time.Time
	startOnce sync.Once
)

// InitStartTime initializes the server start time.
// Should be called when the server starts.
func InitStartTime() {
	startOnce.Do(func() {
		startTime = time.Now()
	})
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  int64  `json:"uptime"`
}

// HealthHandler returns a health check handler.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := int64(0)
		if !startTime.IsZero() {
			uptime = int64(time.Since(startTime).Seconds())
		}

		SendJSON(w, http.StatusOK, HealthResponse{
			Status:  "ok",
			Version: version,
			Uptime:  uptime,
		})
	}
}
