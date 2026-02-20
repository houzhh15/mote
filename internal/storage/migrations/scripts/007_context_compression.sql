-- Migration 007: Context Compression
-- Purpose: Create contexts table to store compressed conversation contexts
-- independently from message history

-- Create contexts table
CREATE TABLE IF NOT EXISTS contexts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    summary TEXT NOT NULL,
    kept_message_ids TEXT,
    total_tokens INTEGER NOT NULL,
    original_tokens INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(session_id, version)
);

-- Create index for efficient queries
CREATE INDEX IF NOT EXISTS idx_contexts_session_version 
    ON contexts(session_id, version DESC);

-- Note: This migration does NOT modify the messages table
-- Messages remain intact for user viewing
