package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeEventRepo struct {
	event *model.EventModel
	saved *model.EventModel

	createErr error
	created   *model.EventModel

	listed     []*model.EventModel
	listOrgIDs []uuid.UUID
	listStatus model.EventStatus

	hasBookings  bool
	deleted      []uuid.UUID
	deleteMisses bool
}

func (f *fakeEventRepo) CreateEvent(event *model.EventModel) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = event
	return nil
}

func (f *fakeEventRepo) GetEventByID(uuid.UUID) (*model.EventModel, error) {
	return f.event, nil
}

func (f *fakeEventRepo) GetAllEvents() ([]*model.EventModel, error) {
	return nil, nil
}

func (f *fakeEventRepo) DeleteEvent(eventID uuid.UUID) (bool, error) {
	if f.deleteMisses {
		return false, nil
	}
	f.deleted = append(f.deleted, eventID)
	return true, nil
}

func (f *fakeEventRepo) ListByOrganization(uuid.UUID) ([]*model.EventModel, error) {
	return nil, nil
}

func (f *fakeEventRepo) ListByOrganizers(organizationIDs []uuid.UUID, status model.EventStatus) ([]*model.EventModel, error) {
	f.listOrgIDs = organizationIDs
	f.listStatus = status
	return f.listed, nil
}

func (f *fakeEventRepo) GetConfirmedTicketCount(uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeEventRepo) HasBookings(uuid.UUID) (bool, error) {
	return f.hasBookings, nil
}

func (f *fakeEventRepo) LockEvent(uuid.UUID) (*model.EventModel, error) {
	return f.event, nil
}
func (f *fakeEventRepo) UpdateEvent(event *model.EventModel) error {
	f.saved = event
	return nil
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

func (f *fakeOrgRepo) ListByKeycloakOrgIDsOrAliases(refs []string) ([]*model.OrganizationModel, error) {
	var orgs []*model.OrganizationModel
	for _, org := range f.orgs {
		if slices.Contains(refs, org.KeycloakOrgID) || slices.Contains(refs, org.Alias) {
			orgs = append(orgs, org)
		}
	}
	return orgs, nil
}

var (
	orgA = &model.OrganizationModel{
		OrganizationID: uuid.New(),
		KeycloakOrgID:  "kc-org-a",
		Alias:          "alias-a",
	}
	orgB = &model.OrganizationModel{
		OrganizationID: uuid.New(),
		KeycloakOrgID:  "kc-org-b",
		Alias:          "alias-b",
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

type fakeTransactor struct {
	events repository.EventRepository
	orgs   repository.OrganizationRepository
	audit  repository.AuditLogRepository
}

func (f *fakeTransactor) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	return fn(repository.Tx{Events: f.events, Organizations: f.orgs, Audit: f.audit})
}

type fakeAuditRepo struct {
	entries []*model.AuditLogModel
	err     error
}

func (f *fakeAuditRepo) Append(entry *model.AuditLogModel) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeAuditRepo) ListByOrganization(string, int, *model.AuditLogCursor) ([]model.AuditLogModel, error) {
	return nil, nil
}

func auditCtx(action audit.Action) context.Context {
	return audit.WithMeta(context.Background(), audit.Meta{
		OperationID:   uuid.New(),
		Action:        action,
		ActorSubject:  "sub-anna",
		ActorUsername: "anna@acme.test",
	})
}

func newTestService(event *model.EventModel) (*EventService, *fakeEventRepo) {
	svc, repo, _ := newTestServiceWithAudit(event)
	return svc, repo
}

func newTestServiceWithAudit(event *model.EventModel) (*EventService, *fakeEventRepo, *fakeAuditRepo) {
	eventRepo := &fakeEventRepo{event: event}
	orgRepo := &fakeOrgRepo{
		orgs: map[uuid.UUID]*model.OrganizationModel{
			orgA.OrganizationID: orgA,
			orgB.OrganizationID: orgB,
		},
	}
	auditRepo := &fakeAuditRepo{}
	tx := &fakeTransactor{events: eventRepo, orgs: orgRepo, audit: auditRepo}
	return NewEventService(eventRepo, orgRepo, tx), eventRepo, auditRepo
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

			_, err := svc.UpdateEvent(auditCtx(audit.EventUpdated), uuid.New(), tt.managedOrgs, updateRequest())

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

	updated, err := svc.UpdateEvent(auditCtx(audit.EventUpdated), event.EventID, []string{"kc-org-b"}, updateRequest())
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

			err := svc.PublishEvent(auditCtx(audit.EventPublished), uuid.New(), tt.managedOrgs)

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

	if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.saved == nil || repo.saved.Status != model.EventStatusPublished {
		t.Fatalf("status not persisted as published: %#v", repo.saved)
	}
}

func TestPublishEvent_OnlyDraftsCanBePublished(t *testing.T) {
	tests := []struct {
		status  model.EventStatus
		wantErr error
	}{
		{status: model.EventStatusPublished, wantErr: ErrAlreadyPublished},
		{status: model.EventStatusCancelled, wantErr: ErrNotDraft},
		{status: model.EventStatusCompleted, wantErr: ErrNotDraft},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			event := eventOwnedBy(orgB, tt.status)
			svc, repo := newTestService(event)

			if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"}); !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
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

			if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrIncomplete) {
				t.Fatalf("expected ErrIncomplete, got %v", err)
			}
			if repo.saved != nil {
				t.Fatal("incomplete event must not be published")
			}
		})
	}
}

