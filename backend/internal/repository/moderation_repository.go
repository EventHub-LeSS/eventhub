package repository

import (
	"backend/internal/model"
	"errors"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModerationRepository changes the visibility of user content. It is meant to be used inside a
// transaction (see Tx), so that the change and its audit entry are written together.
type ModerationRepository interface {
	// LockRating returns the rating with its event and organization and locks the rating row until
	// the transaction ends, or nil if the rating does not exist.
	LockRating(ratingID uuid.UUID) (*model.RatingModerationTarget, error)
	SetRatingVisibility(ratingID uuid.UUID, visible bool) error
	// ListRatings returns one page of ratings, newest first, and the number of all matching ratings.
	ListRatings(filter model.ModeratedRatingFilter, page, limit int) ([]model.ModeratedRating, int64, error)
}

type moderationRepository struct {
	db *gorm.DB
}

func NewModerationRepository(db *gorm.DB) ModerationRepository {
	return &moderationRepository{db: db}
}

func (r *moderationRepository) LockRating(ratingID uuid.UUID) (*model.RatingModerationTarget, error) {
	var targets []model.RatingModerationTarget
	err := r.db.Table("ratings AS r").
		Select("r.rating_id, r.is_visible, b.event_id, o.keycloak_org_id").
		Joins("LEFT JOIN bookings AS b ON b.booking_id = r.booking_id").
		Joins("LEFT JOIN events AS e ON e.event_id = b.event_id").
		Joins("LEFT JOIN organizations AS o ON o.organization_id = e.organizer_id").
		Where("r.rating_id = ?", ratingID).
		Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate, Table: clause.Table{Name: "r"}}).
		Scan(&targets).Error
	if err != nil || len(targets) == 0 {
		return nil, err
	}
	return &targets[0], nil
}

func (r *moderationRepository) SetRatingVisibility(ratingID uuid.UUID, visible bool) error {
	return r.db.Table("ratings").Where("rating_id = ?", ratingID).
		Updates(map[string]any{"is_visible": visible, "updated_at": gorm.Expr("NOW()")}).Error
}

func (r *moderationRepository) ListRatings(filter model.ModeratedRatingFilter, page, limit int) ([]model.ModeratedRating, int64, error) {
	if page < 1 || limit < 1 || page-1 > math.MaxInt/limit {
		return nil, 0, errors.New("invalid rating pagination")
	}
	query := func() *gorm.DB {
		q := r.db.Table("ratings AS r").
			Joins("LEFT JOIN bookings AS b ON b.booking_id = r.booking_id").
			Joins("LEFT JOIN events AS e ON e.event_id = b.event_id").
			Joins("LEFT JOIN organizations AS o ON o.organization_id = e.organizer_id")
		if filter.Hidden != nil {
			q = q.Where("r.is_visible = ?", !*filter.Hidden)
		}
		if filter.EventID != nil {
			q = q.Where("e.event_id = ?", *filter.EventID)
		}
		return q
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.ModeratedRating, 0, limit)
	err := query().
		Select("r.rating_id, r.score, r.text, NOT r.is_visible AS hidden, e.event_id, e.title AS event_title, o.organization_id, o.name AS organization_name, r.created_at, r.updated_at").
		Order("r.created_at DESC, r.rating_id DESC").Offset((page - 1) * limit).Limit(limit).
		Scan(&items).Error
	return items, total, err
}
