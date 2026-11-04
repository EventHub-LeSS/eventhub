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

// PaymentMethod is the way a payment was made with the simulated payment provider.
type PaymentMethod string

const (
	PaymentMethodCreditCard  PaymentMethod = "credit_card"
	PaymentMethodDirectDebit PaymentMethod = "direct_debit"
	PaymentMethodPayPal      PaymentMethod = "paypal"
)

// Valid reports whether the payment method is one the simulated payment provider supports.
func (m PaymentMethod) Valid() bool {
	switch m {
	case PaymentMethodCreditCard, PaymentMethodDirectDebit, PaymentMethodPayPal:
		return true
	}
	return false
}

// CardBrand is the card network of a credit card payment, derived from the card number.
type CardBrand string

const (
	CardBrandVisa       CardBrand = "visa"
	CardBrandMastercard CardBrand = "mastercard"
	CardBrandAmex       CardBrand = "amex"
)

type PaymentModel struct {
	PaymentID     uuid.UUID             `json:"paymentId" gorm:"column:payment_id;type:uuid;primaryKey"`
	Amount        decimal.Decimal       `json:"amount" gorm:"column:amount;type:numeric(12,2)"`
	Status        PaymentStatus         `json:"status" gorm:"column:status"`
	FailureReason *PaymentFailureReason `json:"failureReason,omitempty" gorm:"column:failure_reason"`
	RefundAmount  decimal.Decimal       `json:"refundAmount" gorm:"column:refund_amount;type:numeric(12,2);default:0"`
	// PaymentMethod is set once the simulated payment provider charged or declined the payment;
	// nil while the payment is pending and for payments completed without a payment method.
	PaymentMethod *PaymentMethod `json:"paymentMethod,omitempty" gorm:"column:payment_method"`
	// PaymentAccount is the masked account the payment was made with, e.g. "**** 4242"; card
	// numbers, CVCs and IBANs are never stored in full.
	PaymentAccount *string    `json:"paymentAccount,omitempty" gorm:"column:payment_account"`
	CardBrand      *CardBrand `json:"cardBrand,omitempty" gorm:"column:card_brand"`
	// TransactionID is the reference the simulated payment provider assigned to the payment
	// attempt; for direct debits it doubles as the SEPA mandate reference.
	TransactionID *string   `json:"transactionId,omitempty" gorm:"column:transaction_id"`
	CreatedAt     time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PaymentModel) TableName() string { return "payments" }

// SimulatedPaymentOutcome is the result the simulated payment provider reports. EventHub does not
// connect to a real payment provider (EVENTHUB-235).
type SimulatedPaymentOutcome string

const (
	SimulatedPaymentSuccess SimulatedPaymentOutcome = "success"
	SimulatedPaymentFailure SimulatedPaymentOutcome = "failure"
)

// CreditCardDetails is the card data of a simulated credit card payment. It is validated and
// masked, never stored in full.
type CreditCardDetails struct {
	Cardholder string `json:"cardholder" example:"Max Mustermann"`
	// 12 to 19 digits; spaces and dashes are ignored. Visa, Mastercard and American Express are
	// accepted, and the number must pass the Luhn check.
	Number string `json:"number" example:"4242 4242 4242 4242"`
	// Expiry date as MM/YY; the card is valid through the end of that month.
	Expiry string `json:"expiry" example:"12/30"`
	// 3 digits, 4 for American Express.
	CVC string `json:"cvc" example:"123"`
}

// DirectDebitDetails is the account data of a simulated SEPA direct debit. The IBAN is
// validated and masked, never stored in full.
type DirectDebitDetails struct {
	AccountHolder string `json:"accountHolder" example:"Max Mustermann"`
	// IBAN of the SEPA area; spaces are ignored and the check digits must be valid.
	IBAN string `json:"iban" example:"DE89 3704 0044 0532 0130 00"`
	// The account holder authorises EventHub to collect the amount by direct debit (SEPA
	// mandate); a direct debit without mandate is rejected.
	MandateAccepted bool `json:"mandateAccepted" example:"true"`
}

// PayPalDetails is the PayPal account of a simulated PayPal Express payment.
type PayPalDetails struct {
	// E-mail address of the PayPal account the buyer approved the payment with; optional.
	Email string `json:"email,omitempty" example:"max@example.com"`
}

// PaymentMethodInput is the payment method chosen for a simulated payment together with the
// details of exactly that method.
type PaymentMethodInput struct {
	Method      PaymentMethod
	CreditCard  *CreditCardDetails
	DirectDebit *DirectDebitDetails
	PayPal      *PayPalDetails
}

// EVENTHUB-235: Zahlungsvorgang, 282: Zahlung im Warenkorb
// PaymentItemResponse is one booking of a payment: the tickets of one event of the cart.
type PaymentItemResponse struct {
	BookingID uuid.UUID `json:"bookingId"`
	// Null when the event of the booking no longer exists.
	Event           *OwnBookingEvent `json:"event"`
	NumberOfTickets int              `json:"numberOfTickets" example:"2"`
	// Tickets times the event price at checkout time; the charged amount of this item.
	LineAmount    decimal.Decimal `json:"lineAmount" swaggertype:"string" example:"40"`
	BookingStatus BookingStatus   `json:"bookingStatus"`
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
	// Payment method the simulated payment provider charged or declined; omitted while the
	// payment is pending and when it was completed without a payment method.
	PaymentMethod *PaymentMethod `json:"paymentMethod,omitempty" enums:"credit_card,direct_debit,paypal" example:"credit_card"`
	// Masked account of the payment method: "**** 4242" for cards, "DE89 **** 3000" for IBANs,
	// "m***@example.com" for PayPal; omitted when unknown.
	PaymentAccount *string `json:"paymentAccount,omitempty" example:"**** 4242"`
	// Card network of a credit card payment.
	CardBrand *CardBrand `json:"cardBrand,omitempty" enums:"visa,mastercard,amex" example:"visa"`
	// Reference of the simulated payment provider for this payment attempt; for direct debits
	// the SEPA mandate reference.
	TransactionID *string `json:"transactionId,omitempty" example:"SIM-CC-4F2A9C1E7B3D"`
	// True only when the payment is paid and every item holds valid tickets.
	TicketsValid bool `json:"ticketsValid"`
	// Human readable result of the payment process.
	Message string `json:"message" example:"Payment successful. Your tickets are confirmed."`
	// Deadline for completing a pending payment; omitted once the payment is completed.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
