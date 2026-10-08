package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The tests in this file exercise the real handlers, services and repositories against a real
// Postgres; see testdb.Open for how to enable them.

type seededEvent struct {
	eventID    uuid.UUID
	categoryID uuid.UUID
	locationID uuid.UUID
	ownerOrgID uuid.UUID
}

func seedEventForOrg(t *testing.T, db *gorm.DB, keycloakOrgID string, status model.EventStatus) seededEvent {
	t.Helper()
	// The organization ID is derived from the Keycloak org ID so that several
	// seeded events can share one organization without breaking the unique
	// constraint on keycloak_org_id.
	s := seededEvent{
		eventID:    uuid.New(),
		categoryID: uuid.New(),
		locationID: uuid.New(),
		ownerOrgID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("org-"+keycloakOrgID)),
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO categories (category_id, category) VALUES (?, ?)", []any{s.categoryID, "cat-" + s.categoryID.String()}},
		{"INSERT INTO locations (location_id, name, city, postal_code, street) VALUES (?, ?, ?, ?, ?)", []any{s.locationID, "loc-" + s.locationID.String(), "Bonn", "53111", "Weg"}},
		{"INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?) ON CONFLICT (organization_id) DO NOTHING", []any{s.ownerOrgID, keycloakOrgID, "Org " + keycloakOrgID}},
		{`INSERT INTO events (event_id, title, start_time, end_time, capacity, status, price, category_id, organizer_id, location_id)
		  VALUES (?, ?, NOW() + interval '1 day', NOW() + interval '2 day', 100, ?, 20, ?, ?, ?)`,
			[]any{s.eventID, "Altes Konzert", string(status), s.categoryID, s.ownerOrgID, s.locationID}},
	}
	for _, st := range statements {
		if err := db.Exec(st.sql, st.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return s
}

func principalManaging(keycloakOrgIDs ...string) *middleware.Principal {
	p := &middleware.Principal{Subject: "user-1", Username: "tester"}
	for _, id := range keycloakOrgIDs {
		p.Organizations = append(p.Organizations, &middleware.OrganizationAccess{
			ID:    id,
			Alias: id,
			Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleEventManager: {}},
		})
	}
	return p
}

func newEventRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	eventService := service.NewEventService(
		repository.NewEventRepository(db),
		repository.NewOrganizationRepository(db),
		repository.NewTransactor(db),
	)
	h := NewEventHandler(eventService)

	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}

	r := gin.New()
	r.GET("/api/v1/events", h.ListPublishedEventsHandler)
	r.POST("/api/v1/events/draft", setPrincipal, middleware.Audit(audit.EventCreated), h.SaveEventAsDraftHandler)
	r.GET("/api/v1/events/:eventId", h.GetPublishedEventDetailsHandler)
	r.GET("/api/v1/events/self", setPrincipal, h.ListOwnEventsHandler)
	r.GET("/api/v1/events/org/:id", setPrincipal, h.ListOrganizationEventsHandler)
	r.PUT("/api/v1/events/:id", setPrincipal, middleware.Audit(audit.EventUpdated), h.UpdateEventHandler)
	r.POST("/api/v1/events/:id/publish", setPrincipal, middleware.Audit(audit.EventPublished), h.PublishEventHandler)
	r.POST("/api/v1/events/:id/withdraw", setPrincipal, middleware.Audit(audit.EventCancelled), h.WithdrawEventHandler)
	return r
}

func putEvent(t *testing.T, router http.Handler, eventID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/events/"+eventID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func postPublish(t *testing.T, router http.Handler, eventID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+eventID+"/publish", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func postWithdraw(t *testing.T, router http.Handler, eventID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+eventID+"/withdraw", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getOrganizationEvents(router http.Handler, orgID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/org/"+orgID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertOrganizationEventsProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	var problem model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v; body: %s", err, rec.Body.String())
	}
	if problem.Status != status || problem.Type != "about:blank" || problem.Title != http.StatusText(status) || problem.Detail == "" {
		t.Errorf("unexpected problem response: %+v", problem)
	}
}