func TestWithdrawEvent_Ownership(t *testing.T) {
	tests := []struct {
		name        string
		event       *model.EventModel
		managedOrgs []string
		wantErr     error
	}{
		{name: "own organization", event: eventOwnedBy(orgB, model.EventStatusPublished), managedOrgs: []string{"kc-org-b"}},
		{name: "member of several organizations", event: eventOwnedBy(orgB, model.EventStatusPublished), managedOrgs: []string{"kc-org-a", "kc-org-b"}},
		{name: "event belongs to another organization", event: eventOwnedBy(orgB, model.EventStatusPublished), managedOrgs: []string{"kc-org-a"}, wantErr: ErrForbidden},
		{name: "no managed organizations", event: eventOwnedBy(orgB, model.EventStatusPublished), managedOrgs: nil, wantErr: ErrForbidden},
		{name: "event without organizer", event: eventOwnedBy(nil, model.EventStatusPublished), managedOrgs: []string{"kc-org-b"}, wantErr: ErrForbidden},
		{name: "unknown event", event: nil, managedOrgs: []string{"kc-org-b"}, wantErr: ErrEventNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(tt.event)

			err := svc.WithdrawEvent(auditCtx(audit.EventCancelled), uuid.New(), tt.managedOrgs)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil && repo.saved != nil {
				t.Fatal("rejected withdraw must not be saved")
			}
			if tt.wantErr == nil && repo.saved == nil {
				t.Fatal("accepted withdraw was not saved")
			}
		})
	}
}

func TestWithdrawEvent_PersistsCancelledStatus(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusPublished)
	svc, repo := newTestService(event)

	if err := svc.WithdrawEvent(auditCtx(audit.EventCancelled), event.EventID, []string{"kc-org-b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.saved == nil || repo.saved.Status != model.EventStatusCancelled {
		t.Fatalf("status not persisted as cancelled: %#v", repo.saved)
	}
}

func TestWithdrawEvent_OnlyPublishedCanBeWithdrawn(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			svc, repo := newTestService(event)

			if err := svc.WithdrawEvent(auditCtx(audit.EventCancelled), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrNotPublished) {
				t.Fatalf("expected ErrNotPublished, got %v", err)
			}
			if repo.saved != nil {
				t.Fatal("non-published withdraw must not be saved")
			}
		})
	}
}

func (f *fakeEventRepo) ListPublishedEvents(filter model.PublishedEventFilter) ([]model.PublishedEventResponse, error) {
	return nil, nil
}

func draftRequest(organizationID string) model.CreateDraftRequest {
	return model.CreateDraftRequest{
		OrganizationID: organizationID,
		Title:          "Sommerfest 2026",
		StartTime:      time.Now().Add(48 * time.Hour),
		EndTime:        time.Now().Add(52 * time.Hour),
		Capacity:       100,
		Price:          decimal.NewFromInt(15),
		CategoryID:     uuid.New(),
		LocationID:     uuid.New(),
	}
}

