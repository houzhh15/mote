-- Migration 009: PDA Tracking
-- Purpose: Add PDA-specific columns to delegate_invocations for structured orchestration audit

ALTER TABLE delegate_invocations ADD COLUMN mode TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE delegate_invocations ADD COLUMN executed_steps TEXT;
ALTER TABLE delegate_invocations ADD COLUMN pda_stack_depth INTEGER NOT NULL DEFAULT 0;
