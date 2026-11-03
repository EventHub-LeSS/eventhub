package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB-236: status codes of the own booking endpoints without a database.

const ownBookingSubject = "kc-own-booking-user"

type stubOwnBookingRepo struct {
	bookings []*repository.OwnBooking
	err      error
}

func (r *stubOwnBookingRepo) ListByUserNewestFirst(uuid.UUID) ([]*repository.OwnBooking, error) {
	return r.bookings, r.err
}

func (r *stubOwnBookingRepo) ListByUserByEventStart(uuid.UUID) ([]*repository.OwnBooking, error) {
	return r.bookings, r.err
}

func (r *stubOwnBookingRepo) GetByUser(bookingID, _ uuid.UUID) (*repository.OwnBooking, error) {
	for _, b := range r.bookings {
		if b.Booking.BookingID == bookingID {
			return b, r.err
		}
	}
	return nil, r.err
}

// ownBookingUsers knows exactly one API user, the one of ownBookingSubject.
type ownBookingUsers struct {
	repository.UserRepository
}

func (ownBookingUsers) GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error) {
	if keycloakUserID == ownBookingSubject {
		return &model.UserModel{UserID: uuid.New(), KeycloakUserID: ownBookingSubject}, nil
	}
	return nil, nil
}

func (ownBookingUsers) GetByEmail(string) (*model.UserModel, error) {
	return nil, nil
}

func newOwnBookingUnitRouter(repo repository.OwnBookingRepository, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewOwnBookingHandler(service.NewOwnBookingService(repo), ownBookingUsers{})
	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}
	r := gin.New()
	r.GET("/api/v1/me/tickets", setPrincipal, h.ListOwnTicketsHandler)
	r.GET("/api/v1/me/bookings", setPrincipal, h.ListOwnBookingsHandler)
	r.GET("/api/v1/me/bookings/:bookingId", setPrincipal, h.GetOwnBookingHandler)
	return r
}

func TestOwnBookingHandlers_Rejections_ReturnProblem(t *testing.T) {
	known := &middleware.Principal{Subject: ownBookingSubject}
	notSynced := &middleware.Principal{Subject: "kc-not-synced"}
	cases := []struct {
		name      string
		principal *middleware.Principal
		path      string
		want      int
	}{
		{"tickets without token", nil, "/api/v1/me/tickets", http.StatusUnauthorized},
		{"history without token", nil, "/api/v1/me/bookings", http.StatusUnauthorized},
		{"detail without token", nil, "/api/v1/me/bookings/" + uuid.NewString(), http.StatusUnauthorized},
		{"tickets of user not synced", notSynced, "/api/v1/me/tickets", http.StatusForbidden},
		{"history of user not synced", notSynced, "/api/v1/me/bookings", http.StatusForbidden},
		{"detail of user not synced", notSynced, "/api/v1/me/bookings/" + uuid.NewString(), http.StatusForbidden},
		{"unknown status", known, "/api/v1/me/bookings?status=paid", http.StatusBadRequest},
		{"misspelled status", known, "/api/v1/me/bookings?status=Confirmed", http.StatusBadRequest},
		{"invalid booking id", known, "/api/v1/me/bookings/not-a-uuid", http.StatusBadRequest},
		{"unknown booking", known, "/api/v1/me/bookings/" + uuid.NewString(), http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newOwnBookingUnitRouter(&stubOwnBookingRepo{}, tc.principal).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			wantProblem(t, rec, tc.want)
		})
	}
}

func TestOwnBookingHandlers_DatabaseError_Returns500WithoutDetails(t *testing.T) {
	repo := &stubOwnBookingRepo{err: errors.New("pq: relation bookings is gone")}
	known := &middleware.Principal{Subject: ownBookingSubject}
	for _, path := range []string{"/api/v1/me/tickets", "/api/v1/me/bookings", "/api/v1/me/bookings/" + uuid.NewString()} {
		rec := httptest.NewRecorder()
		newOwnBookingUnitRouter(repo, known).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		wantProblem(t, rec, http.StatusInternalServerError)
		if strings.Contains(rec.Body.String(), "relation") {
			t.Errorf("%s: internal error details leaked: %s", path, rec.Body.String())
		}
	}
}

func TestOwnBookingHandlers_NoBookings_ReturnEmptyArray(t *testing.T) {
	known := &middleware.Principal{Subject: ownBookingSubject}
	for _, path := range []string{"/api/v1/me/tickets", "/api/v1/me/bookings", "/api/v1/me/bookings?status=confirmed"} {
		rec := httptest.NewRecorder()
		newOwnBookingUnitRouter(&stubOwnBookingRepo{}, known).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("%s: expected 200 with [], got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestGetOwnBookingHandler_OwnBooking_ReturnsDetails(t *testing.T) {
	now := time.Now()
	eventID, status := uuid.New(), model.EventStatusPublished
	own := &repository.OwnBooking{
		BookingPayment: repository.BookingPayment{
			Booking:     &model.BookingModel{BookingID: uuid.New(), EventID: &eventID, NumberOfTickets: 3, Status: model.BookingStatusConfirmed, CreatedAt: now},
			Payment:     &model.PaymentModel{PaymentID: uuid.New(), Status: model.PaymentStatusPaid},
			EventStatus: &status,
			Now:         now,
		},
		Event: &model.EventModel{EventID: eventID, Title: "Konzert", StartTime: now.Add(time.Hour), EndTime: now.Add(2 * time.Hour), Status: status},
	}
	rec := httptest.NewRecorder()
	newOwnBookingUnitRouter(&stubOwnBookingRepo{bookings: []*repository.OwnBooking{own}}, &middleware.Principal{Subject: ownBookingSubject}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me/bookings/"+own.Booking.BookingID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got model.OwnBookingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BookingID != own.Booking.BookingID || !got.TicketsValid || got.Event == nil || got.Event.Title != "Konzert" || got.NumberOfTickets != 3 {
		t.Errorf("unexpected details: %+v", got)
	}
	// Internal fields of the stored booking are not part of the response.
	for _, field := range []string{"userId", "paymentId", "keycloak"} {
		if strings.Contains(rec.Body.String(), field) {
			t.Errorf("response exposes %s: %s", field, rec.Body.String())
		}
	}
}
