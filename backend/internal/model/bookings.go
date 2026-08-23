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
	BookingID       uuid.UUID     `json:"bookingId" gorm:"column:booking_id;type:uuid;primaryKey"`
	UserID          uuid.UUID     `json:"userId" gorm:"column:user_id;type:uuid"`
	EventID         uuid.UUID     `json:"eventId" gorm:"column:event_id;type:uuid"`
	PaymentID       *uuid.UUID    `json:"paymentId" gorm:"column:payment_id;type:uuid"`
	NumberOfTickets int           `json:"numberOfTickets" gorm:"column:number_of_tickets"`
	Status          BookingStatus `json:"status" gorm:"column:status"`
	CreatedAt       time.Time     `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time     `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (BookingModel) TableName() string { return "bookings" }
