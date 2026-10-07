package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

var (
	ErrForbidden        = errors.New("forbidden")
	ErrEventNotFound    = errors.New("event not found")
	ErrInvalidStatus    = errors.New("invalid event status")
	ErrIncomplete       = errors.New("event is incomplete")
	ErrNotDraft         = errors.New("only draft events can be published")
	ErrNotPublished     = errors.New("only published events can be withdrawn")
	ErrAlreadyPublished = errors.New("this event is already published")
	// ErrOrganizationRequired means the caller manages events in several organizations and did not
	// say which one a new event belongs to.
	ErrOrganizationRequired = errors.New("organizationId is required when managing events in several organizations")
	// ErrUnknownReference means the event references a category or location that does not exist.
	ErrUnknownReference = repository.ErrUnknownReference
)

type EventService struct {
	eventRepo repository.EventRepository
	orgRepo   repository.OrganizationRepository
	tx        repository.Transactor
}

func NewEventService(eventRepo repository.EventRepository, orgRepo repository.OrganizationRepository, tx repository.Transactor) *EventService {
	return &EventService{eventRepo: eventRepo, orgRepo: orgRepo, tx: tx}
}

func (s *EventService) CreateEvent(event *model.EventModel) error {
	return s.eventRepo.CreateEvent(event)
}

// CreateDraft saves a new event in status draft. keycloakOrgIDs are the organizations in which the
// caller may manage events; req.OrganizationID picks one of them and may be omitted if there is only one.
func (s *EventService) CreateDraft(ctx context.Context, keycloakOrgIDs []string, req model.CreateDraftRequest) (*model.EventModel, error) {
	meta, err := audit.MetaFromContext(ctx, audit.EventCreated)
	if err != nil {
		return nil, err
	}
	orgRef := req.OrganizationID
	if orgRef == "" {
		switch len(keycloakOrgIDs) {
		case 0:
			return nil, ErrForbidden
		case 1:
			orgRef = keycloakOrgIDs[0]
		default:
			return nil, ErrOrganizationRequired
		}
	}

	// Only managed organizations are resolved, so an organizationId outside of them is not found.
	orgs, err := s.orgRepo.ListByKeycloakOrgIDsOrAliases(keycloakOrgIDs)
	if err != nil {
		return nil, err
	}
	idx := slices.IndexFunc(orgs, func(org *model.OrganizationModel) bool {
		return org.KeycloakOrgID == orgRef || org.Alias == orgRef
	})
	if idx < 0 {
		return nil, ErrForbidden
	}
	org := orgs[idx]

	event := &model.EventModel{
		EventID:     uuid.New(),
		Title:       req.Title,
		Description: req.Description,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		Capacity:    req.Capacity,
		Status:      model.EventStatusDraft,
		Price:       req.Price,
		CategoryID:  &req.CategoryID,
		OrganizerID: &org.OrganizationID,
		LocationID:  &req.LocationID,
	}

	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.Events.CreateEvent(event); err != nil {
			return err
		}
		return appendEventAudit(tx, meta, org, event.EventID, model.AuditChanges{"event": eventSnapshot(event)})
	})
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (s *EventService) GetEventByID(eventID uuid.UUID) (*model.EventModel, error) {
	return s.eventRepo.GetEventByID(eventID)
}

func (s *EventService) GetAllEvents() ([]*model.EventModel, error) {
	return s.eventRepo.GetAllEvents()
}

// ListOwnEvents returns the events of all organizations in keycloakOrgIDs, the organizations in
// which the caller may manage events. An empty status returns events of every status.
func (s *EventService) ListOwnEvents(keycloakOrgIDs []string, status model.EventStatus) ([]*model.EventModel, error) {
	orgs, err := s.orgRepo.ListByKeycloakOrgIDsOrAliases(keycloakOrgIDs)
	if err != nil {
		return nil, err
	}
	orgIDs := make([]uuid.UUID, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.OrganizationID)
	}
	return s.eventRepo.ListByOrganizers(orgIDs, status)
}

// keycloakOrgIDs are the organizations in which the caller may manage events.
func (s *EventService) UpdateEvent(
	ctx context.Context,
	eventID uuid.UUID,
	keycloakOrgIDs []string,
	req model.UpdateEventRequest,
) (*model.EventModel, error) {
	meta, err := audit.MetaFromContext(ctx, audit.EventUpdated)
	if err != nil {
		return nil, err
	}
	var updated *model.EventModel
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.OrganizerID == nil {
			return ErrForbidden
		}

		org, err := tx.Organizations.GetByID(*event.OrganizerID)
		if err != nil {
			return err
		}
		if !managesOrganization(org, keycloakOrgIDs) {
			return ErrForbidden
		}

		switch event.Status {
		case model.EventStatusDraft, model.EventStatusPublished:
		default:
			return ErrInvalidStatus
		}

		before := *event
		event.Title = req.Title
		event.Description = req.Description
		event.StartTime = req.StartTime
		event.EndTime = req.EndTime
		event.Capacity = req.Capacity
		event.Price = req.Price
		event.CategoryID = &req.CategoryID
		event.LocationID = &req.LocationID

		if err := tx.Events.UpdateEvent(event); err != nil {
			return err
		}
		// Compare with the stored row: the database rounds the price to its column precision.
		persisted, err := tx.Events.GetEventByID(eventID)
		if err != nil {
			return err
		}
		if persisted == nil {
			return ErrEventNotFound
		}
		if fields := eventFieldChanges(&before, persisted); len(fields) > 0 {
			if err := appendEventAudit(tx, meta, org, eventID, model.AuditChanges{"fields": fields}); err != nil {
				return err
			}
		}
		updated = event
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *EventService) DeleteEvent(eventID uuid.UUID) error {
	return s.eventRepo.DeleteEvent(eventID)
}

