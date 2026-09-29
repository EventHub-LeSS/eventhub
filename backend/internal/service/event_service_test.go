package service

import (
	"backend/internal/model"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
