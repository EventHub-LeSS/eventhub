package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const otherBookingKeycloakUserID = "kc-other-booking-user"

func newPaymentRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	userRepo := repository.NewUserRepository(db)
	bookingService := service.NewBookingService(repository.NewBookingRepository(db), 0)
	payments := NewPaymentHandler(service.NewPaymentService(repository.NewPaymentRepository(db)), userRepo)

	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}

	r := gin.New()
	r.POST("/api/v1/bookings", setPrincipal, CreateBookingHandler(bookingService, userRepo))
	r.POST("/api/v1/bookings/:bookingId/payment", setPrincipal, payments.StartPaymentHandler)
	r.GET("/api/v1/bookings/:bookingId/payment", setPrincipal, payments.GetPaymentHandler)
	r.POST("/api/v1/bookings/:bookingId/payment/simulate", setPrincipal, payments.SimulatePaymentHandler)
	return r
}

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

// paymentPath takes the booking ID as fmt.Stringer or string, so tests can pass malformed IDs.
func paymentPath(bookingID any) string {
	return fmt.Sprintf("/api/v1/bookings/%s/payment", bookingID)
}

func decodePayment(t *testing.T, rec *httptest.ResponseRecorder) model.PaymentResponse {
	t.Helper()
	var resp model.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode payment: %v: %s", err, rec.Body.String())
	}
	return resp
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

// reserveViaAPI books tickets of the seeded event, which costs 20 per ticket.
func reserveViaAPI(t *testing.T, router http.Handler, eventID uuid.UUID, tickets int) uuid.UUID {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/bookings", map[string]any{"eventId": eventID, "numberOfTickets": tickets})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create booking: %d %s", rec.Code, rec.Body.String())
	}
	var booking model.BookingModel
	if err := json.Unmarshal(rec.Body.Bytes(), &booking); err != nil {
		t.Fatalf("decode booking: %v", err)
	}
	return booking.BookingID
}

