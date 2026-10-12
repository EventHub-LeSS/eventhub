package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EVENTHUB-256: Veranstaltungsentwurf löschen. The tests in this file run the real handler, service
// and repositories against a real Postgres; see testdb.Open for how to enable them.

// eventRowCount counts the rows of the event without any GORM scope, so a soft delete would show.
func eventRowCount(t *testing.T, db *gorm.DB, eventID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := db.Unscoped().Model(&model.EventModel{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		t.Fatalf("count event: %v", err)
	}
	return count
}

func wantEventStatus(t *testing.T, db *gorm.DB, eventID uuid.UUID, status model.EventStatus) {
	t.Helper()
	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", eventID).Error; err != nil {
		t.Fatalf("event %s is gone: %v", eventID, err)
	}
	if stored.Status != status {
		t.Errorf("status = %s, want %s", stored.Status, status)
	}
}

// seedBookingFor books the event for a new visitor, together with the payment of the booking.
func seedBookingFor(t *testing.T, db *gorm.DB, eventID uuid.UUID, status model.BookingStatus) (bookingID, paymentID uuid.UUID) {
	t.Helper()
	userID := seedBookingUser(t, db, "kc-"+uuid.NewString())
	bookingID, paymentID = uuid.New(), uuid.New()
	if err := db.Exec("INSERT INTO payments (payment_id, amount, status) VALUES (?, 40, 'paid')", paymentID).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	err := db.Exec(`INSERT INTO bookings (booking_id, user_id, event_id, payment_id, number_of_tickets, status, expires_at)
		VALUES (?, ?, ?, ?, 2, ?, NOW() + interval '15 minutes')`, bookingID, userID, eventID, paymentID, string(status)).Error
	if err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	return bookingID, paymentID
}

// wantBookingIntact checks that the booking still references the event and its payment still exists.
func wantBookingIntact(t *testing.T, db *gorm.DB, bookingID, eventID, paymentID uuid.UUID) {
	t.Helper()
	var booking model.BookingModel
	if err := db.First(&booking, "booking_id = ?", bookingID).Error; err != nil {
		t.Fatalf("booking %s is gone: %v", bookingID, err)
	}
	if booking.EventID == nil || *booking.EventID != eventID {
		t.Errorf("booking references event %v, want %s", booking.EventID, eventID)
	}
	var payments int64
	if err := db.Raw("SELECT count(*) FROM payments WHERE payment_id = ?", paymentID).Scan(&payments).Error; err != nil || payments != 1 {
		t.Errorf("payment %s is gone (count %d, err %v)", paymentID, payments, err)
	}
}

func auditActions(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var actions []string
	for _, e := range loadAuditEntries(t, db) {
		actions = append(actions, e.Action)
	}
	return actions
}

// EVENTHUB-256 / AK1, AK4, T1, T4: Der Entwurf ist danach vollständig aus der Datenbank entfernt,
// Kategorie, Ort und Organisation bleiben.
func TestDeleteEventHandler_OwnDraft_RemovesOnlyTheEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	other := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"))

	wantEventDeleted(t, deleteEvent(router, seeded.eventID.String()))

	if n := eventRowCount(t, db, seeded.eventID); n != 0 {
		t.Errorf("event row still exists (unscoped count %d)", n)
	}
	var raw int64
	if err := db.Raw("SELECT count(*) FROM events WHERE event_id = ?", seeded.eventID).Scan(&raw).Error; err != nil || raw != 0 {
		t.Errorf("SQL still finds the event: count %d, err %v", raw, err)
	}
	wantEventStatus(t, db, other.eventID, model.EventStatusDraft)

	masterData := []struct {
		query string
		id    uuid.UUID
	}{
		{"SELECT count(*) FROM categories WHERE category_id = ?", seeded.categoryID},
		{"SELECT count(*) FROM locations WHERE location_id = ?", seeded.locationID},
		{"SELECT count(*) FROM organizations WHERE organization_id = ?", seeded.ownerOrgID},
	}
	for _, m := range masterData {
		var count int64
		if err := db.Raw(m.query, m.id).Scan(&count).Error; err != nil || count != 1 {
			t.Errorf("%s: count %d, err %v; master data must be kept", m.query, count, err)
		}
	}

	entries := loadAuditEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Action != string(audit.EventDeleted) || e.ResourceID != seeded.eventID.String() || e.OrganizationID != "kc-org-b" ||
		e.ActorSubject != "user-1" || e.ActorUsername != "tester" || e.Phase != audit.PhaseCompleted {
		t.Errorf("unexpected audit entry: %+v", e)
	}
	if snapshot := e.Changes["event"].(map[string]any); snapshot["title"] != "Altes Konzert" || snapshot["status"] != "draft" {
		t.Errorf("snapshot = %v", snapshot)
	}
}