func (s *EventService) ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error) {
	return s.eventRepo.ListByOrganization(organizationID)
}

// managesOrganization reports whether org is one of keycloakOrgIDs, which tokens identify by
// Keycloak ID or, when the claim carries no ID, by alias.
func managesOrganization(org *model.OrganizationModel, keycloakOrgIDs []string) bool {
	return org != nil && (slices.Contains(keycloakOrgIDs, org.KeycloakOrgID) ||
		slices.Contains(keycloakOrgIDs, org.Alias))
}

func isEventComplete(event *model.EventModel) bool {
	if event.Title == "" {
		return false
	}
	if event.StartTime.IsZero() || event.EndTime.IsZero() {
		return false
	}
	if !event.EndTime.After(event.StartTime) {
		return false
	}
	if event.Capacity <= 0 {
		return false
	}
	if event.CategoryID == nil || *event.CategoryID == uuid.Nil ||
		event.LocationID == nil || *event.LocationID == uuid.Nil {
		return false
	}
	return true
}

func (s *EventService) PublishEvent(ctx context.Context, eventID uuid.UUID, keycloakOrgIDs []string) error {
	meta, err := audit.MetaFromContext(ctx, audit.EventPublished)
	if err != nil {
		return err
	}
	return s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.OrganizerID == nil {
			return ErrForbidden
		}
		org, err := tx.Organizations.GetByID(*event.OrganizerID)
		if err != nil {
			return err
		}
		if !managesOrganization(org, keycloakOrgIDs) {
			return ErrForbidden
		}
		if event.Status == model.EventStatusPublished {
			return ErrAlreadyPublished
		}
		if event.Status != model.EventStatusDraft {
			return ErrNotDraft
		}
		if !isEventComplete(event) {
			return ErrIncomplete
		}
		event.Status = model.EventStatusPublished
		if err := tx.Events.UpdateEvent(event); err != nil {
			return err
		}
		return appendEventAudit(tx, meta, org, eventID, statusChange(model.EventStatusDraft, model.EventStatusPublished))
	})
}

func (s *EventService) WithdrawEvent(ctx context.Context, eventID uuid.UUID, keycloakOrgIDs []string) error {
	meta, err := audit.MetaFromContext(ctx, audit.EventCancelled)
	if err != nil {
		return err
	}
	return s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.OrganizerID == nil {
			return ErrForbidden
		}
		org, err := tx.Organizations.GetByID(*event.OrganizerID)
		if err != nil {
			return err
		}
		if !managesOrganization(org, keycloakOrgIDs) {
			return ErrForbidden
		}
		if event.Status != model.EventStatusPublished {
			return ErrNotPublished
		}
		event.Status = model.EventStatusCancelled
		if err := tx.Events.UpdateEvent(event); err != nil {
			return err
		}
		return appendEventAudit(tx, meta, org, eventID, statusChange(model.EventStatusPublished, model.EventStatusCancelled))
	})
}

func (s *EventService) GetEventStatistics(
	eventID uuid.UUID,
	keycloakOrgIDs []string,
) (*model.EventStatistics, error) {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrEventNotFound
	}
	if event.OrganizerID == nil {
		return nil, ErrForbidden
	}

	org, err := s.orgRepo.GetByID(*event.OrganizerID)
	if err != nil {
		return nil, err
	}
	if !managesOrganization(org, keycloakOrgIDs) {
		return nil, ErrForbidden
	}

	soldTickets, err := s.eventRepo.GetConfirmedTicketCount(eventID)
	if err != nil {
		return nil, err
	}

	availableSeats := int64(event.Capacity) - soldTickets
	if availableSeats < 0 {
		availableSeats = 0
	}

	return &model.EventStatistics{
		EventID:        event.EventID,
		Capacity:       event.Capacity,
		SoldTickets:    soldTickets,
		AvailableSeats: availableSeats,
	}, nil
}

// ListPublishedEvents returns the public listing from the shared event database.
func (s *EventService) ListPublishedEvents(filter model.PublishedEventFilter) ([]model.PublishedEventResponse, error) {
	events, err := s.eventRepo.ListPublishedEvents(filter)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = make([]model.PublishedEventResponse, 0)
	}
	return events, nil
}
