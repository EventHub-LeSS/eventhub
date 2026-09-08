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
	ErrIncomplete    = errors.New("event is incomplete")
	ErrNotDraft      = errors.New("only draft events can be published")
	ErrNotPublished  = errors.New("only published events can be withdrawn")
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
func (s *EventService) UpdateEvent(event *model.EventModel) error {
	existing, err := s.eventRepo.GetEventByID(event.EventID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("event not found")
	}
	if existing.Status != model.EventStatusDraft {
		return errors.New("only draft events can be updated")
	}
	return s.eventRepo.UpdateEvent(event)
}
func (s *EventService) DeleteEvent(eventID uuid.UUID) error {
	return s.eventRepo.DeleteEvent(eventID)
}

func (s *EventService) ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error) {
	return s.eventRepo.ListByOrganization(organizationID)
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

func (s *EventService) PublishEvent(eventID uuid.UUID, keycloakOrgID string) error {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return err
	}
	if event == nil {
		return ErrEventNotFound
	}
	if event.OrganizerID == nil {
		return ErrForbidden
	}
	org, err := s.orgRepo.GetByKeycloakOrgID(keycloakOrgID)
	if err != nil {
		return err
	}
	if org == nil || org.OrganizationID != *event.OrganizerID {
		return ErrForbidden
	}
	if event.Status != model.EventStatusDraft {
		return ErrNotDraft
	}
	if !isEventComplete(event) {
		return ErrIncomplete
	}
	event.Status = model.EventStatusPublished
	return s.eventRepo.UpdateEvent(event)
}

func (s *EventService) WithdrawEvent(eventID uuid.UUID, keycloakOrgID string) error {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return err
	}
	if event == nil {
		return ErrEventNotFound
	}
	if event.OrganizerID == nil {
		return ErrForbidden
	}
	org, err := s.orgRepo.GetByKeycloakOrgID(keycloakOrgID)
	if err != nil {
		return err
	}
	if org == nil || org.OrganizationID != *event.OrganizerID {
		return ErrForbidden
	}
	if event.Status != model.EventStatusPublished {
		return ErrNotPublished
	}
	event.Status = model.EventStatusCancelled
	return s.eventRepo.UpdateEvent(event)
}
