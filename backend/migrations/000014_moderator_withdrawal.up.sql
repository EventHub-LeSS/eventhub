-- Preserve the distinction between an organizer's withdrawal and moderation.
ALTER TABLE events ADD COLUMN withdrawn_by_moderator BOOLEAN NOT NULL DEFAULT FALSE;
-- Earlier moderated withdrawals are recorded atomically in the audit log.
UPDATE events AS e SET withdrawn_by_moderator = TRUE
WHERE e.status = 'cancelled' AND EXISTS (
    SELECT 1 FROM audit_logs AS a
    WHERE a.resource_type = 'event' AND a.resource_id = e.event_id::text
      AND a.action = 'event.moderated'
      AND a.changes #>> '{fields,status,to}' = 'cancelled'
);
ALTER TABLE events ADD CONSTRAINT events_moderator_withdrawal_status
    CHECK (NOT withdrawn_by_moderator OR status = 'cancelled');