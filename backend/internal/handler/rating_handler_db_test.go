package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func databaseRatingRouter(db *gorm.DB) http.Handler {
	return ratingRouter(service.NewRatingService(repository.NewRatingRepository(db),
		repository.NewEventRepository(db), repository.NewOrganizationRepository(db)))
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

func readOrganizerRatingSummary(t *testing.T, router http.Handler, id uuid.UUID) model.OrganizerRatingSummary {
	t.Helper()
	rec := getRatingSummary(router, "organizations", id.String())
	var summary model.OrganizerRatingSummary
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &summary) != nil || summary.OrganizationID != id {
		t.Fatalf("invalid organizer summary: %d %s", rec.Code, rec.Body.String())
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

func TestOrganizerRatingSummaryEventWeightingDatabase(t *testing.T) {
	db := testdb.Open(t)
	a := seedEventForOrg(t, db, "weighted", model.EventStatusPublished)
	b := seedEventForOrg(t, db, "weighted", model.EventStatusCompleted)
	unrated := seedEventForOrg(t, db, "weighted", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET start_time = NOW() - interval '2 days', end_time = NOW() - interval '1 day' WHERE event_id IN ?",
		[]uuid.UUID{a.eventID, b.eventID, unrated.eventID}).Error; err != nil {
		t.Fatal(err)
	}
	seedSummaryRating(t, db, &a.eventID, "confirmed", 1, 5, true)
	for i := 0; i < 9; i++ {
		seedSummaryRating(t, db, &b.eventID, "confirmed", 1, 1, true)
	}
	seedSummaryRating(t, db, &a.eventID, "confirmed", 1, 1, false)
	seedSummaryRating(t, db, &b.eventID, "cancelled", 1, 5, true)
	for _, tc := range []struct {
		status model.EventStatus
		ended  bool
	}{
		{model.EventStatusPublished, false},
		{model.EventStatusCompleted, false},
		{model.EventStatusDraft, true},
		{model.EventStatusCancelled, true},
	} {
		event := seedEventForOrg(t, db, "weighted", tc.status)
		if tc.ended {
			if err := db.Exec("UPDATE events SET end_time = NOW() - interval '1 day' WHERE event_id = ?", event.eventID).Error; err != nil {
				t.Fatal(err)
			}
		}
		seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 5, true)
	}
	foreign := seedEventForOrg(t, db, "foreign", model.EventStatusCompleted)
	if err := db.Exec("UPDATE events SET end_time = NOW() - interval '1 day' WHERE event_id = ?", foreign.eventID).Error; err != nil {
		t.Fatal(err)
	}
	seedSummaryRating(t, db, &foreign.eventID, "confirmed", 1, 5, true)
	router := databaseRatingRouter(db)
	summary := readOrganizerRatingSummary(t, router, a.ownerOrgID)
	first := readEventRatingSummary(t, router, a.eventID)
	second := readEventRatingSummary(t, router, b.eventID)
	if summary.AverageRating == nil || *summary.AverageRating != 3 || summary.RatedEventCount != 2 ||
		first.AverageRating == nil || *first.AverageRating != 5 || first.RatingCount != 1 ||
		second.AverageRating == nil || *second.AverageRating != 1 || second.RatingCount != 9 {
		t.Fatalf("inconsistent event weighting: organizer=%+v first=%+v second=%+v", summary, first, second)
	}
	// Preserve fractional event averages rather than rounding before aggregating.
	seedSummaryRating(t, db, &a.eventID, "confirmed", 1, 4, true)
	seedSummaryRating(t, db, &a.eventID, "confirmed", 1, 4, true)
	summary = readOrganizerRatingSummary(t, router, a.ownerOrgID)
	if summary.AverageRating == nil || math.Abs(*summary.AverageRating-8.0/3) > 1e-12 {
		t.Fatalf("rounded intermediate average: %+v", summary)
	}
}

func TestOrganizerRatingSummaryEmptyAndEndBoundaryDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "boundary", model.EventStatusPublished)
	router := databaseRatingRouter(db)
	// An organization with no ended, rated events has no average.
	seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 4, true)
	empty := readOrganizerRatingSummary(t, router, event.ownerOrgID)
	if empty.AverageRating != nil || empty.RatedEventCount != 0 {
		t.Fatalf("future-only organization: %+v", empty)
	}
	// NOW() is fixed within this transaction, exercising equality precisely.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE events SET end_time = NOW() WHERE event_id = ?", event.eventID).Error; err != nil {
			return err
		}
		summary := readOrganizerRatingSummary(t, databaseRatingRouter(tx), event.ownerOrgID)
		if summary.AverageRating == nil || *summary.AverageRating != 4 || summary.RatedEventCount != 1 {
			t.Fatalf("end-time equality: %+v", summary)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	if err := db.Exec("INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, 'Empty')", orgID, uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	empty = readOrganizerRatingSummary(t, router, orgID)
	if empty.AverageRating != nil || empty.RatedEventCount != 0 {
		t.Fatalf("empty organization: %+v", empty)
	}
	assertOrganizationEventsProblem(t, getRatingSummary(router, "organizations", uuid.NewString()), http.StatusNotFound)
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
