-- Migration: 001_create_users
-- Creates the users table for authentication.
-- Passwords are stored as bcrypt hashes — plaintext is never persisted.
-- Roles are stored as a TEXT array so a user can hold multiple roles.

BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id            TEXT        NOT NULL PRIMARY KEY,
    username      TEXT        NOT NULL,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    roles         TEXT[]      NOT NULL DEFAULT '{}',
    tenant_id     TEXT        NOT NULL DEFAULT 'default',
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_email_unique    UNIQUE (email)
);

CREATE INDEX IF NOT EXISTS idx_users_username  ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_email     ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users (tenant_id);

COMMENT ON TABLE  users                IS 'Platform users. Passwords stored as bcrypt hashes only.';
COMMENT ON COLUMN users.roles          IS 'Array of role names e.g. {ADMIN,TELLER}.';
COMMENT ON COLUMN users.tenant_id      IS 'Tenant isolation key.';

-- ── Seed: default admin user ──────────────────────────────────────────────────
-- Password: Admin@12345
-- Hash generated with bcrypt cost 12.
-- CHANGE THIS PASSWORD immediately in any non-local environment.
INSERT INTO users (id, username, email, password_hash, roles, tenant_id)
VALUES (
    'usr_admin_001',
    'admin',
    'admin@banking.local',
    '$2b$12$qD2KBhzxixHH5inNtiNf9ec33TtHOgMKjy.pP76xjvnm2dCXZumHm',
    ARRAY['ADMIN'],
    'default'
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    updated_at    = NOW();

-- ── Seed: teller user ─────────────────────────────────────────────────────────
-- Password: Teller@12345
INSERT INTO users (id, username, email, password_hash, roles, tenant_id)
VALUES (
    'usr_teller_001',
    'teller',
    'teller@banking.local',
    '$2b$12$8kcWz/y02Wia9sf0HI6kuu1rK0uHpaa3hHFjw/k6nkjFsReCPQ6WO',
    ARRAY['TELLER'],
    'default'
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    updated_at    = NOW();

COMMIT;
