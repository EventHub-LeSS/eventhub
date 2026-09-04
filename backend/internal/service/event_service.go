package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidStatus = errors.New("invalid event status")
	ErrIncomplete    = errors.New("event is incomplete")
)

type EventService struct {
	eventRepo repository.EventRepository
}

func NewEventService(eventRepo repository.EventRepository) *EventService {
	return &EventService{eventRepo: eventRepo}
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
	if event.CategoryID == uuid.Nil || event.LocationID == uuid.Nil {
		return false
	}
	return true
}

func (s *EventService) PublishEvent(eventID, userID uuid.UUID) error {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return err
	}
	if event == nil || event.OrganizerID != userID {
		return ErrForbidden
	}
	if event.Status != model.EventStatusDraft {
		return ErrInvalidStatus
	}
	if !isEventComplete(event) {
		return ErrIncomplete
	}
	event.Status = model.EventStatusPublished
	return s.eventRepo.UpdateEvent(event)
}

func (s *EventService) WithdrawEvent(eventID, userID uuid.UUID) error {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return err
	}
	if event == nil || event.OrganizerID != userID {
		return ErrForbidden
	}
	if event.Status != model.EventStatusPublished {
		return ErrInvalidStatus
	}
	event.Status = model.EventStatusCancelled
	return s.eventRepo.UpdateEvent(event)
}
