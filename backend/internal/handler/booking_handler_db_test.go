package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	return r
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
	db := setupHandlerDB(t)
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
	db := setupHandlerDB(t)
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
		})
	}
	if n := bookingCount(t, db); n != 0 {
		t.Errorf("rejected requests must not create bookings, found %d", n)
	}
}
