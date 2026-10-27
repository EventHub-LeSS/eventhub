package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const bookingKeycloakUserID = "kc-booking-user"

func seedBookingUser(t *testing.T, db *gorm.DB, keycloakUserID string) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	err := db.Exec("INSERT INTO users (user_id, keycloak_user_id, first_name, last_name, email) VALUES (?, ?, ?, ?, ?)",
		userID, keycloakUserID, "Max", "Muster", userID.String()+"@test.de").Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

func newBookingRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	bookingService := service.NewBookingService(repository.NewBookingRepository(db), 0)

	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}

	r := gin.New()
	r.POST("/api/v1/bookings", setPrincipal, CreateBookingHandler(bookingService, repository.NewUserRepository(db)))
	r.POST("/api/v1/bookings/:bookingId/cancel", setPrincipal, CancelBooking(bookingService, repository.NewUserRepository(db)))
	return r
}

func TestCancelBooking(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	userID := seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, "other-user")
	booking := &model.BookingModel{
		BookingID: uuid.New(), UserID: &userID, EventID: &event.eventID,
		NumberOfTickets: 2, Status: model.BookingStatusConfirmed,
	}
	if err := db.Create(booking).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		principal *middleware.Principal
		id        string
		want      int
	}{
		{"unauthenticated", nil, booking.BookingID.String(), http.StatusUnauthorized},
		{"invalid id", &middleware.Principal{Subject: bookingKeycloakUserID}, "invalid", http.StatusBadRequest},
		{"unsynced user", &middleware.Principal{Subject: "unknown"}, booking.BookingID.String(), http.StatusForbidden},
		{"other owner", &middleware.Principal{Subject: "other-user"}, booking.BookingID.String(), http.StatusNotFound},
		{"missing booking", &middleware.Principal{Subject: bookingKeycloakUserID}, uuid.NewString(), http.StatusNotFound},
		{"cancel", &middleware.Principal{Subject: bookingKeycloakUserID}, booking.BookingID.String(), http.StatusOK},
		{"repeat", &middleware.Principal{Subject: bookingKeycloakUserID}, booking.BookingID.String(), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newBookingRouter(db, tc.principal).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+tc.id+"/cancel", nil))
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
			stored, err := repository.NewBookingRepository(db).GetBookingByID(booking.BookingID)
			if err != nil {
				t.Fatal(err)
			}
			want := model.BookingStatusConfirmed
			if tc.want == http.StatusOK {
				want = model.BookingStatusCancelled
			}
			if stored.Status != want {
				t.Fatalf("status = %s, want %s", stored.Status, want)
			}
		})
	}
}

func TestCancelBooking_Refunds(t *testing.T) {
	for _, paid := range []bool{true, false} {
		name := "pending payment is not refunded"
		if paid {
			name = "paid payment is fully refunded"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
			owner := seedBookingUser(t, db, bookingKeycloakUserID)
			seedBookingUser(t, db, "other-user")
			if err := db.Exec("UPDATE events SET price = ? WHERE event_id = ?", "12.49", event.eventID).Error; err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			bookings := service.NewBookingService(repository.NewBookingRepository(db), 0)
			payments := service.NewPaymentService(repository.NewTransactor(db), repository.NewBookingRepository(db), repository.NewPaymentRepository(db))
			booking, err := bookings.ReserveTickets(ctx, event.eventID, owner, 2)
			if err != nil {
				t.Fatal(err)
			}
			started, _, err := payments.StartPayment(ctx, booking.BookingID, owner)
			if err != nil {
				t.Fatal(err)
			}
			amount := decimal.RequireFromString("24.98")
			if !started.Amount.Equal(amount) {
				t.Fatalf("payment amount = %s, want %s", started.Amount, amount)
			}
			if paid {
				if _, err := payments.SimulatePayment(ctx, booking.BookingID, owner, model.SimulatedPaymentSuccess, amount); err != nil {
					t.Fatal(err)
				}
			}
			// Refund the original charge even if the event price changes afterwards.
			if err := db.Exec("UPDATE events SET price = ? WHERE event_id = ?", "99.99", event.eventID).Error; err != nil {
				t.Fatal(err)
			}
			path := "/api/v1/bookings/" + booking.BookingID.String() + "/cancel"
			otherRouter := newBookingRouter(db, &middleware.Principal{Subject: "other-user"})
			rec := doPaymentRequest(t, otherRouter, http.MethodPost, path, nil)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("other owner: expected 404, got %d: %s", rec.Code, rec.Body.String())
			}
			paymentRepo := repository.NewPaymentRepository(db)
			before, err := paymentRepo.GetPayment(started.PaymentID)
			if err != nil || before == nil {
				t.Fatalf("read payment: %v", err)
			}
			wantBefore := model.PaymentStatusPending
			if paid {
				wantBefore = model.PaymentStatusPaid
			}
			if before.Status != wantBefore || !before.RefundAmount.IsZero() {
				t.Fatalf("unauthorized cancellation changed payment: %+v", before)
			}
			router := newBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
			var firstPayment *model.PaymentModel
			for attempt := 0; attempt < 2; attempt++ {
				rec := doPaymentRequest(t, router, http.MethodPost, path, nil)
				if rec.Code != http.StatusOK {
					t.Fatalf("attempt %d: expected 200, got %d: %s", attempt, rec.Code, rec.Body.String())
				}
				var cancelled model.BookingModel
				if err := json.Unmarshal(rec.Body.Bytes(), &cancelled); err != nil {
					t.Fatal(err)
				}
				if cancelled.BookingID != booking.BookingID || cancelled.Status != model.BookingStatusCancelled {
					t.Fatalf("unexpected cancellation response: %+v", cancelled)
				}
				stored, err := paymentRepo.GetPayment(started.PaymentID)
				if err != nil || stored == nil {
					t.Fatalf("read payment: %v", err)
				}
				if paid {
					if stored.Status != model.PaymentStatusRefunded || !stored.RefundAmount.Equal(amount) || stored.FailureReason != nil {
						t.Fatalf("expected full refund of %s, got %+v", amount, stored)
					}
				} else if stored.Status != model.PaymentStatusFailed || !stored.RefundAmount.IsZero() || stored.FailureReason == nil || *stored.FailureReason != model.PaymentFailureBookingCancelled {
					t.Fatalf("pending payment should fail without a refund: %+v", stored)
				}
				if !stored.Amount.Equal(amount) {
					t.Fatalf("original payment amount changed: %s", stored.Amount)
				}
				if firstPayment != nil && !stored.UpdatedAt.Equal(firstPayment.UpdatedAt) {
					t.Fatal("repeated cancellation updated the payment again")
				}
				firstPayment = stored
			}
			paymentRouter := newPaymentRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
			rec = doPaymentRequest(t, paymentRouter, http.MethodGet, paymentPath(booking.BookingID), nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("get payment: %d: %s", rec.Code, rec.Body.String())
			}
			result := decodePayment(t, rec)
			if result.Status != firstPayment.Status || result.BookingStatus != model.BookingStatusCancelled || result.TicketsValid {
				t.Fatalf("unexpected payment response after cancellation: %+v", result)
			}
		})
	}
}

