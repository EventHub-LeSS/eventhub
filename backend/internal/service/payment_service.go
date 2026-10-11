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

var (
	ErrBookingNotFound         = errors.New("booking not found")
	ErrBookingNotPayable       = errors.New("booking is not awaiting payment")
	ErrReservationExpired      = errors.New("reservation has expired, please book again")
	ErrEventUnavailable        = errors.New("event is no longer available for booking")
	ErrPaymentNotStarted       = errors.New("no payment process has been started for this booking")
	ErrPaymentAlreadyProcessed = errors.New("payment has already been processed")
	ErrPaymentAmountMismatch   = errors.New("amount does not match the amount of the payment")
	ErrInvalidPaymentAmount    = errors.New("payment amount is out of range")
	ErrInvalidPaymentOutcome   = errors.New("outcome must be success or failure")
)

// maxPaymentAmount is the largest value of the payments.amount column, NUMERIC(12,2).
var maxPaymentAmount = decimal.RequireFromString("9999999999.99")

// PaymentService runs the payment process of a booking. No real payment provider is connected:
// the caller chooses the outcome of the simulated payment (EVENTHUB-235).
type PaymentService struct {
	paymentRepo repository.PaymentRepository
}

func NewPaymentService(paymentRepo repository.PaymentRepository) *PaymentService {
	return &PaymentService{paymentRepo: paymentRepo}
}

