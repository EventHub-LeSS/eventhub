package service

import (
	"backend/internal/model"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The simulated payment provider is plain validation and needs no database.

var simulationNow = time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)

func validCard() *model.CreditCardDetails {
	return &model.CreditCardDetails{Cardholder: "Max Mustermann", Number: "4242 4242 4242 4242", Expiry: "12/30", CVC: "123"}
}

func validDirectDebit() *model.DirectDebitDetails {
	return &model.DirectDebitDetails{AccountHolder: "Max Mustermann", IBAN: "DE89 3704 0044 0532 0130 00", MandateAccepted: true}
}

func cardInput(card *model.CreditCardDetails) *model.PaymentMethodInput {
	return &model.PaymentMethodInput{Method: model.PaymentMethodCreditCard, CreditCard: card}
}

func debitInput(debit *model.DirectDebitDetails) *model.PaymentMethodInput {
	return &model.PaymentMethodInput{Method: model.PaymentMethodDirectDebit, DirectDebit: debit}
}

func payPalInput(email string) *model.PaymentMethodInput {
	return &model.PaymentMethodInput{Method: model.PaymentMethodPayPal, PayPal: &model.PayPalDetails{Email: email}}
}

var transactionIDPattern = regexp.MustCompile(`^SIM-(CC|DD|PP)-[0-9A-F]{12}$`)

func TestPrepareCharge_WithoutMethodLeavesTheOutcomeAlone(t *testing.T) {
	charge, err := prepareCharge(nil, model.SimulatedPaymentSuccess, simulationNow)
	if err != nil || charge != nil {
		t.Fatalf("expected no charge without payment method, got %+v, %v", charge, err)
	}
}

func TestPrepareCharge_CreditCard(t *testing.T) {
	cases := []struct {
		name    string
		number  string
		cvc     string
		brand   model.CardBrand
		account string
	}{
		{"visa", "4242 4242 4242 4242", "123", model.CardBrandVisa, "**** 4242"},
		{"visa with dashes", "4111-1111-1111-1111", "123", model.CardBrandVisa, "**** 1111"},
		{"mastercard 5x", "5555555555554444", "123", model.CardBrandMastercard, "**** 4444"},
		{"mastercard 2x", "2223003122003222", "123", model.CardBrandMastercard, "**** 3222"},
		{"american express", "3782 822463 10005", "1234", model.CardBrandAmex, "**** 0005"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := validCard()
			card.Number, card.CVC = tc.number, tc.cvc
			charge, err := prepareCharge(cardInput(card), model.SimulatedPaymentSuccess, simulationNow)
			if err != nil {
				t.Fatalf("expected a valid card, got %v", err)
			}
			if charge.method != model.PaymentMethodCreditCard || charge.brand == nil || *charge.brand != tc.brand {
				t.Errorf("expected %s card, got %+v", tc.brand, charge)
			}
			if charge.account == nil || *charge.account != tc.account {
				t.Errorf("expected masked account %q, got %v", tc.account, charge.account)
			}
			if charge.declinedByProvider || charge.declines(model.SimulatedPaymentSuccess) {
				t.Error("a valid card that is no test data must be charged")
			}
			if !charge.declines(model.SimulatedPaymentFailure) {
				t.Error("the failure outcome must decline every card")
			}
			if !transactionIDPattern.MatchString(charge.transactionID) || !strings.HasPrefix(charge.transactionID, "SIM-CC-") {
				t.Errorf("unexpected transaction ID %q", charge.transactionID)
			}
		})
	}
}

func TestPrepareCharge_DeclinedTestCard(t *testing.T) {
	card := validCard()
	card.Number = "4000 0000 0000 0002"
	charge, err := prepareCharge(cardInput(card), model.SimulatedPaymentSuccess, simulationNow)
	if err != nil {
		t.Fatalf("the declining test card is a valid card, got %v", err)
	}
	if !charge.declines(model.SimulatedPaymentSuccess) {
		t.Error("the provider must decline its test card")
	}
}

