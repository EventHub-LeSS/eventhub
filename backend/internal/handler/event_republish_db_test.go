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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func postRestoreDraft(router http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/"+id+"/restore-draft", nil))
	return rec
}
func assertEventStatus(t *testing.T, db *gorm.DB, id uuid.UUID, status model.EventStatus) model.EventModel {
	t.Helper()
	var event model.EventModel
	if err := db.First(&event, "event_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if event.Status != status {
		t.Fatalf("status=%s want=%s", event.Status, status)
	}
	return event
}
func assertPublicEventStatus(t *testing.T, router http.Handler, id uuid.UUID, want int) {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+id.String(), nil))
	if rec.Code != want {
		t.Fatalf("public status=%d body=%s", rec.Code, rec.Body.String())
	}
}
func TestWithdrawRestoreEditPublishLifecycle(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "restore", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("restore"))
	user := seedBookingUser(t, db, "restore-booking-user")
	bookings := []model.BookingModel{
		{BookingID: uuid.New(), UserID: &user, EventID: &event.eventID, Status: model.BookingStatusConfirmed, NumberOfTickets: 2},
		{BookingID: uuid.New(), UserID: &user, EventID: &event.eventID, Status: model.BookingStatusCancelled, NumberOfTickets: 1},
	}
	if err := db.Create(&bookings).Error; err != nil {
		t.Fatal(err)
	}
	original := assertEventStatus(t, db, event.eventID, model.EventStatusPublished)
	if rec := postWithdraw(t, router, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw=%s", rec.Body.String())
	}
	assertPublicEventStatus(t, router, event.eventID, http.StatusNotFound)
	rec := postRestoreDraft(router, event.eventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("restore=%s", rec.Body.String())
	}
	var response model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	restored := assertEventStatus(t, db, event.eventID, model.EventStatusDraft)
	if response.Status != model.EventStatusDraft || restored.EventID != original.EventID || restored.Title != original.Title ||
		!restored.StartTime.Equal(original.StartTime) || !restored.EndTime.Equal(original.EndTime) ||
		!restored.Price.Equal(original.Price) || restored.Capacity != original.Capacity ||
		*restored.OrganizerID != *original.OrganizerID || *restored.LocationID != *original.LocationID ||
		*restored.CategoryID != *original.CategoryID || !restored.CreatedAt.Equal(original.CreatedAt) {
		t.Fatal("restoration changed event data")
	}
	assertPublicEventStatus(t, router, event.eventID, http.StatusNotFound)
	tooSmall := validBody(event)
	tooSmall["capacity"] = 1
	if rec := putEvent(t, router, event.eventID.String(), tooSmall); rec.Code != http.StatusConflict {
		t.Fatalf("capacity guard=%s", rec.Body.String())
	}
	assertEventStatus(t, db, event.eventID, model.EventStatusDraft)
	if rec := putEvent(t, router, event.eventID.String(), validBody(event)); rec.Code != http.StatusOK {
		t.Fatalf("edit=%s", rec.Body.String())
	}
	if rec := postPublish(t, router, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("publish=%s", rec.Body.String())
	}
	published := assertEventStatus(t, db, event.eventID, model.EventStatusPublished)
	if published.Title != "Neues Konzert" {
		t.Fatal("edited title was lost")
	}
	assertPublicEventStatus(t, router, event.eventID, http.StatusOK)
	for _, originalBooking := range bookings {
		var stored model.BookingModel
		if err := db.First(&stored, "booking_id=?", originalBooking.BookingID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Status != originalBooking.Status || stored.EventID == nil || *stored.EventID != event.eventID || stored.NumberOfTickets != originalBooking.NumberOfTickets {
			t.Fatalf("booking changed: %+v", stored)
		}
	}
	var count int64
	if err := db.Model(&model.EventModel{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("event was duplicated: %d", count)
	}
	var entries []model.AuditLogModel
	if err := db.Where("resource_id=?", event.eventID.String()).Order("occurred_at ASC").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("audit count=%d", len(entries))
	}
}
func TestRepublishWithdrawnEventDirectly(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "republish", model.EventStatusCancelled)
	router := newEventRouter(db, principalManaging("republish"))
	if rec := postPublish(t, router, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("republish=%s", rec.Body.String())
	}
	assertEventStatus(t, db, event.eventID, model.EventStatusPublished)
	assertPublicEventStatus(t, router, event.eventID, http.StatusOK)
	var entry model.AuditLogModel
	if err := db.First(&entry, "resource_id=?", event.eventID.String()).Error; err != nil {
		t.Fatal(err)
	}
	change := entry.Changes["fields"].(map[string]any)["status"].(map[string]any)
	if entry.Action != string(audit.EventPublished) || change["from"] != "cancelled" || change["to"] != "published" {
		t.Fatalf("audit=%+v", entry)
	}
}
func TestRestoreDraftAndRepublishRejectUnauthorizedOrInvalidState(t *testing.T) {
	db := testdb.Open(t)
	cancelled := seedEventForOrg(t, db, "restore", model.EventStatusCancelled)
	for _, action := range []string{"restore-draft", "publish"} {
		for _, tc := range []struct {
			name, id  string
			principal *middleware.Principal
			want      int
		}{
			{"anonymous", cancelled.eventID.String(), nil, 401},
			{"no role", cancelled.eventID.String(), &middleware.Principal{Subject: "visitor"}, 403},
			{"foreign", cancelled.eventID.String(), principalManaging("foreign"), 403},
			{"unknown", uuid.NewString(), principalManaging("restore"), 404},
			{"invalid", "invalid", principalManaging("restore"), 400},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				router := newEventRouter(db, tc.principal)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/"+tc.id+"/"+action, nil))
				if rec.Code != tc.want {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
				}
				assertEventStatus(t, db, cancelled.eventID, model.EventStatusCancelled)
			})
		}
	}
	router := newEventRouter(db, principalManaging("restore"))
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCompleted} {
		event := seedEventForOrg(t, db, "restore", status)
		if rec := postRestoreDraft(router, event.eventID.String()); rec.Code != 400 {
			t.Fatalf("restore %s=%s", status, rec.Body.String())
		}
		assertEventStatus(t, db, event.eventID, status)
	}
	incomplete := seedEventForOrg(t, db, "restore", model.EventStatusCancelled)
	if err := db.Exec("UPDATE events SET category_id=NULL WHERE event_id=?", incomplete.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if rec := postPublish(t, router, incomplete.eventID.String()); rec.Code != 400 {
		t.Fatalf("incomplete publish=%s", rec.Body.String())
	}
	assertEventStatus(t, db, incomplete.eventID, model.EventStatusCancelled)
	if rec := postRestoreDraft(router, incomplete.eventID.String()); rec.Code != 200 {
		t.Fatalf("incomplete restore=%s", rec.Body.String())
	}
	assertEventStatus(t, db, incomplete.eventID, model.EventStatusDraft)
	completed := seedEventForOrg(t, db, "restore", model.EventStatusCompleted)
	if rec := postPublish(t, router, completed.eventID.String()); rec.Code != 400 {
		t.Fatalf("completed publish=%s", rec.Body.String())
	}
}
func TestRestoreDraftConcurrentRequests(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "restore", model.EventStatusCancelled)
	router := newEventRouter(db, principalManaging("restore"))
	ok, rejected := runConcurrentPosts(t, db, 8, func() int { return postRestoreDraft(router, event.eventID.String()).Code })
	if ok != 1 || rejected != 7 {
		t.Fatalf("success=%d rejected=%d", ok, rejected)
	}
	assertEventStatus(t, db, event.eventID, model.EventStatusDraft)
	var count int64
	if err := db.Model(&model.AuditLogModel{}).Where("action=?", string(audit.EventRestoredAsDraft)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}
