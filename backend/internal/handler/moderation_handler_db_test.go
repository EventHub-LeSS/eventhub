package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func databaseModerationRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	return moderationRouter(service.NewModerationService(repository.NewTransactor(db)), principal)
}

func ratingIDOfBooking(t *testing.T, db *gorm.DB, bookingID uuid.UUID) uuid.UUID {
	t.Helper()
	var id string
	if err := db.Raw("SELECT rating_id::text FROM ratings WHERE booking_id = ?", bookingID).Scan(&id).Error; err != nil {
		t.Fatalf("rating of booking %s: %v", bookingID, err)
	}
	return uuid.MustParse(id)
}

func ratingAuditEntries(t *testing.T, db *gorm.DB, ratingID uuid.UUID) []model.AuditLogModel {
	t.Helper()
	var entries []model.AuditLogModel
	if err := db.Where("resource_type = ? AND resource_id = ?", audit.ResourceRating, ratingID.String()).
		Order("occurred_at, audit_log_id").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestModerateRatingVisibilityDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "moderated", model.EventStatusPublished)
	// Organizer summaries only count ended events.
	if err := db.Exec("UPDATE events SET start_time = NOW() - interval '2 days', end_time = NOW() - interval '1 day' WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	offensive := ratingIDOfBooking(t, db, seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 1, true))
	seedSummaryRating(t, db, &event.eventID, "confirmed", 1, 5, true)
	ratings := databaseRatingRouter(db)
	moderation := databaseModerationRouter(db, globalPrincipal(middleware.RoleModerator))

	if s := readEventRatingSummary(t, ratings, event.eventID); s.RatingCount != 2 || *s.AverageRating != 3 {
		t.Fatalf("initial summary: %+v", s)
	}

	// Hiding takes effect immediately on event and organizer summaries.
	resp := decodeRatingVisibility(t, putRatingVisibility(moderation, offensive.String(), `{"hidden":true,"reason":" Beleidigung "}`))
	if !resp.Hidden || !resp.Changed {
		t.Fatalf("hide: %+v", resp)
	}
	if s := readEventRatingSummary(t, ratings, event.eventID); s.RatingCount != 1 || *s.AverageRating != 5 {
		t.Fatalf("event summary after hiding: %+v", s)
	}
	if s := readOrganizerRatingSummary(t, ratings, event.ownerOrgID); s.AverageRating == nil || *s.AverageRating != 5 {
		t.Fatalf("organizer summary after hiding: %+v", s)
	}

	// Repeating the request changes and records nothing.
	if resp := decodeRatingVisibility(t, putRatingVisibility(moderation, offensive.String(), `{"hidden":true,"reason":"again"}`)); resp.Changed {
		t.Fatalf("repeated hide: %+v", resp)
	}

	if resp := decodeRatingVisibility(t, putRatingVisibility(moderation, offensive.String(), `{"hidden":false,"reason":"Fehlentscheidung"}`)); resp.Hidden || !resp.Changed {
		t.Fatalf("show: %+v", resp)
	}
	if s := readEventRatingSummary(t, ratings, event.eventID); s.RatingCount != 2 {
		t.Fatalf("event summary after showing: %+v", s)
	}

	// Both changes are traceable in the audit log of the organization owning the event.
	entries, err := repository.NewAuditLogRepository(db).ListByOrganization("moderated", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %+v", entries)
	}
	for i, want := range []struct {
		reason   string
		from, to bool
	}{{"Fehlentscheidung", false, true}, {"Beleidigung", true, false}} { // newest first
		e := entries[i]
		fields, _ := e.Changes["fields"].(map[string]any)
		change, _ := fields["isVisible"].(map[string]any)
		if e.Action != string(audit.RatingVisibilityChanged) || e.ResourceID != offensive.String() || e.ActorSubject != "moderator-1" ||
			e.Changes["reason"] != want.reason || e.Changes["eventId"] != event.eventID.String() ||
			change["from"] != want.from || change["to"] != want.to {
			t.Errorf("entry %d: %+v", i, e)
		}
	}
}

func TestModerateRatingVisibilityDatabaseEdgeCases(t *testing.T) {
	db := testdb.Open(t)
	moderation := databaseModerationRouter(db, globalPrincipal(middleware.RoleAdmin))

	unknown := uuid.New()
	assertOrganizationEventsProblem(t, putRatingVisibility(moderation, unknown.String(), `{"hidden":true,"reason":"Scam"}`), http.StatusNotFound)
	if entries := ratingAuditEntries(t, db, unknown); len(entries) != 0 {
		t.Fatalf("unknown rating recorded: %+v", entries)
	}

	// Ratings without an event cannot be attributed to an organization and are recorded platform-wide.
	detached := ratingIDOfBooking(t, db, seedSummaryRating(t, db, nil, "confirmed", 1, 1, true))
	decodeRatingVisibility(t, putRatingVisibility(moderation, detached.String(), `{"hidden":true,"reason":"Scam"}`))
	entries := ratingAuditEntries(t, db, detached)
	if len(entries) != 1 || entries[0].OrganizationID != audit.ScopePlatform || entries[0].Changes["eventId"] != nil {
		t.Fatalf("detached rating audit: %+v", entries)
	}
	var visible bool
	if err := db.Raw("SELECT is_visible FROM ratings WHERE rating_id = ?", detached).Scan(&visible).Error; err != nil || visible {
		t.Fatalf("detached rating still visible: %v", err)
	}

	// Visitors cannot moderate; nothing changes.
	visitor := databaseModerationRouter(db, globalPrincipal(middleware.RoleVisitor))
	if rec := putRatingVisibility(visitor, detached.String(), `{"hidden":false,"reason":"mine"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("visitor: %d %s", rec.Code, rec.Body.String())
	}
	if entries := ratingAuditEntries(t, db, detached); len(entries) != 1 {
		t.Fatalf("visitor request recorded: %+v", entries)
	}
}
