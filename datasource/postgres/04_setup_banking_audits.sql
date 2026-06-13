-- ─────────────────────────────────────────────────────────────────────────────
-- Step 4: Create audit user, database, and schema privileges
-- This is a day-2 migration — audit-svc was not part of the initial bootstrap.
--
-- Part A: Run connected to: postgres (default db) as superuser
-- ─────────────────────────────────────────────────────────────────────────────

CREATE USER audit_svc WITH PASSWORD 'audit_svc_pass_local';

-- Enforce correct password even if user already exists
ALTER USER audit_svc WITH PASSWORD 'audit_svc_pass_local';

CREATE DATABASE banking_audit WITH OWNER = audit_svc ENCODING = 'UTF8';

-- Lock down public access
REVOKE CONNECT ON DATABASE banking_audit FROM PUBLIC;
GRANT  CONNECT ON DATABASE banking_audit TO audit_svc;

-- ─────────────────────────────────────────────────────────────────────────────
-- Part B: Switch to banking_audit and apply schema privileges
-- ─────────────────────────────────────────────────────────────────────────────

\connect banking_audit

GRANT USAGE,  CREATE ON SCHEMA public TO audit_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO audit_svc;
GRANT USAGE,  UPDATE                 ON ALL SEQUENCES IN SCHEMA public TO audit_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO audit_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE,  UPDATE                 ON SEQUENCES TO audit_svc;