package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type eventDetailsStub struct {
	repository.EventRepository
	details *model.PublishedEventDetailsResponse
	err     error
}

func (s *eventDetailsStub) GetPublishedEventDetails(uuid.UUID) (*model.PublishedEventDetailsResponse, error) {
	return s.details, s.err
}

func TestPublishedEventDetailsResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, id string
		repo     *eventDetailsStub
		status   int
	}{
		{"invalid ID", "invalid", &eventDetailsStub{}, http.StatusBadRequest},
		{"not found", uuid.NewString(), &eventDetailsStub{}, http.StatusNotFound},
		{"database error", uuid.NewString(), &eventDetailsStub{err: errors.New("private database error")}, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewEventHandler(service.NewEventService(tc.repo, nil, nil))
			router := gin.New()
			router.GET("/api/v1/events/:eventId", h.GetPublishedEventDetailsHandler)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+tc.id, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var problem model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != tc.status {
				t.Fatalf("invalid problem: %s", rec.Body.String())
			}
			if problem.Detail == "private database error" {
				t.Fatal("database error leaked")
			}
		})
	}
}

func getEventDetails(t *testing.T, router http.Handler, id uuid.UUID) *model.PublishedEventDetailsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+id.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var details model.PublishedEventDetailsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
		t.Fatal(err)
	}
	return &details
}

func TestPublishedEventDetailsDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "details", model.EventStatusPublished)
	description := "Ein Konzert am Abend"
	if err := db.Exec("UPDATE events SET description = ? WHERE event_id = ?", description, event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	router := newEventRouter(db, nil)
	details := getEventDetails(t, router, event.eventID)
	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if details.EventID != stored.EventID || details.Title != stored.Title ||
		details.Description == nil || *details.Description != description ||
		!details.StartTime.Equal(stored.StartTime) || !details.EndTime.Equal(stored.EndTime) ||
		!details.Price.Equal(stored.Price) || details.Status != model.EventStatusPublished ||
		details.Category.CategoryID != event.categoryID || details.Category.Category != "cat-"+event.categoryID.String() ||
		details.Location.LocationID != event.locationID || details.Location.City != "Bonn" ||
		details.Location.Name != "loc-"+event.locationID.String() ||
		details.Capacity != 100 || details.AvailableSeats != 100 || !details.Bookable {
		t.Fatalf("incorrect details: %+v", details)
	}
	// Booking and details must agree about confirmed tickets and live reservations.
	for _, row := range []struct {
		status  string
		count   int
		expires *time.Time
	}{
		{"confirmed", 20, nil},
		{"reserved", 10, ptrTime(time.Now().Add(time.Hour))},
		{"reserved", 40, ptrTime(time.Now().Add(-time.Hour))},
		{"cancelled", 40, nil},
		{"failed", 40, nil},
	} {
		if err := db.Exec("INSERT INTO bookings (booking_id, event_id, number_of_tickets, status, expires_at) VALUES (?, ?, ?, ?, ?)",
			uuid.New(), event.eventID, row.count, row.status, row.expires).Error; err != nil {
			t.Fatal(err)
		}
	}
	details = getEventDetails(t, router, event.eventID)
	if details.AvailableSeats != 70 || !details.Bookable {
		t.Fatalf("availability: %+v", details)
	}
	bookingRepo := repository.NewBookingRepository(db)
	if err := bookingRepo.InTransaction(context.Background(), func(tx repository.BookingTx) error {
		occupied, err := tx.CountOccupiedTickets(event.eventID)
		if err == nil && occupied != int64(details.Capacity)-details.AvailableSeats {
			t.Fatalf("booking occupied=%d", occupied)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, capacity := range []int{30, 25} {
		if err := db.Exec("UPDATE events SET capacity = ? WHERE event_id = ?", capacity, event.eventID).Error; err != nil {
			t.Fatal(err)
		}
		details = getEventDetails(t, router, event.eventID)
		if details.Bookable || details.AvailableSeats != 0 {
			t.Fatalf("unavailable event: %+v", details)
		}
	}
	if err := db.Exec("UPDATE events SET description = NULL WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if getEventDetails(t, router, event.eventID).Description != nil {
		t.Fatal("null description not preserved")
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestPublishedEventDetailsVisibility(t *testing.T) {
	db := testdb.Open(t)
	router := newEventRouter(db, nil)
	ids := []uuid.UUID{uuid.New()}
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
		ids = append(ids, seedEventForOrg(t, db, "details", status).eventID)
	}
	for _, column := range []string{"category_id", "location_id"} {
		event := seedEventForOrg(t, db, "details", model.EventStatusPublished)
		if err := db.Exec("UPDATE events SET "+column+" = NULL WHERE event_id = ?", event.eventID).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, event.eventID)
	}
	for _, id := range ids {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+id.String(), nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("id=%s status=%d", id, rec.Code)
		}
	}
}
