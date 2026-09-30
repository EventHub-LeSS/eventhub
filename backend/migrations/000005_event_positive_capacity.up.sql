-- Fail rather than silently modifying invalid legacy capacities. Existing rows
-- with capacity <= 0 must be corrected before applying this migration.
ALTER TABLE events ADD CONSTRAINT events_capacity_positive CHECK (capacity > 0);

CREATE INDEX idx_bookings_confirmed_user_event
    ON bookings (user_id, event_id) WHERE status = 'confirmed';