// StartPayment turns a live reservation of the user into a pending payment. The amount is the
// number of tickets times the event price and is computed here only, never taken from the client.
// Starting again returns the pending payment unchanged, so the amount shown to the user is the
// amount that is charged. created reports whether a new payment was created.
func (s *PaymentService) StartPayment(ctx context.Context, bookingID, userID uuid.UUID) (resp *model.PaymentResponse, created bool, err error) {
	err = s.paymentRepo.InTransaction(ctx, func(tx repository.PaymentTx) error {
		// The booking lock serializes concurrent starts, so a booking never gets two payments.
		// The event is only read: SimulatePayment locks the event before the booking.
		booking, err := tx.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		if booking.Status != model.BookingStatusReserved {
			return ErrBookingNotPayable
		}
		now, err := tx.Now()
		if err != nil {
			return err
		}
		if reservationExpired(booking, now) {
			return ErrReservationExpired
		}

		var event *model.EventModel
		if booking.EventID != nil {
			if event, err = tx.GetEvent(*booking.EventID); err != nil {
				return err
			}
		}
		if event == nil || event.Status != model.EventStatusPublished {
			return ErrEventUnavailable
		}

		if booking.PaymentID != nil {
			payment, err := loadPayment(tx.GetPayment, booking)
			if err != nil {
				return err
			}
			if payment.Status != model.PaymentStatusPending {
				return ErrPaymentAlreadyProcessed
			}
			return nil
		}

		amount, err := paymentAmount(event.Price, booking.NumberOfTickets)
		if err != nil {
			return err
		}
		payment := &model.PaymentModel{
			PaymentID: uuid.New(),
			Amount:    amount,
			Status:    model.PaymentStatusPending,
		}
		if err := tx.CreatePayment(payment); err != nil {
			return err
		}
		booking.PaymentID = &payment.PaymentID
		if err := tx.UpdateBooking(booking); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	// Like every payment result, the response is read back from the database.
	resp, err = s.GetPayment(ctx, bookingID, userID)
	return resp, created, err
}

// SimulatePayment completes the pending payment of a booking with the outcome of the simulated
// payment provider. confirmedAmount is the amount shown to the user: unless it equals the stored
// amount, nothing changes. A successful payment confirms the booking. A failed payment leaves no
// ticket claim: the booking fails or expires and its tickets are released. The payment fails as
// well, whatever the outcome, when the event was withdrawn or the reservation expired meanwhile.
func (s *PaymentService) SimulatePayment(ctx context.Context, bookingID, userID uuid.UUID, outcome model.SimulatedPaymentOutcome, confirmedAmount decimal.Decimal) (*model.PaymentResponse, error) {
	if outcome != model.SimulatedPaymentSuccess && outcome != model.SimulatedPaymentFailure {
		return nil, ErrInvalidPaymentOutcome
	}

	// Locks are taken in the order of ReserveTickets, event before booking, which rules out
	// deadlocks. The booking is read first to learn its event, which never changes.
	unlocked, err := s.paymentRepo.GetBookingByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !ownsBooking(unlocked, userID) {
		return nil, ErrBookingNotFound
	}

	err = s.paymentRepo.InTransaction(ctx, func(tx repository.PaymentTx) error {
		// The event lock serializes the confirmation with new reservations: once a reservation has
		// expired, ReserveTickets may hand its tickets to others, so it must not be confirmed.
		var event *model.EventModel
		if unlocked.EventID != nil {
			var err error
			if event, err = tx.LockEvent(*unlocked.EventID); err != nil {
				return err
			}
		}
		booking, err := tx.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		if booking.PaymentID == nil {
			return ErrPaymentNotStarted
		}
		payment, err := loadPayment(tx.GetPayment, booking)
		if err != nil {
			return err
		}
		if payment.Status != model.PaymentStatusPending {
			return ErrPaymentAlreadyProcessed
		}
		if booking.Status != model.BookingStatusReserved {
			return ErrBookingNotPayable
		}
		if !confirmedAmount.Equal(payment.Amount) {
			return ErrPaymentAmountMismatch
		}

		now, err := tx.Now()
		if err != nil {
			return err
		}
		var reason model.PaymentFailureReason
		switch {
		case event == nil || event.Status != model.EventStatusPublished:
			reason = model.PaymentFailureEventUnavailable
		case reservationExpired(booking, now):
			reason = model.PaymentFailureReservationExpired
		case outcome == model.SimulatedPaymentFailure:
			reason = model.PaymentFailureDeclined
		}

		switch reason {
		case "":
			payment.Status = model.PaymentStatusPaid
			booking.Status = model.BookingStatusConfirmed
		case model.PaymentFailureReservationExpired:
			payment.Status, payment.FailureReason = model.PaymentStatusFailed, &reason
			booking.Status = model.BookingStatusExpired
		default:
			payment.Status, payment.FailureReason = model.PaymentStatusFailed, &reason
			booking.Status = model.BookingStatusFailed
		}
		if err := tx.UpdatePayment(payment); err != nil {
			return err
		}
		return tx.UpdateBooking(booking)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayment(ctx, bookingID, userID)
}

// GetPayment returns the stored payment process of a booking of the user.
func (s *PaymentService) GetPayment(ctx context.Context, bookingID, userID uuid.UUID) (*model.PaymentResponse, error) {
	booking, err := s.paymentRepo.GetBookingByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !ownsBooking(booking, userID) {
		return nil, ErrBookingNotFound
	}
	if booking.PaymentID == nil {
		return nil, ErrPaymentNotStarted
	}
	payment, err := loadPayment(func(id uuid.UUID) (*model.PaymentModel, error) {
		return s.paymentRepo.GetPaymentByID(ctx, id)
	}, booking)
	if err != nil {
		return nil, err
	}
	return paymentResponse(booking, payment), nil
}

// loadPayment loads the payment a booking references. The foreign key guarantees that it exists.
func loadPayment(get func(uuid.UUID) (*model.PaymentModel, error), booking *model.BookingModel) (*model.PaymentModel, error) {
	payment, err := get(*booking.PaymentID)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		return nil, fmt.Errorf("payment %s of booking %s not found", *booking.PaymentID, booking.BookingID)
	}
	return payment, nil
}

// paymentAmount is the number of tickets times the price of one ticket. Amounts that do not fit
// the payments.amount column are rejected instead of being rounded or failing in the database.
func paymentAmount(price decimal.Decimal, tickets int) (decimal.Decimal, error) {
	if tickets <= 0 {
		return decimal.Decimal{}, ErrInvalidTicketCount
	}
	amount := price.Mul(decimal.NewFromInt(int64(tickets)))
	if amount.IsNegative() || amount.GreaterThan(maxPaymentAmount) || !amount.Equal(amount.Round(2)) {
		return decimal.Decimal{}, ErrInvalidPaymentAmount
	}
	return amount, nil
}

// ownsBooking reports whether the booking exists and belongs to the user. Bookings of other users
// are reported as not found, so their IDs reveal nothing.
func ownsBooking(booking *model.BookingModel, userID uuid.UUID) bool {
	return booking != nil && booking.UserID != nil && *booking.UserID == userID
}

// reservationExpired matches bookingTx.CountOccupiedTickets: a reservation holds its tickets only
// while expires_at lies in the future.
func reservationExpired(booking *model.BookingModel, now time.Time) bool {
	return booking.ExpiresAt == nil || !booking.ExpiresAt.After(now)
}

func paymentResponse(booking *model.BookingModel, payment *model.PaymentModel) *model.PaymentResponse {
	resp := &model.PaymentResponse{
		PaymentID:       payment.PaymentID,
		BookingID:       booking.BookingID,
		EventID:         booking.EventID,
		NumberOfTickets: booking.NumberOfTickets,
		Amount:          payment.Amount,
		Status:          payment.Status,
		FailureReason:   payment.FailureReason,
		BookingStatus:   booking.Status,
		TicketsValid:    payment.Status == model.PaymentStatusPaid && booking.Status == model.BookingStatusConfirmed,
		Message:         paymentMessage(payment),
		CreatedAt:       payment.CreatedAt,
		UpdatedAt:       payment.UpdatedAt,
	}
	if payment.Status == model.PaymentStatusPending {
		resp.ExpiresAt = booking.ExpiresAt
	}
	return resp
}

func paymentMessage(payment *model.PaymentModel) string {
	switch payment.Status {
	case model.PaymentStatusPending:
		return "Payment pending. Confirm the amount to complete your booking."
	case model.PaymentStatusPaid:
		return "Payment successful. Your booking is confirmed."
	case model.PaymentStatusRefunded:
		return "The payment was refunded. Your tickets are no longer valid."
	}
	var reason model.PaymentFailureReason
	if payment.FailureReason != nil {
		reason = *payment.FailureReason
	}
	switch reason {
	case model.PaymentFailureDeclined:
		return "The payment was declined. No tickets were booked; please start a new booking."
	case model.PaymentFailureReservationExpired:
		return "Your reservation expired before the payment was completed. No tickets were booked; please start a new booking."
	case model.PaymentFailureEventUnavailable:
		return "The event is no longer available. No tickets were booked and nothing was charged."
	default:
		return "The payment failed. No tickets were booked; please start a new booking."
	}
}
