package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const DefaultReservationTTL = 15 * time.Minute

var (
	ErrEventNotPublished     = errors.New("event is not published")
	ErrInvalidTicketCount    = errors.New("number of tickets must be positive")
	ErrBookingNotCancellable = errors.New("booking cannot be cancelled in its current status")
)

// CapacityExceededError maps to the CAPACITY_EXCEEDED (409) error code from the API error list.
type CapacityExceededError struct {
	Requested int
	Available int64
}

func (e *CapacityExceededError) Error() string {
	return fmt.Sprintf("capacity exceeded: requested %d, available %d", e.Requested, e.Available)
}

// BookingService cancels the bookings of a user. Reservations are created by the CartService,
// which holds the seats of the cart; cancelling releases them again.
type BookingService struct {
	tx       repository.Transactor
	bookings repository.BookingRepository
	payments repository.PaymentRepository
}

func NewBookingService(tx repository.Transactor, bookings repository.BookingRepository, payments repository.PaymentRepository) *BookingService {
	return &BookingService{tx: tx, bookings: bookings, payments: payments}
}

// CancelBooking cancels a booking of the user and releases its tickets. A reservation without a
// payment is a cart item and is cancelled on its own. A booking of a payment is cancelled with
// the whole payment: while the payment is pending, the checkout is cancelled — nothing was
// charged — and once it is paid, the booking's line amount is refunded. The payment counts as
// refunded once every booking of it is cancelled. Repeated cancellation succeeds without
// another refund.
func (s *BookingService) CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (*model.BookingModel, error) {
	// The booking is read without a lock to learn what it belongs to; the transaction locks in
	// the canonical order of events, bookings, payment.
	unlocked, err := s.bookings.GetBookingByID(bookingID)
	if err != nil {
		return nil, err
	}
	if !ownsBooking(unlocked, userID) {
		return nil, ErrBookingNotFound
	}

	if unlocked.PaymentID == nil {
		return s.cancelReservation(ctx, bookingID, userID)
	}
	return s.cancelPaidOrPendingBooking(ctx, bookingID, userID, *unlocked.PaymentID)
}

// cancelReservation cancels a reservation without a payment, a cart item.
func (s *BookingService) cancelReservation(ctx context.Context, bookingID, userID uuid.UUID) (*model.BookingModel, error) {
	var booking *model.BookingModel
	err := s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		var err error
		booking, err = tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		switch booking.Status {
		case model.BookingStatusCancelled:
			return nil // repeated cancellation succeeds without another change
		case model.BookingStatusReserved:
		default:
			return ErrBookingNotCancellable
		}
		booking.Status = model.BookingStatusCancelled
		booking.ExpiresAt = nil
		return tx.Bookings.UpdateBooking(booking)
	})
	if err != nil {
		return nil, err
	}
	return booking, nil
}

// cancelPaidOrPendingBooking cancels one booking of a payment, together with the payment:
// pending — the checkout is cancelled as a whole and nothing was charged; paid — this booking's
// line amount is refunded, and the payment is refunded once all of its bookings are cancelled.
func (s *BookingService) cancelPaidOrPendingBooking(ctx context.Context, bookingID, userID, paymentID uuid.UUID) (*model.BookingModel, error) {
	// The payment view without locks learns the events and bookings to lock; every check runs
	// again under the locks below.
	view, err := s.payments.GetPaymentView(paymentID)
	if err != nil {
		return nil, err
	}
	if view == nil || !ownsPayment(view, userID) {
		return nil, ErrBookingNotFound
	}
	eventIDs, bookingIDs := viewIDs(view)

	var cancelled *model.BookingModel
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		if _, err := tx.Events.LockEvents(eventIDs); err != nil {
			return err
		}
		bookings, err := tx.Bookings.LockBookings(bookingIDs)
		if err != nil {
			return err
		}
		if !ownsBookings(bookings, userID) {
			return ErrBookingNotFound
		}
		payment, err := tx.Payments.LockPayment(paymentID)
		if err != nil {
			return err
		}
		if payment == nil {
			return fmt.Errorf("payment %s referenced by booking %s is missing", paymentID, bookingID)
		}
		target := bookingByID(bookings, bookingID)
		if target == nil {
			return ErrBookingNotFound
		}

		switch payment.Status {
		case model.PaymentStatusPending:
			// Cancelling one booking cancels the whole checkout: the payment amount covers all
			// of its bookings, so the others cannot be charged without this one.
			payment.Status = model.PaymentStatusFailed
			reason := model.PaymentFailureBookingCancelled
			payment.FailureReason = &reason
			for _, booking := range bookings {
				booking.Status = model.BookingStatusCancelled
				booking.ExpiresAt = nil
				if err := tx.Bookings.UpdateBooking(booking); err != nil {
					return err
				}
			}
			if err := tx.Payments.UpdatePayment(payment); err != nil {
				return err
			}
			cancelled = target
			return nil
		case model.PaymentStatusPaid, model.PaymentStatusRefunded:
			switch target.Status {
			case model.BookingStatusCancelled:
				cancelled = target // repeated cancellation succeeds without another refund
				return nil
			case model.BookingStatusConfirmed:
			default:
				return ErrBookingNotCancellable
			}
			target.Status = model.BookingStatusCancelled
			target.ExpiresAt = nil
			if err := tx.Bookings.UpdateBooking(target); err != nil {
				return err
			}
			// The refund is the sum of the line amounts of the cancelled bookings, so a
			// repeated cancellation never refunds twice. The payment counts as refunded once
			// everything it paid for was cancelled.
			refund := decimal.Zero
			for _, booking := range bookings {
				if booking.Status == model.BookingStatusCancelled && booking.LineAmount != nil {
					refund = refund.Add(*booking.LineAmount)
				}
			}
			payment.RefundAmount = refund
			if refund.GreaterThanOrEqual(payment.Amount) {
				payment.Status = model.PaymentStatusRefunded
			}
			if err := tx.Payments.UpdatePayment(payment); err != nil {
				return err
			}
			cancelled = target
			return nil
		}
		// A failed payment leaves no ticket claim, so its bookings are not cancellable anymore
		// — except one that was cancelled with it: a repeated cancellation succeeds.
		if target.Status == model.BookingStatusCancelled {
			cancelled = target
			return nil
		}
		return ErrBookingNotCancellable
	})
	if err != nil {
		return nil, err
	}
	return cancelled, nil
}

// bookingByID finds the booking in the list, or nil.
func bookingByID(bookings []*model.BookingModel, bookingID uuid.UUID) *model.BookingModel {
	for _, booking := range bookings {
		if booking.BookingID == bookingID {
			return booking
		}
	}
	return nil
}
