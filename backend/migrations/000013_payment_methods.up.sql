-- Simulated payment methods: credit card, SEPA direct debit and PayPal.
--
-- The simulated payment provider records how a payment was made once it charged or declined it.
-- Card numbers, CVCs and IBANs are never stored: only a masked account ("**** 4242"), the card
-- network and the provider's reference of the payment attempt.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_method TEXT;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_account TEXT;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS card_brand TEXT;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS transaction_id TEXT;

ALTER TABLE payments
    ADD CONSTRAINT payments_payment_method_valid
    CHECK (payment_method IS NULL OR payment_method IN ('credit_card', 'direct_debit', 'paypal'));

-- Only card payments have a card network.
ALTER TABLE payments
    ADD CONSTRAINT payments_card_brand_requires_credit_card
    CHECK (card_brand IS NULL OR (payment_method = 'credit_card' AND card_brand IN ('visa', 'mastercard', 'amex')));

-- Account and provider reference describe the payment method; there are none without one.
ALTER TABLE payments
    ADD CONSTRAINT payments_method_details_require_method
    CHECK (payment_method IS NOT NULL OR (payment_account IS NULL AND transaction_id IS NULL));

-- The method is recorded when the provider is asked, which completes the payment, so a pending
-- payment has none yet.
ALTER TABLE payments
    ADD CONSTRAINT payments_method_requires_completion
    CHECK (payment_method IS NULL OR status <> 'pending');