// EVENTHUB-256 / AK1, AK4: Ein angelegter Entwurf verschwindet nach dem Löschen aus den eigenen
// Entwürfen; die Audit-Einträge bleiben erhalten.
func TestDraftLifecycle_CreateAndDelete_KeepsAuditTrail(t *testing.T) {
	db := testdb.Open(t)
	references := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := postDraft(t, router, draftBody(references))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	ownDraftIDs := func() []uuid.UUID {
		rec := getOwnEvents(router, "?status=draft")
		if rec.Code != http.StatusOK {
			t.Fatalf("list drafts: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var ids []uuid.UUID
		for _, e := range decodeEvents(t, rec) {
			ids = append(ids, e.EventID)
		}
		return ids
	}
	if !slices.Contains(ownDraftIDs(), created.EventID) {
		t.Fatal("new draft is not listed")
	}

	wantEventDeleted(t, deleteEvent(router, created.EventID.String()))

	if slices.Contains(ownDraftIDs(), created.EventID) {
		t.Error("deleted draft is still listed")
	}
	var actions []string
	for _, e := range loadAuditEntries(t, db) {
		if e.ResourceID == created.EventID.String() {
			actions = append(actions, e.Action)
		}
	}
	if !slices.Equal(actions, []string{string(audit.EventCreated), string(audit.EventDeleted)}) {
		t.Errorf("audit trail of the draft = %v, want created and deleted", actions)
	}
}

// EVENTHUB-256 / AK2, T2: Veröffentlichte, abgesagte und abgeschlossene Events bleiben unverändert.
func TestDeleteEventHandler_NotDraft_BadRequest(t *testing.T) {
	db := testdb.Open(t)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	for _, status := range []model.EventStatus{model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			seeded := seedEventForOrg(t, db, "kc-org-b", status)

			wantProblemDetail(t, deleteEvent(router, seeded.eventID.String()), http.StatusBadRequest, "only draft events can be deleted")
			wantEventStatus(t, db, seeded.eventID, status)
		})
	}
	if actions := auditActions(t, db); len(actions) != 0 {
		t.Errorf("rejected deletions wrote audit entries: %v", actions)
	}
}

// EVENTHUB-256 / T2: Ein Entwurf mit Buchungen in irgendeinem Status wird nicht gelöscht; Buchung
// und Zahlung bleiben am Event.
func TestDeleteEventHandler_DraftWithBookings_Conflict(t *testing.T) {
	db := testdb.Open(t)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	statuses := []model.BookingStatus{
		model.BookingStatusReserved, model.BookingStatusConfirmed, model.BookingStatusCancelled,
		model.BookingStatusFailed, model.BookingStatusExpired,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
			bookingID, paymentID := seedBookingFor(t, db, seeded.eventID, status)

			wantProblemDetail(t, deleteEvent(router, seeded.eventID.String()), http.StatusConflict, "events with bookings cannot be deleted")
			wantEventStatus(t, db, seeded.eventID, model.EventStatusDraft)
			wantBookingIntact(t, db, bookingID, seeded.eventID, paymentID)
		})
	}
	if actions := auditActions(t, db); len(actions) != 0 {
		t.Errorf("rejected deletions wrote audit entries: %v", actions)
	}
}

