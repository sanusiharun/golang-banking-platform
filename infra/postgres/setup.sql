-- ─────────────────────────────────────────────────────────────────────────────
-- Banking Platform — PostgreSQL Setup Script
-- Run from the monorepo root as a superuser:
--
--   psql -h localhost -U admin -d postgres -f infra/postgres/setup.sql
--
-- Creates:
--   • banking_auth     database  → owned by auth_svc     (auth-svc only)
--   • banking_accounts database  → owned by account_svc  (account-svc only)
--
-- Principle: no service user can connect to another service's database.
-- ─────────────────────────────────────────────────────────────────────────────

-- Continue even if a statement errors (e.g. user already exists on re-run).
\set ON_ERROR_STOP off

-- ── 1. Service users ──────────────────────────────────────────────────────────
-- CREATE USER fails silently if the user already exists (ON_ERROR_STOP off).
-- ALTER USER always runs after, so the password is always correct on re-run.
-- Change passwords before deploying to any non-local environment.

CREATE USER auth_svc    WITH PASSWORD 'auth_svc_pass_local';
CREATE USER account_svc WITH PASSWORD 'account_svc_pass_local';

-- Always enforce the correct password even if CREATE USER was skipped above.
\set ON_ERROR_STOP on
ALTER USER auth_svc    WITH PASSWORD 'auth_svc_pass_local';
ALTER USER account_svc WITH PASSWORD 'account_svc_pass_local';
\set ON_ERROR_STOP off

-- ── 2. Databases ──────────────────────────────────────────────────────────────
-- Inherits locale from the cluster — avoids macOS locale compatibility issues.

CREATE DATABASE banking_auth     WITH OWNER = auth_svc    ENCODING = 'UTF8';
CREATE DATABASE banking_accounts WITH OWNER = account_svc ENCODING = 'UTF8';

-- ── 3. Revoke default public connect ─────────────────────────────────────────
-- PostgreSQL allows any user to connect to any database by default. Lock it.

REVOKE CONNECT ON DATABASE banking_auth     FROM PUBLIC;
REVOKE CONNECT ON DATABASE banking_accounts FROM PUBLIC;

GRANT  CONNECT ON DATABASE banking_auth     TO auth_svc;
GRANT  CONNECT ON DATABASE banking_accounts TO account_svc;

-- ── 4. Schema privileges inside banking_auth ─────────────────────────────────

\connect banking_auth

\set ON_ERROR_STOP off
GRANT USAGE,  CREATE ON SCHEMA public TO auth_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO auth_svc;
GRANT USAGE, UPDATE                  ON ALL SEQUENCES IN SCHEMA public TO auth_svc;

-- Future tables created in this schema automatically grant to auth_svc.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO auth_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, UPDATE                  ON SEQUENCES TO auth_svc;

-- ── 5. Schema privileges inside banking_accounts ─────────────────────────────

\connect banking_accounts

\set ON_ERROR_STOP off
GRANT USAGE,  CREATE ON SCHEMA public TO account_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO account_svc;
GRANT USAGE, UPDATE                  ON ALL SEQUENCES IN SCHEMA public TO account_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO account_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, UPDATE                  ON SEQUENCES TO account_svc;

-- ── Done ──────────────────────────────────────────────────────────────────────

\connect postgres
SELECT 'setup complete — databases: banking_auth, banking_accounts' AS status;
