package model

import (
	"time"

	"github.com/google/uuid"
)

type RatingModel struct {
	RatingID  uuid.UUID `json:"ratingId" db:"rating_id"`
	BookingID uuid.UUID `json:"bookingId" db:"booking_id"`
	Score     int       `json:"score" db:"score"`
	Text      string    `json:"text" db:"text"`
	IsVisible bool      `json:"isVisible" db:"is_visible"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
