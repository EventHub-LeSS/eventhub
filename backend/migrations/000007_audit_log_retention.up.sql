-- Retention of the audit log: entries are deleted ten calendar years after they were recorded.
-- The deletion runs inside PostgreSQL via pg_cron, independent of the API. This migration fails if
-- pg_cron is not available, so a missing retention never goes unnoticed. See core/Dockerfile.api-db.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_cron') THEN
        RAISE EXCEPTION 'pg_cron is not installed; the audit log retention requires it (see core/Dockerfile.api-db)';
    END IF;
    IF current_setting('shared_preload_libraries') NOT LIKE '%pg_cron%' THEN
        RAISE EXCEPTION 'pg_cron is not in shared_preload_libraries; the audit log retention would never run';
    END IF;
    IF current_setting('cron.database_name', true) IS DISTINCT FROM current_database() THEN
        RAISE EXCEPTION 'cron.database_name must be %, it is %', current_database(), current_setting('cron.database_name', true);
    END IF;
END
$$;

CREATE EXTENSION IF NOT EXISTS pg_cron;

CREATE OR REPLACE FUNCTION purge_expired_audit_logs() RETURNS bigint
LANGUAGE plpgsql AS $$
DECLARE
    deleted bigint;
BEGIN
    -- Calendar years in UTC, not an approximation like 3650 days.
    DELETE FROM audit_logs
    WHERE occurred_at <= ((now() AT TIME ZONE 'UTC') - INTERVAL '10 years') AT TIME ZONE 'UTC';
    GET DIAGNOSTICS deleted = ROW_COUNT;
    RETURN deleted;
END
$$;

-- Named jobs are replaced on every call, so this is safe to run again. Daily at 03:00 UTC.
SELECT cron.schedule('audit-log-retention', '0 3 * * *', 'SELECT purge_expired_audit_logs()');
