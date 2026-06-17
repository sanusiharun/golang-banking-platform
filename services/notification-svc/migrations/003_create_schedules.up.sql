-- Migration: 003_create_schedules
-- Creates the schedules table for one-time and recurring notification schedules.

BEGIN;

CREATE TABLE IF NOT EXISTS schedules (
    id            TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    description   TEXT,
    channel       TEXT        NOT NULL,
    template_code TEXT        NOT NULL,
    recipient     TEXT        NOT NULL,
    template_vars JSONB,
    cron_expr     TEXT,
    scheduled_at  TIMESTAMPTZ,
    recurring     BOOLEAN     NOT NULL DEFAULT false,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    last_run_at   TIMESTAMPTZ,
    next_run_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_schedules PRIMARY KEY (id),
    CONSTRAINT chk_schedules_channel
        CHECK (channel IN ('EMAIL','SMS','PUSH','WHATSAPP','WEBHOOK')),
    CONSTRAINT chk_schedules_one_time_or_recurring
        CHECK (
            (recurring = false AND scheduled_at IS NOT NULL)
            OR
            (recurring = true AND cron_expr IS NOT NULL)
        )
);

-- Scheduler claim query: enabled schedules with next_run_at in the past.
CREATE INDEX IF NOT EXISTS idx_schedules_enabled_next_run
    ON schedules (enabled, next_run_at)
    WHERE enabled = true;

COMMENT ON TABLE  schedules IS 'Notification schedules — one-time (scheduled_at) or recurring (cron_expr).';
COMMENT ON COLUMN schedules.cron_expr IS 'Standard 5-field cron expression (min hour dom mon dow). Required when recurring=true.';
COMMENT ON COLUMN schedules.next_run_at IS 'Computed next fire time. Updated by the scheduler worker after each execution.';
COMMENT ON COLUMN schedules.enabled IS 'When false, the schedule is paused and will not fire.';

COMMIT;
