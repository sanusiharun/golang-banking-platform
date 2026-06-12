-- 006: api_keys
-- Stores SHA-256 hashes of API keys. The raw key is NEVER persisted.
-- One service account may have multiple keys (rotation support).

CREATE TABLE IF NOT EXISTS api_keys (
    id                  TEXT        PRIMARY KEY,
    service_account_id  TEXT        NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,       -- human label e.g. "primary", "rotation-2025-06"
    key_hash            TEXT        NOT NULL UNIQUE, -- SHA-256 hex of raw key
    key_prefix          TEXT        NOT NULL,       -- first 10 chars for human identification
    expires_at          TIMESTAMPTZ,                -- NULL = non-expiring
    revoked_at          TIMESTAMPTZ,                -- NULL = active
    last_used_at        TIMESTAMPTZ,                -- updated async, not on hot path
    created_by          TEXT        NOT NULL,       -- user_id
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Hot path: lookup by hash among active keys
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash_active
    ON api_keys(key_hash)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_api_keys_service_account
    ON api_keys(service_account_id);
