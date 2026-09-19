ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'expired';

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Reservations created before this migration have no expiry yet.
UPDATE bookings
SET expires_at = created_at + INTERVAL '15 minutes'
WHERE status = 'reserved' AND expires_at IS NULL;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_reserved_requires_expiry
    CHECK (status <> 'reserved' OR expires_at IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_bookings_event_status
    ON bookings (event_id, status);
