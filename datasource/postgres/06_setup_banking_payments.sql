-- ─────────────────────────────────────────────────────────────────────────────
-- Step 6: Create payment_svc user, database, and schema privileges
-- This is a day-2 migration — payment-svc was not part of the initial bootstrap.
--
-- Part A: Run connected to: postgres (default db) as superuser
-- ─────────────────────────────────────────────────────────────────────────────

CREATE USER payment_svc WITH PASSWORD 'payment_svc_pass_local';

-- Enforce correct password even if user already exists
ALTER USER payment_svc WITH PASSWORD 'payment_svc_pass_local';

CREATE DATABASE banking_payments WITH OWNER = payment_svc ENCODING = 'UTF8';

-- Lock down public access
REVOKE CONNECT ON DATABASE banking_payments FROM PUBLIC;
GRANT  CONNECT ON DATABASE banking_payments TO payment_svc;

-- ─────────────────────────────────────────────────────────────────────────────
-- Part B: Switch to banking_payments and apply schema privileges
-- ─────────────────────────────────────────────────────────────────────────────

\connect banking_payments

GRANT USAGE,  CREATE ON SCHEMA public TO payment_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO payment_svc;
GRANT USAGE,  UPDATE                 ON ALL SEQUENCES IN SCHEMA public TO payment_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO payment_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE,  UPDATE                 ON SEQUENCES TO payment_svc;
