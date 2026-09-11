package service

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

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

// Laeuft gegen eine echte Postgres, weil der Entwurfs-Status vom
// Postgres-ENUM event_status abhaengt. Aktivieren mit:
// TEST_DATABASE_DSN=postgres://eventhub:eventhub@localhost:5432/eventhub?sslmode=disable
func setupEventTestDB(t *testing.T) *gorm.DB {
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

	// Jeder Test startet auf leeren Tabellen.
	err = db.Exec("TRUNCATE bookings, events, organizations, users, categories, locations CASCADE").Error
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

// seedOrganization legt eine Organisation samt Kategorie und Ort an.
func seedOrganization(t *testing.T, db *gorm.DB) (orgID, categoryID, locationID uuid.UUID, keycloakOrgID string) {
	t.Helper()
	orgID, categoryID, locationID = uuid.New(), uuid.New(), uuid.New()
	keycloakOrgID = "kc-org-" + orgID.String()

	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO categories (category_id, category) VALUES (?, ?)",
			[]any{categoryID, "cat-" + categoryID.String()}},
		{"INSERT INTO locations (location_id, name, city, postal_code, street) VALUES (?, ?, ?, ?, ?)",
			[]any{locationID, "loc-" + locationID.String(), "Bonn", "53111", "Weg"}},
		{"INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?)",
			[]any{orgID, keycloakOrgID, "Org " + orgID.String()}},
	}
	for _, st := range statements {
		if err := db.Exec(st.sql, st.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return orgID, categoryID, locationID, keycloakOrgID
}

// seedEvent legt eine Veranstaltung mit frei waehlbarem Status an.
func seedEventWithStatus(t *testing.T, db *gorm.DB, orgID, categoryID, locationID uuid.UUID, status model.EventStatus) uuid.UUID {
	t.Helper()
	eventID := uuid.New()
	err := db.Exec(`INSERT INTO events
		(event_id, title, start_time, end_time, capacity, status, price, category_id, organizer_id, location_id)
		VALUES (?, ?, NOW() + interval '2 day', NOW() + interval '3 day', 100, ?, 10, ?, ?, ?)`,
		eventID, "Seed "+eventID.String(), string(status), categoryID, orgID, locationID).Error
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return eventID
}

func newEventService(db *gorm.DB) *EventService {
	return NewEventService(repository.NewEventRepository(db), repository.NewOrganizationRepository(db))
}

func draftRequest(categoryID, locationID uuid.UUID) model.CreateDraftRequest {
	return model.CreateDraftRequest{
		Title:      "Sommerfest 2026",
		StartTime:  time.Now().Add(48 * time.Hour),
		EndTime:    time.Now().Add(52 * time.Hour),
		Capacity:   100,
		Price:      decimal.NewFromInt(15),
		CategoryID: categoryID,
		LocationID: locationID,
	}
}

// --- AK1: Eine Veranstaltung kann im Status Entwurf gespeichert werden ------

func TestCreateDraft_SavesWithStatusDraft(t *testing.T) {
	db := setupEventTestDB(t)
	orgID, categoryID, locationID, keycloakOrgID := seedOrganization(t, db)
	svc := newEventService(db)

	created, err := svc.CreateDraft(keycloakOrgID, draftRequest(categoryID, locationID))
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if created.Status != model.EventStatusDraft {
		t.Errorf("Status = %q, erwartet %q", created.Status, model.EventStatusDraft)
	}
	if created.EventID == uuid.Nil {
		t.Error("EventID wurde nicht gesetzt - die Migration hat kein DEFAULT auf event_id")
	}
	if created.OrganizerID == nil || *created.OrganizerID != orgID {
		t.Errorf("OrganizerID = %v, erwartet %v", created.OrganizerID, orgID)
	}

	// Gegenprobe direkt in der Datenbank, nicht nur im Rueckgabewert.
	var status string
	err = db.Raw("SELECT status FROM events WHERE event_id = ?", created.EventID).Scan(&status).Error
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != string(model.EventStatusDraft) {
		t.Errorf("persistierter Status = %q, erwartet draft", status)
	}
}

func TestCreateDraft_UnknownOrganization_ReturnsForbidden(t *testing.T) {
	db := setupEventTestDB(t)
	_, categoryID, locationID, _ := seedOrganization(t, db)
	svc := newEventService(db)

	_, err := svc.CreateDraft("kc-org-gibt-es-nicht", draftRequest(categoryID, locationID))
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("Fehler = %v, erwartet ErrForbidden", err)
	}

	var count int64
	db.Raw("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 0 {
		t.Errorf("es wurden %d Events angelegt, erwartet 0", count)
	}
}

// --- AK3: Der Veranstalter kann eigene Entwuerfe sehen ---------------------

func TestListOwnEventsByStatus_ReturnsOnlyOwnDrafts(t *testing.T) {
	db := setupEventTestDB(t)
	orgID, categoryID, locationID, keycloakOrgID := seedOrganization(t, db)
	foreignOrgID, foreignCat, foreignLoc, _ := seedOrganization(t, db)
	svc := newEventService(db)

	ownDraft := seedEventWithStatus(t, db, orgID, categoryID, locationID, model.EventStatusDraft)
	seedEventWithStatus(t, db, orgID, categoryID, locationID, model.EventStatusPublished)
	foreignDraft := seedEventWithStatus(t, db, foreignOrgID, foreignCat, foreignLoc, model.EventStatusDraft)

	events, err := svc.ListOwnEventsByStatus(keycloakOrgID, model.EventStatusDraft)
	if err != nil {
		t.Fatalf("ListOwnEventsByStatus: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, erwartet 1", len(events))
	}
	if events[0].EventID != ownDraft {
		t.Errorf("EventID = %v, erwartet %v", events[0].EventID, ownDraft)
	}
	for _, e := range events {
		if e.EventID == foreignDraft {
			t.Fatal("fremder Entwurf ist in der eigenen Liste aufgetaucht")
		}
	}
}

// --- AK2 (Vorleistung): Entwuerfe tauchen im Status-Filter nicht auf -------

func TestListByStatus_ExcludesDrafts(t *testing.T) {
	db := setupEventTestDB(t)
	orgID, categoryID, locationID, _ := seedOrganization(t, db)
	repo := repository.NewEventRepository(db)

	draft := seedEventWithStatus(t, db, orgID, categoryID, locationID, model.EventStatusDraft)
	published := seedEventWithStatus(t, db, orgID, categoryID, locationID, model.EventStatusPublished)

	events, err := repo.ListByStatus(model.EventStatusPublished)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}

	if len(events) != 1 || events[0].EventID != published {
		t.Fatalf("erwartet genau die veroeffentlichte Veranstaltung, bekommen: %d", len(events))
	}
	for _, e := range events {
		if e.EventID == draft {
			t.Fatal("ein Entwurf wurde von ListByStatus(published) geliefert")
		}
	}
}
