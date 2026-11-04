package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The payment methods of the simulated payment provider over HTTP. The events cost 20 per ticket.

func TestPayment_PaysByEveryMethod(t *testing.T) {
	cases := []struct {
		name    string
		body    map[string]any
		method  model.PaymentMethod
		account string
		brand   model.CardBrand
	}{
		{
			name: "credit card",
			body: map[string]any{"paymentMethod": "credit_card", "creditCard": map[string]any{
				"cardholder": "Max Mustermann", "number": "5555 5555 5555 4444", "expiry": "12/30", "cvc": "123",
			}},
			method: model.PaymentMethodCreditCard, account: "**** 4444", brand: model.CardBrandMastercard,
		},
		{
			name: "direct debit",
			body: map[string]any{"paymentMethod": "direct_debit", "directDebit": map[string]any{
				"accountHolder": "Max Mustermann", "iban": "DE89 3704 0044 0532 0130 00", "mandateAccepted": true,
			}},
			method: model.PaymentMethodDirectDebit, account: "DE89 **** 3000",
		},
		{
			name:   "paypal",
			body:   map[string]any{"paymentMethod": "paypal", "paypal": map[string]any{"email": "max@example.com"}},
			method: model.PaymentMethodPayPal, account: "m***@example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.Open(t)
			event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
			seedBookingUser(t, db, bookingKeycloakUserID)
			router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
			addToCartViaAPI(t, router, event.eventID, 2)
			pending := checkoutViaAPI(t, router)

			body := map[string]any{"outcome": "success", "amount": "40"}
			for k, v := range tc.body {
				body[k] = v
			}
			rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			paid := decodePayment(t, rec)
			if paid.Status != model.PaymentStatusPaid || !paid.TicketsValid {
				t.Fatalf("expected a paid payment with valid tickets, got %+v", paid)
			}
			if paid.PaymentMethod == nil || *paid.PaymentMethod != tc.method || paid.PaymentAccount == nil || *paid.PaymentAccount != tc.account || paid.TransactionID == nil {
				t.Errorf("expected %s from %q, got %+v", tc.method, tc.account, paid)
			}
			if (tc.brand == "") != (paid.CardBrand == nil) || (tc.brand != "" && *paid.CardBrand != tc.brand) {
				t.Errorf("expected card brand %q, got %v", tc.brand, paid.CardBrand)
			}
			// Card number, CVC and IBAN never leave the backend again.
			for _, secret := range []string{"5555 5555 5555 4444", "5555555555554444", "DE89370400440532013000", "\"cvc\""} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Errorf("the response must not contain %q: %s", secret, rec.Body.String())
				}
			}

			// The stored result shows the same method.
			rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(pending.PaymentID), nil)
			if got := decodePayment(t, rec); got.PaymentMethod == nil || *got.PaymentMethod != tc.method || *got.TransactionID != *paid.TransactionID {
				t.Errorf("expected the stored %s payment, got %+v", tc.method, got)
			}
		})
	}
}

