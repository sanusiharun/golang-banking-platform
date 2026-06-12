-- 007: idempotency_requests
-- Stores idempotency state for API key callers on mutating endpoints.
-- Redis is the primary store; this table is the durable fallback + source of truth.

CREATE TABLE IF NOT EXISTS idempotency_requests (
    id               TEXT        PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_key        TEXT        NOT NULL UNIQUE,  -- SHA-256(caller_id|method|path|idem_key)
    idempotency_key  TEXT        NOT NULL,          -- original caller-supplied key (for debugging)
    caller_id        TEXT        NOT NULL,          -- service_account_id
    http_method      TEXT        NOT NULL,
    url_path         TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'processing', -- processing|completed|failed
    status_code      INT,
    response_headers JSONB,
    response_body    BYTEA,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL            -- created_at + 24h; used by cleanup job
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_scope ON idempotency_requests(scope_key);
CREATE INDEX IF NOT EXISTS idx_idempotency_expires ON idempotency_requests(expires_at);
