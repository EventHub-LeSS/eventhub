package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type publishedEventsStub struct {
	repository.EventRepository
	events []model.PublishedEventResponse
	err    error
}

func (s *publishedEventsStub) ListPublishedEvents() ([]model.PublishedEventResponse, error) {
	return s.events, s.err
}

func TestListPublishedEventsResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		repo *publishedEventsStub
		want int
		body string
	}{
		{"empty", &publishedEventsStub{}, http.StatusOK, "[]"},
		{"database error", &publishedEventsStub{err: errors.New("private database details")}, http.StatusInternalServerError, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewEventHandler(service.NewEventService(tc.repo, nil, nil))
			router := gin.New()
			router.GET("/api/v1/events", h.ListPublishedEventsHandler)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.body != "" && rec.Body.String() != tc.body {
				t.Fatalf("body=%s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "private database details") {
				t.Fatal("database details exposed")
			}
			if tc.want == http.StatusInternalServerError {
				var problem model.ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != tc.want {
					t.Fatalf("invalid error response: %s", rec.Body.String())
				}
			}
		})
	}
}

func listPublishedEvents(t *testing.T, router http.Handler) []model.PublishedEventResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var events []model.PublishedEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if events == nil {
		t.Fatal("expected array, got null")
	}
	return events
}

func TestListPublishedEventsDatabase(t *testing.T) {
	db := setupHandlerDB(t)
	router := newEventRouter(db, nil)
	if events := listPublishedEvents(t, router); len(events) != 0 {
		t.Fatal("expected empty list")
	}
	published := seedEventForOrg(t, db, "listing", model.EventStatusPublished)
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
		seedEventForOrg(t, db, "listing", status)
	}
	events := listPublishedEvents(t, router)
	if len(events) != 1 {
		t.Fatalf("count=%d, want 1", len(events))
	}
	event := events[0]
	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", published.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if event.EventID != published.eventID || event.Title != stored.Title || event.Status != model.EventStatusPublished ||
		!event.StartTime.Equal(stored.StartTime) || !event.EndTime.Equal(stored.EndTime) || !event.Price.Equal(stored.Price) {
		t.Fatalf("wrong event data: %+v", event)
	}
	if event.Category.CategoryID != published.categoryID || event.Category.Category != "cat-"+published.categoryID.String() ||
		event.Location.LocationID != published.locationID || event.Location.Name != "loc-"+published.locationID.String() ||
		event.Location.City != "Bonn" || event.Location.PostalCode != "53111" || event.Location.Street != "Weg" || event.Location.HouseNumber != nil {
		t.Fatalf("wrong display data: %+v", event)
	}
	for _, column := range []string{"category_id", "location_id"} {
		if err := db.Exec("UPDATE events SET "+column+" = NULL WHERE event_id = ?", published.eventID).Error; err != nil {
			t.Fatal(err)
		}
		if events := listPublishedEvents(t, router); len(events) != 0 {
			t.Fatalf("event missing %s is visible", column)
		}
		if err := db.Model(&model.EventModel{}).Where("event_id = ?", published.eventID).
			Updates(map[string]any{"category_id": published.categoryID, "location_id": published.locationID}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestListPublishedEventsLifecycle(t *testing.T) {
	db := setupHandlerDB(t)
	seeded := seedEventForOrg(t, db, "listing", model.EventStatusDraft)
	publicRouter := newEventRouter(db, nil)
	managerRouter := newEventRouter(db, principalManaging("listing"))
	if len(listPublishedEvents(t, publicRouter)) != 0 {
		t.Fatal("draft visible")
	}
	if rec := postPublish(t, managerRouter, seeded.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("publish: %s", rec.Body.String())
	}
	if events := listPublishedEvents(t, publicRouter); len(events) != 1 || events[0].EventID != seeded.eventID {
		t.Fatal("published event missing")
	}
	if rec := postWithdraw(t, managerRouter, seeded.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw: %s", rec.Body.String())
	}
	if len(listPublishedEvents(t, publicRouter)) != 0 {
		t.Fatal("withdrawn event visible")
	}
}

func TestListPublishedEventsOrdering(t *testing.T) {
	db := setupHandlerDB(t)
	first := seedEventForOrg(t, db, "ordering", model.EventStatusPublished)
	second := seedEventForOrg(t, db, "ordering", model.EventStatusPublished)
	later := seedEventForOrg(t, db, "ordering", model.EventStatusPublished)
	// Exact ties must use the event ID rather than database insertion order.
	for _, id := range []string{first.eventID.String(), second.eventID.String()} {
		if err := db.Exec("UPDATE events SET start_time = '2030-01-01T10:00:00Z', end_time = '2030-01-01T12:00:00Z' WHERE event_id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE events SET start_time = '2030-01-02T10:00:00Z', end_time = '2030-01-02T12:00:00Z' WHERE event_id = ?", later.eventID).Error; err != nil {
		t.Fatal(err)
	}
	events := listPublishedEvents(t, newEventRouter(db, nil))
	if len(events) != 3 {
		t.Fatalf("count=%d", len(events))
	}
	low, high := first.eventID, second.eventID
	if low.String() > high.String() {
		low, high = high, low
	}
	if events[0].EventID != low || events[1].EventID != high || events[2].EventID != later.eventID {
		t.Fatalf("unexpected order: %+v", events)
	}
}
