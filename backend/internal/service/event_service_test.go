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

type fakeEventRepo struct {
	event *model.EventModel
	saved *model.EventModel
}

func (f *fakeEventRepo) CreateEvent(*model.EventModel) error {
	return nil
}

func (f *fakeEventRepo) GetEventByID(uuid.UUID) (*model.EventModel, error) {
	return f.event, nil
}

func (f *fakeEventRepo) GetAllEvents() ([]*model.EventModel, error) {
	return nil, nil
}

func (f *fakeEventRepo) DeleteEvent(uuid.UUID) error {
	return nil
}

func (f *fakeEventRepo) ListByOrganization(uuid.UUID) ([]*model.EventModel, error) {
	return nil, nil
}

func (f *fakeEventRepo) GetConfirmedTicketCount(uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeEventRepo) UpdateEvent(event *model.EventModel) error {
	f.saved = event
	return nil
}

func (f *fakeEventRepo) ListByStatus(model.EventStatus) ([]*model.EventModel, error) {
	return nil, nil
}

func (f *fakeEventRepo) ListByOrganizerAndStatus(uuid.UUID, model.EventStatus) ([]*model.EventModel, error) {
	return nil, nil
}

type fakeOrgRepo struct {
	orgs map[uuid.UUID]*model.OrganizationModel
}

func (f *fakeOrgRepo) CreateOrganization(*model.OrganizationModel) error {
	return nil
}

func (f *fakeOrgRepo) AddMembership(*model.OrganizationMembershipModel) error {
	return nil
}

func (f *fakeOrgRepo) GetByKeycloakOrgID(string) (*model.OrganizationModel, error) {
	return nil, nil
}

func (f *fakeOrgRepo) GetByID(id uuid.UUID) (*model.OrganizationModel, error) {
	return f.orgs[id], nil
}

var (
	orgA = &model.OrganizationModel{
		OrganizationID: uuid.New(),
		KeycloakOrgID:  "kc-org-a",
	}
	orgB = &model.OrganizationModel{
		OrganizationID: uuid.New(),
		KeycloakOrgID:  "kc-org-b",
	}
)

func eventOwnedBy(org *model.OrganizationModel, status model.EventStatus) *model.EventModel {
	category, location := uuid.New(), uuid.New()

	var organizer *uuid.UUID
	if org != nil {
		id := org.OrganizationID
		organizer = &id
	}

	return &model.EventModel{
		EventID:     uuid.New(),
		Title:       "Altes Konzert",
		StartTime:   time.Now().Add(48 * time.Hour),
		EndTime:     time.Now().Add(52 * time.Hour),
		Capacity:    100,
		Status:      status,
		Price:       decimal.NewFromInt(20),
		CategoryID:  &category,
		OrganizerID: organizer,
		LocationID:  &location,
	}
}

func updateRequest() model.UpdateEventRequest {
	return model.UpdateEventRequest{
		Title:      "Neues Konzert",
		StartTime:  time.Now().Add(72 * time.Hour),
		EndTime:    time.Now().Add(76 * time.Hour),
		Capacity:   250,
		Price:      decimal.NewFromInt(35),
		CategoryID: uuid.New(),
		LocationID: uuid.New(),
	}
}

func newTestService(event *model.EventModel) (*EventService, *fakeEventRepo) {
	eventRepo := &fakeEventRepo{event: event}
	orgRepo := &fakeOrgRepo{
		orgs: map[uuid.UUID]*model.OrganizationModel{
			orgA.OrganizationID: orgA,
			orgB.OrganizationID: orgB,
		},
	}

	return NewEventService(eventRepo, orgRepo), eventRepo
}

