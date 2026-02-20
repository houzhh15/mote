// Package cron provides job scheduling with persistent storage.
package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled job execution with robfig/cron.
type Scheduler struct {
	cron     *cron.Cron
	entries  map[string]cron.EntryID // job name -> entry ID
	store    *JobStore
	history  *HistoryStore
	executor *Executor
	logger   *slog.Logger
	mu       sync.RWMutex
	running  bool

	// Track active executions for graceful shutdown
	wg sync.WaitGroup

	// Track currently executing jobs to prevent overlapping executions
	executing sync.Map // job name -> time.Time (start time)
}

// SchedulerConfig configures the scheduler.
type SchedulerConfig struct {
	// Location for time zone handling
	Location *time.Location
}

// NewScheduler creates a new scheduler with dependencies.
func NewScheduler(
	store *JobStore,
	history *HistoryStore,
	executor *Executor,
	logger *slog.Logger,
	config *SchedulerConfig,
) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	if config == nil {
		config = &SchedulerConfig{}
	}
	if config.Location == nil {
		config.Location = time.Local
	}

	// Create cron with seconds field and timezone support
	c := cron.New(
		cron.WithSeconds(),
		cron.WithLocation(config.Location),
		cron.WithLogger(cron.PrintfLogger(slog.NewLogLogger(logger.Handler(), slog.LevelDebug))),
	)

	return &Scheduler{
		cron:     c,
		entries:  make(map[string]cron.EntryID),
		store:    store,
		history:  history,
		executor: executor,
		logger:   logger,
	}
}

// Start loads enabled jobs from store and starts the scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("scheduler already running")
	}

	// Load enabled jobs from store
	jobs, err := s.store.ListEnabled()
	if err != nil {
		return fmt.Errorf("failed to load enabled jobs: %w", err)
	}

	s.logger.Info("loading scheduled jobs", "count", len(jobs))

	// Register each job
	for _, job := range jobs {
		if err := s.addEntryLocked(job); err != nil {
			s.logger.Error("failed to register job", "job_name", job.Name, "error", err)
			continue
		}
	}

	s.cron.Start()
	s.running = true
	s.logger.Info("scheduler started", "registered_jobs", len(s.entries))

	return nil
}

// Stop gracefully stops the scheduler, waiting for running jobs.
func (s *Scheduler) Stop() context.Context {
	s.mu.Lock()

	if !s.running {
		s.mu.Unlock()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	// Stop accepting new jobs
	ctx := s.cron.Stop()
	s.running = false
	s.mu.Unlock()

	// Wait for active executions
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	s.logger.Info("scheduler stopped")
	return ctx
}

// AddJob creates a new job and registers it for scheduling.
func (s *Scheduler) AddJob(ctx context.Context, create JobCreate) (*Job, error) {
	// Validate cron expression - use parser with seconds support to match scheduler config
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(create.Schedule); err != nil {
		// Also try standard 5-field format for backwards compatibility
		if _, err2 := cron.ParseStandard(create.Schedule); err2 != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", err)
		}
	}

	// Persist to store
	job, err := s.store.Create(&create)
	if err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Register if scheduler is running and job is enabled
	if s.running && job.Enabled {
		if err := s.addEntryLocked(job); err != nil {
			s.logger.Error("failed to register new job", "job_name", job.Name, "error", err)
		}
	}

	s.logger.Info("job added", "job_name", job.Name, "schedule", job.Schedule)
	return job, nil
}

// UpdateJob updates an existing job and re-registers if needed.
func (s *Scheduler) UpdateJob(ctx context.Context, name string, patch JobPatch) (*Job, error) {
	// Validate cron expression if provided - use parser with seconds support
	if patch.Schedule != nil {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if _, err := parser.Parse(*patch.Schedule); err != nil {
			// Also try standard 5-field format for backwards compatibility
			if _, err2 := cron.ParseStandard(*patch.Schedule); err2 != nil {
				return nil, fmt.Errorf("invalid cron expression: %w", err)
			}
		}
	}

	// Update in store
	job, err := s.store.Update(name, &patch)
	if err != nil {
		return nil, fmt.Errorf("failed to update job: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-register if scheduler is running
	if s.running {
		// Remove old entry if exists
		if entryID, ok := s.entries[name]; ok {
			s.cron.Remove(entryID)
			delete(s.entries, name)
		}

		// Add new entry if enabled
		if job.Enabled {
			if err := s.addEntryLocked(job); err != nil {
				s.logger.Error("failed to re-register job", "job_name", name, "error", err)
			}
		}
	}

	s.logger.Info("job updated", "job_name", name)
	return job, nil
}

// RemoveJob removes a job from scheduler and store.
func (s *Scheduler) RemoveJob(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from scheduler
	if entryID, ok := s.entries[name]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, name)
	}

	// Delete from store
	if err := s.store.Delete(name); err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	s.logger.Info("job removed", "job_name", name)
	return nil
}

// EnableJob enables a disabled job.
func (s *Scheduler) EnableJob(ctx context.Context, name string) (*Job, error) {
	enabled := true
	return s.UpdateJob(ctx, name, JobPatch{Enabled: &enabled})
}

// DisableJob disables an enabled job.
func (s *Scheduler) DisableJob(ctx context.Context, name string) (*Job, error) {
	enabled := false
	return s.UpdateJob(ctx, name, JobPatch{Enabled: &enabled})
}

// ListJobs returns all jobs.
func (s *Scheduler) ListJobs(ctx context.Context) ([]*Job, error) {
	return s.store.List()
}

// GetJob returns a job by name.
func (s *Scheduler) GetJob(ctx context.Context, name string) (*Job, error) {
	return s.store.Get(name)
}

