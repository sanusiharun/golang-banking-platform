-- Migration: 001_create_accounts
-- Creates the accounts table.
-- Balance is stored in minor currency units (kobo, cents) as BIGINT — never DECIMAL.
-- The version column enables optimistic concurrency control in the repository layer.

BEGIN;

CREATE TABLE IF NOT EXISTS accounts (
    id           TEXT        NOT NULL PRIMARY KEY,
    customer_id  TEXT        NOT NULL,
    iban         TEXT        NOT NULL,
    currency     CHAR(3)     NOT NULL,
    balance      BIGINT      NOT NULL DEFAULT 0,
    status       TEXT        NOT NULL DEFAULT 'PENDING',
    version      INTEGER     NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT accounts_iban_unique      UNIQUE (iban),
    CONSTRAINT accounts_balance_non_neg  CHECK  (balance >= 0),
    CONSTRAINT accounts_version_positive CHECK  (version >= 1),
    CONSTRAINT accounts_status_valid     CHECK  (status IN ('PENDING','ACTIVE','SUSPENDED','CLOSED')),
    CONSTRAINT accounts_currency_valid   CHECK  (char_length(currency) = 3)
);

CREATE INDEX IF NOT EXISTS idx_accounts_customer_id ON accounts (customer_id);
CREATE INDEX IF NOT EXISTS idx_accounts_status      ON accounts (status);
CREATE INDEX IF NOT EXISTS idx_accounts_created_at  ON accounts (created_at DESC);

COMMENT ON TABLE  accounts           IS 'Core account ledger.';
COMMENT ON COLUMN accounts.balance   IS 'Balance in minor currency units. Never negative.';
COMMENT ON COLUMN accounts.version   IS 'Optimistic lock version — incremented on every write.';

COMMIT;
