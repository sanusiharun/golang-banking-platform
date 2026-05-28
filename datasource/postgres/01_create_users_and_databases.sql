-- ─────────────────────────────────────────────────────────────────────────────
-- Step 1: Create users and databases
-- Run this connected to: postgres (default db) as superuser
-- ─────────────────────────────────────────────────────────────────────────────

-- Users
CREATE USER auth_svc    WITH PASSWORD 'auth_svc_pass_local';
CREATE USER account_svc WITH PASSWORD 'account_svc_pass_local';

-- Enforce correct password even if users already exist
ALTER USER auth_svc    WITH PASSWORD 'auth_svc_pass_local';
ALTER USER account_svc WITH PASSWORD 'account_svc_pass_local';

-- Databases
CREATE DATABASE banking_auth     WITH OWNER = auth_svc    ENCODING = 'UTF8';
CREATE DATABASE banking_accounts WITH OWNER = account_svc ENCODING = 'UTF8';

-- Lock down public access
REVOKE CONNECT ON DATABASE banking_auth     FROM PUBLIC;
REVOKE CONNECT ON DATABASE banking_accounts FROM PUBLIC;

GRANT CONNECT ON DATABASE banking_auth     TO auth_svc;
GRANT CONNECT ON DATABASE banking_accounts TO account_svc;