// EVENTHUB-256 / T2: Ein zurückgezogenes Event mit Buchungen bleibt erhalten.
func TestDeleteEventHandler_WithdrawnEventWithBookings_BadRequest(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	bookingID, paymentID := seedBookingFor(t, db, seeded.eventID, model.BookingStatusConfirmed)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	if rec := postWithdraw(t, router, seeded.eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	wantProblemDetail(t, deleteEvent(router, seeded.eventID.String()), http.StatusBadRequest, "only draft events can be deleted")
	wantEventStatus(t, db, seeded.eventID, model.EventStatusCancelled)
	wantBookingIntact(t, db, bookingID, seeded.eventID, paymentID)
}

// EVENTHUB-256 / AK3, T3: Ohne passende Rolle in der besitzenden Organisation bleibt das Event.
func TestDeleteEventHandler_StatusCodes_EventKept(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)

	tests := []struct {
		name      string
		principal *middleware.Principal
		eventID   string
		want      int
		detail    string
	}{
		{name: "no principal", eventID: seeded.eventID.String(), want: http.StatusUnauthorized, detail: "authentication is required"},
		{name: "visitor without organization", principal: &middleware.Principal{Subject: "visitor"}, eventID: seeded.eventID.String(), want: http.StatusForbidden, detail: "missing organization role"},
		{name: "finance_viewer of the owner", principal: principalWithRole(middleware.RoleFinanceViewer, "kc-org-b"), eventID: seeded.eventID.String(), want: http.StatusForbidden, detail: "missing organization role"},
		{name: "event_manager of another organization", principal: principalManaging("kc-org-a"), eventID: seeded.eventID.String(), want: http.StatusForbidden, detail: "forbidden"},
		{name: "org_admin of another organization", principal: principalWithRole(middleware.RoleOrganizationAdmin, "kc-org-a"), eventID: seeded.eventID.String(), want: http.StatusForbidden, detail: "forbidden"},
		{name: "invalid id", principal: principalManaging("kc-org-b"), eventID: "not-a-uuid", want: http.StatusBadRequest, detail: "invalid event id"},
		{name: "unknown event", principal: principalManaging("kc-org-b"), eventID: uuid.New().String(), want: http.StatusNotFound, detail: "event not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantProblemDetail(t, deleteEvent(newEventRouter(db, tt.principal), tt.eventID), tt.want, tt.detail)
		})
	}

	wantEventStatus(t, db, seeded.eventID, model.EventStatusDraft)
	if actions := auditActions(t, db); len(actions) != 0 {
		t.Errorf("rejected deletions wrote audit entries: %v", actions)
	}
}

// EVENTHUB-256 / T3: Der org_admin löscht Entwürfe seiner Organisation, wie max.multi@eventhub.de
// (org_admin in der einen, event_manager in der anderen Organisation).
func TestDeleteEventHandler_OrganizationAdmin_DeletesOwnDrafts(t *testing.T) {
	db := testdb.Open(t)
	adminDraft := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	managerDraft := seedEventForOrg(t, db, "kc-org-a", model.EventStatusDraft)
	foreignDraft := seedEventForOrg(t, db, "kc-org-c", model.EventStatusDraft)
	multi := principalWithRole(middleware.RoleOrganizationAdmin, "kc-org-b")
	multi.Organizations = append(multi.Organizations, principalManaging("kc-org-a").Organizations...)
	router := newEventRouter(db, multi)

	wantEventDeleted(t, deleteEvent(router, adminDraft.eventID.String()))
	wantEventDeleted(t, deleteEvent(router, managerDraft.eventID.String()))
	wantProblemDetail(t, deleteEvent(router, foreignDraft.eventID.String()), http.StatusForbidden, "forbidden")

	if eventRowCount(t, db, adminDraft.eventID) != 0 || eventRowCount(t, db, managerDraft.eventID) != 0 {
		t.Error("own drafts were not deleted")
	}
	wantEventStatus(t, db, foreignDraft.eventID, model.EventStatusDraft)
	var orgs []string
	for _, e := range loadAuditEntries(t, db) {
		orgs = append(orgs, e.OrganizationID)
	}
	if !slices.Equal(orgs, []string{"kc-org-b", "kc-org-a"}) {
		t.Errorf("audit entries recorded for %v, want kc-org-b and kc-org-a", orgs)
	}
}

func TestDeleteEventHandler_SecondDelete_NotFound(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	wantEventDeleted(t, deleteEvent(router, seeded.eventID.String()))
	wantProblemDetail(t, deleteEvent(router, seeded.eventID.String()), http.StatusNotFound, "event not found")

	if actions := auditActions(t, db); len(actions) != 1 {
		t.Errorf("expected one audit entry, got %v", actions)
	}
}

// EVENTHUB-256: Kann der Audit-Eintrag nicht geschrieben werden, wird das Löschen zurückgerollt.
func TestDeleteEventHandler_AuditUnavailable_RollsBack(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	for _, stmt := range []string{
		`CREATE FUNCTION fail_delete_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit down'; END $$`,
		`CREATE TRIGGER fail_delete_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_delete_audit_insert()`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS fail_delete_audit ON audit_logs")
		db.Exec("DROP FUNCTION IF EXISTS fail_delete_audit_insert()")
	})

	rec := deleteEvent(newEventRouter(db, principalManaging("kc-org-b")), seeded.eventID.String())

	wantProblemDetail(t, rec, http.StatusInternalServerError, "internal error")
	wantEventStatus(t, db, seeded.eventID, model.EventStatusDraft)
}

