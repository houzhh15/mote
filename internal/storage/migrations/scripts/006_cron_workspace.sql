-- Add workspace support to cron_jobs table
-- Migration: 006_cron_workspace.sql

-- Add workspace_path column to cron_jobs table
ALTER TABLE cron_jobs 
ADD COLUMN workspace_path TEXT;

-- Add workspace_alias column for display
ALTER TABLE cron_jobs 
ADD COLUMN workspace_alias TEXT;

-- Index for querying jobs by workspace
CREATE INDEX IF NOT EXISTS idx_cron_jobs_workspace 
ON cron_jobs(workspace_path);

-- No need to update existing rows - NULL is the default for new columns
-- Existing jobs will continue to work without workspace binding