func TestUpdateEvent_Ownership(t *testing.T) {
	tests := []struct {
		name        string
		event       *model.EventModel
		managedOrgs []string
		wantErr     error
	}{
		{
			name:        "single organization",
			event:       eventOwnedBy(orgB, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-b"},
		},
		{
			name:        "member of several organizations",
			event:       eventOwnedBy(orgB, model.EventStatusPublished),
			managedOrgs: []string{"kc-org-a", "kc-org-b"},
		},
		{
			name:        "event belongs to another organization",
			event:       eventOwnedBy(orgB, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-a"},
			wantErr:     ErrForbidden,
		},
		{
			name:    "no managed organizations",
			event:   eventOwnedBy(orgB, model.EventStatusDraft),
			wantErr: ErrForbidden,
		},
		{
			name:        "event without organizer",
			event:       eventOwnedBy(nil, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrForbidden,
		},
		{
			name:        "unknown event",
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrEventNotFound,
		},
		{
			name:        "cancelled event",
			event:       eventOwnedBy(orgB, model.EventStatusCancelled),
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrInvalidStatus,
		},
		{
			name:        "completed event",
			event:       eventOwnedBy(orgB, model.EventStatusCompleted),
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrInvalidStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(tt.event)

			_, err := svc.UpdateEvent(uuid.New(), tt.managedOrgs, updateRequest())

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil && repo.saved != nil {
				t.Fatal("rejected update must not be saved")
			}
			if tt.wantErr == nil && repo.saved == nil {
				t.Fatal("accepted update was not saved")
			}
		})
	}
}

func TestUpdateEvent_KeepsStatusAndOrganizer(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusPublished)
	svc, _ := newTestService(event)

	updated, err := svc.UpdateEvent(event.EventID, []string{"kc-org-b"}, updateRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != model.EventStatusPublished {
		t.Errorf("status changed to %s", updated.Status)
	}
	if updated.OrganizerID == nil || *updated.OrganizerID != orgB.OrganizationID {
		t.Errorf("organizer changed to %v", updated.OrganizerID)
	}
	if updated.Title != "Neues Konzert" || updated.Capacity != 250 {
		t.Errorf("fields not applied: title=%q capacity=%d", updated.Title, updated.Capacity)
	}
}

func TestPublishEvent_Ownership(t *testing.T) {
	tests := []struct {
		name        string
		event       *model.EventModel
		managedOrgs []string
		wantErr     error
	}{
		{
			name:        "own organization",
			event:       eventOwnedBy(orgB, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-b"},
		},
		{
			name:        "member of several organizations",
			event:       eventOwnedBy(orgB, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-a", "kc-org-b"},
		},
		{
			name:        "event belongs to another organization",
			event:       eventOwnedBy(orgB, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-a"},
			wantErr:     ErrForbidden,
		},
		{
			name:    "no managed organizations",
			event:   eventOwnedBy(orgB, model.EventStatusDraft),
			wantErr: ErrForbidden,
		},
		{
			name:        "event without organizer",
			event:       eventOwnedBy(nil, model.EventStatusDraft),
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrForbidden,
		},
		{
			name:        "unknown event",
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrEventNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(tt.event)

			err := svc.PublishEvent(uuid.New(), tt.managedOrgs)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil && repo.saved != nil {
				t.Fatal("rejected publish must not be saved")
			}
			if tt.wantErr == nil && repo.saved == nil {
				t.Fatal("accepted publish was not saved")
			}
		})
	}
}

func TestPublishEvent_PersistsPublishedStatus(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, repo := newTestService(event)

	if err := svc.PublishEvent(event.EventID, []string{"kc-org-b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.saved == nil || repo.saved.Status != model.EventStatusPublished {
		t.Fatalf("status not persisted as published: %#v", repo.saved)
	}
}

func TestPublishEvent_OnlyDraftsCanBePublished(t *testing.T) {
	for _, status := range []model.EventStatus{
		model.EventStatusPublished,
		model.EventStatusCancelled,
		model.EventStatusCompleted,
	} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			svc, repo := newTestService(event)

			if err := svc.PublishEvent(event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrNotDraft) {
				t.Fatalf("expected ErrNotDraft, got %v", err)
			}
			if repo.saved != nil {
				t.Fatal("non-draft publish must not be saved")
			}
		})
	}
}

func TestPublishEvent_RequiresCompleteness(t *testing.T) {
	mutations := map[string]func(*model.EventModel){
		"missing title":    func(e *model.EventModel) { e.Title = "" },
		"zero start time":  func(e *model.EventModel) { e.StartTime = time.Time{} },
		"zero end time":    func(e *model.EventModel) { e.EndTime = time.Time{} },
		"end before start": func(e *model.EventModel) { e.EndTime = e.StartTime.Add(-time.Hour) },
		"end equals start": func(e *model.EventModel) { e.EndTime = e.StartTime },
		"zero capacity":    func(e *model.EventModel) { e.Capacity = 0 },
		"missing category": func(e *model.EventModel) { e.CategoryID = nil },
		"zero category id": func(e *model.EventModel) { id := uuid.Nil; e.CategoryID = &id },
		"missing location": func(e *model.EventModel) { e.LocationID = nil },
		"zero location id": func(e *model.EventModel) { id := uuid.Nil; e.LocationID = &id },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			event := eventOwnedBy(orgB, model.EventStatusDraft)
			mutate(event)
			svc, repo := newTestService(event)

			if err := svc.PublishEvent(event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrIncomplete) {
				t.Fatalf("expected ErrIncomplete, got %v", err)
			}
			if repo.saved != nil {
				t.Fatal("incomplete event must not be published")
			}
		})
	}
}

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