func TestRestoreDraftAndRepublishRollbackWhenAuditFails(t *testing.T) {
	db := testdb.Open(t)
	for _, sql := range []string{
		"CREATE FUNCTION fail_restore_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit down'; END $$",
		"CREATE TRIGGER fail_restore_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_restore_audit_insert()",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS fail_restore_audit ON audit_logs")
		db.Exec("DROP FUNCTION IF EXISTS fail_restore_audit_insert()")
	})
	router := newEventRouter(db, principalManaging("restore"))
	for _, action := range []string{"restore-draft", "publish"} {
		event := seedEventForOrg(t, db, "restore", model.EventStatusCancelled)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/"+event.eventID.String()+"/"+action, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("action=%s status=%d body=%s", action, rec.Code, rec.Body.String())
		}
		assertEventStatus(t, db, event.eventID, model.EventStatusCancelled)
	}
}
func TestRepublishCancelledConcurrentRequests(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "republish", model.EventStatusCancelled)
	router := newEventRouter(db, principalManaging("republish"))
	ok, rejected := runConcurrentPosts(t, db, 8, func() int { return postPublish(t, router, event.eventID.String()).Code })
	if ok != 1 || rejected != 7 {
		t.Fatalf("success=%d rejected=%d", ok, rejected)
	}
	assertEventStatus(t, db, event.eventID, model.EventStatusPublished)
	var count int64
	if err := db.Model(&model.AuditLogModel{}).Where("action=?", string(audit.EventPublished)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}

