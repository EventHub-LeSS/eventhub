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

// ModeratedRating is a rating as moderators see it; the event and organization fields are null if
// the rating no longer belongs to an event or organization.
type ModeratedRating struct {
	RatingID         uuid.UUID  `json:"ratingId" gorm:"column:rating_id"`
	Score            int        `json:"score" gorm:"column:score"`
	Text             string     `json:"text" gorm:"column:text"`
	Hidden           bool       `json:"hidden" gorm:"column:hidden"`
	EventID          *uuid.UUID `json:"eventId" gorm:"column:event_id" extensions:"x-nullable"`
	EventTitle       *string    `json:"eventTitle" gorm:"column:event_title" extensions:"x-nullable"`
	OrganizationID   *uuid.UUID `json:"organizationId" gorm:"column:organization_id" extensions:"x-nullable"`
	OrganizationName *string    `json:"organizationName" gorm:"column:organization_name" extensions:"x-nullable"`
	CreatedAt        time.Time  `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt        time.Time  `json:"updatedAt" gorm:"column:updated_at"`
}

// ModeratedRatingFilter narrows the moderation list; nil fields do not filter.
type ModeratedRatingFilter struct {
	Hidden  *bool
	EventID *uuid.UUID
}

type ModeratedRatingPage struct {
	Items []ModeratedRating `json:"items"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
	Total int64             `json:"total"`
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