// RunNow immediately executes a job.
func (s *Scheduler) RunNow(ctx context.Context, name string) (*ExecuteResult, error) {
	s.mu.RLock()
	if !s.running {
		s.mu.RUnlock()
		return nil, fmt.Errorf("scheduler not running")
	}
	s.mu.RUnlock()

	job, err := s.store.Get(name)
	if err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}

	// Check if already running
	if _, running := s.executing.Load(name); running {
		return nil, fmt.Errorf("job %q is already running", name)
	}

	s.wg.Add(1)
	defer s.wg.Done()

	result := s.executor.Execute(ctx, job)
	if result.Error != nil {
		return result, result.Error
	}

	return result, nil
}

// GetNextRun returns the next scheduled run time for a job.
func (s *Scheduler) GetNextRun(name string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entryID, ok := s.entries[name]
	if !ok {
		return time.Time{}, false
	}

	entry := s.cron.Entry(entryID)
	if entry.ID == 0 {
		return time.Time{}, false
	}

	return entry.Next, true
}

// Entries returns the number of registered cron entries.
func (s *Scheduler) Entries() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// ExecutingJob holds info about a currently executing cron job.
type ExecutingJob struct {
	Name      string    `json:"name"`
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
}

// GetExecutingJobs returns the list of currently executing cron jobs.
func (s *Scheduler) GetExecutingJobs() []ExecutingJob {
	var result []ExecutingJob
	s.executing.Range(func(key, value any) bool {
		name := key.(string)
		startTime := value.(time.Time)
		result = append(result, ExecutingJob{
			Name:      name,
			SessionID: deriveCronSessionID(name),
			StartedAt: startTime,
		})
		return true
	})
	return result
}

// addEntryLocked registers a job with the cron scheduler.
// Caller must hold s.mu.
func (s *Scheduler) addEntryLocked(job *Job) error {
	// robfig/cron expects 6-field (with seconds) expression when using WithSeconds()
	// Detect if schedule is 5-field (standard) or 6-field (with seconds)
	schedule := job.Schedule
	fields := strings.Fields(schedule)
	if len(fields) == 5 {
		// Standard 5-field format, add "0" for seconds field
		schedule = "0 " + schedule
	}
	// 6-field format is used as-is

	entryID, err := s.cron.AddFunc(schedule, func() {
		s.executeJob(job)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron entry: %w", err)
	}

	s.entries[job.Name] = entryID
	return nil
}

// executeJob wraps job execution with tracking.
// It enforces a hard timeout to guarantee the executing map is always cleaned up,
// even if the underlying executor gets stuck (e.g. a tool blocks forever).
func (s *Scheduler) executeJob(job *Job) {
	// Skip if this job is already executing (prevents overlapping executions)
	startTime := time.Now()
	if prev, loaded := s.executing.LoadOrStore(job.Name, startTime); loaded {
		prevStart := prev.(time.Time)
		runningFor := time.Since(prevStart)
		// Safety valve: if the previous execution has been "running" for longer
		// than the hard timeout (+ 1min grace), it's a stale entry from a stuck
		// execution. Force-clean it and proceed.
		const hardTimeout = 30 * time.Minute
		if runningFor > hardTimeout+1*time.Minute {
			s.logger.Warn("force-cleaning stale executing entry",
				"job_name", job.Name,
				"previous_start", prevStart.Format(time.RFC3339),
				"running_for", runningFor.Round(time.Second).String(),
			)
			s.executing.Delete(job.Name)
			// Re-store with our new start time
			s.executing.Store(job.Name, startTime)
		} else {
			s.logger.Warn("skipping overlapping execution, previous run still active",
				"job_name", job.Name,
				"previous_start", prevStart.Format(time.RFC3339),
				"running_for", runningFor.Round(time.Second).String(),
			)
			return
		}
	}
	defer s.executing.Delete(job.Name)

	s.wg.Add(1)
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Reload job to get latest config
	currentJob, err := s.store.Get(job.Name)
	if err != nil {
		s.logger.Error("failed to reload job for execution", "job_name", job.Name, "error", err)
		return
	}

	if !currentJob.Enabled {
		s.logger.Debug("skipping disabled job", "job_name", job.Name)
		return
	}

	s.logger.Info("executing scheduled job", "job_name", job.Name)

	// Run executor in a goroutine so we can enforce the hard timeout.
	// Without this, a stuck tool execution would block forever and
	// the executing map entry would never be cleaned up.
	type execResult struct {
		result *ExecuteResult
	}
	ch := make(chan execResult, 1)
	go func() {
		ch <- execResult{result: s.executor.Execute(ctx, currentJob)}
	}()

	select {
	case res := <-ch:
		result := res.result
		if result.Error != nil {
			s.logger.Error("job execution failed",
				"job_name", job.Name,
				"error", result.Error,
				"history_id", result.HistoryID,
			)
		} else {
			s.logger.Info("job execution completed",
				"job_name", job.Name,
				"history_id", result.HistoryID,
			)
		}
	case <-ctx.Done():
		s.logger.Error("job execution timed out (hard deadline), cleaning up",
			"job_name", job.Name,
			"timeout", "30m",
		)
		// Cancel the runner's session queue so the stuck worker exits
		// and the next execution can start with a fresh worker.
		sessionID := deriveCronSessionID(job.Name)
		if cr, ok := s.executor.runner.(CancellableRunner); ok {
			cr.CancelSession(sessionID)
			s.logger.Info("cancelled stuck runner session", "job_name", job.Name, "sessionID", sessionID)
		}
	}
}
