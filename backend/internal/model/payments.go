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

type PaymentModel struct {
	PaymentID    uuid.UUID       `json:"paymentId" db:"payment_id"`
	Amount       decimal.Decimal `json:"amount" db:"amount"`
	Status       PaymentStatus   `json:"status" db:"status"`
	RefundAmount decimal.Decimal `json:"refundAmount" db:"refund_amount"`
	CreatedAt    time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time       `json:"updatedAt" db:"updated_at"`
}
