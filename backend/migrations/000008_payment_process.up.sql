-- EVENTHUB-235: simulated payment process for bookings.

-- Why a payment failed, e.g. 'declined'. Only failed payments carry a reason.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS failure_reason TEXT;

ALTER TABLE payments
    ADD CONSTRAINT payments_failure_reason_requires_failed
    CHECK (failure_reason IS NULL OR status = 'failed');

-- The amount is computed from the tickets and the event price and can never be negative.
-- Fail rather than silently modifying invalid legacy amounts.
ALTER TABLE payments
    ADD CONSTRAINT payments_amount_non_negative CHECK (amount >= 0);

-- A payment belongs to at most one booking; bookings.payment_id already allows one payment per booking.
ALTER TABLE bookings
    ADD CONSTRAINT bookings_payment_id_key UNIQUE (payment_id);
