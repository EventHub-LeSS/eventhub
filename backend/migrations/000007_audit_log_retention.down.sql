DO $$
BEGIN
    -- The extension itself stays: other jobs may use it.
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_cron') THEN
        PERFORM cron.unschedule(jobid) FROM cron.job WHERE jobname = 'audit-log-retention';
    END IF;
END
$$;

DROP FUNCTION IF EXISTS purge_expired_audit_logs();
