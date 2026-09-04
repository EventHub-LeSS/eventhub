package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrForbidden     = errors.New("forbidden")
	ErrEventNotFound = errors.New("event not found")
	ErrInvalidStatus = errors.New("invalid event status")
	ErrIncomplete    = errors.New("event is incomplete")
	ErrInvalidPrice  = errors.New("price must not be negative")
	ErrStartInPast   = errors.New("start time must not be in the past")
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

func (s *EventService) UpdateEvent(eventID, userID uuid.UUID, req model.UpdateEventRequest) (*model.EventModel, error) {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrEventNotFound
	}
	if event.OrganizerID == nil || *event.OrganizerID != userID {
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

	if !isEventComplete(event) {
		return nil, ErrIncomplete
	}
	if event.Price.IsNegative() {
		return nil, ErrInvalidPrice
	}
	if event.StartTime.Before(time.Now()) {
		return nil, ErrStartInPast
	}

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
	if event.CategoryID == nil || *event.CategoryID == uuid.Nil {
		return false
	}
	if event.LocationID == nil || *event.LocationID == uuid.Nil {
		return false
	}
	return true
}
