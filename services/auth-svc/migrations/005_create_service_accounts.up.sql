-- 005: service_accounts
-- Service accounts are non-human identities (internal services, external partners).
-- They authenticate using API keys rather than passwords.

CREATE TABLE IF NOT EXISTS service_accounts (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT,
    tenant_id   TEXT        NOT NULL DEFAULT 'default',
    roles       TEXT[]      NOT NULL DEFAULT '{}',
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_by  TEXT        NOT NULL,   -- user_id of the human who created it
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_service_accounts_tenant ON service_accounts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_service_accounts_active ON service_accounts(is_active) WHERE is_active = TRUE;
