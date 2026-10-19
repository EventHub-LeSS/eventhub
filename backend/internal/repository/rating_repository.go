package repository

import (
	"backend/internal/model"
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRatingEventNotFound      = errors.New("event not found")
	ErrRatingEventNotEnded      = errors.New("event has not taken place")
	ErrNoConfirmedRatingBooking = errors.New("no confirmed booking for event")
	ErrRatingAlreadyExists      = errors.New("rating already exists for event")
)

type RatingRepository interface {
	GetEventRatingSummary(eventID uuid.UUID) (*float64, int64, error)
	GetOrganizerRatingSummary(organizationID uuid.UUID) (*float64, int64, error)
	CreateEventRating(ctx context.Context, userID, eventID uuid.UUID, score int, text string) (*model.RatingModel, error)
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

func (r *ratingRepository) CreateEventRating(ctx context.Context, userID, eventID uuid.UUID, score int, text string) (*model.RatingModel, error) {
	rating := &model.RatingModel{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize submissions by this visitor so concurrent requests cannot both pass
		// the per-event duplicate check.
		var user model.UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("user_id").First(&user, "user_id = ?", userID).Error; err != nil {
			return err
		}

		// Drafts stay private like in the summaries. Only events that took place can be
		// rated; database time keeps this consistent with the summaries.
		var event struct {
			Status model.EventStatus
			Ended  bool
		}
		res := tx.Table("events").Select("status, end_time <= NOW() AS ended").
			Where("event_id = ?", eventID).Scan(&event)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 || event.Status == model.EventStatusDraft {
			return ErrRatingEventNotFound
		}

		var booking model.BookingModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND event_id = ? AND status = ?", userID, eventID, model.BookingStatusConfirmed).
			Order("created_at ASC").First(&booking).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNoConfirmedRatingBooking
		}
		if err != nil {
			return err
		}

		if !event.Ended || (event.Status != model.EventStatusPublished && event.Status != model.EventStatusCompleted) {
			return ErrRatingEventNotEnded
		}

		var count int64
		if err := tx.Table("ratings AS r").
			Joins("JOIN bookings AS b ON b.booking_id = r.booking_id").
			Where("b.user_id = ? AND b.event_id = ?", userID, eventID).
			Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrRatingAlreadyExists
		}

		rating = &model.RatingModel{
			RatingID:  uuid.New(),
			BookingID: &booking.BookingID,
			Score:     score,
			Text:      text,
			IsVisible: true,
		}
		return tx.Create(rating).Error
	})
	if err != nil {
		return nil, err
	}
	return rating, nil
}
