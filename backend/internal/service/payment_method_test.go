package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The payment methods of the simulated payment provider, end to end against a real Postgres like
// the other payment tests.

type storedMethod struct {
	Status         model.PaymentStatus
	FailureReason  *model.PaymentFailureReason
	PaymentMethod  *model.PaymentMethod
	PaymentAccount *string
	CardBrand      *model.CardBrand
	TransactionID  *string
}

// storedPaymentMethod reads what the payment stored about its payment method, bypassing the service.
func storedPaymentMethod(t *testing.T, db *gorm.DB, paymentID uuid.UUID) storedMethod {
	t.Helper()
	var s storedMethod
	err := db.Raw(`SELECT status, failure_reason, payment_method, payment_account, card_brand, transaction_id
	               FROM payments WHERE payment_id = ?`, paymentID).Scan(&s).Error
	if err != nil {
		t.Fatalf("load payment: %v", err)
	}
	return s
}

func TestSimulatePaymentWithMethod_CreditCardChargesAndRecordsTheCard(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 2)
	pending := f.checkout(t)
	if pending.PaymentMethod != nil || pending.TransactionID != nil {
		t.Fatalf("a pending payment has no payment method yet, got %+v", pending)
	}

	resp, err := f.payments.SimulatePaymentWithMethod(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, cardInput(validCard()))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusPaid || !resp.TicketsValid || resp.FailureReason != nil {
		t.Fatalf("expected a paid payment with valid tickets, got %+v", resp)
	}
	if resp.PaymentMethod == nil || *resp.PaymentMethod != model.PaymentMethodCreditCard ||
		resp.PaymentAccount == nil || *resp.PaymentAccount != "**** 4242" ||
		resp.CardBrand == nil || *resp.CardBrand != model.CardBrandVisa ||
		resp.TransactionID == nil || !transactionIDPattern.MatchString(*resp.TransactionID) {
		t.Errorf("expected the masked Visa card with a transaction ID, got %+v", resp)
	}

	s := storedPaymentMethod(t, f.db, pending.PaymentID)
	if s.PaymentMethod == nil || *s.PaymentMethod != model.PaymentMethodCreditCard || *s.PaymentAccount != "**** 4242" ||
		*s.CardBrand != model.CardBrandVisa || *s.TransactionID != *resp.TransactionID {
		t.Errorf("expected the card stored on the payment, got %+v", s)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 2 {
		t.Errorf("expected 2 sold tickets, got %d", n)
	}

	// The stored result and the own booking show the same method.
	got, err := f.payments.GetPayment(context.Background(), pending.PaymentID, f.userID)
	if err != nil || got.PaymentMethod == nil || *got.PaymentMethod != model.PaymentMethodCreditCard || *got.TransactionID != *resp.TransactionID {
		t.Errorf("expected the stored card payment, got %+v, %v", got, err)
	}
	own, err := NewOwnBookingService(repository.NewOwnBookingRepository(f.db)).GetOwnBooking(bookingID, f.userID)
	if err != nil || own.PaymentMethod == nil || *own.PaymentMethod != model.PaymentMethodCreditCard {
		t.Errorf("expected the payment method on the own booking, got %+v, %v", own, err)
	}
}