// EVENTHUB-77 / AK1: Eine Veranstaltung kann im Status Entwurf gespeichert werden.
func TestCreateDraft_SavesDraftForManagedOrganization(t *testing.T) {
	tests := []struct {
		name           string
		managedOrgs    []string
		organizationID string
		wantOrg        *model.OrganizationModel
	}{
		{name: "single organization, organizationId omitted", managedOrgs: []string{"kc-org-b"}, wantOrg: orgB},
		{name: "token identifies the organization by alias", managedOrgs: []string{"alias-a"}, wantOrg: orgA},
		{name: "several organizations, picked by Keycloak ID", managedOrgs: []string{"kc-org-a", "kc-org-b"}, organizationID: "kc-org-b", wantOrg: orgB},
		{name: "several organizations, picked by alias", managedOrgs: []string{"kc-org-a", "kc-org-b"}, organizationID: "alias-a", wantOrg: orgA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(nil)
			req := draftRequest(tt.organizationID)

			created, err := svc.CreateDraft(auditCtx(audit.EventCreated), tt.managedOrgs, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.created != created {
				t.Fatal("draft was not saved")
			}
			if created.Status != model.EventStatusDraft {
				t.Errorf("status = %s, want draft", created.Status)
			}
			if created.EventID == uuid.Nil {
				t.Error("event ID not set")
			}
			if created.OrganizerID == nil || *created.OrganizerID != tt.wantOrg.OrganizationID {
				t.Errorf("organizer = %v, want %v", created.OrganizerID, tt.wantOrg.OrganizationID)
			}
			if created.Title != req.Title || created.Capacity != req.Capacity || !created.Price.Equal(req.Price) ||
				*created.CategoryID != req.CategoryID || *created.LocationID != req.LocationID {
				t.Errorf("request fields not applied: %#v", created)
			}
		})
	}
}

func TestCreateDraft_Rejections(t *testing.T) {
	tests := []struct {
		name           string
		managedOrgs    []string
		organizationID string
		wantErr        error
	}{
		{name: "no managed organizations", wantErr: ErrForbidden},
		{name: "several organizations without organizationId", managedOrgs: []string{"kc-org-a", "kc-org-b"}, wantErr: ErrOrganizationRequired},
		{name: "organizationId of a foreign organization", managedOrgs: []string{"kc-org-a"}, organizationID: "kc-org-b", wantErr: ErrForbidden},
		{name: "organization not in the database", managedOrgs: []string{"kc-org-unknown"}, wantErr: ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(nil)

			_, err := svc.CreateDraft(auditCtx(audit.EventCreated), tt.managedOrgs, draftRequest(tt.organizationID))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if repo.created != nil {
				t.Fatal("rejected draft must not be saved")
			}
		})
	}
}

func TestCreateDraft_UnknownReference(t *testing.T) {
	svc, repo := newTestService(nil)
	repo.createErr = fmt.Errorf("%w: categoryId does not exist", repository.ErrUnknownReference)

	_, err := svc.CreateDraft(auditCtx(audit.EventCreated), []string{"kc-org-b"}, draftRequest(""))
	if !errors.Is(err, ErrUnknownReference) {
		t.Fatalf("expected ErrUnknownReference, got %v", err)
	}
}

// EVENTHUB-77 / AK3: Der Veranstalter kann eigene Entwürfe sehen.
func TestListOwnEvents_ListsManagedOrganizations(t *testing.T) {
	svc, repo := newTestService(nil)
	repo.listed = []*model.EventModel{eventOwnedBy(orgA, model.EventStatusDraft)}

	events, err := svc.ListOwnEvents([]string{"alias-a", "kc-org-b", "kc-org-unknown"}, model.EventStatusDraft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0] != repo.listed[0] {
		t.Errorf("repository result not returned: %v", events)
	}
	if repo.listStatus != model.EventStatusDraft {
		t.Errorf("status filter = %q, want draft", repo.listStatus)
	}
	if len(repo.listOrgIDs) != 2 ||
		!slices.Contains(repo.listOrgIDs, orgA.OrganizationID) || !slices.Contains(repo.listOrgIDs, orgB.OrganizationID) {
		t.Errorf("queried organizations %v, want %v and %v", repo.listOrgIDs, orgA.OrganizationID, orgB.OrganizationID)
	}
}

func (f *fakeEventRepo) GetPublishedEventDetails(uuid.UUID) (*model.PublishedEventDetailsResponse, error) {
	return nil, nil
}

func eventIDOf(event *model.EventModel) uuid.UUID {
	if event == nil {
		return uuid.New()
	}
	return event.EventID
}

