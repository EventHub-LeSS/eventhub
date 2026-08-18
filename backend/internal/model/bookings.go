package model

import (
	"time"

	"github.com/google/uuid"
)

type BookingStatus string

const (
	BookingStatusReserved  BookingStatus = "reserved"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusFailed    BookingStatus = "failed"
)

type BookingModel struct {
	BookingID       uuid.UUID     `json:"bookingId" db:"booking_id"`
	UserID          uuid.UUID     `json:"userId" db:"user_id"`
	EventID         uuid.UUID     `json:"eventId" db:"event_id"`
	PaymentID       *uuid.UUID    `json:"paymentId" db:"payment_id"`
	NumberOfTickets int           `json:"numberOfTickets" db:"number_of_tickets"`
	Status          BookingStatus `json:"status" db:"status"`
	CreatedAt       time.Time     `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time     `json:"updatedAt" db:"updated_at"`
}
