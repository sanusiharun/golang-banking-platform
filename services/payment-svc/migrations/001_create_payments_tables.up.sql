-- Migration: 001_create_payments_tables
-- Creates all tables for payment-svc: transactions, reversals, idempotency_requests.
-- Amount is stored in minor currency units (kobo, cents) as BIGINT — never DECIMAL.

BEGIN;

-- ── transactions ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS transactions (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key        TEXT        NOT NULL,
    payment_type           TEXT        NOT NULL,
    channel                TEXT        NOT NULL,
    source_account_id      TEXT        NOT NULL,
    destination_account_id TEXT        NOT NULL,
    amount                 BIGINT      NOT NULL,
    currency               CHAR(3)     NOT NULL,
    status                 TEXT        NOT NULL DEFAULT 'PENDING',
    failure_reason         TEXT,
    retry_count            INT         NOT NULL DEFAULT 0,
    max_retries            INT         NOT NULL DEFAULT 3,
    external_reference     TEXT,
    correlation_id         UUID,
    trace_id               TEXT,
    description            TEXT,
    metadata               JSONB,
    initiated_by           TEXT        NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at           TIMESTAMPTZ,
    reversed_at            TIMESTAMPTZ,

    CONSTRAINT transactions_idempotency_key_unique UNIQUE (idempotency_key),
    CONSTRAINT transactions_amount_positive        CHECK  (amount > 0),
    CONSTRAINT transactions_status_valid           CHECK  (status IN ('PENDING','PROCESSING','SUCCESS','FAILED','CANCELLED','REVERSED')),
    CONSTRAINT transactions_type_valid             CHECK  (payment_type IN ('TRANSFER','MERCHANT_PAYMENT','FEE','REFUND','SCHEDULED'))
);

CREATE INDEX IF NOT EXISTS idx_transactions_source_account  ON transactions (source_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_dest_account    ON transactions (destination_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_status          ON transactions (status);
CREATE INDEX IF NOT EXISTS idx_transactions_correlation_id  ON transactions (correlation_id);

COMMENT ON TABLE  transactions        IS 'Central payment transaction ledger.';
COMMENT ON COLUMN transactions.amount IS 'Amount in minor currency units. Never negative.';

-- ── reversals ─────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reversals (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    original_txn_id UUID        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'PENDING',
    failure_reason  TEXT,
    initiated_by    TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,

    CONSTRAINT reversals_original_txn_unique UNIQUE      (original_txn_id),
    CONSTRAINT reversals_original_txn_fk     FOREIGN KEY (original_txn_id) REFERENCES transactions (id),
    CONSTRAINT reversals_status_valid        CHECK       (status IN ('PENDING','SUCCESS','FAILED'))
);

-- ── idempotency_requests ──────────────────────────────────────────────────────
-- Schema matches pkg/idempotency.PostgresStore expectations exactly.

CREATE TABLE IF NOT EXISTS idempotency_requests (
    id               TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    scope_key        TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    caller_id        TEXT        NOT NULL,
    http_method      TEXT        NOT NULL,
    url_path         TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'processing',
    status_code      INT,
    response_headers JSONB,
    response_body    BYTEA,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL,

    CONSTRAINT idempotency_scope_key_unique UNIQUE (scope_key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at ON idempotency_requests (expires_at);

COMMIT;
