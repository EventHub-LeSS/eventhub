package repository

import (
	"backend/internal/model"
	"context"
	"time"

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
	GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error)
	GetAllBookings() ([]*model.BookingModel, error)
	UpdateBooking(booking *model.BookingModel) error
	ListEventsByUser(userID uuid.UUID) ([]*model.EventModel, error)
	GetAvgRatingByOrganizer(organizerID *uuid.UUID) (int, error)
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error) {
	booking := &model.BookingModel{}
	err := r.db.First(booking, "booking_id = ?", bookingID).Error
	if err == gorm.ErrRecordNotFound {
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

func (r *bookingRepository) ListEventsByUser(userID uuid.UUID) ([]*model.EventModel, error) {
	var events []*model.EventModel

	err := r.db.Table("bookings AS b").
		Select("e.*").
		Joins("JOIN events AS e ON e.event_id = b.event_id").
		Where("b.user_id = ?", userID).
		Where("b.status = ?", model.BookingStatusConfirmed).
		Where("e.end_time < ?", time.Now()).
		Scan(&events).Error

	if err != nil {
		return nil, err
	}

	return events, nil
}

func (r *bookingRepository) GetAvgRatingByOrganizer(organizerID *uuid.UUID) (int, error) {
	var rating int

	err := r.db.Table("events AS e").Select("AVG(r.score)").
		Joins("JOIN bookings AS b ON e.event_id = b.event_id JOIN rating AS r on b.booking_id = r.booking_id").
		Where("e.organizer_id = ? AND e.status = ?", organizerID, model.EventStatusCompleted).Scan(&rating).Error

	return rating, err
}
