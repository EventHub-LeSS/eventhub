package repository

import (
	"backend/internal/model"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BookingPayment is a booking with its payment and the state of its event, read in one statement
// and thus from one snapshot. The own-booking views are built from it, and hasValidTickets
// judges the tickets of one booking from it.
type BookingPayment struct {
	Booking *model.BookingModel
	// Payment is nil while no payment process has been started.
	Payment *model.PaymentModel
	// EventStatus is nil when the event of the booking no longer exists.
	EventStatus *model.EventStatus
	// Now is the database clock at the time of the read. Reservations expire by the database
	// clock, see bookingRepository.CountOccupiedTickets.
	Now time.Time
}

// PaymentItem is one booking of a payment with the state of its event, like BookingPayment but
// for the payment views, which cover the bookings of a whole cart checkout.
type PaymentItem struct {
	Booking     *model.BookingModel
	EventStatus *model.EventStatus
	// Event is nil when the event of the booking no longer exists.
	Event *model.EventModel
	// Location is nil when the event has no location or the event no longer exists.
	Location *model.LocationModel
}

// PaymentView is a payment with all of its bookings and the state of their events, read in one
// statement and thus from one snapshot.
type PaymentView struct {
	Payment *model.PaymentModel
	Items   []*PaymentItem
	// Now is the database clock at the time of the read.
	Now time.Time
}

type PaymentRepository interface {
	// DatabaseNow returns the current database time: clock_timestamp, not NOW, which is the start
	// of the transaction and may lie before a lock wait.
	DatabaseNow() (time.Time, error)
	GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	// LockPayment reads the payment with SELECT ... FOR UPDATE; use it inside a transaction,
	// after the locks of its events and bookings.
	LockPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	// TryLockPayment locks the payment with FOR UPDATE SKIP LOCKED and returns nil when it is
	// locked elsewhere, e.g. by a completion running right now.
	TryLockPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	CreatePayment(payment *model.PaymentModel) error
	UpdatePayment(payment *model.PaymentModel) error
	// GetPaymentView returns nil when the payment does not exist. A single statement sees one
	// snapshot, so a booking completed meanwhile never shows up next to the payment state from
	// before; inside a transaction it also sees the transaction's own writes.
	GetPaymentView(paymentID uuid.UUID) (*PaymentView, error)
	// StalePaymentIDs returns the pending payments with at least one reservation that expired,
	// without locking them. The cleanup locks the payments it processes; ones that are busy
	// elsewhere are skipped there and reappear on the next run.
	StalePaymentIDs(limit int) ([]uuid.UUID, error)
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

func (r *paymentRepository) LockPayment(paymentID uuid.UUID) (*model.PaymentModel, error) {
	payment := &model.PaymentModel{}
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(payment, "payment_id = ?", paymentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *paymentRepository) TryLockPayment(paymentID uuid.UUID) (*model.PaymentModel, error) {
	payment := &model.PaymentModel{}
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		First(payment, "payment_id = ?", paymentID).Error
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

// paymentViewRow is one row of the GetPaymentView query; the booking columns repeat for every
// row of the payment, the payment columns are NULL-less because the payment exists.
type paymentViewRow struct {
	PaymentAmount        decimal.NullDecimal         `gorm:"column:payment_amount"`
	PaymentStatus        *model.PaymentStatus        `gorm:"column:payment_status"`
	PaymentFailureReason *model.PaymentFailureReason `gorm:"column:payment_failure_reason"`
	PaymentRefundAmount  decimal.NullDecimal         `gorm:"column:payment_refund_amount"`
	PaymentCreatedAt     *time.Time                  `gorm:"column:payment_created_at"`
	PaymentUpdatedAt     *time.Time                  `gorm:"column:payment_updated_at"`
	model.BookingModel
	EventStatus          *model.EventStatus `gorm:"column:event_status"`
	EventTitle           *string            `gorm:"column:event_title"`
	EventStartTime       *time.Time         `gorm:"column:event_start_time"`
	EventEndTime         *time.Time         `gorm:"column:event_end_time"`
	LocationID           *uuid.UUID         `gorm:"column:location_id"`
	LocationName         *string            `gorm:"column:location_name"`
	LocationCity         *string            `gorm:"column:location_city"`
	LocationPostalCode   *string            `gorm:"column:location_postal_code"`
	LocationStreet       *string            `gorm:"column:location_street"`
	LocationHouseNumber  *string            `gorm:"column:location_house_number"`
	DatabaseNow          time.Time          `gorm:"column:database_now"`
}

// paymentViewQuery selects a payment with its bookings, events and locations. The clock CTE
// reads clock_timestamp() once, so every item is judged by the same database time.
const paymentViewQuery = `WITH clock AS MATERIALIZED (SELECT clock_timestamp() AS database_now)
	SELECT p.amount AS payment_amount, p.status AS payment_status, p.failure_reason AS payment_failure_reason,
	    p.refund_amount AS payment_refund_amount, p.created_at AS payment_created_at, p.updated_at AS payment_updated_at,
	    b.*,
	    e.status AS event_status, e.title AS event_title, e.start_time AS event_start_time, e.end_time AS event_end_time,
	    l.location_id AS location_id, l.name AS location_name, l.city AS location_city,
	    l.postal_code AS location_postal_code, l.street AS location_street, l.house_number AS location_house_number,
	    clock.database_now
	FROM payments p
	CROSS JOIN clock
	JOIN bookings b ON b.payment_id = p.payment_id
	LEFT JOIN events e ON e.event_id = b.event_id
	LEFT JOIN locations l ON l.location_id = e.location_id
	WHERE p.payment_id = ?`

func (r *paymentRepository) GetPaymentView(paymentID uuid.UUID) (*PaymentView, error) {
	var rows []paymentViewRow
	if err := r.db.Raw(paymentViewQuery, paymentID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	view := &PaymentView{
		Payment: &model.PaymentModel{
			PaymentID:     paymentID,
			Amount:        rows[0].PaymentAmount.Decimal,
			Status:        *rows[0].PaymentStatus,
			FailureReason: rows[0].PaymentFailureReason,
			RefundAmount:  rows[0].PaymentRefundAmount.Decimal,
			CreatedAt:     derefTime(rows[0].PaymentCreatedAt),
			UpdatedAt:     derefTime(rows[0].PaymentUpdatedAt),
		},
		Items: make([]*PaymentItem, 0, len(rows)),
		Now:   rows[0].DatabaseNow,
	}
	for i := range rows {
		view.Items = append(view.Items, rows[i].paymentItem())
	}
	return view, nil
}

func (row *paymentViewRow) paymentItem() *PaymentItem {
	booking := row.BookingModel
	item := &PaymentItem{Booking: &booking, EventStatus: row.EventStatus}
	if booking.EventID != nil && row.EventStatus != nil {
		item.Event = &model.EventModel{
			EventID:    *booking.EventID,
			Title:      derefString(row.EventTitle),
			StartTime:  derefTime(row.EventStartTime),
			EndTime:    derefTime(row.EventEndTime),
			Status:     *row.EventStatus,
			LocationID: row.LocationID,
		}
	}
	if row.LocationID != nil {
		item.Location = &model.LocationModel{
			LocationID:   *row.LocationID,
			Name:         derefString(row.LocationName),
			City:         derefString(row.LocationCity),
			PostalCode:   derefString(row.LocationPostalCode),
			Street:       derefString(row.LocationStreet),
			HouseNumber:  row.LocationHouseNumber,
		}
	}
	return item
}

func (r *paymentRepository) StalePaymentIDs(limit int) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.Raw(`SELECT DISTINCT p.payment_id
	    FROM payments p
	    JOIN bookings b ON b.payment_id = p.payment_id
	    WHERE p.status = ? AND b.status = ? AND b.expires_at <= clock_timestamp()
	    ORDER BY p.payment_id
	    LIMIT ?`,
		model.PaymentStatusPending, model.BookingStatusReserved, limit).Scan(&ids).Error
	return ids, err
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