func TestPayment_SuccessfulFlow(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newPaymentRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	bookingID := reserveViaAPI(t, router, event.eventID, 2)

	// A client supplied amount is ignored: the server computes 2 x 20.
	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID), map[string]any{"amount": "0.01", "status": "paid"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("start payment: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	started := decodePayment(t, rec)
	if !started.Amount.Equal(decimal.NewFromInt(40)) || started.Status != model.PaymentStatusPending || started.TicketsValid {
		t.Fatalf("expected pending payment of 40 without valid tickets, got %+v", started)
	}
	if started.ExpiresAt == nil {
		t.Error("a pending payment must show until when it can be completed")
	}

	// The amount is shown before completion and stays the same when the client asks again.
	rec = doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID), nil)
	if rec.Code != http.StatusOK || decodePayment(t, rec).PaymentID != started.PaymentID {
		t.Fatalf("start again: expected 200 with the same payment, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(bookingID), nil)
	if rec.Code != http.StatusOK || !decodePayment(t, rec).Amount.Equal(started.Amount) {
		t.Fatalf("get pending payment: %d %s", rec.Code, rec.Body.String())
	}

	rec = doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID)+"/simulate", map[string]any{"outcome": "success", "amount": "40"})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	paid := decodePayment(t, rec)
	if paid.Status != model.PaymentStatusPaid || paid.BookingStatus != model.BookingStatusConfirmed || !paid.TicketsValid {
		t.Errorf("expected paid payment with confirmed booking and valid tickets, got %+v", paid)
	}
	if paid.Message == "" || paid.FailureReason != nil || paid.ExpiresAt != nil {
		t.Errorf("expected a success message without failure reason or deadline, got %+v", paid)
	}

	// The result is the stored state.
	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(bookingID), nil)
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
	router := newPaymentRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	bookingID := reserveViaAPI(t, router, event.eventID, 1)
	doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID), nil)

	// Amounts may be sent as JSON numbers as well.
	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID)+"/simulate", `{"outcome":"failure","amount":20.00}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	failed := decodePayment(t, rec)
	if failed.Status != model.PaymentStatusFailed || failed.BookingStatus != model.BookingStatusFailed || failed.TicketsValid {
		t.Errorf("expected failed payment and booking without valid tickets, got %+v", failed)
	}
	if failed.FailureReason == nil || *failed.FailureReason != model.PaymentFailureDeclined || failed.Message == "" {
		t.Errorf("expected declined reason with message, got %+v", failed)
	}

	rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(bookingID), nil)
	if got := decodePayment(t, rec); got.Status != model.PaymentStatusFailed || got.TicketsValid {
		t.Errorf("expected stored failed payment, got %+v", got)
	}
	// Retrying a failed payment is no way to a ticket.
	rec = doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID)+"/simulate", map[string]any{"outcome": "success", "amount": "20"})
	wantPaymentProblem(t, rec, http.StatusConflict)
	wantPaymentProblem(t, doPaymentRequest(t, router, http.MethodPost, paymentPath(bookingID), nil), http.StatusConflict)
}

func TestPayment_Rejections(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	owner := &middleware.Principal{Subject: bookingKeycloakUserID}
	other := &middleware.Principal{Subject: otherBookingKeycloakUserID}
	ownerRouter := newPaymentRouter(db, owner)

	started := reserveViaAPI(t, ownerRouter, event.eventID, 1)
	if rec := doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started), nil); rec.Code != http.StatusCreated {
		t.Fatalf("start payment: %d %s", rec.Code, rec.Body.String())
	}
	notStarted := reserveViaAPI(t, ownerRouter, event.eventID, 1)
	expired := reserveViaAPI(t, ownerRouter, event.eventID, 1)
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", expired).Error; err != nil {
		t.Fatalf("expire reservation: %v", err)
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
		{"start without principal", nil, http.MethodPost, paymentPath(started), nil, http.StatusUnauthorized},
		{"simulate without principal", nil, http.MethodPost, paymentPath(started) + "/simulate", valid, http.StatusUnauthorized},
		{"get without principal", nil, http.MethodGet, paymentPath(started), nil, http.StatusUnauthorized},
		{"user not synced", &middleware.Principal{Subject: "kc-unknown"}, http.MethodPost, paymentPath(started), nil, http.StatusForbidden},
		{"invalid booking id", owner, http.MethodPost, paymentPath("not-a-uuid"), nil, http.StatusBadRequest},
		{"invalid booking id on simulate", owner, http.MethodPost, paymentPath("not-a-uuid") + "/simulate", valid, http.StatusBadRequest},
		{"unknown booking", owner, http.MethodPost, paymentPath(uuid.New()), nil, http.StatusNotFound},
		{"start booking of another user", other, http.MethodPost, paymentPath(notStarted), nil, http.StatusNotFound},
		{"simulate booking of another user", other, http.MethodPost, paymentPath(started) + "/simulate", valid, http.StatusNotFound},
		{"get booking of another user", other, http.MethodGet, paymentPath(started), nil, http.StatusNotFound},
		{"get payment not started", owner, http.MethodGet, paymentPath(notStarted), nil, http.StatusNotFound},
		{"simulate payment not started", owner, http.MethodPost, paymentPath(notStarted) + "/simulate", valid, http.StatusNotFound},
		{"start expired reservation", owner, http.MethodPost, paymentPath(expired), nil, http.StatusConflict},
		{"missing body", owner, http.MethodPost, paymentPath(started) + "/simulate", nil, http.StatusBadRequest},
		{"missing amount", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "success"}, http.StatusBadRequest},
		{"null amount", owner, http.MethodPost, paymentPath(started) + "/simulate", `{"outcome":"success","amount":null}`, http.StatusBadRequest},
		{"negative amount", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "success", "amount": "-20"}, http.StatusBadRequest},
		{"amount not a number", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "success", "amount": "twenty"}, http.StatusBadRequest},
		{"missing outcome", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"amount": "20"}, http.StatusBadRequest},
		{"unknown outcome", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "paid", "amount": "20"}, http.StatusBadRequest},
		{"manipulated amount", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "success", "amount": "0.01"}, http.StatusUnprocessableEntity},
		{"amount with extra cent", owner, http.MethodPost, paymentPath(started) + "/simulate", map[string]any{"outcome": "success", "amount": "20.01"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPaymentProblem(t, doPaymentRequest(t, newPaymentRouter(db, tc.principal), tc.method, tc.path, tc.body), tc.want)
		})
	}

	// None of the rejected requests changed the started payment or created another one.
	rec := doPaymentRequest(t, ownerRouter, http.MethodGet, paymentPath(started), nil)
	if got := decodePayment(t, rec); got.Status != model.PaymentStatusPending || got.BookingStatus != model.BookingStatusReserved {
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
	if rec := doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started)+"/simulate", valid); rec.Code != http.StatusOK {
		t.Fatalf("simulate: %d %s", rec.Code, rec.Body.String())
	}
	wantPaymentProblem(t, doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started)+"/simulate", valid), http.StatusConflict)
	wantPaymentProblem(t, doPaymentRequest(t, ownerRouter, http.MethodPost, paymentPath(started), nil), http.StatusConflict)
}