func TestListOrganizationEventsHandler_OnlyRequestedOrganization(t *testing.T) {
	db := testdb.Open(t)
	want := map[uuid.UUID]seededEvent{}
	var orgID uuid.UUID
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		event := seedEventForOrg(t, db, "kc-org-a", status)
		want[event.eventID] = event
		orgID = event.ownerOrgID
	}
	seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	seedEventForOrg(t, db, "kc-org-foreign", model.EventStatusPublished)
	principal := &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{
		{ID: "kc-org-a", Alias: "alias-a"},
		{ID: "kc-org-b", Alias: "alias-b"},
	}}
	principal.ActiveOrganization = principal.Organizations[1]
	rec := getOrganizationEvents(newEventRouter(db, principal), orgID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var events []model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d: %s", len(want), len(events), rec.Body.String())
	}
	for _, event := range events {
		seeded, ok := want[event.EventID]
		if !ok {
			t.Fatalf("unexpected or duplicate event %s", event.EventID)
		}
		if event.OrganizerID == nil || *event.OrganizerID != seeded.ownerOrgID {
			t.Errorf("event %s has unexpected organizer %v", event.EventID, event.OrganizerID)
		}
		delete(want, event.EventID)
	}
}

func TestListOrganizationEventsHandler_EmptyArray(t *testing.T) {
	db := testdb.Open(t)
	orgID := uuid.New()
	if err := db.Exec("INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?)", orgID, "org-1", "Empty organization").Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	seedEventForOrg(t, db, "foreign-org", model.EventStatusPublished)
	principal := &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{{ID: "org-1"}}}
	rec := getOrganizationEvents(newEventRouter(db, principal), orgID.String())
	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Fatalf("expected 200 with [], got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListOrganizationEventsHandler_DatabaseFailure(t *testing.T) {
	for _, table := range []string{"organizations", "events"} {
		t.Run(table, func(t *testing.T) {
			db := testdb.Open(t)
			seeded := seedEventForOrg(t, db, "org-1", model.EventStatusPublished)
			const callback = "test:list_organization_events_failure"
			if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					_ = tx.AddError(errors.New("query failed"))
				}
			}); err != nil {
				t.Fatalf("register query failure: %v", err)
			}
			t.Cleanup(func() { db.Callback().Query().Remove(callback) })
			rec := getOrganizationEvents(newEventRouter(db, principalManaging("org-1")), seeded.ownerOrgID.String())
			assertOrganizationEventsProblem(t, rec, http.StatusInternalServerError)
		})
	}
}

func validBody(s seededEvent) map[string]any {
	return map[string]any{
		"title":      "Neues Konzert",
		"startTime":  time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"endTime":    time.Now().Add(76 * time.Hour).UTC().Format(time.RFC3339),
		"capacity":   250,
		"price":      35,
		"categoryId": s.categoryID.String(),
		"locationId": s.locationID.String(),
	}
}

func TestUpdateEventHandler_UpdatesOwnEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"))

	rec := putEvent(t, router, seeded.eventID.String(), validBody(seeded))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Title != "Neues Konzert" || stored.Capacity != 250 || !stored.Price.Equal(decimal.NewFromInt(35)) {
		t.Errorf("update not persisted: title=%q capacity=%d price=%s", stored.Title, stored.Capacity, stored.Price)
	}
	if stored.Status != model.EventStatusPublished {
		t.Errorf("status changed to %s", stored.Status)
	}
	if stored.OrganizerID == nil || *stored.OrganizerID != seeded.ownerOrgID {
		t.Errorf("organizer changed to %v", stored.OrganizerID)
	}
}

