ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_payment_id_key;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_amount_non_negative;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_failure_reason_requires_failed;
ALTER TABLE payments DROP COLUMN IF EXISTS failure_reason;
