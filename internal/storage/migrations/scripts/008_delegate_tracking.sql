-- Migration 008: Delegate Tracking
-- Purpose: Create delegate_invocations table to track multi-agent delegation execution

CREATE TABLE IF NOT EXISTS delegate_invocations (
    id TEXT PRIMARY KEY,
    parent_session_id TEXT NOT NULL,
    child_session_id TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    depth INTEGER NOT NULL,
    chain TEXT NOT NULL,
    prompt TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    result_length INTEGER DEFAULT 0,
    tokens_used INTEGER DEFAULT 0,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_delegate_parent ON delegate_invocations(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_delegate_status ON delegate_invocations(status);