// EVENTHUB-256 / AK3, T3: Nur wer in der besitzenden Organisation löschen darf, löscht den Entwurf.
func TestDeleteEvent_Ownership_OnlyOwningOrganization(t *testing.T) {
	tests := []struct {
		name        string
		event       *model.EventModel
		managedOrgs []string
		wantErr     error
	}{
		{name: "own organization", event: eventOwnedBy(orgB, model.EventStatusDraft), managedOrgs: []string{"kc-org-b"}},
		{name: "own organization by alias", event: eventOwnedBy(orgB, model.EventStatusDraft), managedOrgs: []string{"alias-b"}},
		{name: "member of several organizations", event: eventOwnedBy(orgB, model.EventStatusDraft), managedOrgs: []string{"kc-org-a", "kc-org-b"}},
		{name: "event belongs to another organization", event: eventOwnedBy(orgB, model.EventStatusDraft), managedOrgs: []string{"kc-org-a"}, wantErr: ErrForbidden},
		{name: "no managed organizations", event: eventOwnedBy(orgB, model.EventStatusDraft), wantErr: ErrForbidden},
		{name: "event without organizer", event: eventOwnedBy(nil, model.EventStatusDraft), managedOrgs: []string{"kc-org-b"}, wantErr: ErrForbidden},
		{name: "unknown event", managedOrgs: []string{"kc-org-b"}, wantErr: ErrEventNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(tt.event)
			eventID := eventIDOf(tt.event)

			err := svc.DeleteEvent(auditCtx(audit.EventDeleted), eventID, tt.managedOrgs)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr != nil && len(repo.deleted) != 0 {
				t.Fatal("rejected deletion must not delete")
			}
			if tt.wantErr == nil && (len(repo.deleted) != 1 || repo.deleted[0] != eventID) {
				t.Fatalf("draft %s was not deleted: %v", eventID, repo.deleted)
			}
		})
	}
}

// EVENTHUB-256: Erst die Berechtigung, dann Status und Buchungen, sonst erfährt eine fremde
// Organisation den Status eines Events.
func TestDeleteEvent_ForeignOrganization_ForbiddenBeforeStatusAndBookings(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			svc, repo := newTestService(event)
			repo.hasBookings = true

			if err := svc.DeleteEvent(auditCtx(audit.EventDeleted), event.EventID, []string{"kc-org-a"}); !errors.Is(err, ErrForbidden) {
				t.Fatalf("expected ErrForbidden, got %v", err)
			}
			if len(repo.deleted) != 0 {
				t.Fatal("rejected deletion must not delete")
			}
		})
	}
}

// EVENTHUB-256 / AK2, T2: Veröffentlichte, abgesagte und abgeschlossene Events werden nicht gelöscht.
func TestDeleteEvent_NotDraft_ReturnsErrNotDeletable(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			svc, repo, auditRepo := newTestServiceWithAudit(event)

			if err := svc.DeleteEvent(auditCtx(audit.EventDeleted), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrNotDeletable) {
				t.Fatalf("expected ErrNotDeletable, got %v", err)
			}
			if len(repo.deleted) != 0 || len(auditRepo.entries) != 0 {
				t.Fatalf("rejected deletion ran: deleted=%v entries=%d", repo.deleted, len(auditRepo.entries))
			}
		})
	}
}

// EVENTHUB-256 / T2: Gebuchte Events werden nie gelöscht, auch nicht als Entwurf.
func TestDeleteEvent_DraftWithBookings_ReturnsErrHasBookings(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, repo, auditRepo := newTestServiceWithAudit(event)
	repo.hasBookings = true

	if err := svc.DeleteEvent(auditCtx(audit.EventDeleted), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrHasBookings) {
		t.Fatalf("expected ErrHasBookings, got %v", err)
	}
	if len(repo.deleted) != 0 || len(auditRepo.entries) != 0 {
		t.Fatalf("rejected deletion ran: deleted=%v entries=%d", repo.deleted, len(auditRepo.entries))
	}
}

func TestDeleteEvent_RowAlreadyGone_ReturnsErrEventNotFound(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, repo, auditRepo := newTestServiceWithAudit(event)
	repo.deleteMisses = true

	if err := svc.DeleteEvent(auditCtx(audit.EventDeleted), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}
	if len(auditRepo.entries) != 0 {
		t.Errorf("deletion without a deleted row recorded %d entries", len(auditRepo.entries))
	}
}
