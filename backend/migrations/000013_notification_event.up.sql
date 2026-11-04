-- EVENTHUB-291: cancellation notices reference the withdrawn event instead of a single booking,
-- because a ticket holder receives one notice for all of their bookings of the event.
ALTER TABLE notification_data
    ADD COLUMN event_id UUID REFERENCES events(event_id) ON DELETE SET NULL;
CREATE UNIQUE INDEX idx_notification_event_cancelled
    ON notification_data (event_id, user_id) WHERE occasion = 'event_cancelled';
