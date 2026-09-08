package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BookingRepository interface {
	CreateBooking(booking *model.BookingModel) error
	GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error)
	GetAllBookings() ([]*model.BookingModel, error)
	UpdateBooking(booking *model.BookingModel) error
	DeleteBooking(bookingID uuid.UUID) error
	CountByEventID(eventID uuid.UUID) (int64, error)
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) CreateBooking(booking *model.BookingModel) error {
	return r.db.Create(booking).Error
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

func (r *bookingRepository) DeleteBooking(bookingID uuid.UUID) error {
	return r.db.Delete(&model.BookingModel{}, "booking_id=?", bookingID).Error
}
func (r *bookingRepository) CountByEventID(eventID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&model.BookingModel{}).Where("event_id = ?", eventID).Count(&count).Error
	return count, err
}
