package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func databaseModerationRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	return moderationRouter(service.NewModerationService(repository.NewTransactor(db), repository.NewModerationRepository(db)), principal)
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

func seedModeratedRating(t *testing.T, db *gorm.DB, eventID *uuid.UUID, score int, text string, visible bool, age time.Duration) uuid.UUID {
	t.Helper()
	bookingID, ratingID := uuid.New(), uuid.New()
	if err := db.Exec("INSERT INTO bookings (booking_id, event_id, status, number_of_tickets, expires_at) VALUES (?, ?, 'confirmed', 1, NOW() + interval '1 hour')",
		bookingID, eventID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO ratings (rating_id, booking_id, score, text, is_visible, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		ratingID, bookingID, score, text, visible, time.Now().Add(-age)).Error; err != nil {
		t.Fatal(err)
	}
	return ratingID
}

func readModeratedRatings(t *testing.T, router http.Handler, query string) (model.ModeratedRatingPage, string) {
	t.Helper()
	rec := listModeratedRatings(router, query)
	var page model.ModeratedRatingPage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("list %q: %d %s", query, rec.Code, rec.Body.String())
	}
	return page, rec.Body.String()
}

func TestListRatingsForModerationDatabase(t *testing.T) {
	db := testdb.Open(t)
	router := databaseModerationRouter(db, globalPrincipal(middleware.RoleModerator))
	if page, body := readModeratedRatings(t, router, ""); page.Total != 0 || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty list: %s", body)
	}

	first := seedEventForOrg(t, db, "list-a", model.EventStatusPublished)
	second := seedEventForOrg(t, db, "list-b", model.EventStatusPublished)
	oldest := seedModeratedRating(t, db, &first.eventID, 5, "Super", true, 4*time.Hour)
	hidden := seedModeratedRating(t, db, &first.eventID, 1, "Betrug!", false, 3*time.Hour)
	other := seedModeratedRating(t, db, &second.eventID, 3, "ok", true, 2*time.Hour)
	detached := seedModeratedRating(t, db, nil, 2, "verwaist", true, time.Hour)

	all, _ := readModeratedRatings(t, router, "")
	if all.Total != 4 || len(all.Items) != 4 {
		t.Fatalf("all ratings: %+v", all)
	}
	got := all.Items[2]
	if got.RatingID != hidden || got.Score != 1 || got.Text != "Betrug!" || !got.Hidden ||
		got.EventID == nil || *got.EventID != first.eventID || got.EventTitle == nil || *got.EventTitle != "Altes Konzert" ||
		got.OrganizationID == nil || *got.OrganizationID != first.ownerOrgID || got.OrganizationName == nil || *got.OrganizationName != "Org list-a" {
		t.Fatalf("hidden rating: %+v", got)
	}
	if got := all.Items[0]; got.RatingID != detached || got.EventID != nil || got.EventTitle != nil || got.OrganizationID != nil || got.OrganizationName != nil {
		t.Fatalf("detached rating: %+v", got)
	}

	for _, tc := range []struct {
		query string
		want  []uuid.UUID
		total int64
	}{
		{"", []uuid.UUID{detached, other, hidden, oldest}, 4},
		{"?hidden=true", []uuid.UUID{hidden}, 1},
		{"?hidden=false", []uuid.UUID{detached, other, oldest}, 3},
		{"?eventId=" + first.eventID.String(), []uuid.UUID{hidden, oldest}, 2},
		{"?hidden=false&eventId=" + first.eventID.String(), []uuid.UUID{oldest}, 1},
		{"?eventId=" + uuid.NewString(), nil, 0},
		{"?limit=2&page=2", []uuid.UUID{hidden, oldest}, 4},
		{"?limit=2&page=3", nil, 4},
	} {
		page, _ := readModeratedRatings(t, router, tc.query)
		ids := make([]uuid.UUID, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.RatingID)
		}
		if page.Total != tc.total || len(ids) != len(tc.want) {
			t.Errorf("query %q: total=%d ids=%v, want total=%d ids=%v", tc.query, page.Total, ids, tc.total, tc.want)
			continue
		}
		for i := range ids {
			if ids[i] != tc.want[i] {
				t.Errorf("query %q: ids=%v, want %v", tc.query, ids, tc.want)
				break
			}
		}
	}

	// Releasing withdraws the hiding: the rating leaves the hidden list.
	decodeRatingVisibility(t, putRatingVisibility(router, hidden.String(), `{"hidden":false,"reason":"Kein Verstoß"}`))
	if page, _ := readModeratedRatings(t, router, "?hidden=true"); page.Total != 0 {
		t.Fatalf("released rating still hidden: %+v", page)
	}
}