func TestRepublishRestoresTicketValidity(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "republish-tickets", model.EventStatusPublished)
	seedBookingUser(t, db, "republish-ticket-buyer")
	visitor := newOwnBookingRouter(db, &middleware.Principal{Subject: "republish-ticket-buyer"})
	bookingID := addToCartViaAPI(t, visitor, event.eventID, 1)
	if paid := payViaAPI(t, visitor, model.SimulatedPaymentSuccess); !paid.TicketsValid {
		t.Fatal("paid ticket should initially be valid")
	}
	wantTickets := func(step string, valid bool) {
		t.Helper()
		booking := getOwnBookingDetail(t, visitor, bookingID)
		if booking.BookingStatus != model.BookingStatusConfirmed || booking.TicketsValid != valid {
			t.Fatalf("%s: %+v", step, booking)
		}
	}
	organizer := newEventRouter(db, principalManaging("republish-tickets"))
	if rec := postWithdraw(t, organizer, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw=%s", rec.Body.String())
	}
	wantTickets("withdrawn", false)
	if rec := postRestoreDraft(organizer, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("restore=%s", rec.Body.String())
	}
	wantTickets("restored draft", false)
	if rec := postPublish(t, organizer, event.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("republish=%s", rec.Body.String())
	}
	wantTickets("republished", true)
}

func TestModerationWithdrawalBlocksRestoreAndRepublish(t *testing.T) {
	db := testdb.Open(t)
	router := newEventRouter(db, principalManaging("moderation-block"))
	moderation := service.NewModerationService(repository.NewTransactor(db), nil)
	for _, status := range []model.EventStatus{model.EventStatusPublished, model.EventStatusCancelled} {
		event := seedEventForOrg(t, db, "moderation-block", status)
		ctx := audit.WithMeta(context.Background(), audit.Meta{OperationID: uuid.New(), Action: audit.EventModerated, ActorSubject: "moderator"})
		changed, err := moderation.WithdrawEvent(ctx, event.eventID, "Policy violation")
		if err != nil || !changed.Changed {
			t.Fatalf("moderate status=%s result=%+v err=%v", status, changed, err)
		}
		blocked := assertEventStatus(t, db, event.eventID, model.EventStatusCancelled)
		if !blocked.WithdrawnByModerator {
			t.Fatal("moderation block not stored")
		}
		if rec := postRestoreDraft(router, event.eventID.String()); rec.Code != 400 {
			t.Fatalf("moderated restore=%s", rec.Body.String())
		}
		if rec := postPublish(t, router, event.eventID.String()); rec.Code != 400 {
			t.Fatalf("moderated publish=%s", rec.Body.String())
		}
		assertEventStatus(t, db, event.eventID, model.EventStatusCancelled)
	}
}
