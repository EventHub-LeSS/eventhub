ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'expired';

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_reserved_requires_expiry
    CHECK (status <> 'reserved' OR expires_at IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_bookings_event_status
    ON bookings (event_id, status);