func TestSimulatePaymentWithMethod_DirectDebitAndPayPalAreCharged(t *testing.T) {
	cases := []struct {
		name    string
		in      *model.PaymentMethodInput
		method  model.PaymentMethod
		account *string
	}{
		{"direct debit", debitInput(validDirectDebit()), model.PaymentMethodDirectDebit, ptr("DE89 **** 3000")},
		{"paypal with account", payPalInput("max@example.com"), model.PaymentMethodPayPal, ptr("m***@example.com")},
		{"paypal express without account", &model.PaymentMethodInput{Method: model.PaymentMethodPayPal}, model.PaymentMethodPayPal, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentFixture(t, 10, "15.00")
			f.reserve(t, 1)
			pending := f.checkout(t)

			resp, err := f.payments.SimulatePaymentWithMethod(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, tc.in)
			if err != nil {
				t.Fatalf("simulate: %v", err)
			}
			if resp.Status != model.PaymentStatusPaid || !resp.TicketsValid {
				t.Fatalf("expected a paid payment, got %+v", resp)
			}
			if resp.PaymentMethod == nil || *resp.PaymentMethod != tc.method || resp.CardBrand != nil || resp.TransactionID == nil {
				t.Errorf("expected a %s payment without card brand, got %+v", tc.method, resp)
			}
			if (tc.account == nil) != (resp.PaymentAccount == nil) || (tc.account != nil && *resp.PaymentAccount != *tc.account) {
				t.Errorf("expected account %v, got %v", tc.account, resp.PaymentAccount)
			}
		})
	}
}

// The provider declines its test data: the payment fails like a simulated failure, with the
// method recorded and a message that names it.
func TestSimulatePaymentWithMethod_ProviderDeclinesItsTestData(t *testing.T) {
	declinedCard := validCard()
	declinedCard.Number = "4000 0000 0000 0002"
	declinedDebit := validDirectDebit()
	declinedDebit.IBAN = "DE62 3704 0044 0532 0130 01"
	cases := []struct {
		name    string
		in      *model.PaymentMethodInput
		method  model.PaymentMethod
		message string
	}{
		{"declined card", cardInput(declinedCard), model.PaymentMethodCreditCard, "credit card was declined"},
		{"declined iban", debitInput(declinedDebit), model.PaymentMethodDirectDebit, "bank rejected the direct debit"},
		{"declined paypal account", payPalInput("paypal-declined@example.com"), model.PaymentMethodPayPal, "PayPal declined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentFixture(t, 2, "20.00")
			bookingID := f.reserve(t, 2)
			pending := f.checkout(t)

			resp, err := f.payments.SimulatePaymentWithMethod(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, tc.in)
			if err != nil {
				t.Fatalf("simulate: %v", err)
			}
			if resp.Status != model.PaymentStatusFailed || resp.TicketsValid || resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureDeclined {
				t.Fatalf("expected a declined payment, got %+v", resp)
			}
			if resp.PaymentMethod == nil || *resp.PaymentMethod != tc.method || resp.TransactionID == nil {
				t.Errorf("a declined payment records its method, got %+v", resp)
			}
			if !strings.Contains(resp.Message, tc.message) {
				t.Errorf("expected the message to contain %q, got %q", tc.message, resp.Message)
			}
			if item := paymentItemFor(resp, bookingID); item == nil || item.BookingStatus != model.BookingStatusFailed {
				t.Errorf("expected the failed item, got %+v", item)
			}
			// The own booking words the result exactly like the payment.
			own, err := NewOwnBookingService(repository.NewOwnBookingRepository(f.db)).GetOwnBooking(bookingID, f.userID)
			if err != nil || own.Message != resp.Message || own.PaymentMethod == nil || *own.PaymentMethod != tc.method {
				t.Errorf("expected the own booking to match the payment, got %+v, %v", own, err)
			}
			// The tickets are free again.
			if _, err := f.carts.AddToCart(context.Background(), f.userID, f.eventID, 2); err != nil {
				t.Errorf("expected the released tickets to be bookable, got %v", err)
			}
		})
	}
}

func TestSimulatePaymentWithMethod_FailureOutcomeDeclinesWithoutDetails(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)

	in := &model.PaymentMethodInput{Method: model.PaymentMethodCreditCard}
	resp, err := f.payments.SimulatePaymentWithMethod(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentFailure, pending.Amount, in)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || *resp.FailureReason != model.PaymentFailureDeclined ||
		resp.PaymentMethod == nil || *resp.PaymentMethod != model.PaymentMethodCreditCard || resp.PaymentAccount != nil {
		t.Errorf("expected a declined card payment without account, got %+v", resp)
	}
}

