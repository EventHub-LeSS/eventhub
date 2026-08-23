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
	PaymentID    uuid.UUID       `json:"paymentId" gorm:"column:payment_id;type:uuid;primaryKey"`
	Amount       decimal.Decimal `json:"amount" gorm:"column:amount;type:numeric(12,2)"`
	Status       PaymentStatus   `json:"status" gorm:"column:status"`
	RefundAmount decimal.Decimal `json:"refundAmount" gorm:"column:refund_amount;type:numeric(12,2);default:0"`
	CreatedAt    time.Time       `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time       `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PaymentModel) TableName() string { return "payments" }
