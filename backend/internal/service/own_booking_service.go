package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"time"

	"github.com/google/uuid"
)

// EVENTHUB-236: Meine Tickets und Buchungshistorie
// OwnBookingService shows visitors their own bookings: the valid tickets, the booking history and
// the details of one booking. It only reads. Whether a booking holds valid tickets follows
// hasValidTickets, the rule of the payment result, so both never contradict each other.
type OwnBookingService struct {
	bookings repository.OwnBookingRepository
}

func NewOwnBookingService(bookings repository.OwnBookingRepository) *OwnBookingService {
	return &OwnBookingService{bookings: bookings}
}

// ListOwnTickets returns the bookings of the user that hold valid tickets: upcoming events first by
// start time, then past events, the latest first. The rule is applied here and not in SQL, so it
// exists only once.
func (s *OwnBookingService) ListOwnTickets(userID uuid.UUID) ([]model.OwnBookingResponse, error) {
	bookings, err := s.bookings.ListByUserByEventStart(userID)
	if err != nil {
		return nil, err
	}
	tickets := make([]model.OwnBookingResponse, 0, len(bookings))
	for _, booking := range bookings {
		if resp := ownBookingResponse(booking); resp.TicketsValid {
			tickets = append(tickets, resp)
		}
	}
	return tickets, nil
}

// ListOwnBookings returns every booking of the user, the latest first, whatever its status. A
// non-empty status keeps the bookings shown in that status, so expired also finds reservations
// that expired by the database clock but are still stored as reserved.
func (s *OwnBookingService) ListOwnBookings(userID uuid.UUID, status model.BookingStatus) ([]model.OwnBookingResponse, error) {
	bookings, err := s.bookings.ListByUserNewestFirst(userID)
	if err != nil {
		return nil, err
	}
	history := make([]model.OwnBookingResponse, 0, len(bookings))
	for _, booking := range bookings {
		if resp := ownBookingResponse(booking); status == "" || resp.BookingStatus == status {
			history = append(history, resp)
		}
	}
	return history, nil
}

// ownBookingResponse builds the view of an own booking. Validity and message come from the same
// functions as the payment result, and expiry is judged by the database clock of the read.
func ownBookingResponse(own *repository.OwnBooking) model.OwnBookingResponse {
	booking, payment := own.Booking, own.Payment
	ticketsValid := hasValidTickets(&own.BookingPayment)
	resp := model.OwnBookingResponse{
		BookingID:       booking.BookingID,
		NumberOfTickets: booking.NumberOfTickets,
		BookingStatus:   shownBookingStatus(booking, own.Now),
		TicketsValid:    ticketsValid,
		BookedAt:        booking.CreatedAt,
	}
	if resp.BookingStatus == model.BookingStatusReserved {
		resp.ExpiresAt = booking.ExpiresAt
	}
	if payment != nil {
		amount, refundAmount := payment.Amount, payment.RefundAmount
		resp.PaymentStatus = &payment.Status
		resp.FailureReason = payment.FailureReason
		resp.Amount, resp.RefundAmount = &amount, &refundAmount
		resp.Message = paymentMessage(&own.BookingPayment, ticketsValid)
	}
	if event := own.Event; event != nil {
		resp.Event = &model.OwnBookingEvent{
			EventID:   event.EventID,
			Title:     event.Title,
			StartTime: event.StartTime,
			EndTime:   event.EndTime,
			Status:    event.Status,
		}
		if location := own.Location; location != nil {
			resp.Event.Location = &model.PublishedEventLocation{
				LocationID:  location.LocationID,
				Name:        location.Name,
				City:        location.City,
				PostalCode:  location.PostalCode,
				Street:      location.Street,
				HouseNumber: location.HouseNumber,
			}
		}
	}
	return resp
}

// shownBookingStatus is the stored status, except that a reservation past its expiry is shown as
// expired: it no longer holds tickets (see reservationExpired). A pending payment of such a
// reservation is failed by the payment cleanup within a minute; a reservation without payment
// stays stored as reserved.
func shownBookingStatus(booking *model.BookingModel, now time.Time) model.BookingStatus {
	if booking.Status == model.BookingStatusReserved && reservationExpired(booking, now) {
		return model.BookingStatusExpired
	}
	return booking.Status
}
