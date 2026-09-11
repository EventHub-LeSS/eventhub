package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"

	"github.com/google/uuid"
)

type BookingService struct {
	bookingRepo repository.BookingRepository
	eventRepo   repository.EventRepository
}

func NewBookingService(bookingRepo repository.BookingRepository, eventRepo repository.EventRepository) *BookingService {
	return &BookingService{
		bookingRepo: bookingRepo,
		eventRepo:   eventRepo,
	}
}

func (s *BookingService) CreateBooking(booking *model.BookingModel) (*model.BookingModel, error) {
	if booking.EventID == nil {
		return nil, errors.New("event_id is required")
	}

	event, err := s.eventRepo.GetEventByID(*booking.EventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, errors.New("event not found")
	}

	if event.Status != model.EventStatusPublished {
		return nil, errors.New("event is not available for booking")
	}

	booked, err := s.bookingRepo.CountByEventID(*booking.EventID)
	if err != nil {
		return nil, err
	}
	available := int64(event.Capacity) - booked
	if available < int64(booking.NumberOfTickets) {
		return nil, errors.New("no available seats left")
	}
	booking.BookingID = uuid.New()
	booking.Status = model.BookingStatusReserved

	if err := s.bookingRepo.CreateBooking(booking); err != nil {
		return nil, err
	}

	return booking, nil
}
