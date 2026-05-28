-- Migration: 002_create_transactions
-- Immutable transaction ledger — rows are never updated after insert.

BEGIN;

CREATE TABLE IF NOT EXISTS transactions (
    id             TEXT        NOT NULL PRIMARY KEY,
    account_id     TEXT        NOT NULL REFERENCES accounts(id),
    type           TEXT        NOT NULL,
    amount         BIGINT      NOT NULL,
    balance_before BIGINT      NOT NULL,
    balance_after  BIGINT      NOT NULL,
    reference      TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT transactions_type_valid   CHECK (type IN ('CREDIT','DEBIT')),
    CONSTRAINT transactions_amount_pos   CHECK (amount > 0),
    CONSTRAINT transactions_bal_non_neg  CHECK (balance_after >= 0)
);

CREATE INDEX IF NOT EXISTS idx_transactions_account_id  ON transactions (account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at  ON transactions (created_at DESC);

COMMIT;
