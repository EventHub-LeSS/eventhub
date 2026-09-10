package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const DefaultReservationTTL = 15 * time.Minute

var (
	ErrEventNotFound      = errors.New("event not found")
	ErrEventNotPublished  = errors.New("event is not published")
	ErrInvalidTicketCount = errors.New("number of tickets must be positive")
)

// CapacityExceededError maps to the CAPACITY_EXCEEDED (409) error code from the API error list.
type CapacityExceededError struct {
	Requested int
	Available int64
}

func (e *CapacityExceededError) Error() string {
	return fmt.Sprintf("capacity exceeded: requested %d, available %d", e.Requested, e.Available)
}

type BookingService struct {
	bookingRepo    repository.BookingRepository
	reservationTTL time.Duration
}

func NewBookingService(bookingRepo repository.BookingRepository, reservationTTL time.Duration) *BookingService {
	if reservationTTL <= 0 {
		reservationTTL = DefaultReservationTTL
	}
	return &BookingService{bookingRepo: bookingRepo, reservationTTL: reservationTTL}
}

// EVENTHUB-92: Überbuchung verhindern
func (s *BookingService) ReserveTickets(ctx context.Context, eventID, userID uuid.UUID, tickets int) (*model.BookingModel, error) {
	if tickets <= 0 {
		return nil, ErrInvalidTicketCount
	}

	var booking *model.BookingModel
	err := s.bookingRepo.InTransaction(ctx, func(tx repository.BookingTx) error {
		event, err := tx.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.Status != model.EventStatusPublished {
			return ErrEventNotPublished
		}

		occupied, err := tx.CountOccupiedTickets(eventID)
		if err != nil {
			return err
		}
		available := int64(event.Capacity) - occupied
		if available < 0 {
			available = 0
		}
		if available < int64(tickets) {
			return &CapacityExceededError{Requested: tickets, Available: available}
		}

		expiresAt := time.Now().Add(s.reservationTTL)
		eventRef, userRef := eventID, userID
		booking = &model.BookingModel{
			BookingID:       uuid.New(),
			UserID:          &userRef,
			EventID:         &eventRef,
			NumberOfTickets: tickets,
			Status:          model.BookingStatusReserved,
			ExpiresAt:       &expiresAt,
		}
		return tx.CreateBooking(booking)
	})
	if err != nil {
		return nil, err
	}
	return booking, nil
}
