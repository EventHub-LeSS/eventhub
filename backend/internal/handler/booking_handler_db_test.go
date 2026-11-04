package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// newBookingRouter wires only the cancel endpoint; bookings are made through the cart.
func newBookingRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	bookingService := service.NewBookingService(repository.NewTransactor(db), repository.NewBookingRepository(db), repository.NewPaymentRepository(db))

	setPrincipal := principalSetter(principal)

	r := gin.New()
	r.POST("/api/v1/bookings/:bookingId/cancel", setPrincipal, CancelBooking(bookingService, repository.NewUserRepository(db)))
	return r
}

func TestCancelBooking(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	userID := seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, "other-user")
	// A live reservation without a payment is a cart item and is cancelled on its own.
	expiresAt := time.Now().Add(15 * time.Minute)
	booking := &model.BookingModel{
		BookingID: uuid.New(), UserID: &userID, EventID: &event.eventID,
		NumberOfTickets: 2, Status: model.BookingStatusReserved, ExpiresAt: &expiresAt,
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
			want := model.BookingStatusReserved
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
		name := "pending checkout is cancelled without a refund"
		if paid {
			name = "paid payment is refunded by the stored line amount"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
			seedBookingUser(t, db, bookingKeycloakUserID)
			seedBookingUser(t, db, "other-user")
			if err := db.Exec("UPDATE events SET price = ? WHERE event_id = ?", "12.49", event.eventID).Error; err != nil {
				t.Fatal(err)
			}
			router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

			bookingID := addToCartViaAPI(t, router, event.eventID, 2)
			started := checkoutViaAPI(t, router)
			amount := decimal.RequireFromString("24.98")
			if !started.Amount.Equal(amount) {
				t.Fatalf("payment amount = %s, want %s", started.Amount, amount)
			}
			if paid {
				simulateViaAPI(t, router, started.PaymentID, model.SimulatedPaymentSuccess, started.Amount)
			}
			// The refund covers the stored line even if the event price changes afterwards.
			if err := db.Exec("UPDATE events SET price = ? WHERE event_id = ?", "99.99", event.eventID).Error; err != nil {
				t.Fatal(err)
			}

			path := "/api/v1/bookings/" + bookingID.String() + "/cancel"
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
				if cancelled.BookingID != bookingID || cancelled.Status != model.BookingStatusCancelled {
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
					t.Fatalf("pending checkout should fail without a refund: %+v", stored)
				}
				if !stored.Amount.Equal(amount) {
					t.Fatalf("original payment amount changed: %s", stored.Amount)
				}
				if firstPayment != nil && !stored.UpdatedAt.Equal(firstPayment.UpdatedAt) {
					t.Fatal("repeated cancellation updated the payment again")
				}
				firstPayment = stored
			}
			rec = doPaymentRequest(t, router, http.MethodGet, paymentPath(started.PaymentID), nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("get payment: %d: %s", rec.Code, rec.Body.String())
			}
			result := decodePayment(t, rec)
			if result.Status != firstPayment.Status || result.TicketsValid {
				t.Fatalf("unexpected payment response after cancellation: %+v", result)
			}
			if item := paymentItemOf(&result, bookingID); item == nil || item.BookingStatus != model.BookingStatusCancelled || item.TicketsValid {
				t.Fatalf("expected the cancelled item without valid tickets, got %+v", item)
			}
		})
	}
}
