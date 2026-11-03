package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/testdb"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	otherBookingKeycloakUserID  = "kc-other-booking-user"
	expiredBookingKeycloakUserID = "kc-expired-booking-user"
)

func doPaymentRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	switch b := body.(type) {
	case nil:
	case string:
		payload = []byte(b)
	default:
		var err error
		if payload, err = json.Marshal(b); err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// paymentPath takes the payment ID as fmt.Stringer or string, so tests can pass malformed IDs.
func paymentPath(paymentID any) string {
	return fmt.Sprintf("/api/v1/payments/%s", paymentID)
}

func decodePayment(t *testing.T, rec *httptest.ResponseRecorder) model.PaymentResponse {
	t.Helper()
	var resp model.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode payment: %v: %s", err, rec.Body.String())
	}
	return resp
}

// paymentItemOf finds the item of the booking in the payment response, or nil.
func paymentItemOf(resp *model.PaymentResponse, bookingID uuid.UUID) *model.PaymentItemResponse {
	for i := range resp.Items {
		if resp.Items[i].BookingID == bookingID {
			return &resp.Items[i]
		}
	}
	return nil
}

func wantPaymentProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("expected %d, got %d: %s", status, rec.Code, rec.Body.String())
		return
	}
	var problem model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != status || problem.Title == "" || problem.Detail == "" {
		t.Errorf("expected RFC 9457 problem body with status %d, got %s", status, rec.Body.String())
	}
}

// The payment process of a checkout: the cart is checked out, the pending payment is shown and
// completed, and the stored result is served afterwards. The events cost 20 per ticket.
func TestPayment_SuccessfulFlow(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	bookingID := addToCartViaAPI(t, router, event.eventID, 2)

	// A client supplied amount is ignored: the server computes 2 x 20.
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", map[string]any{"amount": "0.01"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	started := decodePayment(t, rec)
	if !started.Amount.Equal(decimal.NewFromInt(40)) || started.Status != model.PaymentStatusPending || started.TicketsValid {
		t.Fatalf("expected pending payment of 40 without valid tickets, got %+v", started)
	}
	if started.ExpiresAt == nil {
		t.Error("a pending payment must show until when it can be completed")
	}
	if item := paymentItemOf(&started, bookingID); item == nil || item.BookingStatus != model.BookingStatusReserved || !item.LineAmount.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("expected the reserved item with its line amount, got %+v", item)
	}

	// The amount is shown before completion and stays the same when the client asks again.
	rec = doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", nil)
	if rec.Code != http.StatusOK || decodePayment(t, rec).PaymentID != started.PaymentID {
		t.Fatalf("checkout again: expected 200 with the same payment, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(started.PaymentID), nil)
	if rec.Code != http.StatusOK || !decodePayment(t, rec).Amount.Equal(started.Amount) {
		t.Fatalf("get pending payment: %d %s", rec.Code, rec.Body.String())
	}

	rec = doPaymentRequest(t, router, http.MethodPost, paymentPath(started.PaymentID)+"/simulate", map[string]any{"outcome": "success", "amount": "40"})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	paid := decodePayment(t, rec)
	if paid.Status != model.PaymentStatusPaid || !paid.TicketsValid {
		t.Errorf("expected paid payment with valid tickets, got %+v", paid)
	}
	if item := paymentItemOf(&paid, bookingID); item == nil || item.BookingStatus != model.BookingStatusConfirmed || !item.TicketsValid {
		t.Errorf("expected the confirmed item, got %+v", item)
	}
	if paid.Message == "" || paid.FailureReason != nil || paid.ExpiresAt != nil {
		t.Errorf("expected a success message without failure reason or deadline, got %+v", paid)
	}

	// The result is the stored state.
	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(started.PaymentID), nil)
	if got := decodePayment(t, rec); rec.Code != http.StatusOK || got.Status != model.PaymentStatusPaid || !got.TicketsValid {
		t.Errorf("expected stored paid payment, got %d %+v", rec.Code, got)
	}
	sold, err := repository.NewEventRepository(db).GetConfirmedTicketCount(event.eventID)
	if err != nil {
		t.Fatalf("sold tickets: %v", err)
	}
	if sold != 2 {
		t.Errorf("expected 2 sold tickets, got %d", sold)
	}
}

