package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"context"
	"encoding/json"
	"errors"
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

func readModeratedEvents(t *testing.T, router http.Handler, query string) model.ModeratedEventPage {
	t.Helper()
	rec := requestEventModeration(router, http.MethodGet, query, "")
	var page model.ModeratedEventPage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("list events: %d %s", rec.Code, rec.Body.String())
	}
	if page.Items == nil {
		t.Fatal("items must be an array")
	}
	return page
}

func TestEventModerationListDatabase(t *testing.T) {
	db := testdb.Open(t)
	router := databaseModerationRouter(db, globalPrincipal(middleware.RoleAdmin))
	if page := readModeratedEvents(t, router, ""); page.Total != 0 {
		t.Fatalf("empty database: %+v", page)
	}
	statuses := []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted}
	seeds := make([]seededEvent, 0, len(statuses))
	for index, status := range statuses {
		event := seedEventForOrg(t, db, "moderation-list", status)
		if err := db.Exec("UPDATE events SET description = ?, created_at = ? WHERE event_id = ?", "Full description", time.Now().Add(time.Duration(index)*time.Hour), event.eventID).Error; err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, event)
	}
	if page := readModeratedEvents(t, router, ""); page.Total != 4 || len(page.Items) != 4 || page.Items[0].EventID != seeds[3].eventID || page.Items[0].Description == nil || *page.Items[0].Description != "Full description" || page.Items[0].OrganizationName == nil || *page.Items[0].OrganizationName != "Org moderation-list" {
		t.Fatalf("all statuses and text: %+v", page)
	}
	for index, status := range statuses {
		page := readModeratedEvents(t, router, "?status="+string(status)+"&organizationId="+seeds[0].ownerOrgID.String())
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].EventID != seeds[index].eventID {
			t.Fatalf("filter %s: %+v", status, page)
		}
		rec := requestEventModeration(router, http.MethodGet, "/"+seeds[index].eventID.String(), "")
		var event model.ModeratedEvent
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &event) != nil || event.EventID != seeds[index].eventID || event.Status != status {
			t.Fatalf("inspect %s: %d %s", status, rec.Code, rec.Body.String())
		}
	}
	if page := readModeratedEvents(t, router, "?limit=2&page=2"); page.Total != 4 || len(page.Items) != 2 || page.Items[0].EventID != seeds[1].eventID {
		t.Fatalf("pagination: %+v", page)
	}
	if page := readModeratedEvents(t, router, "?limit=2&page=3"); page.Total != 4 || len(page.Items) != 0 {
		t.Fatalf("past last page: %+v", page)
	}
	if page := readModeratedEvents(t, router, "?organizationId="+uuid.NewString()); page.Total != 0 {
		t.Fatalf("unknown organization: %+v", page)
	}
	if err := db.Exec("UPDATE events SET organizer_id = NULL WHERE event_id = ?", seeds[0].eventID).Error; err != nil {
		t.Fatal(err)
	}
	if page := readModeratedEvents(t, router, "?status=draft"); page.Total != 1 || page.Items[0].OrganizationName != nil {
		t.Fatalf("detached event missing: %+v", page)
	}
}

func TestEventModerationConsistencyDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "moderation-consistency", model.EventStatusPublished)
	userID := seedBookingUser(t, db, "moderation-buyer")
	visitor := newOwnBookingRouter(db, &middleware.Principal{Subject: "moderation-buyer"})
	bookings := service.NewBookingService(repository.NewBookingRepository(db), 0)
	paidBooking, err := bookings.ReserveTickets(context.Background(), event.eventID, userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	paid := payViaAPI(t, visitor, paidBooking.BookingID, model.SimulatedPaymentSuccess)
	if !paid.TicketsValid {
		t.Fatal("paid ticket should initially be valid")
	}
	pendingBooking, err := bookings.ReserveTickets(context.Background(), event.eventID, userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	pending := startPaymentViaAPI(t, visitor, pendingBooking.BookingID)
	moderation := databaseModerationRouter(db, globalPrincipal(middleware.RoleAdmin))
	path := "/" + event.eventID.String()
	for _, body := range []string{`{"title":" Corrected title ","description":"Safe description","reason":" Offensive text "}`, `{"title":"Corrected title","description":"Safe description","reason":"Repeated"}`} {
		if rec := requestEventModeration(moderation, http.MethodPatch, path+"/content", body); rec.Code != http.StatusOK {
			t.Fatalf("correct text: %d %s", rec.Code, rec.Body.String())
		}
	}
	events := repository.NewEventRepository(db)
	details, err := events.GetPublishedEventDetails(event.eventID)
	if err != nil || details == nil || details.Title != "Corrected title" || details.Description == nil || *details.Description != "Safe description" || details.Capacity != 100 {
		t.Fatalf("corrected public details: %+v %v", details, err)
	}
	for range 2 {
		if rec := requestEventModeration(moderation, http.MethodPost, path+"/withdraw", `{"reason":" Confirmed fraud "}`); rec.Code != http.StatusOK {
			t.Fatalf("withdraw: %d %s", rec.Code, rec.Body.String())
		}
	}
	if details, err := events.GetPublishedEventDetails(event.eventID); err != nil || details != nil {
		t.Fatalf("withdrawn public details: %+v %v", details, err)
	}
	if list, err := events.ListPublishedEvents(model.PublishedEventFilter{}); err != nil || len(list) != 0 {
		t.Fatalf("withdrawn public listing: %+v %v", list, err)
	}
	if candidates, err := repository.NewRecommendationRepository(db).ListCandidates(uuid.New()); err != nil || len(candidates) != 0 {
		t.Fatalf("withdrawn recommendation: %+v %v", candidates, err)
	}
	if _, err := bookings.ReserveTickets(context.Background(), event.eventID, userID, 1); !errors.Is(err, service.ErrEventNotPublished) {
		t.Fatalf("new booking accepted: %v", err)
	}
	failed := simulateViaAPI(t, visitor, pendingBooking.BookingID, model.SimulatedPaymentSuccess, pending.Amount)
	if failed.TicketsValid || failed.Status != model.PaymentStatusFailed {
		t.Fatalf("pending payment succeeded after moderation: %+v", failed)
	}
	storedPayment := decodePayment(t, doPaymentRequest(t, visitor, http.MethodGet, paymentPath(paidBooking.BookingID), nil))
	if storedPayment.TicketsValid || storedPayment.Status != model.PaymentStatusPaid {
		t.Fatalf("paid history or ticket validity inconsistent: %+v", storedPayment)
	}
	if rec := postPublish(t, newEventRouter(db, principalManaging("moderation-consistency")), event.eventID.String()); rec.Code != http.StatusBadRequest {
		t.Fatalf("organizer republished moderated event: %d %s", rec.Code, rec.Body.String())
	}
	entries, err := repository.NewAuditLogRepository(db).ListByOrganization("moderation-consistency", 10, nil)
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected one entry per change: %+v %v", entries, err)
	}
	for index, reason := range []string{"Confirmed fraud", "Offensive text"} {
		entry := entries[index]
		if entry.Action != string(audit.EventModerated) || entry.ActorSubject != "moderator-1" || entry.ResourceID != event.eventID.String() || entry.Changes["reason"] != reason || entry.Phase != audit.PhaseCompleted {
			t.Fatalf("audit entry: %+v", entry)
		}
	}
	fields := entries[0].Changes["fields"].(map[string]any)
	change := fields["status"].(map[string]any)
	if change["from"] != "published" || change["to"] != "cancelled" {
		t.Fatalf("missing status transition: %+v", change)
	}
}

func TestEventModerationAuditRollbackDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "moderation-rollback", model.EventStatusPublished)
	for _, statement := range []string{
		`CREATE FUNCTION fail_moderation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit down'; END $$`,
		`CREATE TRIGGER fail_moderation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_moderation_audit()`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS fail_moderation_audit ON audit_logs")
		db.Exec("DROP FUNCTION IF EXISTS fail_moderation_audit()")
	})
	router := databaseModerationRouter(db, globalPrincipal(middleware.RoleAdmin))
	for _, request := range []struct{ method, suffix, body string }{
		{http.MethodPatch, "/content", `{"title":"Changed","reason":"Text"}`},
		{http.MethodPost, "/withdraw", `{"reason":"Fraud"}`},
	} {
		rec := requestEventModeration(router, request.method, "/"+event.eventID.String()+request.suffix, request.body)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("audit failure: %d %s", rec.Code, rec.Body.String())
		}
		stored, err := repository.NewEventRepository(db).GetEventByID(event.eventID)
		if err != nil || stored.Status != model.EventStatusPublished || stored.Title != "Altes Konzert" {
			t.Fatalf("change survived audit failure: %+v %v", stored, err)
		}
	}
}
