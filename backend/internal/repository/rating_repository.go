package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RatingRepository interface {
	GetEventRatingSummary(eventID uuid.UUID) (*float64, int64, error)
	GetOrganizerRatingSummary(organizationID uuid.UUID) (*float64, int64, error)
}

type ratingRepository struct {
	db *gorm.DB
}

func NewRatingRepository(db *gorm.DB) RatingRepository {
	return &ratingRepository{db: db}
}

// validRatings is shared by both summaries so their underlying reviews agree.
// Inner joins exclude ratings detached from bookings or events. Scores are also
// constrained to 1–5 by the ratings table.
func (r *ratingRepository) validRatings() *gorm.DB {
	return r.db.Table("ratings AS r").
		Joins("JOIN bookings AS b ON b.booking_id = r.booking_id").
		Where("r.is_visible = true AND r.score BETWEEN 1 AND 5 AND b.status = ?", model.BookingStatusConfirmed)
}

func (r *ratingRepository) GetEventRatingSummary(eventID uuid.UUID) (*float64, int64, error) {
	var row struct {
		AverageRating *float64
		RatingCount   int64
	}
	err := r.validRatings().
		Select("AVG(r.score) AS average_rating, COUNT(*) AS rating_count").
		Where("b.event_id = ?", eventID).Scan(&row).Error
	if err != nil {
		return nil, 0, err
	}
	return row.AverageRating, row.RatingCount, nil
}

func (r *ratingRepository) GetOrganizerRatingSummary(organizationID uuid.UUID) (*float64, int64, error) {
	// Average unrounded event averages, not individual reviews: every rated,
	// ended public event contributes equally. Unrated events are absent.
	perEvent := r.validRatings().Select("AVG(r.score) AS average_rating").
		Joins("JOIN events AS e ON e.event_id = b.event_id").
		Where("e.organizer_id = ? AND e.status IN ? AND e.end_time <= NOW()", organizationID,
			[]model.EventStatus{model.EventStatusPublished, model.EventStatusCompleted}).
		Group("e.event_id")
	var row struct {
		AverageRating   *float64
		RatedEventCount int64
	}
	err := r.db.Table("(?) AS per_event", perEvent).
		Select("AVG(per_event.average_rating) AS average_rating, COUNT(*) AS rated_event_count").
		Scan(&row).Error
	if err != nil {
		return nil, 0, err
	}
	return row.AverageRating, row.RatedEventCount, nil
}
