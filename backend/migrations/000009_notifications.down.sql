DROP INDEX idx_notification_user_created;
DROP INDEX idx_notification_booking_reminder;
ALTER TABLE notification_data DROP COLUMN simulated, DROP COLUMN scheduled_at,
    DROP COLUMN booking_id, DROP COLUMN occasion;