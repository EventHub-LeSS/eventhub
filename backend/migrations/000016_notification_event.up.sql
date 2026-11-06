-- EVENTHUB-291: cancellation notices reference the withdrawn event instead of a single booking,
-- because a ticket holder receives one notice for all of their bookings of the event.
-- A withdrawn event can be published and withdrawn again (EVENTHUB-290); every withdrawal notifies
-- its ticket holders anew, so a user can hold several notices for the same event and the index is
-- not unique.
ALTER TABLE notification_data
    ADD COLUMN event_id UUID REFERENCES events(event_id) ON DELETE SET NULL;
CREATE INDEX idx_notification_event
    ON notification_data (event_id) WHERE event_id IS NOT NULL;
