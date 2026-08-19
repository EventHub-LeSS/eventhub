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
	tableName struct{} `pg:"bookings"`

	BookingID       uuid.UUID     `json:"bookingId" pg:"booking_id,pk"`
	UserID          uuid.UUID     `json:"userId" pg:"user_id"`
	EventID         uuid.UUID     `json:"eventId" pg:"event_id"`
	PaymentID       *uuid.UUID    `json:"paymentId" pg:"payment_id"`
	NumberOfTickets int           `json:"numberOfTickets" pg:"number_of_tickets"`
	Status          BookingStatus `json:"status" pg:"status"`
	CreatedAt       time.Time     `json:"createdAt" pg:"created_at"`
	UpdatedAt       time.Time     `json:"updatedAt" pg:"updated_at"`
}
