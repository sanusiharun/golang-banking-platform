-- Migration: 004_create_refresh_tokens
-- Stores hashed refresh tokens for session management.
-- Raw tokens are NEVER persisted — only their SHA-256 hash.

BEGIN;

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          TEXT        NOT NULL PRIMARY KEY,
    user_id     TEXT        NOT NULL,
    token_hash  TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT refresh_tokens_hash_unique UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id    ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

COMMENT ON TABLE  refresh_tokens            IS 'Hashed refresh tokens. Raw token is never stored.';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hex digest of the raw refresh token.';

COMMIT;
