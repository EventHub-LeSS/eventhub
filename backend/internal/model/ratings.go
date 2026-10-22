package model

import (
	"time"

	"github.com/google/uuid"
)

type RatingModel struct {
	RatingID  uuid.UUID  `json:"ratingId" gorm:"column:rating_id;type:uuid;primaryKey"`
	BookingID *uuid.UUID `json:"bookingId" gorm:"column:booking_id;type:uuid;uniqueIndex"`
	Score     int        `json:"score" gorm:"column:score"`
	Text      string     `json:"text" gorm:"column:text"`
	IsVisible bool       `json:"isVisible" gorm:"column:is_visible;default:false"`
	CreatedAt time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (RatingModel) TableName() string { return "ratings" }

// RatingModerationTarget is a rating together with the organization that owns the rated event.
// EventID and KeycloakOrgID are nil if the rating is no longer attached to an event or organization.
type RatingModerationTarget struct {
	RatingID      uuid.UUID  `gorm:"column:rating_id"`
	IsVisible     bool       `gorm:"column:is_visible"`
	EventID       *uuid.UUID `gorm:"column:event_id"`
	KeycloakOrgID *string    `gorm:"column:keycloak_org_id"`
}

// RatingVisibilityResponse is the visibility of a rating after moderation. changed is false if the
// rating already had the requested visibility; nothing was changed or recorded then.
type RatingVisibilityResponse struct {
	RatingID uuid.UUID `json:"ratingId"`
	Hidden   bool      `json:"hidden"`
	Changed  bool      `json:"changed"`
}

// EVENTHUB-238: Bewertungsdurchschnitt für Veranstaltungen.
type EventRatingSummary struct {
	EventID       uuid.UUID `json:"eventId"`
	AverageRating *float64  `json:"averageRating" extensions:"x-nullable"`
	RatingCount   int64     `json:"ratingCount"`
}

// EVENTHUB-239: Bewertungsdurchschnitt für Organisator/Veranstalter.
type OrganizerRatingSummary struct {
	OrganizationID  uuid.UUID `json:"organizationId"`
	AverageRating   *float64  `json:"averageRating" extensions:"x-nullable"`
	RatedEventCount int64     `json:"ratedEventCount"`
}