func TestPrepareCharge_InvalidCreditCard(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*model.CreditCardDetails)
	}{
		{"missing cardholder", func(c *model.CreditCardDetails) { c.Cardholder = "  " }},
		{"one letter cardholder", func(c *model.CreditCardDetails) { c.Cardholder = "M" }},
		{"too short number", func(c *model.CreditCardDetails) { c.Number = "42424242424" }},
		{"too long number", func(c *model.CreditCardDetails) { c.Number = "42424242424242424242" }},
		{"letters in number", func(c *model.CreditCardDetails) { c.Number = "4242 4242 4242 424A" }},
		{"luhn check fails", func(c *model.CreditCardDetails) { c.Number = "4242 4242 4242 4241" }},
		{"unsupported brand", func(c *model.CreditCardDetails) { c.Number = "6011111111111117" }},
		// Passes the Luhn check, but Visa numbers have 13, 16 or 19 digits.
		{"visa with wrong length", func(c *model.CreditCardDetails) { c.Number = "424242424242424" }},
		{"expiry without slash", func(c *model.CreditCardDetails) { c.Expiry = "1230" }},
		{"expiry month 13", func(c *model.CreditCardDetails) { c.Expiry = "13/30" }},
		{"expiry with four digit year", func(c *model.CreditCardDetails) { c.Expiry = "12/2030" }},
		{"expired last month", func(c *model.CreditCardDetails) { c.Expiry = "10/26" }},
		{"cvc with two digits", func(c *model.CreditCardDetails) { c.CVC = "12" }},
		{"cvc with four digits on visa", func(c *model.CreditCardDetails) { c.CVC = "1234" }},
		{"cvc with letters", func(c *model.CreditCardDetails) { c.CVC = "12a" }},
		{"amex with three digit cvc", func(c *model.CreditCardDetails) { c.Number, c.CVC = "378282246310005", "123" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := validCard()
			tc.modify(card)
			if _, err := prepareCharge(cardInput(card), model.SimulatedPaymentSuccess, simulationNow); !errors.Is(err, ErrInvalidPaymentDetails) {
				t.Errorf("expected ErrInvalidPaymentDetails, got %v", err)
			}
		})
	}
}

func TestCheckCardExpiry_ValidThroughTheEndOfTheMonth(t *testing.T) {
	endOfMonth := time.Date(2026, time.November, 30, 23, 59, 59, 0, time.UTC)
	if err := checkCardExpiry("11/26", endOfMonth); err != nil {
		t.Errorf("a card is valid through its expiry month, got %v", err)
	}
	if err := checkCardExpiry("1/27", endOfMonth); err != nil {
		t.Errorf("a one digit month is accepted, got %v", err)
	}
	if err := checkCardExpiry("11/26", endOfMonth.Add(time.Second)); !errors.Is(err, ErrInvalidPaymentDetails) {
		t.Errorf("a card expires after its expiry month, got %v", err)
	}
}

func TestPrepareCharge_DirectDebit(t *testing.T) {
	cases := []struct {
		name    string
		iban    string
		account string
	}{
		{"german iban with spaces", "DE89 3704 0044 0532 0130 00", "DE89 **** 3000"},
		{"lower case without spaces", "de89370400440532013000", "DE89 **** 3000"},
		{"austrian iban", "AT61 1904 3002 3457 3201", "AT61 **** 3201"},
		{"dutch iban with letters", "NL91 ABNA 0417 1643 00", "NL91 **** 4300"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			debit := validDirectDebit()
			debit.IBAN = tc.iban
			charge, err := prepareCharge(debitInput(debit), model.SimulatedPaymentSuccess, simulationNow)
			if err != nil {
				t.Fatalf("expected a valid direct debit, got %v", err)
			}
			if charge.method != model.PaymentMethodDirectDebit || charge.brand != nil || charge.account == nil || *charge.account != tc.account {
				t.Errorf("expected direct debit from %q, got %+v", tc.account, charge)
			}
			if charge.declines(model.SimulatedPaymentSuccess) {
				t.Error("a valid IBAN that is no test data must be charged")
			}
			if !strings.HasPrefix(charge.transactionID, "SIM-DD-") || len(charge.transactionID) > 35 {
				t.Errorf("the mandate reference must fit SEPA's 35 characters, got %q", charge.transactionID)
			}
		})
	}

	debit := validDirectDebit()
	debit.IBAN = "DE62 3704 0044 0532 0130 01"
	charge, err := prepareCharge(debitInput(debit), model.SimulatedPaymentSuccess, simulationNow)
	if err != nil || !charge.declines(model.SimulatedPaymentSuccess) {
		t.Errorf("the provider must decline its test IBAN, got %+v, %v", charge, err)
	}
}

