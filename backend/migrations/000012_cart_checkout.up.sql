-- 282: Warenkorb — tickets of several events are collected in a cart and paid together.
--
-- The cart of a user is the set of their live reservations: bookings with status 'reserved',
-- no payment yet and an unexpired expires_at. Adding tickets to the cart reserves capacity
-- right away, like a booking did before; the 15-minute reservation TTL carries over unchanged.
-- No extra cart table is needed.

-- One payment can now pay the bookings of a whole cart checkout. bookings.payment_id stays a
-- foreign key, but several bookings may share one payment, so it loses its uniqueness. The
-- index keeps the payment -> bookings join fast.
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_payment_id_key;

CREATE INDEX IF NOT EXISTS idx_bookings_payment ON bookings (payment_id);

-- What one booking of a payment costs: its tickets times the event price at checkout time.
-- The payment amount is the sum of the line amounts, so prices are handled per event, and
-- refunds of single bookings refund exactly the line amount that was charged for them.
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS line_amount NUMERIC(12,2);

ALTER TABLE bookings
    ADD CONSTRAINT bookings_line_amount_non_negative
    CHECK (line_amount IS NULL OR line_amount >= 0);

-- Legacy rows: every payment covered exactly one booking, so its line is the whole amount.
-- Reservations without a payment (cart items never checked out) keep line_amount NULL.
UPDATE bookings b
SET line_amount = p.amount
FROM payments p
WHERE b.payment_id = p.payment_id AND b.line_amount IS NULL;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_payment_requires_line_amount
    CHECK (payment_id IS NULL OR line_amount IS NOT NULL);
