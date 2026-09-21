package handler

import (
	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercises the real handler, service and repositories against a real Postgres.
// Set TEST_DATABASE_DSN to enable, e.g.:
// postgres://postgres:test@localhost:5599/postgres?sslmode=disable
func setupHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	driver, err := migratePostgres.WithInstance(sqlDB, &migratePostgres.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	sqlDB.Close()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	t.Cleanup(func() {
		if gormDB, err := db.DB(); err == nil {
			gormDB.Close()
		}
	})
	if err := db.Exec("TRUNCATE bookings, events, organizations, users, categories, locations CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

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

func newEventRouter(db *gorm.DB, principal *middleware.Principal, t *testing.T, fakes ...*keycloakmock.Fake) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eventService := service.NewEventService(repository.NewEventRepository(db), repository.NewOrganizationRepository(db))
	fake := keycloakmock.New()
	if len(fakes) > 0 {
		fake = fakes[0]
	}
	kc := fake.KeycloakService(t)
	h := NewEventHandler(eventService, kc)

	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}

	r := gin.New()
	r.GET("/api/v1/events/self", setPrincipal, h.ListOwnEventsHandler)
	r.PUT("/api/v1/events/:id", setPrincipal, h.UpdateEventHandler)
	r.POST("/api/v1/events/:id/publish", setPrincipal, h.PublishEventHandler)
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

func getOwnEvents(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/self", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertOwnEventsProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) {
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

func TestListOwnEventsHandler_AggregatesManagedOrganizations(t *testing.T) {
	db := setupHandlerDB(t)
	want := map[uuid.UUID]seededEvent{}
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		event := seedEventForOrg(t, db, "kc-org-a", status)
		want[event.eventID] = event
	}
	event := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	want[event.eventID] = event
	seedEventForOrg(t, db, "kc-org-finance", model.EventStatusPublished)
	seedEventForOrg(t, db, "kc-org-foreign", model.EventStatusPublished)

	fake := keycloakmock.New()
	fake.OrgAliases = map[string]string{"alias-a": "kc-org-a", "alias-b": "kc-org-b", "alias-finance": "kc-org-finance"}
	principal := principalManaging("alias-a", "alias-b")
	principal.AccessToken = "svc-token"
	principal.ActiveOrganization = principal.Organizations[0]
	principal.Organizations = append(principal.Organizations, &middleware.OrganizationAccess{
		ID: "alias-finance", Alias: "alias-finance",
		Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleFinanceViewer: {}},
	})

	rec := getOwnEvents(newEventRouter(db, principal, t, fake))
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

func TestListOwnEventsHandler_EmptyArray(t *testing.T) {
	db := setupHandlerDB(t)
	if err := db.Exec("INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?)", uuid.New(), "org-1", "Empty organization").Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	seedEventForOrg(t, db, "foreign-org", model.EventStatusPublished)
	principal := principalManaging("alias-1")
	principal.AccessToken = "svc-token"
	rec := getOwnEvents(newEventRouter(db, principal, t))
	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Fatalf("expected 200 with [], got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListOwnEventsHandler_AccessChecks(t *testing.T) {
	tests := []struct {
		name      string
		principal *middleware.Principal
		want      int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized},
		{name: "no organization role", principal: &middleware.Principal{Subject: "u"}, want: http.StatusForbidden},
		{name: "finance role only", principal: &middleware.Principal{Subject: "u", Organizations: []*middleware.OrganizationAccess{
			{ID: "alias-1", Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleFinanceViewer: {}}},
		}}, want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := keycloakmock.New()
			rec := getOwnEvents(newEventRouter(nil, tt.principal, t, fake))
			assertOwnEventsProblem(t, rec, tt.want)
			fake.Mu.Lock()
			defer fake.Mu.Unlock()
			if len(fake.AdminRequests) != 0 || fake.TokenRequests != 0 {
				t.Error("access rejection should not contact Keycloak")
			}
		})
	}
}

func TestListOwnEventsHandler_KeycloakFailure(t *testing.T) {
	fake := keycloakmock.New()
	fake.FailAdminRequest = func(method, path string) bool { return true }
	principal := principalManaging("alias-1")
	principal.AccessToken = "svc-token"
	rec := getOwnEvents(newEventRouter(nil, principal, t, fake))
	assertOwnEventsProblem(t, rec, http.StatusInternalServerError)
}

func TestListOwnEventsHandler_DoesNotReturnPartialResults(t *testing.T) {
	db := setupHandlerDB(t)
	seedEventForOrg(t, db, "org-1", model.EventStatusPublished)
	fake := keycloakmock.New()
	fake.OrgAliases["alias-2"] = "org-2"
	requests := 0
	fake.FailAdminRequest = func(method, path string) bool {
		requests++
		return requests > 1
	}
	principal := principalManaging("alias-1", "alias-2")
	principal.AccessToken = "svc-token"
	rec := getOwnEvents(newEventRouter(db, principal, t, fake))
	assertOwnEventsProblem(t, rec, http.StatusInternalServerError)
	fake.Mu.Lock()
	defer fake.Mu.Unlock()
	if requests < 2 {
		t.Fatal("expected lookup of the second organization after loading the first")
	}
}

func TestListOwnEventsHandler_DatabaseFailure(t *testing.T) {
	db := setupHandlerDB(t)
	seedEventForOrg(t, db, "org-1", model.EventStatusPublished)
	// Fail only the event query, after the organization lookup succeeds.
	const callback = "test:list_own_events_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "events" {
			err := tx.AddError(errors.New("event query failed"))
			if err != nil {
				t.Fatalf("issue mocking callback: %v", err)
			}
		}
	}); err != nil {
		t.Fatalf("register query failure: %v", err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove(callback) })
	principal := principalManaging("alias-1")
	principal.AccessToken = "svc-token"
	rec := getOwnEvents(newEventRouter(db, principal, t))
	assertOwnEventsProblem(t, rec, http.StatusInternalServerError)
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
	db := setupHandlerDB(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"), t)

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
	db := setupHandlerDB(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a"), t)

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
	db := setupHandlerDB(t)
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
			rec := putEvent(t, newEventRouter(db, tt.principal, t), tt.eventID, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPublishEventHandler_PublishesOwnDraftEvent(t *testing.T) {
	db := setupHandlerDB(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"), t)

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
	db := setupHandlerDB(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	if err := db.Exec("UPDATE events SET category_id = NULL WHERE event_id = ?", seeded.eventID).Error; err != nil {
		t.Fatalf("break event: %v", err)
	}
	router := newEventRouter(db, principalManaging("kc-org-b"), t)

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
	db := setupHandlerDB(t)
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
			rec := postPublish(t, newEventRouter(db, tt.principal, t), tt.eventID)
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
