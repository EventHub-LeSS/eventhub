package repository

import (
	"backend/internal/model"

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
