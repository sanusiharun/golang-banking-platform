-- ─────────────────────────────────────────────────────────────────────────────
-- Step 2: Schema privileges for banking_auth
-- Run this connected to: banking_auth database as superuser
-- ─────────────────────────────────────────────────────────────────────────────

GRANT USAGE,  CREATE ON SCHEMA public TO auth_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO auth_svc;
GRANT USAGE,  UPDATE                 ON ALL SEQUENCES IN SCHEMA public TO auth_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO auth_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE,  UPDATE                 ON SEQUENCES TO auth_svc;
