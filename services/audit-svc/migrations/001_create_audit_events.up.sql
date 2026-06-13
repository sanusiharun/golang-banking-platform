-- Migration: 001_create_audit_events
-- Creates the append-only audit_events table with indexes for common query patterns.
-- No UPDATE or DELETE is ever allowed on this table.

CREATE TABLE IF NOT EXISTS audit_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type   TEXT        NOT NULL,
    actor_id     TEXT        NOT NULL,
    actor_email  TEXT,
    action       TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'success',
    resource     TEXT,
    resource_id  TEXT,
    service_name TEXT        NOT NULL,
    trace_id     TEXT,
    ip_address   TEXT,
    user_agent   TEXT,
    metadata     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enforce append-only at the database level.
-- This is a belt-and-suspenders control; the repository interface also omits
-- Update and Delete methods.
REVOKE UPDATE, DELETE ON audit_events FROM PUBLIC;

-- Query pattern: all events for a given actor, newest first
CREATE INDEX IF NOT EXISTS idx_audit_actor
    ON audit_events (actor_id, created_at DESC);

-- Query pattern: filter by action type, newest first
CREATE INDEX IF NOT EXISTS idx_audit_action
    ON audit_events (action, created_at DESC);

-- Query pattern: all events on a specific resource record
CREATE INDEX IF NOT EXISTS idx_audit_resource
    ON audit_events (resource, resource_id, created_at DESC);

-- Query pattern: correlate with distributed traces
CREATE INDEX IF NOT EXISTS idx_audit_trace
    ON audit_events (trace_id)
    WHERE trace_id IS NOT NULL;

-- Query pattern: per-service event history, newest first
CREATE INDEX IF NOT EXISTS idx_audit_service_time
    ON audit_events (service_name, created_at DESC);
