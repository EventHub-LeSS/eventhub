package handler

import (
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
	s := seededEvent{eventID: uuid.New(), categoryID: uuid.New(), locationID: uuid.New(), ownerOrgID: uuid.New()}
	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO categories (category_id, category) VALUES (?, ?)", []any{s.categoryID, "cat-" + s.categoryID.String()}},
		{"INSERT INTO locations (location_id, name, city, postal_code, street) VALUES (?, ?, ?, ?, ?)", []any{s.locationID, "loc-" + s.locationID.String(), "Bonn", "53111", "Weg"}},
		{"INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?)", []any{s.ownerOrgID, keycloakOrgID, "Org " + keycloakOrgID}},
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
	eventService := service.NewEventService(repository.NewEventRepository(db), repository.NewOrganizationRepository(db))
	h := NewEventHandler(eventService)

	r := gin.New()
	r.PUT("/api/v1/events/:id", func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}, h.UpdateEventHandler)
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
	db := setupHandlerDB(t)
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
			rec := putEvent(t, newEventRouter(db, tt.principal), tt.eventID, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}
