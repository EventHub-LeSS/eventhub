package repository

import (
	"backend/internal/model"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BookingPayment is a booking with its payment and the state of its event, read in one statement
// and thus from one snapshot.
type BookingPayment struct {
	Booking *model.BookingModel
	// Payment is nil while no payment process has been started.
	Payment *model.PaymentModel
	// EventStatus is nil when the event of the booking no longer exists.
	EventStatus *model.EventStatus
	// Now is the database clock at the time of the read. Reservations expire by the database
	// clock, see bookingTx.CountOccupiedTickets.
	Now time.Time
}

type PaymentRepository interface {
	// DatabaseNow returns the current database time: clock_timestamp, not NOW, which is the start
	// of the transaction and may lie before a lock wait.
	DatabaseNow() (time.Time, error)
	GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	CreatePayment(payment *model.PaymentModel) error
	UpdatePayment(payment *model.PaymentModel) error
	// GetBookingPayment returns nil when the booking does not exist. A single statement sees one
	// snapshot, so a payment completed meanwhile never shows up next to the booking state from
	// before; inside a transaction it also sees the transaction's own writes.
	GetBookingPayment(bookingID uuid.UUID) (*BookingPayment, error)
	// LockStalePendingBookings locks reserved bookings whose reservation has expired while their
	// payment is still pending. Bookings locked elsewhere, e.g. by a payment being completed right
	// now, are skipped.
	LockStalePendingBookings(limit int) ([]*model.BookingModel, error)
}

type paymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{db: db}
}

func (r *paymentRepository) DatabaseNow() (time.Time, error) {
	var now time.Time
	err := r.db.Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now, err
}

func (r *paymentRepository) GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error) {
	payment := &model.PaymentModel{}
	err := r.db.First(payment, "payment_id = ?", paymentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *paymentRepository) CreatePayment(payment *model.PaymentModel) error {
	return r.db.Create(payment).Error
}

func (r *paymentRepository) UpdatePayment(payment *model.PaymentModel) error {
	return r.db.Save(payment).Error
}

// bookingPaymentRow is one row of GetBookingPayment; the payment columns are NULL while no
// payment process has been started.
type bookingPaymentRow struct {
	model.BookingModel
	PaymentAmount        decimal.NullDecimal         `gorm:"column:payment_amount"`
	PaymentStatus        *model.PaymentStatus        `gorm:"column:payment_status"`
	PaymentFailureReason *model.PaymentFailureReason `gorm:"column:payment_failure_reason"`
	PaymentRefundAmount  decimal.NullDecimal         `gorm:"column:payment_refund_amount"`
	PaymentCreatedAt     *time.Time                  `gorm:"column:payment_created_at"`
	PaymentUpdatedAt     *time.Time                  `gorm:"column:payment_updated_at"`
	EventStatus          *model.EventStatus          `gorm:"column:event_status"`
	DatabaseNow          time.Time                   `gorm:"column:database_now"`
}

func (r *paymentRepository) GetBookingPayment(bookingID uuid.UUID) (*BookingPayment, error) {
	var row bookingPaymentRow
	res := r.db.Raw(`SELECT b.*,
	        p.amount AS payment_amount, p.status AS payment_status, p.failure_reason AS payment_failure_reason,
	        p.refund_amount AS payment_refund_amount, p.created_at AS payment_created_at, p.updated_at AS payment_updated_at,
	        e.status AS event_status, clock_timestamp() AS database_now
	    FROM bookings b
	    LEFT JOIN payments p ON p.payment_id = b.payment_id
	    LEFT JOIN events e ON e.event_id = b.event_id
	    WHERE b.booking_id = ?`, bookingID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}

	booking := row.BookingModel
	view := &BookingPayment{Booking: &booking, EventStatus: row.EventStatus, Now: row.DatabaseNow}
	if booking.PaymentID != nil && row.PaymentStatus != nil {
		view.Payment = &model.PaymentModel{
			PaymentID:     *booking.PaymentID,
			Amount:        row.PaymentAmount.Decimal,
			Status:        *row.PaymentStatus,
			FailureReason: row.PaymentFailureReason,
			RefundAmount:  row.PaymentRefundAmount.Decimal,
			CreatedAt:     derefTime(row.PaymentCreatedAt),
			UpdatedAt:     derefTime(row.PaymentUpdatedAt),
		}
	}
	return view, nil
}

func (r *paymentRepository) LockStalePendingBookings(limit int) ([]*model.BookingModel, error) {
	var bookings []*model.BookingModel
	err := r.db.Raw(`SELECT b.* FROM bookings b
	    JOIN payments p ON p.payment_id = b.payment_id
	    WHERE b.status = ? AND p.status = ? AND b.expires_at <= clock_timestamp()
	    ORDER BY b.expires_at
	    LIMIT ?
	    FOR UPDATE OF b SKIP LOCKED`,
		model.BookingStatusReserved, model.PaymentStatusPending, limit).Scan(&bookings).Error
	return bookings, err
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
