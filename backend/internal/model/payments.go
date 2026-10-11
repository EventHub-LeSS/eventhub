package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

// PaymentFailureReason explains why a payment failed.
type PaymentFailureReason string

const (
	// PaymentFailureDeclined means the (simulated) payment provider declined the payment.
	PaymentFailureDeclined PaymentFailureReason = "declined"
	// PaymentFailureReservationExpired means the reservation ran out before the payment was completed.
	PaymentFailureReservationExpired PaymentFailureReason = "reservation_expired"
	// PaymentFailureEventUnavailable means the event was withdrawn before the payment was completed.
	PaymentFailureEventUnavailable PaymentFailureReason = "event_unavailable"
)

type PaymentModel struct {
	PaymentID     uuid.UUID             `json:"paymentId" gorm:"column:payment_id;type:uuid;primaryKey"`
	Amount        decimal.Decimal       `json:"amount" gorm:"column:amount;type:numeric(12,2)"`
	Status        PaymentStatus         `json:"status" gorm:"column:status"`
	FailureReason *PaymentFailureReason `json:"failureReason,omitempty" gorm:"column:failure_reason"`
	RefundAmount  decimal.Decimal       `json:"refundAmount" gorm:"column:refund_amount;type:numeric(12,2);default:0"`
	CreatedAt     time.Time             `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time             `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PaymentModel) TableName() string { return "payments" }

// SimulatedPaymentOutcome is the result the simulated payment provider reports. EventHub does not
// connect to a real payment provider (EVENTHUB-235).
type SimulatedPaymentOutcome string

const (
	SimulatedPaymentSuccess SimulatedPaymentOutcome = "success"
	SimulatedPaymentFailure SimulatedPaymentOutcome = "failure"
)

// EVENTHUB-235: Zahlungsvorgang
// PaymentResponse is the payment process of a booking, built from the stored payment and booking.
type PaymentResponse struct {
	PaymentID       uuid.UUID  `json:"paymentId"`
	BookingID       uuid.UUID  `json:"bookingId"`
	EventID         *uuid.UUID `json:"eventId"`
	NumberOfTickets int        `json:"numberOfTickets" example:"2"`
	// Number of tickets times the event price when the payment process was started.
	Amount        decimal.Decimal       `json:"amount" swaggertype:"string" example:"40"`
	Status        PaymentStatus         `json:"status"`
	FailureReason *PaymentFailureReason `json:"failureReason,omitempty"`
	BookingStatus BookingStatus         `json:"bookingStatus"`
	// True only when the payment is paid and the booking is confirmed.
	TicketsValid bool `json:"ticketsValid"`
	// Human readable result of the payment process.
	Message string `json:"message" example:"Payment successful. Your booking is confirmed."`
	// Deadline for completing a pending payment; omitted once the payment is completed.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
