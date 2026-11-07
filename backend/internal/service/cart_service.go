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

// MaxTicketsPerEvent caps the tickets one cart item — one reservation — may hold, so the total
// stays reviewable at a glance. The frontend enforces the same limit.
const MaxTicketsPerEvent = 10

var (
	// ErrCartEmpty maps to 422: the checkout needs at least one live reservation.
	ErrCartEmpty = errors.New("cart is empty")
	// ErrCartItemNotFound maps to 404: unknown, foreign and no-longer-open reservations look
	// the same, so their IDs reveal nothing.
	ErrCartItemNotFound = errors.New("cart item not found")
	// ErrTicketLimitExceeded maps to 422: more tickets for one event than the cart allows.
	ErrTicketLimitExceeded = fmt.Errorf("at most %d tickets per event fit in the cart", MaxTicketsPerEvent)
)

// CartService runs the cart of a user. The cart is the set of the user's live reservations
// (status reserved, no payment, expires_at in the future): adding tickets to the cart reserves
// their seats right away, for the reservation TTL, so no two carts can claim the same seats.
// The checkout turns all live items into one pending payment with a per-event price, and the
// payment completes them together (see PaymentService).
type CartService struct {
	tx             repository.Transactor
	bookings       repository.BookingRepository
	carts          repository.CartRepository
	payments       repository.PaymentRepository
	reservationTTL time.Duration
}

func NewCartService(tx repository.Transactor, bookings repository.BookingRepository, carts repository.CartRepository, payments repository.PaymentRepository, reservationTTL time.Duration) *CartService {
	if reservationTTL <= 0 {
		reservationTTL = DefaultReservationTTL
	}
	return &CartService{tx: tx, bookings: bookings, carts: carts, payments: payments, reservationTTL: reservationTTL}
}