// pauseEventWrite holds the first UPDATE or DELETE ("update", "delete") on the events table until
// release is called. The statement runs in the transaction of a request, which keeps its row lock on
// the event while it is held. reached is closed once the statement is held.
func pauseEventWrite(t *testing.T, db *gorm.DB, kind string) (reached <-chan struct{}, release func()) {
	t.Helper()
	reachedCh, releaseCh := make(chan struct{}), make(chan struct{})
	var holdOnce, releaseOnce sync.Once
	release = func() { releaseOnce.Do(func() { close(releaseCh) }) }
	hold := func(tx *gorm.DB) {
		if tx.Statement.Table != "events" {
			return
		}
		holdOnce.Do(func() {
			close(reachedCh)
			<-releaseCh
		})
	}

	name := "test:pause_event_" + kind
	var err error
	switch kind {
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register(name, hold)
	case "delete":
		err = db.Callback().Delete().Before("gorm:delete").Register(name, hold)
	default:
		t.Fatalf("unknown statement kind %q", kind)
	}
	if err != nil {
		t.Fatalf("register pause: %v", err)
	}
	t.Cleanup(func() {
		release()
		if kind == "update" {
			db.Callback().Update().Remove(name)
		} else {
			db.Callback().Delete().Remove(name)
		}
	})
	return reachedCh, release
}

// waitForRowLockWaiter returns once a statement on the events table waits for a lock, i.e. the
// second request is queued behind the paused one. It polls only to observe that state; the order of
// the two transactions is fixed by the lock, not by timing.
func waitForRowLockWaiter(t *testing.T, db *gorm.DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int64
		err := db.Raw(`SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock' AND query LIKE '%events%'`).Scan(&waiting).Error
		if err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the second request never waited for the lock on the event")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func receive(t *testing.T, ch <-chan *httptest.ResponseRecorder, what string) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-ch:
		return rec
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not finish", what)
		return nil
	}
}

func waitUntilClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s", what)
	}
}

// EVENTHUB-256: Hält das Löschen die Zeilensperre, wartet ein paralleles Veröffentlichen und findet
// danach kein Event mehr; ein gelöschter Entwurf kann nicht mehr veröffentlicht werden.
func TestDeleteEventHandler_DeleteHoldsRowLock_PublishNotFound(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	reached, release := pauseEventWrite(t, db, "delete")

	deleted := make(chan *httptest.ResponseRecorder, 1)
	go func() { deleted <- deleteEvent(router, seeded.eventID.String()) }()
	waitUntilClosed(t, reached, "the deletion never reached its DELETE statement")

	published := make(chan *httptest.ResponseRecorder, 1)
	go func() { published <- postPublish(t, router, seeded.eventID.String()) }()
	waitForRowLockWaiter(t, db)
	release()

	wantEventDeleted(t, receive(t, deleted, "delete"))
	wantProblemDetail(t, receive(t, published, "publish"), http.StatusNotFound, "event not found")
	if n := eventRowCount(t, db, seeded.eventID); n != 0 {
		t.Errorf("event row still exists (count %d)", n)
	}
	if actions := auditActions(t, db); !slices.Equal(actions, []string{string(audit.EventDeleted)}) {
		t.Errorf("audit entries = %v, want only the deletion", actions)
	}
}

// EVENTHUB-256: Hält das Veröffentlichen die Zeilensperre, wartet das Löschen und lehnt danach ab;
// ein veröffentlichtes Event wird nie gelöscht, auch nicht im Wettlauf.
func TestDeleteEventHandler_PublishHoldsRowLock_DeleteBadRequest(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	reached, release := pauseEventWrite(t, db, "update")

	published := make(chan *httptest.ResponseRecorder, 1)
	go func() { published <- postPublish(t, router, seeded.eventID.String()) }()
	waitUntilClosed(t, reached, "the publication never reached its UPDATE statement")

	deleted := make(chan *httptest.ResponseRecorder, 1)
	go func() { deleted <- deleteEvent(router, seeded.eventID.String()) }()
	waitForRowLockWaiter(t, db)
	release()

	if rec := receive(t, published, "publish"); rec.Code != http.StatusOK {
		t.Fatalf("publish: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	wantProblemDetail(t, receive(t, deleted, "delete"), http.StatusBadRequest, "only draft events can be deleted")
	wantEventStatus(t, db, seeded.eventID, model.EventStatusPublished)
	if actions := auditActions(t, db); !slices.Equal(actions, []string{string(audit.EventPublished)}) {
		t.Errorf("audit entries = %v, want only the publication", actions)
	}
}
