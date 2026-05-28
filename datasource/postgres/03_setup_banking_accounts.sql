-- ─────────────────────────────────────────────────────────────────────────────
-- Step 3: Schema privileges for banking_accounts
-- Run this connected to: banking_accounts database as superuser
-- ─────────────────────────────────────────────────────────────────────────────

GRANT USAGE,  CREATE ON SCHEMA public TO account_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO account_svc;
GRANT USAGE,  UPDATE                 ON ALL SEQUENCES IN SCHEMA public TO account_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO account_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE,  UPDATE                 ON SEQUENCES TO account_svc;