// GetCart returns the live cart of the user: every reservation without a payment that has not
// expired, with the current price and availability of its event.
func (s *CartService) GetCart(ctx context.Context, userID uuid.UUID) (*model.CartResponse, error) {
	items, err := s.carts.ListCart(userID)
	if err != nil {
		return nil, err
	}
	resp := cartResponse(items)
	// The items of an open checkout left the cart but keep their seats; the cart reports the
	// checkout, so returning from the payment view does not look like lost items.
	resp.OpenPayment, err = s.carts.OpenPayment(userID)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// AddToCart reserves tickets of a published event for the user, like a booking did before, and
// thus adds them to the cart. Adding to an item that already exists raises its quantity and
// refreshes its reservation; an item whose reservation expired is replaced. The response is the
// cart after the change.
func (s *CartService) AddToCart(ctx context.Context, userID, eventID uuid.UUID, tickets int) (*model.CartResponse, error) {
	if tickets <= 0 {
		return nil, ErrInvalidTicketCount
	}
	if tickets > MaxTicketsPerEvent {
		return nil, ErrTicketLimitExceeded
	}

	err := s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// The event lock serializes the reservation with every other capacity change of the
		// event, so the count below is exact at this moment.
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.Status != model.EventStatusPublished {
			return ErrEventNotPublished
		}
		// Events of organizations that an admin has not approved yet are not public.
		active, err := tx.Events.OrganizerIsActive(eventID)
		if err != nil {
			return err
		}
		if !active {
			return ErrEventNotFound
		}

		existing, err := tx.Bookings.LatestOpenReservation(userID, eventID)
		if err != nil {
			return err
		}
		occupied, err := tx.Bookings.CountOccupiedTickets(eventID)
		if err != nil {
			return err
		}
		available := int64(event.Capacity) - occupied
		if available < 0 {
			available = 0
		}

		if existing != nil && !reservationExpired(existing, time.Now()) {
			// A live reservation: its tickets are part of the occupied count, so only the
			// added tickets need free seats. Its expiry is refreshed, like every change.
			target := existing.NumberOfTickets + tickets
			if target > MaxTicketsPerEvent {
				return ErrTicketLimitExceeded
			}
			if available < int64(tickets) {
				return &CapacityExceededError{Requested: tickets, Available: available}
			}
			expiresAt := time.Now().Add(s.reservationTTL)
			existing.NumberOfTickets = target
			existing.ExpiresAt = &expiresAt
			return tx.Bookings.UpdateBooking(existing)
		}

		// No reservation to build on — an expired one holds no seats and is replaced — so all
		// of the tickets need free seats.
		if available < int64(tickets) {
			return &CapacityExceededError{Requested: tickets, Available: available}
		}
		if existing != nil {
			expiresAt := time.Now().Add(s.reservationTTL)
			existing.NumberOfTickets = tickets
			existing.ExpiresAt = &expiresAt
			return tx.Bookings.UpdateBooking(existing)
		}
		expiresAt := time.Now().Add(s.reservationTTL)
		return tx.Bookings.CreateBooking(&model.BookingModel{
			BookingID:       uuid.New(),
			UserID:          &userID,
			EventID:         &eventID,
			NumberOfTickets: tickets,
			Status:          model.BookingStatusReserved,
			ExpiresAt:       &expiresAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

// UpdateQuantity sets the ticket count of one cart item. The reservation keeps its seats while
// it lives, so the item can always keep what it holds; more tickets need free seats. Changing
// the quantity refreshes the reservation. The response is the cart after the change.
func (s *CartService) UpdateQuantity(ctx context.Context, userID, bookingID uuid.UUID, quantity int) (*model.CartResponse, error) {
	if quantity <= 0 {
		return nil, ErrInvalidTicketCount
	}
	if quantity > MaxTicketsPerEvent {
		return nil, ErrTicketLimitExceeded
	}

	// The item is read without a lock to learn its event, which never changes; the transaction
	// below locks the event before the booking, the canonical order.
	item, err := s.bookings.GetBookingByID(bookingID)
	if err != nil {
		return nil, err
	}
	if !isOpenCartItem(item, userID) {
		return nil, ErrCartItemNotFound
	}

	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(*item.EventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventUnavailable
		}
		booking, err := tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !isOpenCartItem(booking, userID) {
			return ErrCartItemNotFound
		}
		if reservationExpired(booking, time.Now()) {
			return ErrReservationExpired
		}
		occupied, err := tx.Bookings.CountOccupiedTickets(*booking.EventID)
		if err != nil {
			return err
		}
		// The item's own tickets are part of the occupied count, so it may always keep them.
		available := int64(event.Capacity) - occupied + int64(booking.NumberOfTickets)
		if available < int64(quantity) {
			return &CapacityExceededError{Requested: quantity, Available: available}
		}
		expiresAt := time.Now().Add(s.reservationTTL)
		booking.NumberOfTickets = quantity
		booking.ExpiresAt = &expiresAt
		return tx.Bookings.UpdateBooking(booking)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

// RemoveFromCart deletes one cart item and releases its reserved seats. Only reservations
// without a payment can be removed; a paid or in-checkout booking is cancelled via
// POST /bookings/{bookingId}/cancel instead.
func (s *CartService) RemoveFromCart(ctx context.Context, userID, bookingID uuid.UUID) (*model.CartResponse, error) {
	item, err := s.bookings.GetBookingByID(bookingID)
	if err != nil {
		return nil, err
	}
	if !isOpenCartItem(item, userID) {
		return nil, ErrCartItemNotFound
	}

	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		booking, err := tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !isOpenCartItem(booking, userID) {
			return ErrCartItemNotFound
		}
		return tx.Bookings.DeleteBooking(bookingID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

// ClearCart removes every cart item of the user and releases their seats. Reservations that
// belong to an open checkout are not cart items and stay untouched.
func (s *CartService) ClearCart(ctx context.Context, userID uuid.UUID) (*model.CartResponse, error) {
	open, err := s.bookings.OpenReservations(userID)
	if err != nil {
		return nil, err
	}
	if len(open) > 0 {
		err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
			// The events are locked in the canonical order before the bookings, so a parallel
			// checkout or add of the same events cannot deadlock.
			eventIDs := make([]uuid.UUID, 0, len(open))
			bookingIDs := make([]uuid.UUID, 0, len(open))
			for _, booking := range open {
				if booking.EventID != nil {
					eventIDs = append(eventIDs, *booking.EventID)
				}
				bookingIDs = append(bookingIDs, booking.BookingID)
			}
			if _, err := tx.Events.LockEvents(eventIDs); err != nil {
				return err
			}
			bookings, err := tx.Bookings.LockBookings(bookingIDs)
			if err != nil {
				return err
			}
			for _, booking := range bookings {
				// Re-checked under the lock: the item may have been checked out or removed
				// meanwhile; anything else is not a cart item.
				if !isOpenCartItem(booking, userID) {
					continue
				}
				if err := tx.Bookings.DeleteBooking(booking.BookingID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return s.GetCart(ctx, userID)
}

// Checkout turns the live cart of the user into one pending payment. Every item becomes a
// booking of that payment with its own line amount — its tickets times the current price of its
// event — and the payment amount is the sum of the line amounts, computed and stored here only,
// never taken from the client. All bookings share one refreshed expiry, so the payment has the
// full reservation TTL to complete. Items whose reservation expired hold no seats and are left
// out; an event that is no longer published fails the whole checkout, because the payment
// completes all items together or not at all.
//
// A checkout that is already open is returned unchanged (created false), so the amount shown to
// the user is the amount that is charged. Its items stay reserved until the payment completes.
func (s *CartService) Checkout(ctx context.Context, userID uuid.UUID) (resp *model.PaymentResponse, created bool, err error) {
	// Read without a lock to learn what to lock; a payment can only be pending while its
	// bookings are reserved, so a reservation with a payment marks the user's open checkout.
	reservations, err := s.bookings.Reservations(userID)
	if err != nil {
		return nil, false, err
	}
	if len(reservations) == 0 {
		return nil, false, ErrCartEmpty
	}
	eventIDs := make([]uuid.UUID, 0, len(reservations))
	bookingIDs := make([]uuid.UUID, 0, len(reservations))
	for _, booking := range reservations {
		if booking.PaymentID == nil && booking.EventID != nil {
			eventIDs = append(eventIDs, *booking.EventID)
		}
		bookingIDs = append(bookingIDs, booking.BookingID)
	}

	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// The event locks are taken first, in event_id order, and the booking locks after, in
		// booking_id order: the canonical order of every payment transaction, which rules out
		// deadlocks with completions, cancellations and the cleanup.
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

		var live []*model.BookingModel
		expired := false
		for _, booking := range bookings {
			if booking.PaymentID != nil {
				// An open checkout exists: return it unchanged, like a repeated checkout.
				payment, err := tx.Payments.GetPayment(*booking.PaymentID)
				if err != nil {
					return err
				}
				if payment == nil {
					return fmt.Errorf("payment %s referenced by booking %s is missing", *booking.PaymentID, booking.BookingID)
				}
				if payment.Status == model.PaymentStatusPending {
					view, err := tx.Payments.GetPaymentView(payment.PaymentID)
					if err != nil {
						return err
					}
					resp = paymentResponse(view)
					created = false
					return nil
				}
				// Any other payment status means the booking is not reserved anymore; only a
				// pending payment keeps its bookings reserved. Skip it defensively.
				continue
			}
			if reservationExpired(booking, time.Now()) {
				expired = true
				continue
			}
			live = append(live, booking)
		}
		if len(live) == 0 {
			if expired {
				return ErrReservationExpired
			}
			return ErrCartEmpty
		}

		// Each event of the cart is judged on its own, and every event must still take place,
		// because the payment completes all items together.
		lines := make([]*model.BookingModel, 0, len(live))
		amount := decimal.Zero
		for _, booking := range live {
			var event *model.EventModel
			if booking.EventID != nil {
				event = eventByID[*booking.EventID]
			}
			if !eventOpenForBooking(event) {
				return fmt.Errorf("%w: %q", ErrEventUnavailable, eventTitle(event, booking))
			}
			line, err := paymentAmount(event.Price, booking.NumberOfTickets)
			if err != nil {
				return err
			}
			booking.LineAmount = &line
			lines = append(lines, booking)
			amount = amount.Add(line)
		}
		if err := validPaymentAmount(amount); err != nil {
			return err
		}

		payment := &model.PaymentModel{
			PaymentID: uuid.New(),
			Amount:    amount,
			Status:    model.PaymentStatusPending,
		}
		if err := tx.Payments.CreatePayment(payment); err != nil {
			return err
		}
		expiresAt := time.Now().Add(s.reservationTTL)
		for _, booking := range lines {
			paymentID := payment.PaymentID
			booking.PaymentID = &paymentID
			booking.ExpiresAt = &expiresAt
			if err := tx.Bookings.UpdateBooking(booking); err != nil {
				return err
			}
		}

		// Read the stored result inside the transaction: once it commits, the response is ready.
		view, err := tx.Payments.GetPaymentView(payment.PaymentID)
		if err != nil {
			return err
		}
		if view == nil {
			return fmt.Errorf("payment %s not found after writing it", payment.PaymentID)
		}
		resp = paymentResponse(view)
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return resp, created, nil
}

// isOpenCartItem reports whether the booking is a cart item of the user: their reservation
// without a payment, in any state of life. Everything else — unknown, foreign, checked out,
// cancelled — reads as "not found".
func isOpenCartItem(booking *model.BookingModel, userID uuid.UUID) bool {
	return booking != nil && booking.EventID != nil && ownsBooking(booking, userID) &&
		booking.Status == model.BookingStatusReserved &&
		booking.PaymentID == nil
}

// eventTitle names the event of a booking for error details; events without a row have no name.
func eventTitle(event *model.EventModel, booking *model.BookingModel) string {
	if event != nil {
		return event.Title
	}
	if booking.EventID != nil {
		return booking.EventID.String()
	}
	return "unknown event"
}

// cartResponse builds the cart view from one snapshot of the cart items.
func cartResponse(items []*repository.CartItem) *model.CartResponse {
	resp := &model.CartResponse{
		Items:         make([]model.CartItemResponse, 0, len(items)),
		TotalAmount:   decimal.Zero,
		CheckoutReady: len(items) > 0,
	}
	for _, item := range items {
		booking := item.Booking
		out := model.CartItemResponse{
			BookingID:       booking.BookingID,
			NumberOfTickets: booking.NumberOfTickets,
			ExpiresAt:       *booking.ExpiresAt,
			CreatedAt:       booking.CreatedAt,
		}
		if item.Event != nil {
			// The seats the item holds are its own: it may always keep them, more need free
			// seats. Occupied includes the item's tickets.
			available := int64(item.Event.Capacity) - item.Occupied + int64(booking.NumberOfTickets)
			if available < 0 {
				available = 0
			}
			out.Event = ownEvent(item.Event, item.Location)
			out.UnitPrice = item.Event.Price
			out.LineTotal = item.Event.Price.Mul(decimal.NewFromInt(int64(booking.NumberOfTickets)))
			out.AvailableSeats = available
			out.Bookable = item.Event.Status == model.EventStatusPublished
			resp.TotalTickets += booking.NumberOfTickets
			resp.TotalAmount = resp.TotalAmount.Add(out.LineTotal)
			if !out.Bookable {
				resp.CheckoutReady = false
			}
		} else {
			// The event is gone, which the event update guard makes impossible while bookings
			// exist; still, such an item can never be checked out.
			resp.CheckoutReady = false
		}
		resp.Items = append(resp.Items, out)
	}
	return resp
}

// ownEvent maps an event with its location to the event shape of the own-booking and cart views.
func ownEvent(event *model.EventModel, location *model.LocationModel) *model.OwnBookingEvent {
	out := &model.OwnBookingEvent{
		EventID:   event.EventID,
		Title:     event.Title,
		StartTime: event.StartTime,
		EndTime:   event.EndTime,
		Status:    event.Status,
	}
	if location != nil {
		shown := model.LocationResponse(location)
		out.Location = &shown
	}
	return out
}