func TestPrepareCharge_InvalidDirectDebit(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*model.DirectDebitDetails)
	}{
		{"missing account holder", func(d *model.DirectDebitDetails) { d.AccountHolder = "" }},
		{"missing iban", func(d *model.DirectDebitDetails) { d.IBAN = " " }},
		{"country outside sepa", func(d *model.DirectDebitDetails) { d.IBAN = "US89370400440532013000" }},
		{"too short for the country", func(d *model.DirectDebitDetails) { d.IBAN = "DE8937040044053201300" }},
		{"invalid characters", func(d *model.DirectDebitDetails) { d.IBAN = "DE89-3704-0044-0532-0130-00" }},
		{"wrong check digits", func(d *model.DirectDebitDetails) { d.IBAN = "DE00370400440532013000" }},
		{"mandate not accepted", func(d *model.DirectDebitDetails) { d.MandateAccepted = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			debit := validDirectDebit()
			tc.modify(debit)
			if _, err := prepareCharge(debitInput(debit), model.SimulatedPaymentSuccess, simulationNow); !errors.Is(err, ErrInvalidPaymentDetails) {
				t.Errorf("expected ErrInvalidPaymentDetails, got %v", err)
			}
		})
	}
}

func TestPrepareCharge_PayPal(t *testing.T) {
	charge, err := prepareCharge(payPalInput("max.mustermann@example.com"), model.SimulatedPaymentSuccess, simulationNow)
	if err != nil {
		t.Fatalf("expected a valid PayPal account, got %v", err)
	}
	if charge.method != model.PaymentMethodPayPal || charge.account == nil || *charge.account != "m***@example.com" || charge.brand != nil {
		t.Errorf("expected masked PayPal account, got %+v", charge)
	}
	if charge.declines(model.SimulatedPaymentSuccess) || !strings.HasPrefix(charge.transactionID, "SIM-PP-") {
		t.Errorf("expected a charged PayPal payment, got %+v", charge)
	}

	// PayPal Express: the buyer approves at PayPal, so no account needs to be sent.
	for _, in := range []*model.PaymentMethodInput{{Method: model.PaymentMethodPayPal}, payPalInput("")} {
		charge, err := prepareCharge(in, model.SimulatedPaymentSuccess, simulationNow)
		if err != nil || charge.account != nil || charge.declines(model.SimulatedPaymentSuccess) {
			t.Errorf("expected PayPal Express without account, got %+v, %v", charge, err)
		}
	}

	charge, err = prepareCharge(payPalInput("PayPal-Declined@Example.com"), model.SimulatedPaymentSuccess, simulationNow)
	if err != nil || !charge.declines(model.SimulatedPaymentSuccess) {
		t.Errorf("the provider must decline its test account, got %+v, %v", charge, err)
	}

	for _, email := range []string{"not-an-email", "Max <max@example.com>", "max@", "  ", strings.Repeat("a", 250) + "@example.com"} {
		if _, err := prepareCharge(payPalInput(email), model.SimulatedPaymentSuccess, simulationNow); !errors.Is(err, ErrInvalidPaymentDetails) {
			t.Errorf("%q: expected ErrInvalidPaymentDetails, got %v", email, err)
		}
	}
}

