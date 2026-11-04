package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type BookingStatus string

const (
	BookingStatusReserved  BookingStatus = "reserved"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusFailed    BookingStatus = "failed"
	BookingStatusExpired   BookingStatus = "expired"
)

// IsValidBookingStatus reports whether status is one of the booking statuses of the booking_status enum.
// EVENTHUB-236: validates the status filter of the booking history.
func IsValidBookingStatus(status BookingStatus) bool {
	switch status {
	case BookingStatusReserved, BookingStatusConfirmed, BookingStatusCancelled, BookingStatusFailed, BookingStatusExpired:
		return true
	}
	return false
}

type BookingModel struct {
	BookingID       uuid.UUID     `json:"bookingId" gorm:"column:booking_id;type:uuid;primaryKey"`
	UserID          *uuid.UUID    `json:"userId" gorm:"column:user_id;type:uuid"`
	EventID         *uuid.UUID    `json:"eventId" gorm:"column:event_id;type:uuid"`
	PaymentID       *uuid.UUID    `json:"paymentId" gorm:"column:payment_id;type:uuid"`
	NumberOfTickets int           `json:"numberOfTickets" gorm:"column:number_of_tickets"`
	Status          BookingStatus `json:"status" gorm:"column:status"`
	ExpiresAt       *time.Time    `json:"expiresAt" gorm:"column:expires_at"`
	// LineAmount is what the booking costs within its payment: its tickets times the event
	// price at checkout time. Null while the reservation sits in the cart without a payment.
	LineAmount *decimal.Decimal `json:"lineAmount,omitempty" gorm:"column:line_amount;type:numeric(12,2)"`
	CreatedAt  time.Time        `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time        `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (BookingModel) TableName() string { return "bookings" }

// TicketHolder is a user with the summed tickets of their confirmed bookings of one event.
// EVENTHUB-291: ticket holders are notified when the event is withdrawn.
type TicketHolder struct {
	UserID  uuid.UUID `gorm:"column:user_id"`
	Tickets int64     `gorm:"column:tickets"`
}