func TestUpdateEventHandler_RejectsForeignEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a"))

	rec := putEvent(t, router, seeded.eventID.String(), validBody(seeded))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != http.StatusForbidden || problem.Type != "about:blank" {
		t.Errorf("expected RFC 9457 body, got %s", rec.Body.String())
	}

	var stored model.EventModel
	db.First(&stored, "event_id = ?", seeded.eventID)
	if stored.Title != "Altes Konzert" {
		t.Errorf("foreign update was persisted: %q", stored.Title)
	}
}

func TestUpdateEventHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusCancelled)

	tests := []struct {
		name      string
		principal *middleware.Principal
		eventID   string
		body      map[string]any
		want      int
	}{
		{name: "no principal", principal: nil, eventID: seeded.eventID.String(), body: validBody(seeded), want: http.StatusUnauthorized},
		{name: "no event_manager role", principal: &middleware.Principal{Subject: "u"}, eventID: seeded.eventID.String(), body: validBody(seeded), want: http.StatusForbidden},
		{name: "invalid id", principal: principalManaging("kc-org-b"), eventID: "not-a-uuid", body: validBody(seeded), want: http.StatusBadRequest},
		{name: "invalid body", principal: principalManaging("kc-org-b"), eventID: seeded.eventID.String(), body: map[string]any{"title": "x"}, want: http.StatusBadRequest},
		{name: "unknown event", principal: principalManaging("kc-org-b"), eventID: uuid.New().String(), body: validBody(seeded), want: http.StatusNotFound},
		{name: "cancelled event", principal: principalManaging("kc-org-b"), eventID: seeded.eventID.String(), body: validBody(seeded), want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := putEvent(t, newEventRouter(db, tt.principal), tt.eventID, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPublishEventHandler_PublishesOwnDraftEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"))

	rec := postPublish(t, router, seeded.eventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response model.EventActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Message != "event published" {
		t.Errorf("unexpected response body: %s", rec.Body.String())
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusPublished {
		t.Errorf("status not persisted in database: %s", stored.Status)
	}
}

func TestPublishEventHandler_RejectsIncompleteEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	if err := db.Exec("UPDATE events SET category_id = NULL WHERE event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("break event: %v", err)
	}
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := postPublish(t, router, seeded.eventID.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var stored model.EventModel
	db.First(&stored, "event_id = ?", seeded.eventID)
	if stored.Status != model.EventStatusDraft {
		t.Errorf("rejected publish changed status to %s", stored.Status)
	}
}

func TestPublishEventHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	draft := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	published := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)

	tests := []struct {
		name      string
		principal *middleware.Principal
		eventID   string
		want      int
	}{
		{name: "no principal", principal: nil, eventID: draft.eventID.String(), want: http.StatusUnauthorized},
		{name: "no event_manager role", principal: &middleware.Principal{Subject: "u"}, eventID: draft.eventID.String(), want: http.StatusForbidden},
		{name: "foreign organization", principal: principalManaging("kc-org-a"), eventID: draft.eventID.String(), want: http.StatusForbidden},
		{name: "invalid id", principal: principalManaging("kc-org-b"), eventID: "not-a-uuid", want: http.StatusBadRequest},
		{name: "unknown event", principal: principalManaging("kc-org-b"), eventID: uuid.New().String(), want: http.StatusNotFound},
		{name: "already published", principal: principalManaging("kc-org-b"), eventID: published.eventID.String(), want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postPublish(t, newEventRouter(db, tt.principal), tt.eventID)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}

	var stored model.EventModel
	db.First(&stored, "event_id = ?", draft.eventID)
	if stored.Status != model.EventStatusDraft {
		t.Errorf("rejected publish changed status to %s", stored.Status)
	}
}

func TestWithdrawEventHandler_WithdrawsOwnPublishedEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"))

	rec := postWithdraw(t, router, seeded.eventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response model.EventWithdrawnResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Message != "event withdrawn" {
		t.Errorf("unexpected response body: %s", rec.Body.String())
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusCancelled {
		t.Errorf("status not persisted in database: %s", stored.Status)
	}
}

func TestWithdrawEventHandler_RejectsDraftEvent(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := postWithdraw(t, router, seeded.eventID.String())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var stored model.EventModel
	db.First(&stored, "event_id = ?", seeded.eventID)
	if stored.Status != model.EventStatusDraft {
		t.Errorf("rejected withdraw changed status to %s", stored.Status)
	}
}

func TestWithdrawEventHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	published := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	draft := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	cancelled := seedEventForOrg(t, db, "kc-org-b", model.EventStatusCancelled)

	tests := []struct {
		name      string
		principal *middleware.Principal
		eventID   string
		want      int
	}{
		{name: "no principal", principal: nil, eventID: published.eventID.String(), want: http.StatusUnauthorized},
		{name: "no event_manager role", principal: &middleware.Principal{Subject: "u"}, eventID: published.eventID.String(), want: http.StatusForbidden},
		{name: "foreign organization", principal: principalManaging("kc-org-a"), eventID: published.eventID.String(), want: http.StatusForbidden},
		{name: "invalid id", principal: principalManaging("kc-org-b"), eventID: "not-a-uuid", want: http.StatusBadRequest},
		{name: "unknown event", principal: principalManaging("kc-org-b"), eventID: uuid.New().String(), want: http.StatusNotFound},
		{name: "draft event", principal: principalManaging("kc-org-b"), eventID: draft.eventID.String(), want: http.StatusBadRequest},
		{name: "already cancelled", principal: principalManaging("kc-org-b"), eventID: cancelled.eventID.String(), want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postWithdraw(t, newEventRouter(db, tt.principal), tt.eventID)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}

	var stored model.EventModel
	db.First(&stored, "event_id = ?", published.eventID)
	if stored.Status != model.EventStatusPublished {
		t.Errorf("rejected withdraw changed status to %s", stored.Status)
	}
}

func runConcurrentPosts(t *testing.T, db *gorm.DB, n int, fn func() int) (ok, rejected int) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(n)
	sqlDB.SetMaxIdleConns(n)

	start := make(chan struct{})
	codes := make(chan int, n)
	var ready, wg sync.WaitGroup
	for i := 0; i < n; i++ {
		ready.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			codes <- fn()
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(codes)

	for code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusBadRequest:
			rejected++
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	return ok, rejected
}

func TestPublishEventHandler_ConcurrentPublishesSerialize(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	ok, rejected := runConcurrentPosts(t, db, 20, func() int {
		return postPublish(t, router, seeded.eventID.String()).Code
	})
	if ok != 1 {
		t.Fatalf("expected exactly 1 successful publish, got %d ok and %d rejected", ok, rejected)
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusPublished {
		t.Errorf("expected published, got %s", stored.Status)
	}
}

func TestUpdateEventHandler_ConcurrentUpdateKeepsPublishedStatus(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))
	body := validBody(seeded)
	eventID := seeded.eventID.String()

	var publishOnce sync.Once
	ok, rejected := runConcurrentPosts(t, db, 20, func() int {
		publish := false
		publishOnce.Do(func() { publish = true })
		if publish {
			return postPublish(t, router, eventID).Code
		}
		return putEvent(t, router, eventID, body).Code
	})
	if ok != 20 || rejected != 0 {
		t.Fatalf("expected 20 successful requests, got %d ok and %d rejected", ok, rejected)
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusPublished {
		t.Errorf("expected published, got %s", stored.Status)
	}
	if stored.Title != "Neues Konzert" {
		t.Errorf("expected updated title, got %q", stored.Title)
	}
}

func TestWithdrawEventHandler_ConcurrentWithdrawsSerialize(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	ok, rejected := runConcurrentPosts(t, db, 20, func() int {
		return postWithdraw(t, router, seeded.eventID.String()).Code
	})
	if ok != 1 {
		t.Fatalf("expected exactly 1 successful withdraw, got %d ok and %d rejected", ok, rejected)
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusCancelled {
		t.Errorf("expected cancelled, got %s", stored.Status)
	}
}
