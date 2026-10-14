package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// PaymentCleanupInterval is how often RunPaymentCleanup fails pending payments of expired
// reservations.
const PaymentCleanupInterval = time.Minute

// stalePaymentBatch bounds the bookings one cleanup transaction locks; a variable for tests.
var stalePaymentBatch = 500

// maxPaymentAmount is the largest value of the payments.amount column, NUMERIC(12,2).
var maxPaymentAmount = decimal.RequireFromString("9999999999.99")

// PaymentService runs the payment process of a booking. No real payment provider is connected:
// the caller chooses the outcome of the simulated payment (EVENTHUB-235).
type PaymentService struct {
	tx       repository.Transactor
	bookings repository.BookingRepository
	payments repository.PaymentRepository
}

func NewPaymentService(tx repository.Transactor, bookings repository.BookingRepository, payments repository.PaymentRepository) *PaymentService {
	return &PaymentService{tx: tx, bookings: bookings, payments: payments}
}

// StartPayment turns a live reservation of the user into a pending payment. The amount is the
// number of tickets times the event price and is computed here only, never taken from the client.
// Starting again returns the pending payment unchanged, so the amount shown to the user is the
// amount that is charged. created reports whether a new payment was created.
//
// A pending payment whose reservation expired or whose event was withdrawn is failed right away,
// in the same transaction that detects it, and the error says why.
func (s *PaymentService) StartPayment(ctx context.Context, bookingID, userID uuid.UUID) (resp *model.PaymentResponse, created bool, err error) {
	var outcome error
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// The booking lock serializes concurrent starts, so a booking never gets two payments.
		// The event is only read: SimulatePayment locks the event before the booking.
		booking, err := tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		switch booking.Status {
		case model.BookingStatusReserved:
		case model.BookingStatusExpired:
			return ErrReservationExpired
		default:
			return ErrBookingNotPayable
		}
		var payment *model.PaymentModel
		if booking.PaymentID != nil {
			if payment, err = loadPayment(tx.Payments, booking); err != nil {
				return err
			}
			if payment.Status != model.PaymentStatusPending {
				return ErrPaymentAlreadyProcessed
			}
		}

		var event *model.EventModel
		if booking.EventID != nil {
			if event, err = tx.Events.GetEventByID(*booking.EventID); err != nil {
				return err
			}
		}
		now, err := tx.Payments.DatabaseNow()
		if err != nil {
			return err
		}
		if reason := unpayableReason(event, booking, now); reason != "" {
			outcome = ErrReservationExpired
			if reason == model.PaymentFailureEventUnavailable {
				outcome = ErrEventUnavailable
			}
			if payment == nil {
				return outcome
			}
			// Commit the failed payment and report the outcome afterwards.
			return failPayment(tx, booking, payment, reason)
		}

		if payment == nil {
			amount, err := paymentAmount(event.Price, booking.NumberOfTickets)
			if err != nil {
				return err
			}
			payment = &model.PaymentModel{
				PaymentID: uuid.New(),
				Amount:    amount,
				Status:    model.PaymentStatusPending,
			}
			if err := tx.Payments.CreatePayment(payment); err != nil {
				return err
			}
			booking.PaymentID = &payment.PaymentID
			if err := tx.Bookings.UpdateBooking(booking); err != nil {
				return err
			}
			created = true
		}
		// Read the stored result inside the transaction: once it commits, the response is ready.
		resp, err = readPayment(tx.Payments, bookingID)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if outcome != nil {
		return nil, false, outcome
	}
	return resp, created, nil
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
	unlocked, err := s.bookings.GetBookingByID(bookingID)
	if err != nil {
		return nil, err
	}
	if !ownsBooking(unlocked, userID) {
		return nil, ErrBookingNotFound
	}

	var resp *model.PaymentResponse
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// The event lock serializes the confirmation with new reservations: once a reservation has
		// expired, ReserveTickets may hand its tickets to others, so it must not be confirmed.
		var event *model.EventModel
		if unlocked.EventID != nil {
			var err error
			if event, err = tx.Events.LockEvent(*unlocked.EventID); err != nil {
				return err
			}
		}
		booking, err := tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		if booking.PaymentID == nil {
			return ErrPaymentNotStarted
		}
		payment, err := loadPayment(tx.Payments, booking)
		if err != nil {
			return err
		}
		if payment.Status != model.PaymentStatusPending {
			return ErrPaymentAlreadyProcessed
		}
		if booking.Status != model.BookingStatusReserved {
			return ErrBookingNotPayable
		}
		// An unbounded exponent can never match a stored amount, and comparing it would stall.
		if !model.IsBoundedDecimal(confirmedAmount) || !confirmedAmount.Equal(payment.Amount) {
			return ErrPaymentAmountMismatch
		}

		now, err := tx.Payments.DatabaseNow()
		if err != nil {
			return err
		}
		reason := unpayableReason(event, booking, now)
		if reason == "" && outcome == model.SimulatedPaymentFailure {
			reason = model.PaymentFailureDeclined
		}
		if reason != "" {
			err = failPayment(tx, booking, payment, reason)
		} else {
			err = confirmPayment(tx, booking, payment)
		}
		if err != nil {
			return err
		}
		// Read the stored result inside the transaction: once it commits, the response is ready.
		resp, err = readPayment(tx.Payments, bookingID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPayment returns the stored payment process of a booking of the user.
func (s *PaymentService) GetPayment(ctx context.Context, bookingID, userID uuid.UUID) (*model.PaymentResponse, error) {
	view, err := s.payments.GetBookingPayment(bookingID)
	if err != nil {
		return nil, err
	}
	if view == nil || !ownsBooking(view.Booking, userID) {
		return nil, ErrBookingNotFound
	}
	if view.Payment == nil {
		return nil, ErrPaymentNotStarted
	}
	return paymentResponse(view), nil
}

// ExpireStalePayments fails pending payments whose reservation has expired, so no payment process
// stays open after its tickets were released. It returns the number of failed payments.
func (s *PaymentService) ExpireStalePayments(ctx context.Context) (int, error) {
	total := 0
	for {
		n := 0
		err := s.tx.InTransaction(ctx, func(tx repository.Tx) error {
			bookings, err := tx.Payments.LockStalePendingBookings(stalePaymentBatch)
			if err != nil {
				return err
			}
			for _, booking := range bookings {
				payment, err := loadPayment(tx.Payments, booking)
				if err != nil {
					return err
				}
				var event *model.EventModel
				if booking.EventID != nil {
					if event, err = tx.Events.GetEventByID(*booking.EventID); err != nil {
						return err
					}
				}
				// The reservation has expired, unless the event is gone, which takes precedence
				// like in SimulatePayment.
				reason := model.PaymentFailureReservationExpired
				if !eventOpenForBooking(event) {
					reason = model.PaymentFailureEventUnavailable
				}
				if err := failPayment(tx, booking, payment, reason); err != nil {
					return err
				}
			}
			n = len(bookings)
			return nil
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < stalePaymentBatch {
			return total, nil
		}
	}
}

// RunPaymentCleanup calls ExpireStalePayments every interval until ctx is done.
func (s *PaymentService) RunPaymentCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.ExpireStalePayments(ctx)
			if err != nil {
				slog.Error("payment cleanup failed", "error", err)
			} else if n > 0 {
				slog.Info("payment cleanup closed stale payments of expired reservations", "count", n)
			}
		}
	}
}

// unpayableReason reports why a reserved booking can no longer be paid, or "" if it can. An
// unavailable event takes precedence over an expired reservation.
func unpayableReason(event *model.EventModel, booking *model.BookingModel, now time.Time) model.PaymentFailureReason {
	switch {
	case !eventOpenForBooking(event):
		return model.PaymentFailureEventUnavailable
	case reservationExpired(booking, now):
		return model.PaymentFailureReservationExpired
	}
	return ""
}

func eventOpenForBooking(event *model.EventModel) bool {
	return event != nil && event.Status == model.EventStatusPublished
}

func confirmPayment(tx repository.Tx, booking *model.BookingModel, payment *model.PaymentModel) error {
	payment.Status = model.PaymentStatusPaid
	booking.Status = model.BookingStatusConfirmed
	if err := tx.Payments.UpdatePayment(payment); err != nil {
		return err
	}
	return tx.Bookings.UpdateBooking(booking)
}

// failPayment fails the payment and leaves the booking without a ticket claim: expired for an
// expired reservation, failed otherwise. Either way its tickets are released.
func failPayment(tx repository.Tx, booking *model.BookingModel, payment *model.PaymentModel, reason model.PaymentFailureReason) error {
	payment.Status, payment.FailureReason = model.PaymentStatusFailed, &reason
	booking.Status = model.BookingStatusFailed
	if reason == model.PaymentFailureReservationExpired {
		booking.Status = model.BookingStatusExpired
	}
	if err := tx.Payments.UpdatePayment(payment); err != nil {
		return err
	}
	return tx.Bookings.UpdateBooking(booking)
}

// loadPayment loads the payment a booking references. The foreign key guarantees that it exists.
func loadPayment(payments repository.PaymentRepository, booking *model.BookingModel) (*model.PaymentModel, error) {
	payment, err := payments.GetPayment(*booking.PaymentID)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		return nil, fmt.Errorf("payment %s of booking %s not found", *booking.PaymentID, booking.BookingID)
	}
	return payment, nil
}

