package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"

	"github.com/google/uuid"
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
