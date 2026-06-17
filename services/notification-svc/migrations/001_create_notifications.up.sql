-- Migration: 001_create_notifications
-- Creates the notifications table for tracking all notification lifecycle events.

BEGIN;

CREATE TABLE IF NOT EXISTS notifications (
    id              TEXT        NOT NULL,
    channel         TEXT        NOT NULL,
    recipient       TEXT        NOT NULL,
    template_id     TEXT,
    template_code   TEXT,
    template_vars   JSONB,
    payload         JSONB,
    status          TEXT        NOT NULL DEFAULT 'PENDING',
    provider_ref    TEXT,
    provider_resp   JSONB,
    error_message   TEXT,
    retry_count     INTEGER     NOT NULL DEFAULT 0,
    max_retries     INTEGER     NOT NULL DEFAULT 3,
    idempotency_key TEXT,
    schedule_id     TEXT,
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_notifications PRIMARY KEY (id),
    CONSTRAINT chk_notifications_status
        CHECK (status IN ('PENDING','PROCESSING','SENT','DELIVERED','FAILED','RETRYING','CANCELLED')),
    CONSTRAINT chk_notifications_channel
        CHECK (channel IN ('EMAIL','SMS','PUSH','WHATSAPP','WEBHOOK')),
    CONSTRAINT chk_notifications_retry_count CHECK (retry_count >= 0),
    CONSTRAINT chk_notifications_max_retries CHECK (max_retries >= 0)
);

-- Partial unique index: idempotency_key must be unique when present.
CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_idempotency_key
    ON notifications (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Worker claim query: fetch PENDING/RETRYING where scheduled_at has arrived.
CREATE INDEX IF NOT EXISTS idx_notifications_worker_claim
    ON notifications (status, scheduled_at)
    WHERE status IN ('PENDING', 'RETRYING');

-- Filtering indexes.
CREATE INDEX IF NOT EXISTS idx_notifications_channel    ON notifications (channel);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient  ON notifications (recipient);
CREATE INDEX IF NOT EXISTS idx_notifications_template_code ON notifications (template_code);
CREATE INDEX IF NOT EXISTS idx_notifications_schedule_id   ON notifications (schedule_id);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at    ON notifications (created_at DESC);

COMMENT ON TABLE  notifications IS 'Notification delivery records — one row per notification attempt lifecycle.';
COMMENT ON COLUMN notifications.status IS 'PENDING | PROCESSING | SENT | DELIVERED | FAILED | RETRYING | CANCELLED';
COMMENT ON COLUMN notifications.idempotency_key IS 'Caller-supplied key to prevent duplicate deliveries.';
COMMENT ON COLUMN notifications.provider_resp IS 'Raw response from the channel provider after Send().';

COMMIT;
