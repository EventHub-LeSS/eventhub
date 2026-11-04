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
	ErrPaymentNotFound         = errors.New("payment not found")
	ErrBookingNotFound         = errors.New("booking not found")
	ErrBookingNotPayable       = errors.New("booking is not awaiting payment")
	ErrReservationExpired      = errors.New("reservation has expired, please book again")
	ErrEventUnavailable        = errors.New("event is no longer available for booking")
	ErrPaymentAlreadyProcessed = errors.New("payment has already been processed")
	ErrPaymentAmountMismatch   = errors.New("amount does not match the amount of the payment")
	ErrInvalidPaymentAmount    = errors.New("payment amount is out of range")
	ErrInvalidPaymentOutcome   = errors.New("outcome must be success or failure")
)

// PaymentCleanupInterval is how often RunPaymentCleanup fails pending payments of expired
// reservations.
const PaymentCleanupInterval = time.Minute

// stalePaymentBatch bounds the payments one cleanup run processes; a variable for tests.
var stalePaymentBatch = 500

// maxPaymentAmount is the largest value of the payments.amount column, NUMERIC(12,2).
var maxPaymentAmount = decimal.RequireFromString("9999999999.99")

// PaymentService runs the payment process of a cart checkout. No real payment provider is
// connected: the caller chooses the outcome of the simulated payment (EVENTHUB-235) and pays by
// credit card, SEPA direct debit or PayPal with the simulated payment provider of
// payment_simulation.go. One payment covers the bookings of a whole checkout and completes them
// together, or not at all.
type PaymentService struct {
	tx       repository.Transactor
	bookings repository.BookingRepository
	payments repository.PaymentRepository
}

func NewPaymentService(tx repository.Transactor, bookings repository.BookingRepository, payments repository.PaymentRepository) *PaymentService {
	return &PaymentService{tx: tx, bookings: bookings, payments: payments}
}

// SimulatePayment completes the pending payment like SimulatePaymentWithMethod, without a payment
// method: the outcome alone decides and no payment method is recorded.
func (s *PaymentService) SimulatePayment(ctx context.Context, paymentID, userID uuid.UUID, outcome model.SimulatedPaymentOutcome, confirmedAmount decimal.Decimal) (*model.PaymentResponse, error) {
	return s.SimulatePaymentWithMethod(ctx, paymentID, userID, outcome, confirmedAmount, nil)
}

