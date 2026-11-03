package repository

import (
	"backend/internal/model"
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BookingTx interface {
	LockEvent(eventID uuid.UUID) (*model.EventModel, error)
	LockBooking(bookingID uuid.UUID) (*model.BookingModel, error)
	UpdateBooking(booking *model.BookingModel) error
	GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error)
	UpdatePayment(payment *model.PaymentModel) error
	CountOccupiedTickets(eventID uuid.UUID) (int64, error)
	CreateBooking(booking *model.BookingModel) error
}

type BookingRepository interface {
	InTransaction(ctx context.Context, fn func(tx BookingTx) error) error
	GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error)
	// LockBooking reads the booking with SELECT ... FOR UPDATE; use it inside a transaction.
	LockBooking(bookingID uuid.UUID) (*model.BookingModel, error)
	// CountOccupiedTickets sums the tickets of confirmed bookings and live reservations of the event.
	CountOccupiedTickets(eventID uuid.UUID) (int64, error)
	GetAllBookings() ([]*model.BookingModel, error)
	UpdateBooking(booking *model.BookingModel) error
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error) {
	return r.getBooking(bookingID, false)
}

func (r *bookingRepository) LockBooking(bookingID uuid.UUID) (*model.BookingModel, error) {
	return r.getBooking(bookingID, true)
}

func (r *bookingRepository) getBooking(bookingID uuid.UUID, lock bool) (*model.BookingModel, error) {
	booking := &model.BookingModel{}
	q := r.db
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(booking, "booking_id = ?", bookingID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *bookingRepository) GetAllBookings() ([]*model.BookingModel, error) {
	var bookings []*model.BookingModel
	err := r.db.Find(&bookings).Error
	if err != nil {
		return nil, err
	}
	return bookings, err
}

func (r *bookingRepository) UpdateBooking(booking *model.BookingModel) error {
	return r.db.Save(booking).Error
}

func (r *bookingRepository) InTransaction(ctx context.Context, fn func(tx BookingTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&bookingTx{db: tx})
	})
}

type bookingTx struct {
	db *gorm.DB
}

func (t *bookingTx) LockBooking(bookingID uuid.UUID) (*model.BookingModel, error) {
	return NewBookingRepository(t.db).LockBooking(bookingID)
}

func (t *bookingTx) UpdateBooking(booking *model.BookingModel) error {
	return NewBookingRepository(t.db).UpdateBooking(booking)
}

func (t *bookingTx) GetPayment(paymentID uuid.UUID) (*model.PaymentModel, error) {
	return NewPaymentRepository(t.db).GetPayment(paymentID)
}

func (t *bookingTx) UpdatePayment(payment *model.PaymentModel) error {
	return NewPaymentRepository(t.db).UpdatePayment(payment)
}

func (t *bookingTx) LockEvent(eventID uuid.UUID) (*model.EventModel, error) {
	event := &model.EventModel{}
	err := t.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(event, "event_id = ?", eventID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (t *bookingTx) CountOccupiedTickets(eventID uuid.UUID) (int64, error) {
	return countOccupiedTickets(t.db, eventID)
}

func (r *bookingRepository) CountOccupiedTickets(eventID uuid.UUID) (int64, error) {
	return countOccupiedTickets(r.db, eventID)
}

func countOccupiedTickets(db *gorm.DB, eventID uuid.UUID) (int64, error) {
	var occupied int64
	err := db.Model(&model.BookingModel{}).
		Where("event_id = ? AND (status = ? OR (status = ? AND expires_at > NOW()))",
			eventID, model.BookingStatusConfirmed, model.BookingStatusReserved).
		Select("COALESCE(SUM(number_of_tickets), 0)").
		Scan(&occupied).Error
	return occupied, err
}

func (t *bookingTx) CreateBooking(booking *model.BookingModel) error {
	return t.db.Create(booking).Error
}
