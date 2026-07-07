-- Migration: 002_create_qris_tables
-- Adds QRIS (Quick Response Code Indonesian Standard) support:
--   • merchants      — registry of QRIS merchants and their internal credit account
--   • qris_charges   — generated QR intents (static/dynamic) and their pay state
--   • extends the transactions payment_type CHECK to allow 'QRIS'
-- Amounts are stored in minor currency units (BIGINT) — never DECIMAL.

BEGIN;

-- ── merchants ─────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS merchants (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    nmid         TEXT        NOT NULL,               -- National Merchant ID
    name         TEXT        NOT NULL,
    city         TEXT        NOT NULL,
    postal_code  TEXT,
    mcc          CHAR(4)     NOT NULL,               -- Merchant Category Code
    country      CHAR(2)     NOT NULL DEFAULT 'ID',
    account_id   UUID        NOT NULL,               -- internal account credited on payment
    currency     CHAR(3)     NOT NULL DEFAULT 'IDR',
    status       TEXT        NOT NULL DEFAULT 'ACTIVE',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT merchants_nmid_unique   UNIQUE (nmid),
    CONSTRAINT merchants_status_valid  CHECK  (status IN ('ACTIVE','INACTIVE'))
);

COMMENT ON TABLE  merchants            IS 'QRIS merchant registry (simulated — no external acquirer).';
COMMENT ON COLUMN merchants.account_id IS 'Internal banking_accounts account credited when a QR charge is paid.';

-- ── qris_charges ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS qris_charges (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID        NOT NULL,
    qr_type         TEXT        NOT NULL,            -- STATIC | DYNAMIC
    qr_string       TEXT        NOT NULL,            -- EMVCo MPM payload
    amount          BIGINT,                          -- minor units; NULL for static
    currency        CHAR(3)     NOT NULL DEFAULT 'IDR',
    reference_label TEXT,
    bill_number     TEXT,
    status          TEXT        NOT NULL DEFAULT 'PENDING',
    paid_txn_id     UUID,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT qris_charges_merchant_fk     FOREIGN KEY (merchant_id) REFERENCES merchants (id),
    CONSTRAINT qris_charges_paid_txn_fk     FOREIGN KEY (paid_txn_id) REFERENCES transactions (id),
    CONSTRAINT qris_charges_type_valid      CHECK (qr_type IN ('STATIC','DYNAMIC')),
    CONSTRAINT qris_charges_status_valid    CHECK (status  IN ('PENDING','PAID','EXPIRED','CANCELLED')),
    CONSTRAINT qris_charges_amount_positive CHECK (amount IS NULL OR amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_qris_charges_merchant ON qris_charges (merchant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_qris_charges_status   ON qris_charges (status);

-- ── extend transactions payment_type ──────────────────────────────────────────

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_type_valid;
ALTER TABLE transactions ADD  CONSTRAINT transactions_type_valid
    CHECK (payment_type IN ('TRANSFER','MERCHANT_PAYMENT','FEE','REFUND','SCHEDULED','QRIS'));

COMMIT;
