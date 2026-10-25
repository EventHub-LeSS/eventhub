ALTER TABLE notification_data
    ADD COLUMN occasion TEXT NOT NULL DEFAULT 'general',
    ADD COLUMN booking_id UUID REFERENCES bookings(booking_id) ON DELETE SET NULL,
    ADD COLUMN scheduled_at TIMESTAMPTZ,
    ADD COLUMN simulated BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX idx_notification_booking_reminder
    ON notification_data (booking_id) WHERE occasion = 'event_reminder';
CREATE INDEX idx_notification_user_created
    ON notification_data (user_id, created_at DESC, notification_id DESC);