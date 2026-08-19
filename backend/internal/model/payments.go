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
	tableName struct{} `pg:"payments"`

	PaymentID    uuid.UUID       `json:"paymentId" pg:"payment_id,pk"`
	Amount       decimal.Decimal `json:"amount" pg:"amount"`
	Status       PaymentStatus   `json:"status" pg:"status"`
	RefundAmount decimal.Decimal `json:"refundAmount" pg:"refund_amount,use_zero"`
	CreatedAt    time.Time       `json:"createdAt" pg:"created_at"`
	UpdatedAt    time.Time       `json:"updatedAt" pg:"updated_at"`
}