func TestPrepareCharge_MethodAndDetailsMustMatch(t *testing.T) {
	cases := []struct {
		name string
		in   *model.PaymentMethodInput
		want error
	}{
		{"details without method", &model.PaymentMethodInput{CreditCard: validCard()}, ErrPaymentMethodRequired},
		{"unknown method", &model.PaymentMethodInput{Method: "bitcoin"}, ErrInvalidPaymentMethod},
		{"card details for paypal", &model.PaymentMethodInput{Method: model.PaymentMethodPayPal, CreditCard: validCard()}, ErrInvalidPaymentDetails},
		{"debit details for card", &model.PaymentMethodInput{Method: model.PaymentMethodCreditCard, CreditCard: validCard(), DirectDebit: validDirectDebit()}, ErrInvalidPaymentDetails},
		{"paypal details for direct debit", &model.PaymentMethodInput{Method: model.PaymentMethodDirectDebit, DirectDebit: validDirectDebit(), PayPal: &model.PayPalDetails{}}, ErrInvalidPaymentDetails},
		{"card payment without card", &model.PaymentMethodInput{Method: model.PaymentMethodCreditCard}, ErrInvalidPaymentDetails},
		{"direct debit without account", &model.PaymentMethodInput{Method: model.PaymentMethodDirectDebit}, ErrInvalidPaymentDetails},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := prepareCharge(tc.in, model.SimulatedPaymentSuccess, simulationNow); !errors.Is(err, tc.want) {
				t.Errorf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

// Declining needs no card or account data, but data that is sent is still validated.
func TestPrepareCharge_DeclineNeedsNoDetails(t *testing.T) {
	for _, method := range []model.PaymentMethod{model.PaymentMethodCreditCard, model.PaymentMethodDirectDebit, model.PaymentMethodPayPal} {
		charge, err := prepareCharge(&model.PaymentMethodInput{Method: method}, model.SimulatedPaymentFailure, simulationNow)
		if err != nil || charge.method != method || charge.account != nil || !charge.declines(model.SimulatedPaymentFailure) {
			t.Errorf("%s: expected a declined charge without details, got %+v, %v", method, charge, err)
		}
	}
	card := validCard()
	card.Number = "1234"
	if _, err := prepareCharge(cardInput(card), model.SimulatedPaymentFailure, simulationNow); !errors.Is(err, ErrInvalidPaymentDetails) {
		t.Errorf("invalid details must be rejected on decline as well, got %v", err)
	}
}

func TestRecord_StoresOnlyMaskedData(t *testing.T) {
	card := validCard()
	charge, err := prepareCharge(cardInput(card), model.SimulatedPaymentSuccess, simulationNow)
	if err != nil {
		t.Fatal(err)
	}
	payment := &model.PaymentModel{Status: model.PaymentStatusPending}
	charge.record(payment)
	if payment.PaymentMethod == nil || *payment.PaymentMethod != model.PaymentMethodCreditCard ||
		payment.PaymentAccount == nil || *payment.PaymentAccount != "**** 4242" ||
		payment.CardBrand == nil || *payment.CardBrand != model.CardBrandVisa ||
		payment.TransactionID == nil || *payment.TransactionID != charge.transactionID {
		t.Errorf("expected the charge on the payment, got %+v", payment)
	}
	if strings.Contains(*payment.PaymentAccount, "4242 4242") {
		t.Error("the card number must never be stored in full")
	}
}

func TestDeclinedMessage_NamesTheMethod(t *testing.T) {
	seen := map[string]bool{}
	methods := []*model.PaymentMethod{nil}
	for _, m := range []model.PaymentMethod{model.PaymentMethodCreditCard, model.PaymentMethodDirectDebit, model.PaymentMethodPayPal} {
		methods = append(methods, &m)
	}
	for _, method := range methods {
		msg := declinedMessage(method)
		if !strings.Contains(msg, "No tickets were booked") || seen[msg] {
			t.Errorf("expected a distinct decline message, got %q", msg)
		}
		seen[msg] = true
	}
}