func TestPayment_FailedFlowShowsReasonAndNoTicket(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	addToCartViaAPI(t, router, event.eventID, 1)
	pending := checkoutViaAPI(t, router)

	// Amounts may be sent as JSON numbers as well.
	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", `{"outcome":"failure","amount":20.00}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	failed := decodePayment(t, rec)
	if failed.Status != model.PaymentStatusFailed || failed.TicketsValid {
		t.Errorf("expected failed payment without valid tickets, got %+v", failed)
	}
	if item := paymentItemOf(&failed, pending.Items[0].BookingID); item == nil || item.BookingStatus != model.BookingStatusFailed || item.TicketsValid {
		t.Errorf("expected the failed item, got %+v", item)
	}
	if failed.FailureReason == nil || *failed.FailureReason != model.PaymentFailureDeclined || failed.Message == "" {
		t.Errorf("expected declined reason with message, got %+v", failed)
	}

	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(pending.PaymentID), nil)
	if got := decodePayment(t, rec); got.Status != model.PaymentStatusFailed || got.TicketsValid {
		t.Errorf("expected stored failed payment, got %+v", got)
	}
	// Retrying a failed payment is no way to a ticket, and the failed booking is no cart item.
	wantPaymentProblem(t, doPaymentRequest(t, router, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", map[string]any{"outcome": "success", "amount": "20"}), http.StatusConflict)
	wantPaymentProblem(t, doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", nil), http.StatusUnprocessableEntity)
	// The tickets are free again: a new reservation succeeds.
	addToCartViaAPI(t, router, event.eventID, 1)
}

func TestPayment_Rejections(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	seedBookingUser(t, db, expiredBookingKeycloakUserID)
	owner := &middleware.Principal{Subject: bookingKeycloakUserID}
	other := &middleware.Principal{Subject: otherBookingKeycloakUserID}
	expired := &middleware.Principal{Subject: expiredBookingKeycloakUserID}
	ownerRouter := newCartRouter(db, owner)

	// The owner has an open checkout; the expired user holds only an expired reservation.
	addToCartViaAPI(t, ownerRouter, event.eventID, 1)
	started := checkoutViaAPI(t, ownerRouter)
	expiredItem := addToCartViaAPI(t, newCartRouter(db, expired), event.eventID, 1)
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", expiredItem).Error; err != nil {
		t.Fatal(err)
	}

	valid := map[string]any{"outcome": "success", "amount": "20"}
	cases := []struct {
		name      string
		principal *middleware.Principal
		method    string
		path      string
		body      any
		want      int
	}{
		{"get without principal", nil, http.MethodGet, paymentPath(started.PaymentID), nil, http.StatusUnauthorized},
		{"simulate without principal", nil, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", valid, http.StatusUnauthorized},
		{"checkout without principal", nil, http.MethodPost, "/api/v1/cart/checkout", nil, http.StatusUnauthorized},
		{"user not synced", &middleware.Principal{Subject: "kc-unknown"}, http.MethodGet, paymentPath(started.PaymentID), nil, http.StatusForbidden},
		{"invalid payment id", owner, http.MethodGet, paymentPath("not-a-uuid"), nil, http.StatusBadRequest},
		{"invalid payment id on simulate", owner, http.MethodPost, paymentPath("not-a-uuid") + "/simulate", valid, http.StatusBadRequest},
		{"unknown payment", owner, http.MethodGet, paymentPath(uuid.New()), nil, http.StatusNotFound},
		{"simulate unknown payment", owner, http.MethodPost, paymentPath(uuid.New()) + "/simulate", valid, http.StatusNotFound},
		{"get payment of another user", other, http.MethodGet, paymentPath(started.PaymentID), nil, http.StatusNotFound},
		{"simulate payment of another user", other, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", valid, http.StatusNotFound},
		{"checkout with an empty cart", other, http.MethodPost, "/api/v1/cart/checkout", nil, http.StatusUnprocessableEntity},
		{"checkout with only expired items", expired, http.MethodPost, "/api/v1/cart/checkout", nil, http.StatusConflict},
		{"missing body", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", nil, http.StatusBadRequest},
		{"missing amount", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "success"}, http.StatusBadRequest},
		{"null amount", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", `{"outcome":"success","amount":null}`, http.StatusBadRequest},
		{"negative amount", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "success", "amount": "-20"}, http.StatusBadRequest},
		{"body larger than 4 KiB", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", `{"outcome":"success","amount":"` + strings.Repeat("9", 5000) + `"}`, http.StatusRequestEntityTooLarge},
		{"amount with huge exponent", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", `{"outcome":"success","amount":"1e2000000000"}`, http.StatusBadRequest},
		{"amount not a number", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "success", "amount": "twenty"}, http.StatusBadRequest},
		{"missing outcome", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"amount": "20"}, http.StatusBadRequest},
		{"unknown outcome", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "paid", "amount": "20"}, http.StatusBadRequest},
		{"manipulated amount", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "success", "amount": "0.01"}, http.StatusUnprocessableEntity},
		{"amount with extra cent", owner, http.MethodPost, paymentPath(started.PaymentID) + "/simulate", map[string]any{"outcome": "success", "amount": "20.01"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPaymentProblem(t, doPaymentRequest(t, newCartRouter(db, tc.principal), tc.method, tc.path, tc.body), tc.want)
		})
	}

	// None of the rejected requests changed the started payment or created another one.
	rec := doPaymentRequest(t, ownerRouter, http.MethodGet, paymentPath(started.PaymentID), nil)
	if got := decodePayment(t, rec); got.Status != model.PaymentStatusPending || got.Items[0].BookingStatus != model.BookingStatusReserved {
		t.Errorf("rejected requests must not complete the payment, got %+v", got)
	}
	var payments int64
	if err := db.Model(&model.PaymentModel{}).Count(&payments).Error; err != nil {
		t.Fatalf("count payments: %v", err)
	}
	if payments != 1 {
		t.Errorf("rejected requests must not create payments, found %d", payments)
	}

	// After the rejections the correct amount still completes the payment, exactly once.
	if rec := doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started.PaymentID)+"/simulate", valid); rec.Code != http.StatusOK {
		t.Fatalf("simulate: %d %s", rec.Code, rec.Body.String())
	}
	wantPaymentProblem(t, doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started.PaymentID)+"/simulate", valid), http.StatusConflict)
	// The paid booking is not a reservation anymore, so the cart stays empty.
	wantPaymentProblem(t, doPaymentRequest(t, ownerRouter, http.MethodPost, "/api/v1/cart/checkout", nil), http.StatusUnprocessableEntity)
}

