package repository

import (
	"backend/internal/model"
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BookingTx interface {
	LockEvent(eventID uuid.UUID) (*model.EventModel, error)
	CountOccupiedTickets(eventID uuid.UUID) (int64, error)
	CreateBooking(booking *model.BookingModel) error
}

type BookingRepository interface {
	InTransaction(ctx context.Context, fn func(tx BookingTx) error) error
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) InTransaction(ctx context.Context, fn func(tx BookingTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&bookingTx{db: tx})
	})
}

type bookingTx struct {
	db *gorm.DB
}

func (t *bookingTx) LockEvent(eventID uuid.UUID) (*model.EventModel, error) {
	event := &model.EventModel{}
	err := t.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(event, "event_id = ?", eventID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (t *bookingTx) CountOccupiedTickets(eventID uuid.UUID) (int64, error) {
	var occupied int64
	err := t.db.Model(&model.BookingModel{}).
		Where("event_id = ? AND (status = ? OR (status = ? AND expires_at > NOW()))",
			eventID, model.BookingStatusConfirmed, model.BookingStatusReserved).
		Select("COALESCE(SUM(number_of_tickets), 0)").
		Scan(&occupied).Error
	return occupied, err
}

func (t *bookingTx) CreateBooking(booking *model.BookingModel) error {
	return t.db.Create(booking).Error
}
