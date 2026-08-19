package model

import (
	"time"

	"github.com/google/uuid"
)

type RatingModel struct {
	tableName struct{} `pg:"ratings"`

	RatingID  uuid.UUID `json:"ratingId" pg:"rating_id,pk"`
	BookingID uuid.UUID `json:"bookingId" pg:"booking_id"`
	Score     int       `json:"score" pg:"score,use_zero"`
	Text      string    `json:"text" pg:"text"`
	IsVisible bool      `json:"isVisible" pg:"is_visible,use_zero"`
	CreatedAt time.Time `json:"createdAt" pg:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" pg:"updated_at"`
}
