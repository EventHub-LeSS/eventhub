package handler

import (
	"net/http"
	"testing"
	"time"

	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"

	"gorm.io/gorm"
)

func loadAuditEntries(t *testing.T, db *gorm.DB) []model.AuditLogModel {
	t.Helper()
	var entries []model.AuditLogModel
	if err := db.Order("occurred_at, audit_log_id").Find(&entries).Error; err != nil {
		t.Fatalf("load audit log: %v", err)
	}
	return entries
}

func TestEventUpdate_WritesAuditEntryWithPersonalAccountAndStoredPrice(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	body := validBody(seeded)
	body["price"] = 35.555 // stored with two decimals

	before := time.Now().Add(-time.Minute)
	if rec := putEvent(t, router, seeded.eventID.String(), body); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	entries := loadAuditEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ActorSubject != "user-1" || e.ActorUsername != "tester" || e.OrganizationID != "kc-org-b" ||
		e.Action != string(audit.EventUpdated) || e.Phase != audit.PhaseCompleted || e.ResourceID != seeded.eventID.String() {
		t.Errorf("unexpected entry: %+v", e)
	}
	if e.OccurredAt.Before(before) || e.OccurredAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("occurredAt = %s is not the time of the change", e.OccurredAt)
	}
	price := e.Changes["fields"].(map[string]any)["price"].(map[string]any)
	if price["from"] != "20" || price["to"] != "35.56" {
		t.Errorf("price change = %v, want 20 -> 35.56 (as stored)", price)
	}
}

func TestEventActions_AreAttributedToTheIndividualAccounts(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	anna := principalManaging("kc-org-b")
	anna.Subject, anna.Username = "sub-anna", "anna@acme.test"
	ben := principalManaging("kc-org-b")
	ben.Subject, ben.Username = "sub-ben", "ben@acme.test"

	if rec := putEvent(t, newEventRouter(db, anna), seeded.eventID.String(), validBody(seeded)); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postPublish(t, newEventRouter(db, ben), seeded.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postWithdraw(t, newEventRouter(db, anna), seeded.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw: %d %s", rec.Code, rec.Body.String())
	}

	var got [][2]string
	for _, e := range loadAuditEntries(t, db) {
		got = append(got, [2]string{e.Action, e.ActorUsername})
	}
	want := [][2]string{
		{string(audit.EventUpdated), "anna@acme.test"},
		{string(audit.EventPublished), "ben@acme.test"},
		{string(audit.EventCancelled), "anna@acme.test"},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSaveDraft_WritesAuditEntry(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	body := validBody(seeded)
	body["title"] = "Neuer Entwurf"
	rec := postDraft(t, router, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	entries := loadAuditEntries(t, db)
	if len(entries) != 1 || entries[0].Action != string(audit.EventCreated) || entries[0].OrganizationID != "kc-org-b" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if entries[0].Changes["event"].(map[string]any)["title"] != "Neuer Entwurf" {
		t.Errorf("snapshot = %v", entries[0].Changes)
	}
}

func TestEventChange_IsRolledBackWhenAuditCannotBeWritten(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	for _, stmt := range []string{
		`CREATE FUNCTION fail_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit down'; END $$`,
		`CREATE TRIGGER fail_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_audit_insert()`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS fail_audit ON audit_logs")
		db.Exec("DROP FUNCTION IF EXISTS fail_audit_insert()")
	})
	router := newEventRouter(db, principalManaging("kc-org-b"))

	if rec := putEvent(t, router, seeded.eventID.String(), validBody(seeded)); rec.Code != http.StatusInternalServerError {
		t.Fatalf("update: expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postPublish(t, router, seeded.eventID.String()); rec.Code != http.StatusInternalServerError {
		t.Fatalf("publish: expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Altes Konzert" || stored.Status != model.EventStatusDraft {
		t.Errorf("change survived the failed audit write: title=%q status=%s", stored.Title, stored.Status)
	}
}

func TestAuditedRoutes_RefuseWithoutAuthenticatedPrincipal(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, nil)

	if rec := postPublish(t, router, seeded.eventID.String()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if n := len(loadAuditEntries(t, db)); n != 0 {
		t.Errorf("rejected request wrote %d entries", n)
	}
	_ = middleware.Audit
}