func TestPayment_DeclinedTestCardFailsThePayment(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	addToCartViaAPI(t, router, event.eventID, 1)
	pending := checkoutViaAPI(t, router)

	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", map[string]any{
		"outcome": "success", "amount": "20", "paymentMethod": "credit_card",
		"creditCard": map[string]any{"cardholder": "Max Mustermann", "number": "4000000000000002", "expiry": "12/30", "cvc": "123"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	failed := decodePayment(t, rec)
	if failed.Status != model.PaymentStatusFailed || failed.FailureReason == nil || *failed.FailureReason != model.PaymentFailureDeclined || failed.TicketsValid {
		t.Errorf("expected a declined payment, got %+v", failed)
	}
	if failed.PaymentMethod == nil || *failed.PaymentMethod != model.PaymentMethodCreditCard || !strings.Contains(failed.Message, "credit card") {
		t.Errorf("expected the declined card with its message, got %+v", failed)
	}
}

func TestPayment_PaymentMethodRejections(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	addToCartViaAPI(t, router, event.eventID, 1)
	pending := checkoutViaAPI(t, router)
	path := paymentPath(pending.PaymentID) + "/simulate"

	card := map[string]any{"cardholder": "Max Mustermann", "number": "4242424242424242", "expiry": "12/30", "cvc": "123"}
	simulateBody := func(extra map[string]any) map[string]any {
		body := map[string]any{"outcome": "success", "amount": "20"}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	cases := []struct {
		name string
		body any
	}{
		{"unknown payment method", simulateBody(map[string]any{"paymentMethod": "cash"})},
		{"details without payment method", simulateBody(map[string]any{"creditCard": card})},
		{"details of another method", simulateBody(map[string]any{"paymentMethod": "paypal", "creditCard": card})},
		{"card payment without card", simulateBody(map[string]any{"paymentMethod": "credit_card"})},
		{"invalid card number", simulateBody(map[string]any{"paymentMethod": "credit_card", "creditCard": map[string]any{
			"cardholder": "Max Mustermann", "number": "4242424242424241", "expiry": "12/30", "cvc": "123",
		}})},
		{"expired card", simulateBody(map[string]any{"paymentMethod": "credit_card", "creditCard": map[string]any{
			"cardholder": "Max Mustermann", "number": "4242424242424242", "expiry": "01/20", "cvc": "123",
		}})},
		{"invalid iban", simulateBody(map[string]any{"paymentMethod": "direct_debit", "directDebit": map[string]any{
			"accountHolder": "Max Mustermann", "iban": "DE00370400440532013000", "mandateAccepted": true,
		}})},
		{"direct debit without mandate", simulateBody(map[string]any{"paymentMethod": "direct_debit", "directDebit": map[string]any{
			"accountHolder": "Max Mustermann", "iban": "DE89370400440532013000",
		}})},
		{"invalid paypal email", simulateBody(map[string]any{"paymentMethod": "paypal", "paypal": map[string]any{"email": "max"}})},
		{"card number of the wrong type", `{"outcome":"success","amount":"20","paymentMethod":"credit_card","creditCard":{"number":4242424242424242}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPaymentProblem(t, doPaymentRequest(t, router, http.MethodPost, path, tc.body), http.StatusBadRequest)
		})
	}

	// None of the rejections touched the payment.
	rec := doPaymentRequest(t, router, http.MethodGet, paymentPath(pending.PaymentID), nil)
	got := decodePayment(t, rec)
	if got.Status != model.PaymentStatusPending || got.PaymentMethod != nil {
		t.Fatalf("rejected payment details must not change the payment, got %+v", got)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["paymentMethod"]; ok {
		t.Errorf("a pending payment omits paymentMethod, got %s", rec.Body.String())
	}

	// PayPal Express needs no account data, and a simulated decline needs no card data.
	rec = doPaymentRequest(t, router, http.MethodPost, path, simulateBody(map[string]any{"paymentMethod": "paypal"}))
	if paid := decodePayment(t, rec); rec.Code != http.StatusOK || paid.Status != model.PaymentStatusPaid || paid.PaymentAccount != nil {
		t.Fatalf("expected PayPal Express to pay, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPayment_DeclineWithMethodNeedsNoDetails(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	addToCartViaAPI(t, router, event.eventID, 1)
	pending := checkoutViaAPI(t, router)

	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", map[string]any{
		"outcome": "failure", "amount": "20", "paymentMethod": "direct_debit",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	failed := decodePayment(t, rec)
	if failed.Status != model.PaymentStatusFailed || failed.PaymentMethod == nil || *failed.PaymentMethod != model.PaymentMethodDirectDebit || !strings.Contains(failed.Message, "direct debit") {
		t.Errorf("expected a declined direct debit, got %+v", failed)
	}
}