// The organizer cannot lower the capacity below the tickets of a running checkout (review of PR #63).
func TestPayment_CapacityChangeDuringCheckout(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	visitor := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	organizer := newEventRouter(db, principalManaging("org-a"))

	addToCartViaAPI(t, visitor, event.eventID, 2)
	pending := checkoutViaAPI(t, visitor)

	wantPaymentProblem(t, putEvent(t, organizer, event.eventID.String(), with(validBody(event), "capacity", 1)), http.StatusConflict)
	if rec := putEvent(t, organizer, event.eventID.String(), with(validBody(event), "capacity", 2)); rec.Code != http.StatusOK {
		t.Fatalf("lowering to the reserved tickets: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := doPaymentRequest(t, visitor, http.MethodPost, paymentPath(pending.PaymentID)+"/simulate", map[string]any{"outcome": "success", "amount": "40"})
	if got := decodePayment(t, rec); rec.Code != http.StatusOK || got.Status != model.PaymentStatusPaid || !got.TicketsValid {
		t.Fatalf("simulate: %d %s", rec.Code, rec.Body.String())
	}
	sold, err := repository.NewEventRepository(db).GetConfirmedTicketCount(event.eventID)
	if err != nil {
		t.Fatalf("sold tickets: %v", err)
	}
	var capacity int
	if err := db.Raw("SELECT capacity FROM events WHERE event_id = ?", event.eventID).Scan(&capacity).Error; err != nil {
		t.Fatalf("capacity: %v", err)
	}
	if sold != 2 || capacity != 2 {
		t.Errorf("expected 2 sold tickets for capacity 2, got %d for %d", sold, capacity)
	}
}