// readPayment reads the stored payment process of a booking that has one.
func readPayment(payments repository.PaymentRepository, bookingID uuid.UUID) (*model.PaymentResponse, error) {
	view, err := payments.GetBookingPayment(bookingID)
	if err != nil {
		return nil, err
	}
	if view == nil || view.Payment == nil {
		return nil, fmt.Errorf("payment of booking %s not found after writing it", bookingID)
	}
	return paymentResponse(view), nil
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

// eventHonorsTickets reports whether the event still takes place, or took place, so that its
// tickets are valid.
func eventHonorsTickets(eventStatus *model.EventStatus) bool {
	return eventStatus != nil && (*eventStatus == model.EventStatusPublished || *eventStatus == model.EventStatusCompleted)
}

func paymentResponse(view *repository.BookingPayment) *model.PaymentResponse {
	booking, payment := view.Booking, view.Payment
	ticketsValid := payment.Status == model.PaymentStatusPaid &&
		booking.Status == model.BookingStatusConfirmed &&
		eventHonorsTickets(view.EventStatus)
	resp := &model.PaymentResponse{
		PaymentID:       payment.PaymentID,
		BookingID:       booking.BookingID,
		EventID:         booking.EventID,
		NumberOfTickets: booking.NumberOfTickets,
		Amount:          payment.Amount,
		Status:          payment.Status,
		FailureReason:   payment.FailureReason,
		BookingStatus:   booking.Status,
		TicketsValid:    ticketsValid,
		Message:         paymentMessage(view, ticketsValid),
		CreatedAt:       payment.CreatedAt,
		UpdatedAt:       payment.UpdatedAt,
	}
	if payment.Status == model.PaymentStatusPending {
		resp.ExpiresAt = booking.ExpiresAt
	}
	return resp
}

// paymentMessage words the result for the user. It never contradicts the result: only valid
// tickets are reported as a confirmed booking, and only a payable booking is asked to complete
// its payment. Expiry is judged by the database clock like everywhere else.
func paymentMessage(view *repository.BookingPayment, ticketsValid bool) string {
	booking, payment := view.Booking, view.Payment
	switch payment.Status {
	case model.PaymentStatusPending:
		switch {
		case view.EventStatus == nil || *view.EventStatus != model.EventStatusPublished:
			return "The event is no longer available, so the payment can no longer be completed. No tickets were booked and nothing was charged."
		case booking.Status != model.BookingStatusReserved || reservationExpired(booking, view.Now):
			return "Your reservation is no longer valid, so the payment can no longer be completed. No tickets were booked; please start a new booking."
		}
		return "Payment pending. Confirm the amount to complete your booking."
	case model.PaymentStatusPaid:
		switch {
		case ticketsValid:
			return "Payment successful. Your booking is confirmed."
		case booking.Status == model.BookingStatusConfirmed:
			return "The payment was received, but the event is no longer taking place. Your tickets are not valid."
		}
		return "The payment was received, but the booking is no longer active. Your tickets are not valid."
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
