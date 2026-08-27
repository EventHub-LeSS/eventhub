package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"

	"github.com/google/uuid"
)

var ErrEventNotFound = errors.New("event not found")

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
		return ErrEventNotFound
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

func (s *EventService) GetEventStatistics(eventID uuid.UUID) (*model.EventStatistics, error) {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrEventNotFound
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
