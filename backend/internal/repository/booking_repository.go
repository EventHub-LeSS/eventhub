package repository

import (
	"backend/internal/model"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BookingRepository reads and writes bookings. Reservations (status reserved without a payment)
// are the items of the cart of their user; bookings with a payment belong to the checkout that
// paid them. Everywhere that locks events and bookings of one payment, the locks are taken in
// the order of the IDs: events by event_id, then bookings by booking_id, then the payment.
// That order rules out deadlocks between the cart, checkout, payment and cleanup code.
type BookingRepository interface {
	GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error)
	// LockBooking reads the booking with SELECT ... FOR UPDATE; use it inside a transaction.
	LockBooking(bookingID uuid.UUID) (*model.BookingModel, error)
	// LockBookings locks the bookings by ID, ordered by booking_id; use it inside a transaction.
	// It returns every booking that exists, in the same order as the IDs.
	LockBookings(bookingIDs []uuid.UUID) ([]*model.BookingModel, error)
	// TryLockBookings locks the bookings by ID with FOR UPDATE SKIP LOCKED, ordered by booking_id,
	// and returns only the ones it got. Bookings locked elsewhere, e.g. by a payment being
	// completed right now, are skipped.
	TryLockBookings(bookingIDs []uuid.UUID) ([]*model.BookingModel, error)
	// LockUserReservations locks every reservation of the user, with or without a payment,
	// ordered by booking_id; use it inside a transaction. It serializes the checkout of the
	// user's cart against itself and against payment completions of the same bookings.
	LockUserReservations(userID uuid.UUID) ([]*model.BookingModel, error)
	// Reservations returns every reservation of the user, with or without a payment, without
	// locking; the latest first. A payment can only be pending while its bookings are reserved,
	// so a reservation with a payment marks the user's open checkout.
	Reservations(userID uuid.UUID) ([]*model.BookingModel, error)
	// OpenReservations returns the reservations of the user without a payment — the cart items,
	// live or expired — without locking, ordered by booking_id.
	OpenReservations(userID uuid.UUID) ([]*model.BookingModel, error)
	// LatestOpenReservation returns the newest payment-less reservation of the user for the
	// event, live or expired, or nil when there is none; locked for update.
	LatestOpenReservation(userID, eventID uuid.UUID) (*model.BookingModel, error)
	CreateBooking(booking *model.BookingModel) error
	UpdateBooking(booking *model.BookingModel) error
	// DeleteBooking removes the booking row. Only cart items — reservations without a payment —
	// may be deleted; nothing references them.
	DeleteBooking(bookingID uuid.UUID) error
	// CountOccupiedTickets sums the tickets of confirmed bookings and live reservations of the event.
	CountOccupiedTickets(eventID uuid.UUID) (int64, error)
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

func (r *bookingRepository) LockBookings(bookingIDs []uuid.UUID) ([]*model.BookingModel, error) {
	return r.lockBookings(bookingIDs, false)
}

func (r *bookingRepository) TryLockBookings(bookingIDs []uuid.UUID) ([]*model.BookingModel, error) {
	return r.lockBookings(bookingIDs, true)
}

func (r *bookingRepository) lockBookings(bookingIDs []uuid.UUID, skipLocked bool) ([]*model.BookingModel, error) {
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	q := r.db.Where("booking_id IN ?", bookingIDs).Order("booking_id")
	if skipLocked {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	} else {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var bookings []*model.BookingModel
	if err := q.Find(&bookings).Error; err != nil {
		return nil, err
	}
	return bookings, nil
}

func (r *bookingRepository) LockUserReservations(userID uuid.UUID) ([]*model.BookingModel, error) {
	var bookings []*model.BookingModel
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND status = ?", userID, model.BookingStatusReserved).
		Order("booking_id").
		Find(&bookings).Error
	return bookings, err
}

func (r *bookingRepository) Reservations(userID uuid.UUID) ([]*model.BookingModel, error) {
	var bookings []*model.BookingModel
	err := r.db.Where("user_id = ? AND status = ?", userID, model.BookingStatusReserved).
		Order("created_at DESC, booking_id DESC").
		Find(&bookings).Error
	return bookings, err
}

func (r *bookingRepository) OpenReservations(userID uuid.UUID) ([]*model.BookingModel, error) {
	var bookings []*model.BookingModel
	err := r.db.Where("user_id = ? AND status = ? AND payment_id IS NULL", userID, model.BookingStatusReserved).
		Order("booking_id").
		Find(&bookings).Error
	return bookings, err
}

func (r *bookingRepository) LatestOpenReservation(userID, eventID uuid.UUID) (*model.BookingModel, error) {
	booking := &model.BookingModel{}
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND event_id = ? AND status = ? AND payment_id IS NULL",
			userID, eventID, model.BookingStatusReserved).
		Order("created_at DESC, booking_id DESC").
		First(booking).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *bookingRepository) CreateBooking(booking *model.BookingModel) error {
	return r.db.Create(booking).Error
}

func (r *bookingRepository) UpdateBooking(booking *model.BookingModel) error {
	return r.db.Save(booking).Error
}

func (r *bookingRepository) DeleteBooking(bookingID uuid.UUID) error {
	return r.db.Delete(&model.BookingModel{}, "booking_id = ?", bookingID).Error
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
