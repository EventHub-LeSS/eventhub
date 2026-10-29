package repository

import (
	"backend/internal/model"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// EVENTHUB-236: Meine Tickets und Buchungshistorie
// OwnBooking is a booking of a user with its payment, event and location. Like GetBookingPayment it
// is read in one statement, so the booking, payment and event state come from one snapshot.
type OwnBooking struct {
	BookingPayment
	// RatingScore is the score the user gave the event of this booking; nil while it is not rated
	// yet. The rating counts per user and event, not per booking.
	RatingScore *int
	// Event is nil when the event of the booking no longer exists.
	Event *model.EventModel
	// Location is nil when the event has no location or the event no longer exists.
	Location *model.LocationModel
}

// OwnBookingRepository reads the bookings of one user for the ticket list, the booking history and
// the ticket details. Every query is restricted to the bookings of the given user.
type OwnBookingRepository interface {
	// ListByUserNewestFirst returns the bookings of the user, the latest booking first.
	ListByUserNewestFirst(userID uuid.UUID) ([]*OwnBooking, error)
	// ListByUserByEventStart returns the bookings of the user for the ticket list: bookings of
	// upcoming events first by start time, then those of past events, the latest first. An event
	// is upcoming until it has ended by the database clock.
	ListByUserByEventStart(userID uuid.UUID) ([]*OwnBooking, error)
	// GetByUser returns nil when the booking does not exist or belongs to another user, so both look
	// the same to the caller.
	GetByUser(bookingID, userID uuid.UUID) (*OwnBooking, error)
}

type ownBookingRepository struct {
	db *gorm.DB
}

func NewOwnBookingRepository(db *gorm.DB) OwnBookingRepository {
	return &ownBookingRepository{db: db}
}

// ownBookingQuery selects the bookings of a user with payment, event and location. The clock CTE
// reads clock_timestamp() once, so every row of a list is judged by the same database time.
const ownBookingQuery = `WITH clock AS MATERIALIZED (SELECT clock_timestamp() AS database_now)
	SELECT b.*,
	    p.amount AS payment_amount, p.status AS payment_status, p.failure_reason AS payment_failure_reason,
	    p.refund_amount AS payment_refund_amount, p.created_at AS payment_created_at, p.updated_at AS payment_updated_at,
	    e.status AS event_status, e.title AS event_title, e.start_time AS event_start_time, e.end_time AS event_end_time,
	    l.location_id AS location_id, l.name AS location_name, l.city AS location_city,
	    l.postal_code AS location_postal_code, l.street AS location_street, l.house_number AS location_house_number,
	    (SELECT r.score FROM ratings r JOIN bookings rb ON rb.booking_id = r.booking_id
	     WHERE rb.user_id = b.user_id AND rb.event_id = b.event_id LIMIT 1) AS rating_score,
	    clock.database_now
	FROM bookings b
	CROSS JOIN clock
	LEFT JOIN payments p ON p.payment_id = b.payment_id
	LEFT JOIN events e ON e.event_id = b.event_id
	LEFT JOIN locations l ON l.location_id = e.location_id
	WHERE b.user_id = ?`

// ownBookingRow is one row of ownBookingQuery; the payment, event and location columns are NULL
// when the booking has none.
type ownBookingRow struct {
	model.BookingModel
	PaymentAmount        decimal.NullDecimal         `gorm:"column:payment_amount"`
	PaymentStatus        *model.PaymentStatus        `gorm:"column:payment_status"`
	PaymentFailureReason *model.PaymentFailureReason `gorm:"column:payment_failure_reason"`
	PaymentRefundAmount  decimal.NullDecimal         `gorm:"column:payment_refund_amount"`
	PaymentCreatedAt     *time.Time                  `gorm:"column:payment_created_at"`
	PaymentUpdatedAt     *time.Time                  `gorm:"column:payment_updated_at"`
	EventStatus          *model.EventStatus          `gorm:"column:event_status"`
	EventTitle           *string                     `gorm:"column:event_title"`
	EventStartTime       *time.Time                  `gorm:"column:event_start_time"`
	EventEndTime         *time.Time                  `gorm:"column:event_end_time"`
	LocationID           *uuid.UUID                  `gorm:"column:location_id"`
	LocationName         *string                     `gorm:"column:location_name"`
	LocationCity         *string                     `gorm:"column:location_city"`
	LocationPostalCode   *string                     `gorm:"column:location_postal_code"`
	LocationStreet       *string                     `gorm:"column:location_street"`
	LocationHouseNumber  *string                     `gorm:"column:location_house_number"`
	RatingScore          *int                        `gorm:"column:rating_score"`
	DatabaseNow          time.Time                   `gorm:"column:database_now"`
}

func (r *ownBookingRepository) ListByUserNewestFirst(userID uuid.UUID) ([]*OwnBooking, error) {
	return r.list(ownBookingQuery+` ORDER BY b.created_at DESC, b.booking_id DESC`, userID)
}

func (r *ownBookingRepository) ListByUserByEventStart(userID uuid.UUID) ([]*OwnBooking, error) {
	return r.list(ownBookingQuery+` ORDER BY
	    (e.end_time >= clock.database_now) DESC NULLS LAST,
	    CASE WHEN e.end_time >= clock.database_now THEN e.start_time END ASC NULLS LAST,
	    CASE WHEN e.end_time < clock.database_now THEN e.start_time END DESC NULLS LAST,
	    b.booking_id ASC`, userID)
}

func (r *ownBookingRepository) GetByUser(bookingID, userID uuid.UUID) (*OwnBooking, error) {
	bookings, err := r.list(ownBookingQuery+` AND b.booking_id = ?`, userID, bookingID)
	if err != nil || len(bookings) == 0 {
		return nil, err
	}
	return bookings[0], nil
}

func (r *ownBookingRepository) list(query string, args ...any) ([]*OwnBooking, error) {
	var rows []ownBookingRow
	if err := r.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	bookings := make([]*OwnBooking, 0, len(rows))
	for i := range rows {
		bookings = append(bookings, rows[i].ownBooking())
	}
	return bookings, nil
}

func (row *ownBookingRow) ownBooking() *OwnBooking {
	booking := row.BookingModel
	own := &OwnBooking{BookingPayment: BookingPayment{Booking: &booking, EventStatus: row.EventStatus, Now: row.DatabaseNow}, RatingScore: row.RatingScore}
	if booking.PaymentID != nil && row.PaymentStatus != nil {
		own.Payment = &model.PaymentModel{
			PaymentID:     *booking.PaymentID,
			Amount:        row.PaymentAmount.Decimal,
			Status:        *row.PaymentStatus,
			FailureReason: row.PaymentFailureReason,
			RefundAmount:  row.PaymentRefundAmount.Decimal,
			CreatedAt:     derefTime(row.PaymentCreatedAt),
			UpdatedAt:     derefTime(row.PaymentUpdatedAt),
		}
	}
	if booking.EventID != nil && row.EventStatus != nil {
		own.Event = &model.EventModel{
			EventID:    *booking.EventID,
			Title:      derefString(row.EventTitle),
			StartTime:  derefTime(row.EventStartTime),
			EndTime:    derefTime(row.EventEndTime),
			Status:     *row.EventStatus,
			LocationID: row.LocationID,
		}
	}
	if row.LocationID != nil {
		own.Location = &model.LocationModel{
			LocationID:  *row.LocationID,
			Name:        derefString(row.LocationName),
			City:        derefString(row.LocationCity),
			PostalCode:  derefString(row.LocationPostalCode),
			Street:      derefString(row.LocationStreet),
			HouseNumber: row.LocationHouseNumber,
		}
	}
	return own
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
