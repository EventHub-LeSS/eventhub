package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// EVENTHUB-236: Meine Tickets und Buchungshistorie
// OwnBookingResponse is a booking of the caller as the ticket list, the booking history and the
// ticket details show it. All three read the same data and apply the validity rule of PaymentResponse.
type OwnBookingResponse struct {
	// ID for GET /me/bookings/{bookingId} and the booking routes.
	BookingID       uuid.UUID `json:"bookingId"`
	NumberOfTickets int       `json:"numberOfTickets" example:"2"`
	// Stored booking status, except that a reservation whose expiresAt has passed by the database
	// clock is shown as expired. GET /bookings/{bookingId}/payment shows the stored status.
	BookingStatus BookingStatus `json:"bookingStatus"`
	// Null while no payment process has been started.
	PaymentStatus *PaymentStatus        `json:"paymentStatus"`
	FailureReason *PaymentFailureReason `json:"failureReason,omitempty"`
	// Stored amount of the payment; null without payment.
	Amount *decimal.Decimal `json:"amount" swaggertype:"string" example:"40"`
	// Refunded amount of the payment; null without payment.
	RefundAmount *decimal.Decimal `json:"refundAmount" swaggertype:"string" example:"0"`
	// True only when the payment is paid, the booking is confirmed and the event is published or
	// completed, like ticketsValid of GET /bookings/{bookingId}/payment.
	TicketsValid bool `json:"ticketsValid"`
	// Human readable result of the payment process, the same text as GET /bookings/{bookingId}/payment;
	// omitted while no payment process has been started.
	Message string `json:"message,omitempty" example:"Payment successful. Your booking is confirmed."`
	// Time the booking was made.
	BookedAt time.Time `json:"bookedAt"`
	// Deadline of a live reservation; omitted otherwise.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// Null when the event no longer exists.
	Event *OwnBookingEvent `json:"event"`
}

// OwnBookingEvent is the event of an own booking.
type OwnBookingEvent struct {
	EventID   uuid.UUID   `json:"eventId"`
	Title     string      `json:"title"`
	StartTime time.Time   `json:"startTime"`
	EndTime   time.Time   `json:"endTime"`
	Status    EventStatus `json:"status"`
	// Null when the location of the event was deleted.
	Location *PublishedEventLocation `json:"location"`
}
