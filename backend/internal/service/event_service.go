package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrForbidden     = errors.New("forbidden")
	ErrEventNotFound = errors.New("event not found")
	ErrInvalidStatus = errors.New("invalid event status")
)

type EventService struct {
	eventRepo repository.EventRepository
	orgRepo   repository.OrganizationRepository
}

func NewEventService(eventRepo repository.EventRepository, orgRepo repository.OrganizationRepository) *EventService {
	return &EventService{eventRepo: eventRepo, orgRepo: orgRepo}
}

func (s *EventService) CreateEvent(event *model.EventModel) error {
	return s.eventRepo.CreateEvent(event)
}

func (s *EventService) GetEventByID(eventID uuid.UUID) (*model.EventModel, error) {
	return s.eventRepo.GetEventByID(eventID)
}

func (s *EventService) GetAllEvents() ([]*model.EventModel, error) {
	return s.eventRepo.GetAllEvents()
}

func (s *EventService) UpdateEvent(eventID uuid.UUID, keycloakOrgID string, req model.UpdateEventRequest) (*model.EventModel, error) {
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
	org, err := s.orgRepo.GetByKeycloakOrgID(keycloakOrgID)
	if err != nil {
		return nil, err
	}
	if org == nil || org.OrganizationID != *event.OrganizerID {
		return nil, ErrForbidden
	}

	switch event.Status {
	case model.EventStatusDraft, model.EventStatusPublished:
	default:
		return nil, ErrInvalidStatus
	}

	event.Title = req.Title
	event.Description = req.Description
	event.StartTime = req.StartTime
	event.EndTime = req.EndTime
	event.Capacity = req.Capacity
	event.Price = req.Price
	event.CategoryID = &req.CategoryID
	event.LocationID = &req.LocationID

	if err := s.eventRepo.UpdateEvent(event); err != nil {
		return nil, err
	}
	return event, nil
}

func (s *EventService) DeleteEvent(eventID uuid.UUID) error {
	return s.eventRepo.DeleteEvent(eventID)
}

func (s *EventService) ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error) {
	return s.eventRepo.ListByOrganization(organizationID)
}
