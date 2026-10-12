package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func databaseRatingRouter(db *gorm.DB) http.Handler {
	return ratingRouter(service.NewRatingService(repository.NewRatingRepository(db),
		repository.NewEventRepository(db)))
}

func seedSummaryRating(t *testing.T, db *gorm.DB, eventID *uuid.UUID, status string, tickets, score int, visible bool) uuid.UUID {
	t.Helper()
	bookingID := uuid.New()
	if err := db.Exec("INSERT INTO bookings (booking_id, event_id, status, number_of_tickets, expires_at) VALUES (?, ?, ?, ?, NOW() + interval '1 hour')",
		bookingID, eventID, status, tickets).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO ratings (rating_id, booking_id, score, text, is_visible) VALUES (?, ?, ?, 'test', ?)",
		uuid.New(), bookingID, score, visible).Error; err != nil {
		t.Fatal(err)
	}
	return bookingID
}

func readEventRatingSummary(t *testing.T, router http.Handler, id uuid.UUID) model.EventRatingSummary {
	t.Helper()
	rec := getRatingSummary(router, "events", id.String())
	var summary model.EventRatingSummary
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &summary) != nil || summary.EventID != id {
		t.Fatalf("invalid event summary: %d %s", rec.Code, rec.Body.String())
	}
	return summary
}

func TestEventRatingSummaryValidReviewsDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "ratings", model.EventStatusPublished)
	other := seedEventForOrg(t, db, "other", model.EventStatusPublished)
	router := databaseRatingRouter(db)
	empty := readEventRatingSummary(t, router, event.eventID)
	if empty.AverageRating != nil || empty.RatingCount != 0 {
		t.Fatalf("empty summary: %+v", empty)
	}
	// Review counts and averages must not be weighted by ticket quantities.
	seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 1, true)
	seedSummaryRating(t, db, &event.eventID, "confirmed", 9, 5, true)
	seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 5, false)
	for _, status := range []string{"reserved", "cancelled", "failed", "expired"} {
		seedSummaryRating(t, db, &event.eventID, status, 1, 5, true)
	}
	seedSummaryRating(t, db, &other.eventID, "confirmed", 1, 5, true)
	seedSummaryRating(t, db, nil, "confirmed", 1, 5, true)
	detached := seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 5, true)
	if err := db.Exec("DELETE FROM bookings WHERE booking_id = ?", detached).Error; err != nil {
		t.Fatal(err)
	}
	summary := readEventRatingSummary(t, router, event.eventID)
	if summary.AverageRating == nil || *summary.AverageRating != 3 || summary.RatingCount != 2 {
		t.Fatalf("valid-review summary: %+v", summary)
	}
	// Moderation changes must immediately affect both average and count.
	if err := db.Exec("UPDATE ratings SET is_visible = false WHERE booking_id IN (SELECT booking_id FROM bookings WHERE event_id = ?)", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	empty = readEventRatingSummary(t, router, event.eventID)
	if empty.AverageRating != nil || empty.RatingCount != 0 {
		t.Fatalf("hidden-only summary: %+v", empty)
	}
}

func TestEventRatingSummaryVisibilityDatabase(t *testing.T) {
	db := testdb.Open(t)
	router := databaseRatingRouter(db)
	for _, tc := range []struct {
		name   string
		status model.EventStatus
		ended  bool
		want   int
	}{
		{"future published", model.EventStatusPublished, false, http.StatusOK},
		{"ended published", model.EventStatusPublished, true, http.StatusOK},
		{"ended completed", model.EventStatusCompleted, true, http.StatusOK},
		{"future completed", model.EventStatusCompleted, false, http.StatusNotFound},
		{"draft", model.EventStatusDraft, true, http.StatusNotFound},
		{"cancelled", model.EventStatusCancelled, true, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := seedEventForOrg(t, db, "visibility", tc.status)
			if tc.ended {
				if err := db.Exec("UPDATE events SET start_time = NOW() - interval '2 days', end_time = NOW() - interval '1 day' WHERE event_id = ?", event.eventID).Error; err != nil {
					t.Fatal(err)
				}
			}
			rec := getRatingSummary(router, "events", event.eventID.String())
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	assertOrganizationEventsProblem(t, getRatingSummary(router, "events", uuid.NewString()), http.StatusNotFound)
}

func TestRatingScoreRangeDatabase(t *testing.T) {
	db := testdb.Open(t)
	for _, score := range []int{0, 6} {
		if err := db.Exec("INSERT INTO ratings (rating_id, score, text, is_visible, created_at) VALUES (?, ?, 'invalid', true, ?)",
			uuid.New(), score, time.Now()).Error; err == nil {
			t.Fatalf("database accepted invalid score %d", score)
		}
	}
}
