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
	// PaymentFailureBookingCancelled means the owner cancelled before payment completed.
	PaymentFailureBookingCancelled PaymentFailureReason = "booking_cancelled"
	// PaymentFailureReservationExpired means the reservation ran out before the payment was completed.
	PaymentFailureReservationExpired PaymentFailureReason = "reservation_expired"
	// PaymentFailureEventUnavailable means the event was withdrawn or deleted before the payment was completed.
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

// EVENTHUB-235: Zahlungsvorgang, 282: Zahlung im Warenkorb
// PaymentItemResponse is one booking of a payment: the tickets of one event of the cart.
type PaymentItemResponse struct {
	BookingID uuid.UUID `json:"bookingId"`
	// Null when the event of the booking no longer exists.
	Event *OwnBookingEvent `json:"event"`
	NumberOfTickets int `json:"numberOfTickets" example:"2"`
	// Tickets times the event price at checkout time; the charged amount of this item.
	LineAmount decimal.Decimal `json:"lineAmount" swaggertype:"string" example:"40"`
	BookingStatus BookingStatus `json:"bookingStatus"`
	// True only when the payment is paid, this booking is confirmed and the event takes place
	// or took place.
	TicketsValid bool `json:"ticketsValid"`
}

// PaymentResponse is the payment process of a cart checkout, built from the stored payment and
// its bookings. The amount is the sum of the line amounts of all items.
type PaymentResponse struct {
	PaymentID uuid.UUID `json:"paymentId"`
	// Sum of the line amounts of the items, computed and stored by the backend only.
	Amount        decimal.Decimal       `json:"amount" swaggertype:"string" example:"40"`
	Items         []PaymentItemResponse `json:"items"`
	Status        PaymentStatus         `json:"status"`
	FailureReason *PaymentFailureReason `json:"failureReason,omitempty"`
	// True only when the payment is paid and every item holds valid tickets.
	TicketsValid bool `json:"ticketsValid"`
	// Human readable result of the payment process.
	Message string `json:"message" example:"Payment successful. Your tickets are confirmed."`
	// Deadline for completing a pending payment; omitted once the payment is completed.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
