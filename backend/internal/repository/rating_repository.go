package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RatingRepository interface {
	GetEventRatingSummary(eventID uuid.UUID) (*float64, int64, error)
}

type ratingRepository struct {
	db *gorm.DB
}

func NewRatingRepository(db *gorm.DB) RatingRepository {
	return &ratingRepository{db: db}
}

// validRatings selects the ratings an event summary averages. Inner joins
// exclude ratings detached from bookings or events. Scores are also
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
