package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"
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

	if booking.NumberOfTickets <= 0 {
		return nil, errors.New("number of tickets must be at least 1")
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

	err = s.bookingRepo.WithTransaction(func(txRepo repository.BookingRepository) error {
		booked, err := txRepo.CountByEventID(*booking.EventID)
		if err != nil {
			return err
		}

		available := int64(event.Capacity) - booked
		if available < int64(booking.NumberOfTickets) {
			return errors.New("no available seats left")
		}

		booking.Status = model.BookingStatusReserved
		return txRepo.CreateBooking(booking)
	})
	if err != nil {
		return nil, err
	}

	return booking, nil
}
