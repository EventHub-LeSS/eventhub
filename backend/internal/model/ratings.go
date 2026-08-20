package model

import (
	"time"

	"github.com/google/uuid"
)

type RatingModel struct {
	RatingID  uuid.UUID `json:"ratingId" gorm:"column:rating_id;type:uuid;primaryKey"`
	BookingID uuid.UUID `json:"bookingId" gorm:"column:booking_id;type:uuid"`
	Score     int       `json:"score" gorm:"column:score"`
	Text      string    `json:"text" gorm:"column:text"`
	IsVisible bool      `json:"isVisible" gorm:"column:is_visible;default:false"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (RatingModel) TableName() string { return "ratings" }
