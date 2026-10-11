package repository

import (
	"backend/internal/model"
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentTx interface {
	// Now returns the current database time. Reservations expire by the database clock, see
	// bookingTx.CountOccupiedTickets.
	Now() (time.Time, error)
	GetEvent(eventID uuid.UUID) (*model.EventModel, error)
	LockEvent(eventID uuid.UUID) (*model.EventModel, error)
	LockBooking(bookingID uuid.UUID) (*model.BookingModel, error)
	GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	CreatePayment(payment *model.PaymentModel) error
	UpdatePayment(payment *model.PaymentModel) error
	UpdateBooking(booking *model.BookingModel) error
}

type PaymentRepository interface {
	InTransaction(ctx context.Context, fn func(tx PaymentTx) error) error
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*model.BookingModel, error)
	GetPaymentByID(ctx context.Context, paymentID uuid.UUID) (*model.PaymentModel, error)
}

type paymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{db: db}
}

func (r *paymentRepository) InTransaction(ctx context.Context, fn func(tx PaymentTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&paymentTx{db: tx})
	})
}

func (r *paymentRepository) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*model.BookingModel, error) {
	return findBooking(r.db.WithContext(ctx), bookingID)
}

func (r *paymentRepository) GetPaymentByID(ctx context.Context, paymentID uuid.UUID) (*model.PaymentModel, error) {
	return findPayment(r.db.WithContext(ctx), paymentID)
}

type paymentTx struct {
	db *gorm.DB
}

func (t *paymentTx) Now() (time.Time, error) {
	// clock_timestamp, not NOW: NOW is the start of the transaction, which may lie before a lock wait.
	var now time.Time
	err := t.db.Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now, err
}

func (t *paymentTx) GetEvent(eventID uuid.UUID) (*model.EventModel, error) {
	return NewEventRepository(t.db).GetEventByID(eventID)
}

func (t *paymentTx) LockEvent(eventID uuid.UUID) (*model.EventModel, error) {
	return NewEventRepository(t.db).LockEvent(eventID)
}

func (t *paymentTx) LockBooking(bookingID uuid.UUID) (*model.BookingModel, error) {
	return findBooking(t.db.Clauses(clause.Locking{Strength: "UPDATE"}), bookingID)
}

func (t *paymentTx) GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error) {
	return findPayment(t.db, paymentID)
}

func (t *paymentTx) CreatePayment(payment *model.PaymentModel) error {
	return t.db.Create(payment).Error
}

func (t *paymentTx) UpdatePayment(payment *model.PaymentModel) error {
	return t.db.Save(payment).Error
}

func (t *paymentTx) UpdateBooking(booking *model.BookingModel) error {
	return t.db.Save(booking).Error
}

func findBooking(db *gorm.DB, bookingID uuid.UUID) (*model.BookingModel, error) {
	booking := &model.BookingModel{}
	err := db.First(booking, "booking_id = ?", bookingID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func findPayment(db *gorm.DB, paymentID uuid.UUID) (*model.PaymentModel, error) {
	payment := &model.PaymentModel{}
	err := db.First(payment, "payment_id = ?", paymentID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return payment, nil
}