func postBooking(t *testing.T, router http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func bookingCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.BookingModel{}).Count(&count).Error; err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	return count
}

func TestCreateBooking_ReservesTicketsForTokenUser(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	userID := seedBookingUser(t, db, bookingKeycloakUserID)
	router := newBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	// Client supplied userId and status must be ignored.
	rec := postBooking(t, router, map[string]any{
		"eventId":         seeded.eventID,
		"numberOfTickets": 2,
		"userId":          uuid.New(),
		"status":          "confirmed",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created model.BookingModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.UserID == nil || *created.UserID != userID {
		t.Errorf("expected booking for user %s, got %v", userID, created.UserID)
	}
	if created.Status != model.BookingStatusReserved || created.ExpiresAt == nil {
		t.Errorf("expected reserved booking with expiry, got status=%s expiresAt=%v", created.Status, created.ExpiresAt)
	}
	if created.NumberOfTickets != 2 {
		t.Errorf("expected 2 tickets, got %d", created.NumberOfTickets)
	}
}

func TestCreateBooking_Rejections(t *testing.T) {
	db := testdb.Open(t)
	published := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	draft := seedEventForOrg(t, db, "org-a", model.EventStatusDraft)
	seedBookingUser(t, db, bookingKeycloakUserID)
	knownUser := &middleware.Principal{Subject: bookingKeycloakUserID}

	cases := []struct {
		name      string
		principal *middleware.Principal
		body      map[string]any
		want      int
	}{
		{"missing principal", nil, map[string]any{"eventId": published.eventID, "numberOfTickets": 1}, http.StatusUnauthorized},
		{"user not synced", &middleware.Principal{Subject: "kc-unknown"}, map[string]any{"eventId": published.eventID, "numberOfTickets": 1}, http.StatusForbidden},
		{"missing event id", knownUser, map[string]any{"numberOfTickets": 1}, http.StatusBadRequest},
		{"zero tickets", knownUser, map[string]any{"eventId": published.eventID, "numberOfTickets": 0}, http.StatusBadRequest},
		{"negative tickets", knownUser, map[string]any{"eventId": published.eventID, "numberOfTickets": -3}, http.StatusBadRequest},
		{"unknown event", knownUser, map[string]any{"eventId": uuid.New(), "numberOfTickets": 1}, http.StatusNotFound},
		{"draft event", knownUser, map[string]any{"eventId": draft.eventID, "numberOfTickets": 1}, http.StatusConflict},
		{"capacity exceeded", knownUser, map[string]any{"eventId": published.eventID, "numberOfTickets": 101}, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postBooking(t, newBookingRouter(db, tc.principal), tc.body)
			if rec.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
			var problem model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != tc.want || problem.Title == "" {
				t.Errorf("expected RFC 9457 problem body with status %d, got %s", tc.want, rec.Body.String())
			}
		})
	}
	if n := bookingCount(t, db); n != 0 {
		t.Errorf("rejected requests must not create bookings, found %d", n)
	}
}
