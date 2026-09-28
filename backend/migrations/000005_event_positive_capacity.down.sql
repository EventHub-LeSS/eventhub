DROP INDEX IF EXISTS idx_bookings_confirmed_user_event;
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_capacity_positive;