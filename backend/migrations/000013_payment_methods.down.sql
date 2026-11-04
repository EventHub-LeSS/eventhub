ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_method_requires_completion;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_method_details_require_method;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_card_brand_requires_credit_card;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_payment_method_valid;

ALTER TABLE payments DROP COLUMN IF EXISTS transaction_id;
ALTER TABLE payments DROP COLUMN IF EXISTS card_brand;
ALTER TABLE payments DROP COLUMN IF EXISTS payment_account;
ALTER TABLE payments DROP COLUMN IF EXISTS payment_method;