// SimulatePaymentWithMethod completes the pending payment of the user's checkout with the
// simulated payment provider. confirmedAmount is the amount shown to the user: unless it equals
// the stored amount, nothing changes. method is the payment method with its details; invalid
// details are rejected before anything changes. The provider declines the payment when the
// outcome is failure or the details are its declining test data, and charges it otherwise; either
// way the payment records the method, the masked account and the provider's reference. Without a
// method the outcome alone decides and no method is recorded.
//
// A successful payment confirms every booking of the payment. A failed payment leaves no ticket
// claim: the bookings fail or expire and their tickets are released. The payment fails as well,
// whatever the outcome and without asking the provider, when an event was withdrawn or a
// reservation expired meanwhile.
func (s *PaymentService) SimulatePaymentWithMethod(ctx context.Context, paymentID, userID uuid.UUID, outcome model.SimulatedPaymentOutcome, confirmedAmount decimal.Decimal, method *model.PaymentMethodInput) (*model.PaymentResponse, error) {
	if outcome != model.SimulatedPaymentSuccess && outcome != model.SimulatedPaymentFailure {
		return nil, ErrInvalidPaymentOutcome
	}
	charge, err := prepareCharge(method, outcome, time.Now())
	if err != nil {
		return nil, err
	}

	// The payment is read without a lock to learn its events and bookings, which never change.
	// The transaction then locks in the canonical order of every payment transaction: events by
	// event_id, bookings by booking_id, then the payment. That order rules out deadlocks.
	view, err := s.payments.GetPaymentView(paymentID)
	if err != nil {
		return nil, err
	}
	if view == nil || !ownsPayment(view, userID) {
		return nil, ErrPaymentNotFound
	}
	eventIDs, bookingIDs := viewIDs(view)

	var resp *model.PaymentResponse
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// The event locks serialize the confirmation with new reservations: once a reservation
		// has expired, AddToCart may hand its tickets to others, so it must not be confirmed.
		events, err := tx.Events.LockEvents(eventIDs)
		if err != nil {
			return err
		}
		eventByID := make(map[uuid.UUID]*model.EventModel, len(events))
		for _, event := range events {
			eventByID[event.EventID] = event
		}
		bookings, err := tx.Bookings.LockBookings(bookingIDs)
		if err != nil {
			return err
		}
		if !ownsBookings(bookings, userID) {
			return ErrPaymentNotFound
		}
		payment, err := tx.Payments.LockPayment(paymentID)
		if err != nil {
			return err
		}
		if payment == nil {
			return ErrPaymentNotFound
		}
		if payment.Status != model.PaymentStatusPending {
			return ErrPaymentAlreadyProcessed
		}
		for _, booking := range bookings {
			if booking.Status != model.BookingStatusReserved {
				return ErrBookingNotPayable
			}
		}
		// An unbounded exponent can never match a stored amount, and comparing it would stall.
		if !model.IsBoundedDecimal(confirmedAmount) || !confirmedAmount.Equal(payment.Amount) {
			return ErrPaymentAmountMismatch
		}

		now, err := tx.Payments.DatabaseNow()
		if err != nil {
			return err
		}
		reason := unpayableReason(eventByID, bookings, now)
		if reason == "" {
			// Only a payable checkout reaches the payment provider.
			declined := outcome == model.SimulatedPaymentFailure
			if charge != nil {
				declined = charge.declines(outcome)
				charge.record(payment)
			}
			if declined {
				reason = model.PaymentFailureDeclined
			}
		}
		if reason != "" {
			err = failPaymentBookings(tx, payment, bookings, reason, now)
		} else {
			err = confirmPaymentBookings(tx, payment, bookings)
		}
		if err != nil {
			return err
		}
		// Read the stored result inside the transaction: once it commits, the response is ready.
		resp, err = readPaymentView(tx.Payments, paymentID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPayment returns the stored payment process of the user's checkout.
func (s *PaymentService) GetPayment(ctx context.Context, paymentID, userID uuid.UUID) (*model.PaymentResponse, error) {
	view, err := s.payments.GetPaymentView(paymentID)
	if err != nil {
		return nil, err
	}
	if view == nil || !ownsPayment(view, userID) {
		return nil, ErrPaymentNotFound
	}
	return paymentResponse(view), nil
}

// ExpireStalePayments fails pending payments whose reservations have expired, so no payment
// process stays open after its tickets were released. A checkout gives all of its bookings one
// expiry, so one expired reservation ends the whole payment. Payments that are being completed,
// cancelled or cleaned up elsewhere are deferred to the next run. It returns the number of
// failed payments.
func (s *PaymentService) ExpireStalePayments(ctx context.Context) (int, error) {
	ids, err := s.payments.StalePaymentIDs(stalePaymentBatch)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, paymentID := range ids {
		failed, err := s.expireStalePayment(ctx, paymentID)
		if err != nil {
			return total, err
		}
		if failed {
			total++
		}
	}
	return total, nil
}

// expireStalePayment fails one pending payment whose reservation expired. Every lock is taken
// with FOR UPDATE SKIP LOCKED in the canonical order — events, bookings, payment — so a payment
// busy elsewhere is deferred instead of being waited for.
func (s *PaymentService) expireStalePayment(ctx context.Context, paymentID uuid.UUID) (bool, error) {
	// The snapshot without locks learns the events and bookings to lock; the checks run again
	// under the locks below.
	view, err := s.payments.GetPaymentView(paymentID)
	if err != nil {
		return false, err
	}
	if view == nil || view.Payment.Status != model.PaymentStatusPending {
		return false, nil
	}
	eventIDs, bookingIDs := viewIDs(view)

	failed := false
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		events, err := tx.Events.TryLockEvents(eventIDs)
		if err != nil {
			return err
		}
		if len(events) < len(eventIDs) {
			return nil // busy elsewhere; the next run tries again
		}
		eventByID := make(map[uuid.UUID]*model.EventModel, len(events))
		for _, event := range events {
			eventByID[event.EventID] = event
		}
		bookings, err := tx.Bookings.TryLockBookings(bookingIDs)
		if err != nil {
			return err
		}
		if len(bookings) < len(bookingIDs) {
			return nil // busy elsewhere; the next run tries again
		}
		payment, err := tx.Payments.TryLockPayment(paymentID)
		if err != nil {
			return err
		}
		if payment == nil {
			return nil // busy elsewhere; the next run tries again
		}
		if payment.Status != model.PaymentStatusPending {
			return nil // completed or failed meanwhile
		}

		now, err := tx.Payments.DatabaseNow()
		if err != nil {
			return err
		}
		stale := false
		for _, booking := range bookings {
			if booking.Status == model.BookingStatusReserved && reservationExpired(booking, now) {
				stale = true
			}
		}
		if !stale {
			return nil // the reservation was refreshed or completed meanwhile
		}
		// An event that no longer takes place fails the payment like a completion would, and
		// takes precedence over the expiry, like everywhere else.
		reason := model.PaymentFailureReservationExpired
		if unpayableReason(eventByID, bookings, now) == model.PaymentFailureEventUnavailable {
			reason = model.PaymentFailureEventUnavailable
		}
		if err := failPaymentBookings(tx, payment, bookings, reason, now); err != nil {
			return err
		}
		failed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return failed, nil
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

// unpayableReason reports why a pending payment can no longer be completed, or "" if it can. An
// unavailable event takes precedence over an expired reservation, and every event and every
// booking of the payment is judged, because the payment completes all of them together.
func unpayableReason(eventByID map[uuid.UUID]*model.EventModel, bookings []*model.BookingModel, now time.Time) model.PaymentFailureReason {
	for _, booking := range bookings {
		var event *model.EventModel
		if booking.EventID != nil {
			event = eventByID[*booking.EventID]
		}
		if !eventOpenForBooking(event) {
			return model.PaymentFailureEventUnavailable
		}
	}
	for _, booking := range bookings {
		if reservationExpired(booking, now) {
			return model.PaymentFailureReservationExpired
		}
	}
	return ""
}

func eventOpenForBooking(event *model.EventModel) bool {
	return event != nil && event.Status == model.EventStatusPublished
}

// confirmPaymentBookings pays the payment and confirms every booking of it.
func confirmPaymentBookings(tx repository.Tx, payment *model.PaymentModel, bookings []*model.BookingModel) error {
	payment.Status = model.PaymentStatusPaid
	for _, booking := range bookings {
		booking.Status = model.BookingStatusConfirmed
		if err := tx.Bookings.UpdateBooking(booking); err != nil {
			return err
		}
	}
	return tx.Payments.UpdatePayment(payment)
}

// failPaymentBookings fails the payment and leaves no booking with a ticket claim: expired for
// an expired reservation, failed otherwise. Either way all tickets are released.
func failPaymentBookings(tx repository.Tx, payment *model.PaymentModel, bookings []*model.BookingModel, reason model.PaymentFailureReason, now time.Time) error {
	payment.Status, payment.FailureReason = model.PaymentStatusFailed, &reason
	for _, booking := range bookings {
		booking.Status = model.BookingStatusFailed
		if reason == model.PaymentFailureReservationExpired || reservationExpired(booking, now) {
			booking.Status = model.BookingStatusExpired
		}
		if err := tx.Bookings.UpdateBooking(booking); err != nil {
			return err
		}
	}
	return tx.Payments.UpdatePayment(payment)
}

// viewIDs returns the event IDs and booking IDs of a payment view, the event IDs without
// duplicates.
func viewIDs(view *repository.PaymentView) (eventIDs, bookingIDs []uuid.UUID) {
	seen := make(map[uuid.UUID]bool, len(view.Items))
	eventIDs = make([]uuid.UUID, 0, len(view.Items))
	bookingIDs = make([]uuid.UUID, 0, len(view.Items))
	for _, item := range view.Items {
		bookingIDs = append(bookingIDs, item.Booking.BookingID)
		if item.Booking.EventID != nil && !seen[*item.Booking.EventID] {
			seen[*item.Booking.EventID] = true
			eventIDs = append(eventIDs, *item.Booking.EventID)
		}
	}
	return eventIDs, bookingIDs
}

// ownsPayment reports whether the payment exists and belongs to the user. A payment covers only
// the bookings of one checkout, which are one user's. Payments of other users are reported as
// not found, so their IDs reveal nothing.
func ownsPayment(view *repository.PaymentView, userID uuid.UUID) bool {
	return ownsBookings(viewBookings(view), userID)
}

// viewBookings returns the bookings of a payment view.
func viewBookings(view *repository.PaymentView) []*model.BookingModel {
	bookings := make([]*model.BookingModel, 0, len(view.Items))
	for _, item := range view.Items {
		bookings = append(bookings, item.Booking)
	}
	return bookings
}

// ownsBookings reports whether there are bookings and all of them belong to the user.
func ownsBookings(bookings []*model.BookingModel, userID uuid.UUID) bool {
	if len(bookings) == 0 {
		return false
	}
	for _, booking := range bookings {
		if !ownsBooking(booking, userID) {
			return false
		}
	}
	return true
}

// readPaymentView reads the stored payment process of a payment that exists.
func readPaymentView(payments repository.PaymentRepository, paymentID uuid.UUID) (*model.PaymentResponse, error) {
	view, err := payments.GetPaymentView(paymentID)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("payment %s not found after writing it", paymentID)
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
	if err := validPaymentAmount(amount); err != nil {
		return decimal.Decimal{}, err
	}
	return amount, nil
}

// validPaymentAmount rejects amounts that do not fit the payments.amount column, NUMERIC(12,2),
// instead of rounding them or letting the database fail.
func validPaymentAmount(amount decimal.Decimal) error {
	if amount.IsNegative() || amount.GreaterThan(maxPaymentAmount) || !amount.Equal(amount.Round(2)) {
		return ErrInvalidPaymentAmount
	}
	return nil
}

// ownsBooking reports whether the booking exists and belongs to the user. Bookings of other users
// are reported as not found, so their IDs reveal nothing.
func ownsBooking(booking *model.BookingModel, userID uuid.UUID) bool {
	return booking != nil && booking.UserID != nil && *booking.UserID == userID
}

// reservationExpired matches bookingRepository.CountOccupiedTickets: a reservation holds its
// tickets only while expires_at lies in the future.
func reservationExpired(booking *model.BookingModel, now time.Time) bool {
	return booking.ExpiresAt == nil || !booking.ExpiresAt.After(now)
}

// eventHonorsTickets reports whether the event still takes place, or took place, so that its
// tickets are valid.
func eventHonorsTickets(eventStatus *model.EventStatus) bool {
	return eventStatus != nil && (*eventStatus == model.EventStatusPublished || *eventStatus == model.EventStatusCompleted)
}

// hasValidTickets is the one rule for whether a booking holds valid tickets: its payment is paid,
// the booking is confirmed and the event takes place or took place. The payment result and the
// ticket views of the visitor (EVENTHUB-236) both use it, so they never contradict each other.
func hasValidTickets(view *repository.BookingPayment) bool {
	return view.Payment != nil && view.Payment.Status == model.PaymentStatusPaid &&
		view.Booking.Status == model.BookingStatusConfirmed &&
		eventHonorsTickets(view.EventStatus)
}

// paymentResponse builds the payment view of a whole checkout: every booking with its line
// amount and validity, and the payment with the amount that was charged.
func paymentResponse(view *repository.PaymentView) *model.PaymentResponse {
	payment := view.Payment
	resp := &model.PaymentResponse{
		PaymentID:      payment.PaymentID,
		Amount:         payment.Amount,
		Items:          make([]model.PaymentItemResponse, 0, len(view.Items)),
		Status:         payment.Status,
		FailureReason:  payment.FailureReason,
		PaymentMethod:  payment.PaymentMethod,
		PaymentAccount: payment.PaymentAccount,
		CardBrand:      payment.CardBrand,
		TransactionID:  payment.TransactionID,
		TicketsValid:   len(view.Items) > 0,
		CreatedAt:      payment.CreatedAt,
		UpdatedAt:      payment.UpdatedAt,
	}
	for _, item := range view.Items {
		booking := item.Booking
		single := &repository.BookingPayment{Booking: booking, Payment: payment, EventStatus: item.EventStatus, Now: view.Now}
		valid := hasValidTickets(single)
		lineAmount := decimal.Zero
		if booking.LineAmount != nil {
			lineAmount = *booking.LineAmount
		}
		out := model.PaymentItemResponse{
			BookingID:       booking.BookingID,
			Event:           paymentEvent(item),
			NumberOfTickets: booking.NumberOfTickets,
			LineAmount:      lineAmount,
			BookingStatus:   booking.Status,
			TicketsValid:    valid,
		}
		if !valid {
			resp.TicketsValid = false
		}
		resp.Items = append(resp.Items, out)
	}
	if payment.Status == model.PaymentStatusPending {
		for _, item := range view.Items {
			if item.Booking.ExpiresAt != nil {
				if resp.ExpiresAt == nil || item.Booking.ExpiresAt.Before(*resp.ExpiresAt) {
					expiresAt := *item.Booking.ExpiresAt
					resp.ExpiresAt = &expiresAt
				}
			}
		}
	}
	resp.Message = paymentMessage(view, resp.TicketsValid)
	return resp
}

// paymentEvent maps one payment item to the event shape of the views; nil when the event is gone.
func paymentEvent(item *repository.PaymentItem) *model.OwnBookingEvent {
	if item.Event == nil {
		return nil
	}
	return ownEvent(item.Event, item.Location)
}

// paymentMessage words the result of a whole checkout for the user. It never contradicts the
// result: only valid tickets are reported as confirmed, and only a payable checkout is asked to
// complete its payment. Expiry is judged by the database clock like everywhere else.
func paymentMessage(view *repository.PaymentView, ticketsValid bool) string {
	payment := view.Payment
	switch payment.Status {
	case model.PaymentStatusPending:
		switch {
		case pendingEventGone(view):
			return "The event is no longer available, so the payment can no longer be completed. No tickets were booked and nothing was charged."
		case pendingReservationGone(view):
			return "Your reservation is no longer valid, so the payment can no longer be completed. No tickets were booked; please put the tickets in the cart again."
		}
		return "Payment pending. Confirm the amount to book your tickets."
	case model.PaymentStatusPaid:
		switch {
		case ticketsValid:
			return "Payment successful. Your tickets are confirmed."
		case anyBooking(view, model.BookingStatusConfirmed):
			return "The payment was received, but an event is no longer taking place. The tickets for it are not valid."
		}
		return "The payment was received, but the bookings are no longer active. Your tickets are not valid."
	case model.PaymentStatusRefunded:
		return "The payment was refunded. Your tickets are no longer valid."
	}
	var reason model.PaymentFailureReason
	if payment.FailureReason != nil {
		reason = *payment.FailureReason
	}
	switch reason {
	case model.PaymentFailureBookingCancelled:
		return "Your booking was cancelled. No tickets were booked and nothing was charged."
	case model.PaymentFailureDeclined:
		return declinedMessage(payment.PaymentMethod)
	case model.PaymentFailureReservationExpired:
		return "Your reservation expired before the payment was completed. No tickets were booked; please put the tickets in the cart again."
	case model.PaymentFailureEventUnavailable:
		return "The event is no longer available. No tickets were booked and nothing was charged."
	default:
		return "The payment failed. No tickets were booked; please put the tickets in the cart again."
	}
}

// declinedMessage words a payment the simulated payment provider declined, by payment method.
func declinedMessage(method *model.PaymentMethod) string {
	const retry = " No tickets were booked; please put the tickets in the cart again."
	if method == nil {
		return "The payment was declined." + retry
	}
	switch *method {
	case model.PaymentMethodCreditCard:
		return "Your credit card was declined." + retry
	case model.PaymentMethodDirectDebit:
		return "Your bank rejected the direct debit." + retry
	case model.PaymentMethodPayPal:
		return "PayPal declined the payment." + retry
	}
	return "The payment was declined." + retry
}

// bookingMessage words the result for one booking of a payment, the message of the own-booking
// views. It delegates to paymentMessage with the booking as the only item, so the booking view
// states exactly what the payment result states and the two wordings cannot drift apart.
func bookingMessage(view *repository.BookingPayment, ticketsValid bool) string {
	item := &repository.PaymentItem{Booking: view.Booking, EventStatus: view.EventStatus}
	return paymentMessage(&repository.PaymentView{
		Payment: view.Payment,
		Items:   []*repository.PaymentItem{item},
		Now:     view.Now,
	}, ticketsValid)
}

// pendingEventGone reports whether an event of a pending payment no longer takes place.
func pendingEventGone(view *repository.PaymentView) bool {
	for _, item := range view.Items {
		if item.EventStatus == nil || *item.EventStatus != model.EventStatusPublished {
			return true
		}
	}
	return false
}

// pendingReservationGone reports whether a booking of a pending payment is no longer a live
// reservation.
func pendingReservationGone(view *repository.PaymentView) bool {
	for _, item := range view.Items {
		if item.Booking.Status != model.BookingStatusReserved || reservationExpired(item.Booking, view.Now) {
			return true
		}
	}
	return false
}

// anyBooking reports whether a booking of the payment is in the given status.
func anyBooking(view *repository.PaymentView, status model.BookingStatus) bool {
	for _, item := range view.Items {
		if item.Booking.Status == status {
			return true
		}
	}
	return false
}
