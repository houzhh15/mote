-- Cron jobs and history tables
-- Migration: 002_cron.sql

-- Cron jobs table: stores scheduled task definitions
CREATE TABLE IF NOT EXISTS cron_jobs (
    name TEXT PRIMARY KEY,
    schedule TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('prompt', 'tool', 'script')),
    payload TEXT,
    enabled INTEGER DEFAULT 1,
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for querying enabled jobs
CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled);

-- Index for finding next jobs to run
CREATE INDEX IF NOT EXISTS idx_cron_jobs_next_run ON cron_jobs(next_run);

-- Cron history table: stores execution history of scheduled jobs
CREATE TABLE IF NOT EXISTS cron_history (
    id TEXT PRIMARY KEY,
    job_name TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
    result TEXT,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    FOREIGN KEY (job_name) REFERENCES cron_jobs(name) ON DELETE CASCADE
);

-- Index for querying history by job name
CREATE INDEX IF NOT EXISTS idx_cron_history_job ON cron_history(job_name);

-- Index for ordering history by start time (descending for recent first)
CREATE INDEX IF NOT EXISTS idx_cron_history_started ON cron_history(started_at DESC);