// Invalid payment details are rejected before anything changes: the payment stays pending and the
// payer can correct the details.
func TestSimulatePaymentWithMethod_InvalidDetailsChangeNothing(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	bookingID := f.reserve(t, 1)
	pending := f.checkout(t)
	ctx := context.Background()

	expired := validCard()
	expired.Expiry = "01/20"
	noMandate := validDirectDebit()
	noMandate.MandateAccepted = false
	for _, in := range []*model.PaymentMethodInput{
		cardInput(expired),
		debitInput(noMandate),
		payPalInput("not-an-email"),
		{Method: model.PaymentMethodCreditCard},
		{Method: model.PaymentMethodPayPal, CreditCard: validCard()},
		{Method: "cash"},
		{DirectDebit: validDirectDebit()},
	} {
		_, err := f.payments.SimulatePaymentWithMethod(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, in)
		if !errors.Is(err, ErrInvalidPaymentDetails) && !errors.Is(err, ErrInvalidPaymentMethod) && !errors.Is(err, ErrPaymentMethodRequired) {
			t.Errorf("%+v: expected a rejection of the payment details, got %v", in, err)
		}
	}

	s := storedPaymentMethod(t, f.db, pending.PaymentID)
	if s.Status != model.PaymentStatusPending || s.PaymentMethod != nil || s.TransactionID != nil {
		t.Fatalf("rejected details must not change the payment, got %+v", s)
	}
	if b := stored(t, f.db, bookingID); b.BookingStatus != model.BookingStatusReserved {
		t.Fatalf("rejected details must not change the booking, got %s", b.BookingStatus)
	}

	// Corrected details complete the payment.
	resp, err := f.payments.SimulatePaymentWithMethod(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, debitInput(validDirectDebit()))
	if err != nil || resp.Status != model.PaymentStatusPaid {
		t.Fatalf("expected the corrected details to pay, got %+v, %v", resp, err)
	}
}

// A checkout that can no longer be paid fails without asking the provider, so no method is
// recorded.
func TestSimulatePaymentWithMethod_UnpayableCheckoutRecordsNoMethod(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	bookingID := f.reserve(t, 1)
	pending := f.checkout(t)
	expireReservation(t, f.db, bookingID)

	resp, err := f.payments.SimulatePaymentWithMethod(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount, cardInput(validCard()))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || *resp.FailureReason != model.PaymentFailureReservationExpired {
		t.Fatalf("expected the expired reservation to fail the payment, got %+v", resp)
	}
	if resp.PaymentMethod != nil || resp.TransactionID != nil {
		t.Errorf("the provider was not asked, so no method may be recorded, got %+v", resp)
	}
}

// Without a payment method the outcome alone decides, as before the payment methods.
func TestSimulatePayment_WithoutMethodRecordsNoMethod(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	f.reserve(t, 1)
	resp := f.pay(t, model.SimulatedPaymentSuccess)
	if resp.Status != model.PaymentStatusPaid || resp.PaymentMethod != nil || resp.PaymentAccount != nil || resp.TransactionID != nil {
		t.Errorf("expected a paid payment without method, got %+v", resp)
	}
}

// The database enforces what the service guarantees.
func TestPaymentMethodConstraints(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"unknown method", "UPDATE payments SET status = 'paid', payment_method = 'cash' WHERE payment_id = ?"},
		{"method on a pending payment", "UPDATE payments SET payment_method = 'paypal' WHERE payment_id = ?"},
		{"card brand on paypal", "UPDATE payments SET status = 'paid', payment_method = 'paypal', card_brand = 'visa' WHERE payment_id = ?"},
		{"account without method", "UPDATE payments SET status = 'paid', payment_account = '**** 4242' WHERE payment_id = ?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.db.Exec(tc.sql, pending.PaymentID).Error; err == nil {
				t.Errorf("expected a constraint violation")
			}
		})
	}
}

func ptr(s string) *string { return &s }
