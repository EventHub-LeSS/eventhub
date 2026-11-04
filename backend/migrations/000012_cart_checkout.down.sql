-- Undoing the cart checkout requires that no payment covers more than one booking; the
-- unique constraint below fails on such data, by design.
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_payment_requires_line_amount;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_line_amount_non_negative;
ALTER TABLE bookings DROP COLUMN IF EXISTS line_amount;

DROP INDEX IF EXISTS idx_bookings_payment;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_payment_id_key UNIQUE (payment_id);
