-- ─────────────────────────────────────────────────────────────────────────────
-- Step 5: Create notification_svc user, database, and schema privileges
-- This is a day-2 migration — notification-svc was not part of the initial bootstrap.
--
-- Part A: Run connected to: postgres (default db) as superuser
-- ─────────────────────────────────────────────────────────────────────────────

CREATE USER notif_svc WITH PASSWORD 'notif_svc_pass_local';

-- Enforce correct password even if user already exists
ALTER USER notif_svc WITH PASSWORD 'notif_svc_pass_local';

CREATE DATABASE banking_notifications WITH OWNER = notif_svc ENCODING = 'UTF8';

-- Lock down public access
REVOKE CONNECT ON DATABASE banking_notifications FROM PUBLIC;
GRANT  CONNECT ON DATABASE banking_notifications TO notif_svc;

-- ─────────────────────────────────────────────────────────────────────────────
-- Part B: Switch to banking_notifications and apply schema privileges
-- ─────────────────────────────────────────────────────────────────────────────

\connect banking_notifications

GRANT USAGE,  CREATE ON SCHEMA public TO notif_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES    IN SCHEMA public TO notif_svc;
GRANT USAGE,  UPDATE                 ON ALL SEQUENCES IN SCHEMA public TO notif_svc;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES    TO notif_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE,  UPDATE                 ON SEQUENCES TO notif_svc;
