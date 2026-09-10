DROP INDEX IF EXISTS idx_bookings_event_status;

ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_reserved_requires_expiry;

ALTER TABLE bookings DROP COLUMN IF EXISTS expires_at;

-- Postgres cannot remove a value from an enum type; rebuild the type without 'expired'.
UPDATE bookings SET status = 'cancelled' WHERE status = 'expired';

ALTER TYPE booking_status RENAME TO booking_status_old;
CREATE TYPE booking_status AS ENUM ('reserved', 'confirmed', 'cancelled', 'failed');
ALTER TABLE bookings
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE booking_status USING status::text::booking_status,
    ALTER COLUMN status SET DEFAULT 'reserved';
DROP TYPE booking_status_old;
